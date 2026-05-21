package main

/*
import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"unicode"

	"gotrl/internal/render"
	"gotrl/internal/ui"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)*/

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
	"unsafe"

	"gotrl/internal/parser"
	rb "gotrl/internal/ringbuffer"
	"gotrl/internal/workers"

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
				ud.SpttURL = valStr
			case "-tts", "--textToSpeechURL":
				ud.TtsURL = valStr
			}
		})
	}

	if ud.InpType == "" || ud.OutType == "" || ud.TrlURL == "" || ud.SpttURL == "" || ud.TtsURL == "" {
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

	var wg sync.WaitGroup

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

	if ud.TrlURL != "" {
		wg.Go(func() {
			trl := workers.NewTranslator(ud.TrlURL, log)
			if err := trl.Translate(stdin, trBuf.WriteSimple); err != nil {
				fmt.Printf("Translate error: %s\n", err.Error())
				return
			}
		})
	}

	infBufWrite := func(buf []byte) int {
		n := infBuf.WriteSimple(buf)
		stdout(buf)
		return n
	}

	if ud.InfURL != "" {
		wg.Go(func() {
			infl := workers.NewInflector(ud.InfURL, log)
			if err := infl.Inflect(stdin, trBuf.ReadSimple, infBufWrite); err != nil {
				fmt.Printf("Inflect error: %s\n", err.Error())
				return
			}
		})
	}

	if ud.TtsURL != "" {
		wg.Go(func() {
			tts := workers.NewTTS(ud.TtsURL, log)
			if err := tts.TTS(infBuf.ReadSimple); err != nil {
				fmt.Printf("TTS error: %s\n", err.Error())
				return
			}
		})
	}

	wg.Wait()
}

/*
	   	if len(os.Args) < 3 {
	   		fmt.Fprintf(os.Stderr, "%sUsage: './trl <image/text/file> <ui/cli> <command_for_call_ai> <data>'%s\n",
	   			redOpen, redClose)
	   		return
	   	}

	   inpMode := os.Args[1]
	   appMode := os.Args[2]
	   call := os.Args[3]
	   data := os.Args[4]

	   var wg sync.WaitGroup
	   com := make(chan string, 100)

	   	if appMode == "ui" {
	   		wg.Go(func() {
	   			sendAi(inpMode, appMode, call, data, com)
	   		})
	   		uiStart(com)
	   	} else {

	   		sendAi(inpMode, appMode, call, data, com)
	   	}

}
/*
/*
func sendAi(inpMode, appMode, call, data string, com chan string) {
	toLan := "английский"

	if inpMode == "file" {
		d, err := os.ReadFile(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%sRead file error.\nPath:%s\nErr:%s%s",
				redOpen, data, err.Error(), redClose)
			return
		}
		data = string(d)
	}
	if !isRussian(data) {
		toLan = "русский"
	}

	promt := fmt.Sprintf("Ты — профессиональный переводчик. Твоя задача: перевести следующий текст на %s. Выводи ТОЛЬКО перевод, без лишних слов, без вступлений, без объяснений. Текст для перевода: {%s}", toLan, data)

	command := fmt.Sprintf("%s \"%s\"", call, promt)
	cmd := exec.Command("bash", "-c", command)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sCreate pipe error.\nCommand:%s\nErr:%s%s",
			redOpen, command, err.Error(), redClose)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%sStart command error.\nCommand:%s\nErr:%s%s",
			redOpen, command, err.Error(), redClose)
		return
	}

	if appMode != "ui" {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Print(scanner.Text())
		}
	} else {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			com <- scanner.Text()
		}
	}

	cmd.Wait()
	com <- "quit"
	fmt.Println()
}

func isRussian(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func init() {
	runtime.LockOSThread()
}

func uiStart(com chan string) {
	const op = "main.uiStart"

	if err := glfw.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize GLFW."+
			"\nop: %s\nerr: %s\n",
			op, err.Error())
		os.Exit(1)
	}
	defer glfw.Terminate()

	if err := gl.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize OpenGL."+
			"\nop: %s\nerr: %s\n",
			op, err.Error())
		os.Exit(1)
	}

	win, err := ui.PrimaryWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create window."+
			"\nop: %s\nerr: %s\n",
			op, err.Error())
		os.Exit(1)
	}

	win.MakeContextCurrent()
	win.SetAttrib(glfw.Floating, glfw.True)
	glfw.SwapInterval(1)
	glfw.WaitEventsTimeout(0.1)

	pg, unfrs := render.Setup()
	defer gl.DeleteProgram(pg)

	view := ui.CreateHomeView(win, pg, unfrs)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case msg := <-com:
				if msg != "quit" {
					view.Update(msg)
				} else {
					return
				}
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
}*/
