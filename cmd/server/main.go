// Command server is an HTTP bridge to a CSP companion-mode host that also serves
// an embedded web UI (at /) for driving every command from a browser.
//
// Usage:
//
//	# auto-detect: screenshot every display and find the CSP companion QR code
//	go run ./cmd/server
//
//	# or pass the decoded QR URL explicitly
//	go run ./cmd/server "https://companion.clip-studio.com/rc/en-us?s=XXXX"
//
// Then open http://localhost:8089 in a browser, or drive CSP over HTTP directly:
//
//	# any command + JSON detail -> JSON response
//	curl 'http://localhost:8089/request?command=GetModifyKeyString&detail={"ShiftPushed":false,"CtrlPushed":false,"AltPushed":false}'
//	curl 'http://localhost:8089/request?command=SetBrushSize&detail=42'
//	curl 'http://localhost:8089/request?command=DoNavigator&detail={"CommandType":"ZoomIn"}'
//	# webtoon preview block -> PNG (or BMP without format=png)
//	curl 'http://localhost:8089/preview?format=png&gallery_identification_number=1&canvas_index=0&block_index=0&block_left=0&block_top=0&block_right=690&block_bottom=1024' -o block.png
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chocolatkey/clipremote"
	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/qrscan"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/bmp"
)

const (
	listenAddr = ":8089"
	// scanTimeout bounds how long auto-detect waits for the QR dialog to appear.
	scanTimeout = 90 * time.Second
)

// webuiFS holds the browser UI, compiled into the binary so the server needs no
// external files or build step.
//
//go:embed webui
var webuiFS embed.FS

func main() {
	if os.Getenv("CLIPREMOTE_DEBUG") != "" {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	shareURL, scanned, err := resolveShareURL()
	if err != nil {
		logrus.Fatalln(err)
	}

	client, password, err := clipremote.ConnectFromURL(shareURL)
	if err != nil {
		logrus.Fatalf("connect failed: %v%s", err, staleQRHint(scanned))
	}

	// Fan host pushes out to both the log and any connected browser (SSE).
	hub := newHub()
	registerPushLoggers(client, hub)

	res, err := client.Authenticate(password)
	if err != nil {
		logrus.Fatalf("authentication failed: %v%s", err, staleQRHint(scanned))
	}
	logrus.Infof("authenticated (host spec %s, quick-access=%v)", res.ServerSpecVersion, res.QuickAccessAvailable)
	status := &serverStatus{client: client, auth: res}

	sub, err := fs.Sub(webuiFS, "webui")
	if err != nil {
		logrus.Fatalln("embed sub:", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/request", requestHandler(client))
	mux.HandleFunc("/preview", previewHandler(client))
	mux.HandleFunc("/canvas", canvasHandler(client))
	mux.HandleFunc("/api/status", statusHandler(status))
	mux.HandleFunc("/api/events", hub.handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if client.Alive() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})
	// Everything else is the embedded web UI (the catch-all "/" pattern is only
	// matched after the more specific patterns above).
	mux.Handle("/", http.FileServer(http.FS(sub)))

	logrus.Infoln("clipremote web UI + HTTP bridge listening on http://localhost" + listenAddr)
	srv := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	logrus.Fatalln(srv.ListenAndServe())
}

// resolveShareURL returns the companion share URL to connect with and whether it
// came from a screen scan. If the first argument is a URL it is used directly;
// otherwise (no argument, or the literal "scan") it screenshots the display(s)
// and waits for the CSP companion QR code to appear, decoding it automatically.
func resolveShareURL() (string, bool, error) {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "scan", "--scan", "-scan":
			// fall through to screen scanning
		default:
			return os.Args[1], false, nil
		}
	}

	displays := parseScanDisplays(os.Getenv("CLIPREMOTE_SCAN_DISPLAY"))
	logrus.Infof("no QR URL given — repeatedly screenshotting %s to find the CSP companion QR code "+
		"(keep the \"Connect to smartphone\" dialog visible; waiting up to %s)…", scanScopeLabel(displays), scanTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	// Wall-clock progress so the wait never looks hung regardless of per-scan cost.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if dl, ok := ctx.Deadline(); ok {
					logrus.Infof("…still scanning for the QR code (%s left)", time.Until(dl).Round(time.Second))
				}
			}
		}
	}()

	url, err := qrscan.Find(ctx, clipremote.IsShareURL, 0, nil, displays...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", true, fmt.Errorf("no CSP companion QR code found on screen within %s "+
				"(is the QR dialog open and fully visible?) — you can also pass the URL explicitly: "+
				"server \"https://companion.clip-studio.com/rc/...\"", scanTimeout)
		}
		return "", true, fmt.Errorf("%w", err) // e.g. ErrNoDisplays, already actionable
	}

	// Show where this QR will connect (password redacted), and guard against a
	// decoy QR on screen redirecting the connection off the LAN.
	if ips, port, _, _, derr := clipremote.DecodeConfig(url); derr == nil {
		logrus.Infof("found the CSP companion QR code on screen ✔ — host %s:%d", strings.Join(ips, ","), port)
		if os.Getenv("CLIPREMOTE_ALLOW_PUBLIC") == "" {
			for _, ip := range ips {
				if !isLANAddr(ip) {
					return "", true, fmt.Errorf("the scanned QR points to non-LAN address %q, which is unexpected for "+
						"CSP companion mode (a decoy QR on screen could redirect this password-bearing connection); "+
						"set CLIPREMOTE_ALLOW_PUBLIC=1 to override, or pass the URL explicitly", ip)
				}
			}
		}
	} else {
		logrus.Infoln("found the CSP companion QR code on screen ✔")
	}
	return url, true, nil
}

