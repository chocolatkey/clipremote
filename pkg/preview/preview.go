package preview

import (
	"fmt"
	"image"
)

func Decode(bin []byte, width int, height int) (*image.RGBA, error) {
	if need := width * height * 3; len(bin) < need {
		return nil, fmt.Errorf("preview: need %d bytes for %dx%d image, got %d", need, width, height, len(bin))
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	pix := img.Pix
	for si, di := 0, 0; di < len(pix); si, di = si+3, di+4 {
		pix[di+0] = bin[si+0] // R
		pix[di+1] = bin[si+1] // G
		pix[di+2] = bin[si+2] // B
		pix[di+3] = 0xFF      // A
	}
	return img, nil
}
