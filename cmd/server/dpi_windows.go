//go:build windows

package main

import "syscall"

// init makes the process per-monitor DPI aware before any screen capture runs.
//
// The screenshot library reports each monitor's PHYSICAL pixel bounds, but a
// DPI-unaware process only gets the DWM-virtualized (scaled) desktop when it
// BitBlts — so on a high-DPI/scaled monitor the captured image is clipped and
// stretched, and a small QR code can blur enough to fail decoding. Declaring
// per-monitor-v2 awareness makes the captured pixels match the queried bounds.
//
// This is done via the runtime API (not an embedded manifest/.syso) so the tool
// still builds and runs with a plain `go run`/`go build`, no extra tooling.
func init() {
	user32 := syscall.NewLazyDLL("user32.dll")

	// Preferred: SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2). The context
	// value DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is (HANDLE)-4; ^uintptr(3)
	// is -4 in two's complement. Available on Windows 10 1703+.
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		if r, _, _ := p.Call(^uintptr(3)); r != 0 {
			return
		}
	}

	// Fallback: shcore!SetProcessDpiAwareness(PROCESS_PER_MONITOR_DPI_AWARE = 2)
	// (Windows 8.1+). Returns S_OK (0) on success.
	if p := syscall.NewLazyDLL("shcore.dll").NewProc("SetProcessDpiAwareness"); p.Find() == nil {
		if r, _, _ := p.Call(uintptr(2)); r == 0 {
			return
		}
	}

	// Last resort: user32!SetProcessDPIAware (system-DPI aware; Vista+).
	if p := user32.NewProc("SetProcessDPIAware"); p.Find() == nil {
		_, _, _ = p.Call()
	}
}
