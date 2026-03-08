package render

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v4.1-core/gl"
)

const vertexShaderSource = `
#version 410 core
layout (location = 0) in vec3 aPos;
layout (location = 1) in vec2 aTexCoord;

out vec2 TexCoord;

void main()
{
    gl_Position = vec4(aPos, 1.0);
		TexCoord = aTexCoord;
}` + "\x00"

const fragmentShaderSource = `
#version 410 core
out vec4 FragColor;
in vec2 TexCoord;

uniform vec4 color;
uniform sampler2D uTex;

void main()
{
    FragColor = texture(uTex, TexCoord) * color;
}` + "\x00"

func attachShaders(pg uint32) []uint32 {
	vertexShader := compileShader(gl.VERTEX_SHADER, vertexShaderSource)
	fragmentShader := compileShader(gl.FRAGMENT_SHADER, fragmentShaderSource)

	gl.AttachShader(pg, vertexShader)
	gl.AttachShader(pg, fragmentShader)

	return []uint32{vertexShader, fragmentShader}
}

func compileShader(shaderType uint32, source string) uint32 {
	shader := gl.CreateShader(shaderType)
	cs, free := gl.Strs(source)
	defer free()

	gl.ShaderSource(shader, 1, cs, nil)
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)

		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(log))

		panic(fmt.Sprintf("Failed to compile %v: %v", shaderType, log))
	}

	return shader
}

func detachShaders(pg uint32, shaders []uint32) {
	for _, shader := range shaders {
		gl.DetachShader(pg, shader)
		gl.DeleteShader(shader)
	}
}
