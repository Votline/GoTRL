package render

import (
	"github.com/go-gl/gl/v4.1-core/gl"
)

func Setup() uint32 {
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	pg := gl.CreateProgram()

	shaders := attachShaders(pg)

	gl.LinkProgram(pg)
	gl.UseProgram(pg)

	detachShaders(pg, shaders)

	return pg
}

func ElemRender(pg, vao uint32, vtq int32) {
	gl.UseProgram(pg)

	gl.BindVertexArray(vao)
	gl.DrawElements(gl.TRIANGLES, vtq, gl.UNSIGNED_INT, nil)

	gl.BindVertexArray(0)
}
