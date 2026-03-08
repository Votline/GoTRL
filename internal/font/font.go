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

	img := image.NewRGBA(image.Rect(0, 0, 410, 240))

	drawer.SetClip(img.Bounds())
	drawer.SetDPI(72)
	drawer.SetFont(f)
	drawer.SetFontSize(fontSize)
	drawer.SetDst(img)
	drawer.SetSrc(image.NewUniform(color.White))
	drawer.SetHinting(font.HintingNone)

	currentY := fontSize + 5
	for _, line := range lines {
		pt := freetype.Pt(10, currentY)
		_, err = drawer.DrawString(line, pt)
		if err != nil {
			log.Fatalln("Failed to draw string. \nErr: ", err)
		}
		currentY += fontSize + 10
		if currentY > 250 {
			break
		}
	}

	return img
}
