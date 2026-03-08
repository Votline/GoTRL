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
	mode := os.Args[1]
	call := os.Args[2]
	data := os.Args[3]
	toLan := "английский"

	if mode == "file" {
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

	var wg sync.WaitGroup
	if mode == "ui" {
		wg.Go(uiStart)
	}

	promt := fmt.Sprintf("Ты — профессиональный переводчик. Твоя задача: перевести следующий текст на %s. Выводи ТОЛЬКО перевод, без лишних слов, без вступлений, без объяснений. Текст для перевода: {%s}", toLan, data)

	command := fmt.Sprintf("%s \"%s\"", call, promt)
	cmd := exec.Command("bash", "-c", command)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sCreate pipe error.\nCommand:%s\nErr:%s%s",
			redOpen, command, err.Error(), redClose)
		return
	}
	cmd.Start()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Print(scanner.Text())
	}

	cmd.Wait()
	wg.Wait()
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

func uiStart() {
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

	pg := render.Setup()
	defer gl.DeleteProgram(pg)

	view := ui.CreateHomeView(pg)

	gl.ClearColor(0.0, 0.0, 0.0, 0.7)
	for !win.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT)

		view.Render()

		glfw.PollEvents()
		win.SwapBuffers()
	}
}
