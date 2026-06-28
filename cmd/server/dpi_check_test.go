//go:build windows

package main

import (
	"syscall"
	"testing"
	"unsafe"
)

// TestDPIAwarenessApplied is a regression guard that the init() in
// dpi_windows.go made this process DPI-aware (PROCESS_PER_MONITOR_DPI_AWARE = 2).
// If this regresses, high-DPI/scaled monitors capture clipped/blurred and QR
// auto-detect silently degrades.
func TestDPIAwarenessApplied(t *testing.T) {
	p := syscall.NewLazyDLL("shcore.dll").NewProc("GetProcessDpiAwareness")
	if p.Find() != nil {
		t.Skip("GetProcessDpiAwareness unavailable")
	}
	var awareness uint32
	r, _, _ := p.Call(0, uintptr(unsafe.Pointer(&awareness)))
	if r != 0 {
		t.Fatalf("GetProcessDpiAwareness HRESULT=%#x", r)
	}
	t.Logf("process DPI awareness = %d (0=unaware,1=system,2=per-monitor)", awareness)
	if awareness != 2 {
		t.Errorf("want per-monitor (2), got %d", awareness)
	}
}
