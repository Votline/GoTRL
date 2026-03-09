package font

import (
	"image"
	"image/color"
	"io/ioutil"
	"log"

	"github.com/golang/freetype"
	"golang.org/x/image/font"
)

const fontSize = 15

const fontPath = "assets/JetBrainsMono-Regular.ttf"

func CreateImage(lines []string) *image.RGBA {
	fontBytes, err := ioutil.ReadFile(fontPath)
	if err != nil {
		log.Fatalln("Read ttf file error. \nErr: ", err)
	}

	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		log.Fatalln("Parse font error. \nErr: ", err)
	}

	drawer := freetype.NewContext()

	img := image.NewRGBA(image.Rect(-10, -10, 410, 260))

	drawer.SetClip(img.Bounds())
	drawer.SetDPI(144)
	drawer.SetFont(f)
	drawer.SetFontSize(fontSize)
	drawer.SetDst(img)
	drawer.SetSrc(image.NewUniform(color.White))
	drawer.SetHinting(font.HintingNone)

	currentY := fontSize + 10
	for _, line := range lines {
		pt := freetype.Pt(10, currentY)
		_, err = drawer.DrawString(line, pt)
		if err != nil {
			log.Fatalln("Failed to draw string. \nErr: ", err)
		}
		currentY += fontSize + 20
		if currentY > 250 {
			break
		}
	}

	return img
}
