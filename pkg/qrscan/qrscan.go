// Package qrscan locates a QR code on the user's screen(s) so the app can start
// without manually photographing the CSP companion QR and decoding it to a URL.
//
// It screenshots every active display and decodes any QR codes it finds with a
// pure-Go ZXing port (no cgo). A caller-supplied predicate selects the QR of
// interest (e.g. "is this a CSP companion share URL?"), so unrelated QR codes
// on screen are ignored.
//
// NOTE: on Windows the host process must be DPI-aware before the first capture,
// otherwise high-DPI/scaled monitors are captured clipped and stretched and a
// small QR may fail to decode (see cmd/server/dpi_windows.go).
package qrscan

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// ErrNotFound is returned when a capture succeeded but no accepted QR code was
// present. It is the retryable case (unlike a capture-subsystem failure).
var ErrNotFound = errors.New("qrscan: no matching QR code found on screen")

// ErrNoDisplays is returned when no display could be captured at all (a headless
// or remote session, or an unsupported build such as macOS without cgo). It is
// not retryable.
var ErrNoDisplays = errors.New("qrscan: no displays could be captured (headless/remote session, or unsupported build) — pass the QR URL explicitly")

// qrHints asks the reader to spend extra effort (rotation/scale) finding a code,
// which matters when a small QR sits on a large, busy screenshot.
var qrHints = map[gozxing.DecodeHintType]interface{}{
	gozxing.DecodeHintType_TRY_HARDER: true,
}

// DecodeImage returns the payload of the first QR code found in img.
func DecodeImage(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", fmt.Errorf("qrscan: build bitmap: %w", err)
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, qrHints)
	if err != nil {
		return "", err // not found / not decodable
	}
	return res.GetText(), nil
}

// CaptureDisplays grabs a screenshot of every active display, or only the given
// display indices when only is non-empty. Displays that fail to capture are
// skipped; ErrNoDisplays is returned only if nothing could be grabbed.
func CaptureDisplays(only ...int) ([]image.Image, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, ErrNoDisplays
	}
	indices := only
	if len(indices) == 0 {
		indices = make([]int, n)
		for i := range indices {
			indices[i] = i
		}
	}
	imgs := make([]image.Image, 0, len(indices))
	for _, i := range indices {
		if i < 0 || i >= n {
			continue
		}
		img, err := screenshot.CaptureDisplay(i)
		if err != nil || img == nil {
			continue
		}
		imgs = append(imgs, img)
	}
	if len(imgs) == 0 {
		return nil, ErrNoDisplays
	}
	return imgs, nil
}

// ScanOnce captures the selected displays and returns the first decoded QR
// payload that satisfies accept. A nil accept matches any QR code. It returns
// ErrNotFound when capture worked but nothing matched, or ErrNoDisplays when
// nothing could be captured.
func ScanOnce(accept func(string) bool, only ...int) (string, error) {
	imgs, err := CaptureDisplays(only...)
	if err != nil {
		return "", err
	}
	for _, img := range imgs {
		txt, err := DecodeImage(img)
		if err != nil {
			continue
		}
		if accept == nil || accept(txt) {
			return txt, nil
		}
	}
	return "", ErrNotFound
}

// Find repeatedly scans the selected displays until an accepted QR code appears
// or ctx is cancelled. Only the retryable "not found yet" case is retried; a
// capture-subsystem failure (ErrNoDisplays) returns immediately rather than
// spinning for the whole timeout. interval is the pause between scans (each scan
// also takes time). The optional onAttempt callback is invoked after each
// unsuccessful scan with the attempt count, for progress reporting.
func Find(ctx context.Context, accept func(string) bool, interval time.Duration, onAttempt func(int), only ...int) (string, error) {
	if interval <= 0 {
		interval = 750 * time.Millisecond
	}
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		txt, err := ScanOnce(accept, only...)
		if err == nil {
			return txt, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err // genuine capture failure — do not spin until timeout
		}
		if onAttempt != nil {
			onAttempt(attempt)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}
