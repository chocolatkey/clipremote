package clipremote

import (
	"context"
	"fmt"
	"image"
	"sync"
)

// CSP refuses (times out on) a single webtoon-preview block larger than ~2.62 MP,
// so a whole canvas must be read as multiple sub-blocks and reassembled. These
// defaults keep every block safely under that limit.
const (
	defaultMaxBlockPixels    = 2_400_000 // px per block (safely under CSP's ~2.62 MP)
	defaultMaxBlockSide      = 2000      // px; keep each block dimension within known-good range
	defaultCanvasConcurrency = 4         // concurrent block requests
)

// CanvasReadOptions tunes PreviewWebtoonReadCanvas. The zero value (or nil) uses
// the defaults above.
type CanvasReadOptions struct {
	MaxBlockPixels int // max pixels per requested block
	MaxBlockSide   int // max width/height of a requested block
	Concurrency    int // number of blocks fetched in parallel
}

func (o *CanvasReadOptions) withDefaults() CanvasReadOptions {
	out := CanvasReadOptions{
		MaxBlockPixels: defaultMaxBlockPixels,
		MaxBlockSide:   defaultMaxBlockSide,
		Concurrency:    defaultCanvasConcurrency,
	}
	if o != nil {
		if o.MaxBlockPixels > 0 {
			out.MaxBlockPixels = o.MaxBlockPixels
		}
		if o.MaxBlockSide > 0 {
			out.MaxBlockSide = o.MaxBlockSide
		}
		if o.Concurrency > 0 {
			out.Concurrency = o.Concurrency
		}
	}
	return out
}

// planCanvasTiles splits a width×height canvas into block rectangles whose area
// is ≤ budget and whose sides are ≤ maxSide, row-major from the top-left. Blocks
// abut exactly (no gaps, no overlap), so reassembling them is seamless.
func planCanvasTiles(width, height, budget, maxSide int) []image.Rectangle {
	if budget < 50_000 {
		budget = defaultMaxBlockPixels
	}
	if maxSide < 1 {
		maxSide = defaultMaxBlockSide
	}
	tileW := minInt(width, maxSide, budget)
	if tileW < 1 {
		tileW = 1
	}
	tileH := minInt(height, maxSide, budget/tileW)
	if tileH < 1 {
		tileH = 1
	}
	var tiles []image.Rectangle
	for top := 0; top < height; top += tileH {
		bottom := top + tileH
		if bottom > height {
			bottom = height
		}
		for left := 0; left < width; left += tileW {
			right := left + tileW
			if right > width {
				right = width
			}
			tiles = append(tiles, image.Rect(left, top, right, bottom))
		}
	}
	return tiles
}

// PreviewWebtoonReadCanvas reads an entire webtoon-preview canvas and assembles
// it into a single in-memory image. It tiles the canvas into sub-blocks under
// CSP's per-block pixel limit, fetches them in parallel, and blits each block's
// pixels into one image.NewRGBA at its offset (the same way preview.Decode fills
// an RGBA from raw pixels). Pass nil opts for the defaults.
func (c *Client) PreviewWebtoonReadCanvas(ctx context.Context, galleryID, canvasIndex, width, height int, opts *CanvasReadOptions) (*image.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("clipremote: invalid canvas size %dx%d", width, height)
	}
	o := opts.withDefaults()
	tiles := planCanvasTiles(width, height, o.MaxBlockPixels, o.MaxBlockSide)
	full := image.NewRGBA(image.Rect(0, 0, width, height))

	type job struct {
		idx  int
		rect image.Rectangle
	}
	type result struct {
		rect image.Rectangle
		img  *image.RGBA
		err  error
	}

	jobs := make(chan job, len(tiles))
	for i, r := range tiles {
		jobs <- job{idx: i, rect: r}
	}
	close(jobs)
	results := make(chan result, len(tiles))

	conc := o.Concurrency
	if conc > len(tiles) {
		conc = len(tiles)
	}
	var wg sync.WaitGroup
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				block, err := c.PreviewWebtoonReadBlock(ctx, galleryID, canvasIndex, j.idx, j.rect)
				if err != nil {
					results <- result{rect: j.rect, err: err}
					continue
				}
				results <- result{rect: j.rect, img: block.Image}
			}
		}()
	}

	// Collect every block and blit it into the full canvas. Drawing happens here
	// in a single goroutine, so there is no contention on the shared image.
	var firstErr error
	drawn := 0
	for range tiles {
		r := <-results
		switch {
		case r.err != nil:
			if firstErr == nil {
				firstErr = r.err
			}
		case r.img == nil:
			if firstErr == nil {
				firstErr = fmt.Errorf("clipremote: preview block %v returned no pixels", r.rect)
			}
		default:
			blitRGBA(full, r.img, r.rect.Min.X, r.rect.Min.Y)
			drawn++
		}
	}
	wg.Wait()

	if firstErr != nil {
		return nil, fmt.Errorf("clipremote: assembled %d/%d preview blocks: %w", drawn, len(tiles), firstErr)
	}
	return full, nil
}

// blitRGBA copies src into dst with its top-left at (ox, oy), row by row.
func blitRGBA(dst, src *image.RGBA, ox, oy int) {
	rowBytes := src.Rect.Dx() * 4
	for y := 0; y < src.Rect.Dy(); y++ {
		si := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y)
		di := dst.PixOffset(ox, oy+y)
		copy(dst.Pix[di:di+rowBytes], src.Pix[si:si+rowBytes])
	}
}

func minInt(vs ...int) int {
	m := vs[0]
	for _, v := range vs[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