// staleQRHint adds guidance to a connect/auth failure when the URL was scanned,
// since a stale on-screen QR is a common cause.
func staleQRHint(scanned bool) string {
	if !scanned {
		return ""
	}
	return "\n(auto-detect may have read a stale/old QR — re-open the CSP \"Connect to smartphone\" dialog, " +
		"or pass the current URL explicitly: server \"https://companion.clip-studio.com/rc/...\")"
}

// isLANAddr reports whether host is the kind of address a CSP companion host on
// a local network should advertise: private (RFC1918/ULA), loopback, link-local,
// or CGNAT/Tailscale (100.64.0.0/10). A non-literal host (a name) is allowed.
func isLANAddr(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return true // hostname, not classifiable here — allow
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true // 100.64.0.0/10 shared address space (CGNAT, Tailscale)
	}
	return false
}

// parseScanDisplays parses CLIPREMOTE_SCAN_DISPLAY ("0" or "0,2") into indices.
func parseScanDisplays(s string) []int {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 0 {
			out = append(out, n)
		}
	}
	return out
}

func scanScopeLabel(displays []int) string {
	if len(displays) == 0 {
		return "all displays"
	}
	return fmt.Sprintf("display %v", displays)
}

func printUsage() {
	fmt.Printf(`clipremote server — CSP companion-mode web UI + HTTP bridge

Usage:
  server                     auto-detect: screenshot the display(s) and find the QR
  server scan                same as above (explicit)
  server <QR share URL>      connect using a decoded QR URL
  server -h | --help         show this help

Auto-detect repeatedly screenshots every display for up to %s, waiting for the
CSP "Connect to smartphone" QR dialog, then connects. Then open
http://localhost:8089 for the web UI.

Environment:
  CLIPREMOTE_DEBUG          if set to any value, log every protocol frame
                            (the connection password is redacted)
  CLIPREMOTE_SCAN_DISPLAY   limit auto-detect to display index(es), e.g. 0 or 0,2
  CLIPREMOTE_ALLOW_PUBLIC   if set, allow a scanned QR that points off the LAN
`, scanTimeout)
}

// serverStatus is the small snapshot exposed at /api/status.
type serverStatus struct {
	client *clipremote.Client
	auth   commands.AuthResult
}

