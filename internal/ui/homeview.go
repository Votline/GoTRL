package ui

import (
	"image"
	"sync"

	"gotrl/internal/font"
	"gotrl/internal/render"

	"github.com/go-gl/gl/v4.1-core/gl"
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
	maxSymbols  = 28
	maxLines    = 8
)

func CreateHomeView(pg uint32, unfrs map[string]int32) *HomeView {
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

	return &HomeView{pg: pg, img: nil, vao: vao, uniforms: unfrs}
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
		if len(hv.lines[lastIdx]) < maxSymbols {
			toAp, rem := improveLast(len(hv.lines[lastIdx]), newLines)
			hv.lines[lastIdx] += toAp
			newLines[0] = rem
		}
	} else {
		hv.lines = newLines
	}

	start := 0
	if len(hv.lines) > 8 {
		start = len(hv.lines) - 8
	}
	visibleLines := hv.lines[start:]

	hv.img = font.CreateImage(visibleLines)
}

func splitMsg(msg string) []string {
	runes := []rune(msg)

	if len(runes) < maxSymbols {
		return []string{msg}
	}

	res := make([]string, 0, maxLines)
	for i := 0; i < len(runes); i += maxSymbols {
		end := i + 28
		if end > len(runes) {
			end = len(runes)
		}
		res = append(res, string(runes[i:end]))
	}

	return res
}

func improveLast(lastLen int, newLines []string) (toAppend string, remainingFirstLine string) {
	if len(newLines) == 0 {
		return "", ""
	}

	remained := maxSymbols - lastLen

	if len(newLines[0]) < remained {
		return newLines[0], ""
	}

	toAppend = newLines[0][:remained]
	remainingFirstLine = newLines[0][remained:]

	return toAppend, remainingFirstLine
}
