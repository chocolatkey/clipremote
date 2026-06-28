package clipremote

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/packets"
	"github.com/chocolatkey/clipremote/pkg/protocol"
)

func TestPlanCanvasTiles(t *testing.T) {
	cases := []struct {
		w, h, budget, maxSide, wantTiles int
	}{
		{4096, 5684, 2_400_000, 2000, 15}, // the manga page
		{720, 21818, 2_400_000, 2000, 11}, // a tall webtoon (full-width strips)
		{600, 600, 62_500, 250, 9},        // small grid (both column and row splits)
	}
	for _, tc := range cases {
		tiles := planCanvasTiles(tc.w, tc.h, tc.budget, tc.maxSide)
		if len(tiles) != tc.wantTiles {
			t.Errorf("planCanvasTiles(%d,%d): %d tiles, want %d", tc.w, tc.h, len(tiles), tc.wantTiles)
		}
		area := 0
		for _, r := range tiles {
			if r.Dx() > tc.maxSide || r.Dy() > tc.maxSide {
				t.Errorf("tile %v exceeds max side %d", r, tc.maxSide)
			}
			if r.Dx()*r.Dy() > tc.budget {
				t.Errorf("tile %v area %d exceeds budget %d", r, r.Dx()*r.Dy(), tc.budget)
			}
			if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > tc.w || r.Max.Y > tc.h {
				t.Errorf("tile %v out of bounds %dx%d", r, tc.w, tc.h)
			}
			area += r.Dx() * r.Dy()
		}
		// Exact tiling: areas sum to the whole canvas (no gaps, no overlaps).
		if area != tc.w*tc.h {
			t.Errorf("covered area %d, want %d (gaps or overlaps)", area, tc.w*tc.h)
		}
	}
}

// TestPreviewWebtoonReadCanvas assembles a whole canvas from tiled blocks served
// by a fake host that renders a GLOBAL gradient, then asserts the reassembled
// image is seamless (continuous across every block boundary).
func TestPreviewWebtoonReadCanvas(t *testing.T) {
	const W, H = 600, 600
	grad := func(X, Y int) (byte, byte, byte) { return byte(X * 255 / W), byte(Y * 255 / H), 128 }

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		for {
			data, err := r.ReadBytes(0x00)
			if err != nil {
				return
			}
			var sc packets.ServerCommand
			if sc.Parse(data) != nil || sc.Type != packets.TypeClientCommand {
				continue
			}
			var reply []byte
			switch sc.Command {
			case commands.Authenticate:
				reply = []byte(`{"AuthErrorReason":"","RemoteCommandSpecVersionOfServer":"1.0","IsQuickAccessAvailable":true}`)
			case commands.PreviewWebtoonFromClient:
				var req struct{ BlockLeft, BlockTop, BlockRight, BlockBottom int }
				_ = json.Unmarshal(sc.RawDetail, &req)
				bw, bh := req.BlockRight-req.BlockLeft, req.BlockBottom-req.BlockTop
				rgb := make([]byte, 0, bw*bh*3)
				for y := 0; y < bh; y++ {
					for x := 0; x < bw; x++ {
						cr, cg, cb := grad(req.BlockLeft+x, req.BlockTop+y)
						rgb = append(rgb, cr, cg, cb)
					}
				}
				reply = append([]byte(`{"Operation":"ReadPreviewBlock"}`), protocol.DetailSeparator)
				reply = append(reply, []byte(base64.StdEncoding.EncodeToString(rgb))...)
			}
			if packets.WriteFrame(conn, packets.TypeServerResponseSuccess, sc.Command, sc.Serial, reply) != nil {
				return
			}
		}
	}()

	client, err := Connect([]string{"127.0.0.1"}, port, "G#1:test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Authenticate("password"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	img, err := client.PreviewWebtoonReadCanvas(ctx, 1, 0, W, H, &CanvasReadOptions{
		MaxBlockPixels: 62_500, MaxBlockSide: 250, Concurrency: 3, // forces a 3x3 grid of blocks
	})
	if err != nil {
		t.Fatalf("PreviewWebtoonReadCanvas: %v", err)
	}
	if img.Bounds().Dx() != W || img.Bounds().Dy() != H {
		t.Fatalf("assembled size %v, want %dx%d", img.Bounds(), W, H)
	}

	// Sample the corners and points straddling every block boundary (x/y = 250, 500).
	for _, p := range [][2]int{
		{0, 0}, {599, 599}, {300, 300},
		{249, 300}, {250, 300}, {251, 300}, // column seam at x=250
		{499, 300}, {500, 300}, {501, 300}, // column seam at x=500
		{300, 249}, {300, 250}, {300, 251}, // row seam at y=250
		{250, 250}, {500, 500}, // four-tile corners
	} {
		X, Y := p[0], p[1]
		i := img.PixOffset(X, Y)
		wr, wg, wb := grad(X, Y)
		if absInt(int(img.Pix[i])-int(wr)) > 1 || absInt(int(img.Pix[i+1])-int(wg)) > 1 ||
			img.Pix[i+2] != wb || img.Pix[i+3] != 0xFF {
			t.Errorf("pixel (%d,%d) = %v, want ~(%d,%d,%d,255) — seam/misplacement",
				X, Y, img.Pix[i:i+4], wr, wg, wb)
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
