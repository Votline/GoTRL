package ui

import (
	"image"
	"sync"

	"gotrl/internal/font"
	"gotrl/internal/render"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type HomeView struct {
	pg           uint32
	vao          uint32
	lines        []string
	scrollOffset int
	uniforms     map[string]int32
	img          *image.RGBA
	mu           sync.Mutex
}

const (
	orangeOpen  = "\033[33m"
	orangeClose = "\033[0m"
	maxSymbols  = 20
	maxLines    = 7
)

func CreateHomeView(win *glfw.Window, pg uint32, unfrs map[string]int32) *HomeView {
	// x, y, z, u, v
	quad := []float32{
		-1.0, -1.0, 0.0, 0.0, 1.0, // down-left
		1.0, -1.0, 0.0, 1.0, 1.0, // down-right
		-1.0, 1.0, 0.0, 0.0, 0.0, // top-left

		-1.0, 1.0, 0.0, 0.0, 0.0, // top-left
		1.0, -1.0, 0.0, 1.0, 1.0, // down-right
		1.0, 1.0, 0.0, 1.0, 0.0, // top-right
	}
	vao := render.Canvas(quad)

	view := &HomeView{pg: pg, img: nil, vao: vao, uniforms: unfrs}

	win.SetScrollCallback(func(w *glfw.Window, xoff float64, yoff float64) {
		if yoff > 0 {
			view.ScrollUp()
		} else {
			view.ScrollDown()
		}
	})

	win.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press && key == glfw.KeySpace {
			view.ScrollDown()
		}
	})

	return view
}

func (hv *HomeView) Render() {
	hv.mu.Lock()
	defer hv.mu.Unlock()

	if hv.img != nil {
		tex := render.CreateTexture(hv.img)
		defer gl.DeleteTextures(1, &tex)

		gl.UseProgram(hv.pg)
		gl.Uniform1i(hv.uniforms["tex"], 0)
		gl.Uniform4f(hv.uniforms["color"], 1.0, 1.0, 1.0, 1.0)
		gl.Uniform2f(hv.uniforms["offset"], 0.0, 0.0)
		gl.Uniform2f(hv.uniforms["scale"], 1.0, 1.0)
		render.ElemRender(tex, hv.vao)
	}
}

func (hv *HomeView) Update(msg string) {
	hv.mu.Lock()
	defer hv.mu.Unlock()

	newLines := splitMsg(msg)
	if len(hv.lines) > 0 {
		lastIdx := len(hv.lines) - 1
		if len([]rune(hv.lines[lastIdx])) < maxSymbols {
			toAp, rem := improveLast(len([]rune(hv.lines[lastIdx])), newLines)
			hv.lines[lastIdx] += toAp
			newLines[0] = rem
		}
	}

	for _, line := range newLines {
		r := []rune(line)
		if len(r) > 0 {
			hv.lines = append(hv.lines, line)
		}
	}

	hv.refreshImage()
}

func (hv *HomeView) refreshImage() {
	start := hv.scrollOffset

	if start > len(hv.lines)-maxLines {
		start = len(hv.lines) - maxLines
	}
	if start < 0 {
		start = 0
	}

	end := start + maxLines
	if end > len(hv.lines) {
		end = len(hv.lines)
	}

	hv.img = font.CreateImage(hv.lines[start:end])
}

func (hv *HomeView) ScrollUp() {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	hv.scrollOffset--
	hv.refreshImage()
}

func (hv *HomeView) ScrollDown() {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	hv.scrollOffset++
	if hv.scrollOffset < 0 {
		hv.scrollOffset = 0
	}
	hv.refreshImage()
}

func splitMsg(msg string) []string {
	runes := []rune(msg)

	if len(runes) < maxSymbols {
		return []string{msg}
	}

	res := make([]string, 0, maxLines)
	for i := 0; i < len(runes); i += maxSymbols {
		end := min(i+maxSymbols, len(runes))
		res = append(res, string(runes[i:end]))
	}

	return res
}

func improveLast(lastLen int, newLines []string) (toAppend string, remainingFirstLine string) {
	if len(newLines) == 0 {
		return "", ""
	}

	runes := []rune(newLines[0])
	remained := maxSymbols - lastLen

	if len(runes) < remained {
		return string(runes), ""
	}

	toAppend = string(runes[:remained])
	remainingFirstLine = string(runes[remained:])

	return toAppend, remainingFirstLine
}
