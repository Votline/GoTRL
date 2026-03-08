package ui

import (
	"gotrl/internal/render"
)

type elemMesh struct {
	vao uint32
	vtq int32
}

type HomeView struct {
	pg uint32
}

func CreateHomeView(pg uint32) *HomeView {
	return &HomeView{pg: pg}
}

func (hv *HomeView) Render() {
	render.ElemRender(hv.pg, 0, 0)
}