// statusHandler reports live connection health and the negotiated host info.
func statusHandler(s *serverStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"alive":                s.client.Alive(),
			"remoteAddr":           s.client.RemoteAddr(),
			"specVersion":          s.auth.ServerSpecVersion,
			"quickAccessAvailable": s.auth.QuickAccessAvailable,
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// registerPushLoggers wires the host-initiated commands to log lines (and lets
// the client auto-ACK them), and mirrors each push to the SSE hub so the browser
// can display live UI-state updates.
func registerPushLoggers(client *clipremote.Client, hub *hub) {
	client.OnPreNotifyOfServerShutdown(func(e commands.PreNotifyOfServerShutdownRequest) {
		logrus.Warnf("host announced shutdown (manual=%v)", e.IsManualShutdown)
		hub.broadcast("PreNotifyOfServerShutdown", e)
	})
	client.OnSyncColorCircleUIState(func(s *commands.ColorCircleUIState) {
		logrus.Debugf("push SyncColorCircleUIState: %+v", s)
		hub.broadcast("SyncColorCircleUIState", s)
	})
	client.OnSyncColorMixUIState(func(s *commands.ColorRGBUIState) {
		logrus.Debugf("push SyncColorMixUIState: %+v", s)
		hub.broadcast("SyncColorMixUIState", s)
	})
	client.OnSyncSubViewUIState(func(s *commands.ColorRGBUIState) {
		logrus.Debugf("push SyncSubViewUIState: %+v", s)
		hub.broadcast("SyncSubViewUIState", s)
	})
	client.OnSyncGesturePadUIState(func(s *commands.GesturePadUIState) {
		logrus.Debugf("push SyncGesturePadUIState: %+v", s)
		hub.broadcast("SyncGesturePadUIState", s)
	})
	client.OnSyncSettingUIState(func(t commands.TabKind) {
		logrus.Debugf("push SyncSettingUIState: %s", t)
		hub.broadcast("SyncSettingUIState", map[string]any{"ServerSelectedTabKind": t})
	})
	client.OnSyncQuickAccessUIState(func(s *commands.QuickAccessUIState) {
		logrus.Debugf("push SyncQuickAccessUIState: %+v", s)
		hub.broadcast("SyncQuickAccessUIState", s)
	})
	client.OnPreviewWebtoonFromServer(func(pkt *clipremote.ServerPacket) {
		logrus.Debugf("push PreviewWebtoonFromServer: %s", string(pkt.RawDetail))
		hub.broadcast("PreviewWebtoonFromServer", pkt)
	})
}

// requestHandler sends any command with a JSON detail and returns the response.
func requestHandler(client *clipremote.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if !client.Alive() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		command := r.Form.Get("command")
		if command == "" {
			http.Error(w, "missing command", http.StatusBadRequest)
			return
		}

		var detail any
		if raw := r.Form.Get("detail"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &detail); err != nil {
				http.Error(w, "detail must be valid JSON", http.StatusBadRequest)
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		scp, err := client.SendCommandSync(ctx, commands.Command(command), detail)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(scp)
	}
}

// previewHandler reads a webtoon preview block and returns it as a PNG
// (format=png) or BMP (default) image.
func previewHandler(client *clipremote.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		atoi := func(name string) (int, bool) {
			n, err := strconv.Atoi(r.FormValue(name))
			if err != nil {
				http.Error(w, "invalid/empty "+name, http.StatusBadRequest)
				return 0, false
			}
			return n, true
		}
		galleryID, ok := atoi("gallery_identification_number")
		if !ok {
			return
		}
		canvasIndex, ok := atoi("canvas_index")
		if !ok {
			return
		}
		blockIndex, ok := atoi("block_index")
		if !ok {
			return
		}
		left, ok := atoi("block_left")
		if !ok {
			return
		}
		top, ok := atoi("block_top")
		if !ok {
			return
		}
		right, ok := atoi("block_right")
		if !ok {
			return
		}
		bottom, ok := atoi("block_bottom")
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		block, err := client.PreviewWebtoonReadBlock(ctx, galleryID, canvasIndex, blockIndex,
			image.Rect(left, top, right, bottom))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if block.Image == nil {
			http.Error(w, "no image data in preview block", http.StatusInternalServerError)
			return
		}
		if r.FormValue("format") == "png" {
			w.Header().Set("content-type", "image/png")
			if err := png.Encode(w, block.Image); err != nil {
				logrus.Warnln("png encode failed:", err)
			}
			return
		}
		w.Header().Set("content-type", "image/bmp")
		if err := bmp.Encode(w, block.Image); err != nil {
			logrus.Warnln("bmp encode failed:", err)
		}
	}
}

