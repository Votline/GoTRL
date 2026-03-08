package ui

import (
	"fmt"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	winW = 250
	winH = 100
)

func PrimaryWindow() (*glfw.Window, error) {
	const op = "ui.PrimaryWindow"

	glfw.WindowHint(glfw.RefreshRate, 60)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.Decorated, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.DoubleBuffer, glfw.True)
	glfw.WindowHint(glfw.TransparentFramebuffer, glfw.True)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCompatProfile)

	win, err := glfw.CreateWindow(winW, winH, "Replay", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%s glfw.CreateWindow: %w", op, err)
	}

	return win, nil
}
