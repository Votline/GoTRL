package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"syscall"
	"time"
	"unsafe"

	"gotrl/internal/parser"
	rb "gotrl/internal/ringbuffer"
	"gotrl/internal/workers"

	gd "github.com/Votline/Go-audio"
	gurlf "github.com/Votline/Gurlf"
	gscan "github.com/Votline/Gurlf/pkg/scanner"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const helpMsg = `
Supported offline AIs.

Usage (choose your way):
    1. From file:   gotrl <config_path> <source> <args>
    2. From string: gotrl "[config]...[/config]" <source> <args>
    3. From flags:  gotrl -it=<type> -ot=<type> -trl=<url> <source> <args>

Settings (Flags):
    -it    Input Type:  'text' (default), 'file', 'image', 'stream'
    -ot    Output Type: 'cli' (default), 'ui', 'audio'
    -trl   Translate:   URL for translation AI
    -stt   STT:         URL for Speech-to-Text AI
    -tts   TTS:         URL for Text-to-Speech AI

Source:
    <text>              Plain text to translate
    <file_path>         Path to file (if input type is 'file')

Args:
    '-d' or '--debug'   Enable debug mode

Examples:
    gotrl ./cfg.gurlf "Hello world"
    gotrl -it=file -ot=audio ./notes.txt
    gotrl "[config]...[/config]" "Check this"

Config fields (case sensitive):
	InputType
	OutputType
	TranslationURL
	SpeechToTextURL
	TextToSpeechURL
`

const (
	sttSinkName = "STT_only"
	appName     = "PipeWire ALSA [main]"
)

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
	ud, dbg, err := handleCfgPath(args)
	if err != nil {
		fmt.Printf("Invalid args. %s\n", err.Error())
		return
	}

	log := initLog(dbg)
	defer log.Sync()

	log.Debug("Args",
		zap.Strings("args", args),
		zap.Any("user_data", ud))

	trBuf := rb.NewRB[byte](512)
	infBuf := rb.NewRB[byte](512)

	stdin := func(buf []byte) int {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				return 0
			}
			fmt.Printf("Read error: %s\n", err.Error())
			return 0
		}
		return n
	}

	stdout := func(buf []byte) int {
		n, err := os.Stdout.Write(buf)
		if err != nil {
			fmt.Printf("Write error: %s\n", err.Error())
			return 0
		}
		return n
	}

	acl, err := gd.InitAudioClient(
		workers.BufferSize, 0, 0, workers.BufferSize,
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

	sttBuf := rb.NewRB[byte](workers.BufferSize)
	if ud.SttURL != "" {
		go func() {
			stt := workers.NewStt(ud.SttURL, acl, log)

			go func() {
				time.Sleep(2 * time.Second)

				defSink, err := exec.Command("pactl", "get-default-sink").Output()
				if err != nil {
					log.Fatal("Failed to get default sink", zap.Error(err))
				}
				trimSpaceBytes(&defSink)
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

			if err := stt.Stt(sttBuf.WriteSimple); err != nil {
				fmt.Printf("Stt error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.TrlURL != "" {
		go func() {
			trl := workers.NewTranslator(ud.TrlURL, log)
			if err := trl.Translate(stdin, trBuf.WriteSimple); err != nil {
				fmt.Printf("Translate error: %s\n", err.Error())
				return
			}
		}()
	}

	infBufWrite := func(buf []byte) int {
		n := infBuf.WriteSimple(buf)
		stdout(buf)
		return n
	}

	if ud.InfURL != "" {
		go func() {
			infl := workers.NewInflector(ud.InfURL, log)
			if err := infl.Inflect(stdin, trBuf.ReadSimple, infBufWrite); err != nil {
				fmt.Printf("Inflect error: %s\n", err.Error())
				return
			}
		}()
	}

	if ud.TtsURL != "" {
		go func() {
			tts := workers.NewTTS(ud.TtsURL, acl, log)
			if err := tts.TTS(sttBuf.ReadSimple); err != nil {
				fmt.Printf("TTS error: %s\n", err.Error())
				return
			}
		}()
	}

	<-ctx.Done()
	if err := acl.RemoveMonitor("STT_only"); err != nil {
		log.Fatal("RemoveMonitor", zap.Error(err))
	}
	log.Info("Exit")
}

func parseArgs(args []string) (*parser.UserData, error) {
	const op = "main.parseArgs"

	ud := parser.UserData{}

	for _, arg := range args {
		argByte := unsafe.Slice(unsafe.StringData(arg), len(arg))
		parser.RangeByByte(argByte, ' ', func(key, val []byte) {
			keyStr := unsafe.String(unsafe.SliceData(key), len(key))
			valStr := unsafe.String(unsafe.SliceData(val), len(val))
			switch keyStr {
			case "-it", "--inputType":
				ud.InpType = valStr
			case "-ot", "--outputType":
				ud.OutType = valStr
			case "-trl", "--translationURL":
				ud.TrlURL = valStr
			case "-stt", "--speechToTextURL":
				ud.SttURL = valStr
			case "-tts", "--textToSpeechURL":
				ud.TtsURL = valStr
			}
		})
	}

	if ud.InpType == "" || ud.OutType == "" || ud.TrlURL == "" || ud.SttURL == "" || ud.TtsURL == "" {
		return nil, fmt.Errorf("%s: invalid args", op)
	}

	return &ud, nil
}

func handleCfgPath(args []string) (*parser.UserData, bool, error) {
	const op = "main.handleCfgPath"

	dbg := slices.Contains(args, "-d") || slices.Contains(args, "--debug")

	if len(args) >= 5 {
		ud, err := parseArgs(args)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: parse args: %w", op, err)
		}
		return ud, dbg, nil
	}

	var gData []gscan.Data
	var err error
	arg := args[0]

	if _, err := os.Stat(arg); err == nil {
		gData, err = gurlf.ScanFile(arg)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: scan file: %w", op, err)
		}
	} else if os.IsNotExist(err) {
		argBytes := unsafe.Slice(unsafe.StringData(arg), len(arg))
		gData, err = gurlf.Scan(argBytes)
		if err != nil {
			return nil, dbg, fmt.Errorf("%s: scan string: %w", op, err)
		}
	}

	ud, err := parser.Parse(gData)
	if err != nil {
		return nil, dbg, fmt.Errorf("%s: parse: %w", op, err)
	}
	return ud, dbg, nil
}

func initLog(dbg bool) *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.TimeKey = ""
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.ConsoleSeparator = " | "
	cfg.Level.SetLevel(zap.ErrorLevel)

	if dbg {
		cfg.Level.SetLevel(zap.DebugLevel)
	}
	log, _ := cfg.Build()

	return log
}

// trimSpaceBytes trims spaces in slice by pointer
func trimSpaceBytes(b *[]byte) {
	tempB := *b

	start := 0
	end := len(tempB) - 1
	for start < end && isSpace(tempB[start]) {
		start++
	}
	for end > start && isSpace(tempB[end]) {
		end--
	}

	*b = tempB[start : end+1]
}

// isSpace is a helper function to check if a byte is a space.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
