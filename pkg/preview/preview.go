package preview

import (
	"image"
	"image/color"
)

func Decode(bin []byte, width int, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := img.Bounds()
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, color.RGBA{
				R: bin[i],
				G: bin[i+1],
				B: bin[i+2],
				A: 255,
			})
			i += 3
		}
	}
	return img
}
