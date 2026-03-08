package font

import (
	"image"
	"image/color"
	"io/ioutil"
	"log"

	"github.com/golang/freetype"
	"golang.org/x/image/font"
)

const fontSize = 25

const fontPath = "assets/NotoSans-Regular.ttf"

func CreateImage(txt string) *image.RGBA {
	fontBytes, err := ioutil.ReadFile(fontPath)
	if err != nil {
		log.Fatalln("Read ttf file error. \nErr: ", err)
	}

	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		log.Fatalln("Parse font error. \nErr: ", err)
	}

	drawer := freetype.NewContext()

	fontWidth := int(float64(fontSize) * float64(len(txt)) * 0.6)
	fontHeight := fontSize + 10
	img := image.NewRGBA(image.Rect(0, 0, fontWidth, fontHeight))

	drawer.SetClip(img.Bounds())
	drawer.SetDPI(72)
	drawer.SetFont(f)
	drawer.SetFontSize(fontSize)
	drawer.SetDst(img)
	drawer.SetSrc(image.NewUniform(color.White))
	drawer.SetHinting(font.HintingFull)

	pt := freetype.Pt(10, fontSize+5)
	_, err = drawer.DrawString(txt, pt)
	if err != nil {
		log.Fatalln("Failed to draw string. \nErr: ", err)
	}

	return img
}
