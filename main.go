package main

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
)

const (
	redOpen  = "\033[31m"
	redClose = "\033[0m"
)

func main() {
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
}
