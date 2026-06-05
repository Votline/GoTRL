package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"gotrl/internal/parser"
	"gotrl/internal/render"
	rb "gotrl/internal/ringbuffer"
	"gotrl/internal/ui"
	"gotrl/internal/utils"
	"gotrl/internal/workers"

	gd "github.com/Votline/Go-audio"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"go.uber.org/zap"
)

const helpMsg = `
Supported offline AIs.

Usage (choose your way):
    1. From file:   gotrl <config_path> <args>
    2. From string: gotrl "[config]...[/config]" <args>
    3. From flags:  gotrl -it=<type> -ot=<type> -trl=<url> <args>

Settings (Flags):
    -it    Input Type:  'text' (default), 'file', 'image', 'stream'
    -ot    Output Type: 'cli' (default), 'ui', 'audio'
    -trl   Translate:   URL for translation AI
    -stt   STT:         URL for Speech-to-Text AI
    -tts   TTS:         URL for Text-to-Speech AI

Args:
    '-d' or '--debug'   Enable debug mode

Examples:
    gotrl cfg.gurlf
		gotrl -trl=https://localhost:8080/trl
    gotrl "[config]...[/config]" -inf=https://localhost:8080/inflector

Config fields (case sensitive):
	TranslatorURL
	SpeechToTextURL
	TextToSpeechURL
	InflectorURL
	ImageToTextURL
`

const (
	sttSinkName = "STT_only"
	appName     = "PipeWire ALSA [main]"
)