// canvasHandler reads and assembles an ENTIRE webtoon-preview canvas server-side
// (tiling under CSP's per-block limit and stitching the blocks into one image),
// returning it as a single PNG (or BMP with format=bmp).
//
//	GET /canvas?canvas_index=2                       (gallery + size auto-discovered)
//	GET /canvas?canvas_index=2&max_block_pixels=2000000&concurrency=6&format=png
//	GET /canvas?canvas_index=2&gallery_identification_number=2&canvas_width=4096&canvas_height=5684
func canvasHandler(client *clipremote.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if !client.Alive() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		canvasIndex, err := strconv.Atoi(r.FormValue("canvas_index"))
		if err != nil || canvasIndex < 0 {
			http.Error(w, "invalid/missing canvas_index", http.StatusBadRequest)
			return
		}

		// A whole canvas can be many blocks; give it room.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		// Discover the gallery id and the canvas size (mirrors the UI's scan step),
		// unless the caller supplied them explicitly.
		width := atoiDefault(r.FormValue("canvas_width"), 0)
		height := atoiDefault(r.FormValue("canvas_height"), 0)
		galleryID := atoiDefault(r.FormValue("gallery_identification_number"), -1)
		if width <= 0 || height <= 0 || galleryID < 0 {
			_, _ = client.PreviewWebtoonSyncPreview(ctx) // best-effort sync
			gal, err := client.PreviewWebtoonUpdateGallery(ctx, atoiDefault(r.FormValue("max_length"), 4096))
			if err != nil {
				http.Error(w, "gallery scan failed: "+err.Error(), http.StatusBadGateway)
				return
			}
			if canvasIndex >= len(gal.CanvasSizeArray) {
				http.Error(w, fmt.Sprintf("canvas_index %d out of range (gallery has %d canvases)", canvasIndex, len(gal.CanvasSizeArray)), http.StatusBadRequest)
				return
			}
			if galleryID < 0 {
				galleryID = gal.GalleryIdentificationNumber
			}
			if width <= 0 {
				width = gal.CanvasSizeArray[canvasIndex].CanvasWidth
			}
			if height <= 0 {
				height = gal.CanvasSizeArray[canvasIndex].CanvasHeight
			}
		}

		opts := &clipremote.CanvasReadOptions{
			MaxBlockPixels: atoiDefault(r.FormValue("max_block_pixels"), 0),
			Concurrency:    atoiDefault(r.FormValue("concurrency"), 0),
		}
		img, err := client.PreviewWebtoonReadCanvas(ctx, galleryID, canvasIndex, width, height, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if r.FormValue("format") == "bmp" {
			w.Header().Set("content-type", "image/bmp")
			if err := bmp.Encode(w, img); err != nil {
				logrus.Warnln("bmp encode failed:", err)
			}
			return
		}
		w.Header().Set("content-type", "image/png")
		if err := png.Encode(w, img); err != nil {
			logrus.Warnln("png encode failed:", err)
		}
	}
}

// atoiDefault parses s as an int, returning def for empty or invalid input.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// ---------------------------------------------------------------------------
// SSE hub: broadcasts host-pushed commands to every connected browser.
// ---------------------------------------------------------------------------

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub() *hub { return &hub{subs: make(map[chan []byte]struct{})} }

// broadcast marshals an event and delivers it to every subscriber, dropping the
// message for any subscriber whose buffer is full (a slow browser never blocks
// the receive goroutine).
func (h *hub) broadcast(event string, payload any) {
	msg, err := json.Marshal(map[string]any{
		"event": event,
		"data":  payload,
		"ts":    time.Now().UnixMilli(),
	})
	if err != nil {
		logrus.Debugln("hub marshal:", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// handler streams events to a browser as Server-Sent Events.
func (h *hub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch := make(chan []byte, 32)
		h.mu.Lock()
		h.subs[ch] = struct{}{}
		h.mu.Unlock()
		defer func() {
			h.mu.Lock()
			delete(h.subs, ch)
			close(ch)
			h.mu.Unlock()
		}()

		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("cache-control", "no-cache")
		w.Header().Set("connection", "keep-alive")
		_, _ = w.Write([]byte("retry: 2000\n\n"))
		fl.Flush()

		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-ch:
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(msg)
				_, _ = w.Write([]byte("\n\n"))
				fl.Flush()
			case <-ping.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				fl.Flush()
			}
		}
	}
}
