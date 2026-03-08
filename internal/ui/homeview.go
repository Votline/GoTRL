package ui

import (
	"fmt"
	"image"
	"sync"

	"gotrl/internal/font"
	"gotrl/internal/render"

	"github.com/go-gl/gl/v4.1-core/gl"
)

type HomeView struct {
	pg       uint32
	vao      uint32
	text     string
	uniforms map[string]int32
	img      *image.RGBA
	mu       sync.Mutex
}

const (
	orangeOpen  = "\033[33m"
	orangeClose = "\033[0m"
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

	hv.text += msg
	img := font.CreateImage(hv.text)

	if img == nil || img.Rect.Dx() == 0 || img.Rect.Dy() == 0 {
		fmt.Printf("%sWARNING: Image is empty, nothing to render!%s",
			orangeOpen, orangeClose)
		return
	}

	hv.img = img
}