func findFirst(bufs []*rb.RingBuffer[byte], ids ...int) (*rb.RingBuffer[byte], int) {
	for _, id := range ids {
		if bufs[id] != nil {
			return bufs[id], id
		}
	}
	return bufs[workers.BufStdin], workers.BufStdin
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Invalid args.\n%s", helpMsg)
		return
	}
	if os.Args[1] == "help" || os.Args[1] == "h" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Printf("%s", helpMsg)
		return
	}

	args := os.Args[1:]
	ud, dbg, err := utils.HandleCfgPath(args)
	if err != nil {
		fmt.Printf("Invalid args. %s\n", err.Error())
		return
	}

	log := utils.InitLog(dbg)
	defer log.Sync()

	log.Debug("Args",
		zap.Strings("args", args),
		zap.Any("user_data", ud))

	acl, err := gd.InitAudioClient(
		workers.BufferAudioSize, 0, 0, workers.BufferAudioSize,
		workers.Channels, workers.SampleRate, workers.SampleRate, workers.Duration,
		false, nil,
	)
	if err != nil {
		log.Fatal("InitAudioClient", zap.Error(err))
	}
	if err := acl.AutoRouteMonitor(); err != nil {
		log.Fatal("AutoRouteMonitor", zap.Error(err))
	}

	defer func() {
		if err := acl.RemoveMonitor(sttSinkName); err != nil {
			log.Fatal("RemoveMonitor", zap.Error(err))
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lastOut := -1
	buffers := make([]*rb.RingBuffer[byte], workers.BufMax)
	if ud.IttURL != "" {
		buffers[workers.BufItt] = rb.NewRB[byte](workers.BufferTextSize)
		lastOut = workers.BufItt
	}
	if ud.SttURL != "" {
		buffers[workers.BufStt] = rb.NewRB[byte](workers.BufferAudioSize)
		lastOut = workers.BufStt
	}
	if ud.TrlURL != "" {
		buffers[workers.BufTrl] = rb.NewRB[byte](workers.BufferTextSize)
		buffers[workers.BufTrlOrig] = rb.NewRB[byte](workers.BufferTextSize)
		lastOut = workers.BufTrl
	}
	if ud.InfURL != "" {
		buffers[workers.BufInf] = rb.NewRB[byte](workers.BufferTextSize)
		lastOut = workers.BufInf
	}
	if ud.TtsURL != "" {
		buffers[workers.BufTts] = rb.NewRB[byte](workers.BufferAudioSize)
		lastOut = workers.BufTts
	}
	buffers[workers.BufStdin] = rb.NewRB[byte](workers.BufferTextSize)

	nopFunc := func(buf []byte) int {
		return 0
	}

	if ud.SttURL != "" {
		go func() {
			go func() {
				time.Sleep(2 * time.Second)

				defSink, err := exec.Command("pactl", "get-default-sink").Output()
				if err != nil {
					log.Fatal("Failed to get default sink", zap.Error(err))
				}
				utils.TrimSpaceBytes(&defSink)
				defSinkStr := unsafe.String(unsafe.SliceData(defSink), len(defSink))

				if err := acl.IsolateInput(sttSinkName, appName); err != nil {
					log.Fatal("IsolateInput", zap.Error(err))
				}

				if err := gd.SetDefaultSink(defSinkStr); err != nil {
					log.Fatal("Failed to set default sink", zap.Error(err))
				}

				if err := gd.MoveAllSinkInputs(sttSinkName); err != nil {
					log.Fatal("moveAllSinkInputs", zap.Error(err))
				}
				if err := gd.MoveStreams(sttSinkName, appName, true); err != nil {
					log.Fatal("Failed to change STT's sink", zap.Error(err))
				}

				if err := gd.MoveStreams(defSinkStr, appName, false); err != nil {
					log.Fatal("Failed to force headphones", zap.Error(err))
				}

				log.Debug("Default devices",
					zap.String("raw", string(defSink)),
					zap.String("str", defSinkStr))
			}()

			log.Debug("Add STT worker",
				zap.Int("Writer", workers.BufStt))

			stt := workers.NewStt(ud.SttURL, acl, log)
			buf := buffers[workers.BufStt]
			if err := stt.Stt(buf.WriteSimple); err != nil {
				fmt.Printf("Stt error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.TrlURL != "" {
		trl := workers.NewTranslator(ud.TrlURL, log)

		prev, id := findFirst(buffers, workers.BufStt, workers.BufItt)
		next := buffers[workers.BufTrl]

		idOrig := -1
		origWriteFunc := nopFunc
		if buffers[workers.BufTrlOrig] != nil {
			origWriteFunc = buffers[workers.BufTrlOrig].WriteSimple
			idOrig = workers.BufTrlOrig
		}

		log.Debug("Add Translator worker",
			zap.Int("Reader", id),
			zap.Int("Writer", workers.BufTrl),
			zap.Int("Writer orig", idOrig))

		go func() {
			if err := trl.Translate(prev.ReadSimple, next.WriteSimple, origWriteFunc); err != nil {
				fmt.Printf("Translate error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.InfURL != "" {
		go func() {
			infl := workers.NewInflector(ud.InfURL, log)

			prev, id := findFirst(buffers, workers.BufTrl, workers.BufStt, workers.BufItt)
			next := buffers[workers.BufInf]

			log.Debug("Add Inflect worker",
				zap.Int("Reader", id),
				zap.Int("Writer", workers.BufInf))

			origReadFunc := nopFunc
			if buffers[workers.BufTrlOrig] != nil {
				origReadFunc = buffers[workers.BufTrlOrig].ReadSimple
			}

			if err := infl.Inflect(origReadFunc, prev.ReadSimple, next.WriteSimple); err != nil {
				fmt.Printf("Inflect error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.TtsURL != "" {
		go func() {
			tts := workers.NewTTS(ud.TtsURL, acl, log)

			prev, id := findFirst(buffers, workers.BufInf, workers.BufTrl, workers.BufItt, workers.BufStt)

			log.Debug("Add TTS worker",
				zap.Int("Reader", id))

			if err := tts.TTS(prev.ReadSimple); err != nil {
				fmt.Printf("TTS error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.IttURL != "" {
		go func() {
			itt := workers.NewITT(ud.IttURL, log)

			next := buffers[workers.BufItt]
			id := workers.BufItt

			log.Debug("Add ITT worker",
				zap.Int("Writer", id))

			if err := itt.ITT(next.WriteSimple); err != nil {
				fmt.Printf("ITT error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.IttURL == "" {
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				data := scanner.Bytes()
				if len(data) == 0 {
					continue
				}
				buffers[workers.BufStdin].WriteSimple(data)

				time.Sleep(10 * time.Millisecond)
			}
			if err := scanner.Err(); err != nil {
				log.Fatal("Stdin", zap.Error(err))
			}
		}()
	}

	uiBuf := rb.NewRB[byte](workers.BufferTextSize)
	go func() {
		buf := make([]byte, workers.BufferTextSize)
		for {
			n := buffers[lastOut].ReadSimple(buf)
			if n == 0 {
				continue
			}
			temp := buf[:n]
			utils.TrimSpaceBytes(&temp)
			strData := unsafe.String(unsafe.SliceData(temp), len(temp))
			fmt.Println(strData)
			if ud.Mode == parser.UImode {
				uiBuf.WriteSimple(temp)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if ud.Mode == parser.UImode {
		uiStart(uiBuf, log)
	} else {
		<-ctx.Done()
		if err := acl.RemoveMonitor("STT_only"); err != nil {
			log.Fatal("RemoveMonitor", zap.Error(err))
		}
		log.Info("Exit")
	}
}

func init() {
	runtime.LockOSThread()
}

func uiStart(com *rb.RingBuffer[byte], log *zap.Logger) {
	const op = "main.uiStart"

	if err := glfw.Init(); err != nil {
		log.Fatal("Init glfw",
			zap.String("op", op),
			zap.Error(err))
	}
	defer glfw.Terminate()

	if err := gl.Init(); err != nil {
		log.Fatal("Init gl",
			zap.String("op", op),
			zap.Error(err))
	}

	win, err := ui.PrimaryWindow()
	if err != nil {
		log.Fatal("PrimaryWindow",
			zap.String("op", op),
			zap.Error(err))
	}

	win.MakeContextCurrent()
	win.SetAttrib(glfw.Floating, glfw.True)
	glfw.SwapInterval(1)
	glfw.WaitEventsTimeout(0.1)

	pg, unfrs := render.Setup()
	defer gl.DeleteProgram(pg)

	view := ui.CreateHomeView(win, pg, unfrs)

	buf := make([]byte, workers.BufferTextSize)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			n := com.ReadSimple(buf)
			if n == 0 {
				continue
			}
			msg := unsafe.String(unsafe.SliceData(buf), n)
			if msg != "quit" {
				view.Update(msg)
			} else {
				return
			}
		}
	})

	gl.ClearColor(0.0, 0.0, 0.0, 0.7)
	for !win.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT)

		view.Render()

		glfw.PollEvents()
		win.SwapBuffers()
	}
}
