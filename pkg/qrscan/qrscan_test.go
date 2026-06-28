package qrscan

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// encodeQR renders text as a size×size QR bitmap (black on white).
func encodeQR(t *testing.T, text string, size int) *image.Gray {
	t.Helper()
	bm, err := qrcode.NewQRCodeWriter().Encode(text, gozxing.BarcodeFormat_QR_CODE, size, size, nil)
	if err != nil {
		t.Fatalf("encode QR: %v", err)
	}
	w, h := bm.GetWidth(), bm.GetHeight()
	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(255)
			if bm.Get(x, y) {
				v = 0
			}
			g.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return g
}

func TestDecodeRoundTrip(t *testing.T) {
	const text = "https://companion.clip-studio.com/rc/en-us?s=deadbeefcafe"
	got, err := DecodeImage(encodeQR(t, text, 300))
	if err != nil {
		t.Fatalf("DecodeImage: %v", err)
	}
	if got != text {
		t.Fatalf("got %q want %q", got, text)
	}
}

// TestDecodeSmallQROnLargeScreen places a realistic-length companion URL QR on a
// 2560×1440 white "screen" to confirm small-code-on-big-image detection works
// (the actual use case: a QR dialog somewhere on a full desktop screenshot).
func TestDecodeSmallQROnLargeScreen(t *testing.T) {
	text := "https://companion.clip-studio.com/rc/en-us?s=" +
		"458ba0757b17e25a83bc697910f64582a2757210f44585aa757213d34780a2687f28f915e7ee6c0e6d867df5b16a7013ea4680bc6a78"
	qr := encodeQR(t, text, 420)

	big := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
	draw.Draw(big, big.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(big, qr.Bounds().Add(image.Pt(1700, 250)), qr, image.Point{}, draw.Src)

	got, err := DecodeImage(big)
	if err != nil {
		t.Fatalf("DecodeImage(large): %v", err)
	}
	if got != text {
		t.Fatalf("got %q want %q", got, text)
	}
}

// TestCaptureDisplaysSmoke proves the screen-capture path works in this
// environment; it is skipped where no display is available.
func TestCaptureDisplaysSmoke(t *testing.T) {
	imgs, err := CaptureDisplays()
	if err != nil {
		t.Skipf("no displays to capture: %v", err)
	}
	for i, img := range imgs {
		b := img.Bounds()
		t.Logf("display %d: %dx%d", i, b.Dx(), b.Dy())
	}
}
