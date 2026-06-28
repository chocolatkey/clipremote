package main

import (
	"reflect"
	"testing"
)

func TestIsLANAddr(t *testing.T) {
	lan := []string{
		"192.168.1.50", "10.0.0.3", "172.16.5.5", "127.0.0.1",
		"169.254.1.1",  // link-local
		"100.115.92.1", // CGNAT / Tailscale
		"fc00::1", "::1", "fe80::1",
		"some-host.local", // hostname — not classifiable, allowed
	}
	for _, a := range lan {
		if !isLANAddr(a) {
			t.Errorf("isLANAddr(%q) = false, want true", a)
		}
	}
	public := []string{
		"8.8.8.8", "1.1.1.1", "203.0.113.7", "2606:4700:4700::1111",
	}
	for _, a := range public {
		if isLANAddr(a) {
			t.Errorf("isLANAddr(%q) = true, want false", a)
		}
	}
}

func TestParseScanDisplays(t *testing.T) {
	cases := map[string][]int{
		"":        nil,
		"  ":      nil,
		"0":       {0},
		"0,2":     {0, 2},
		" 1 , 3 ": {1, 3},
		"x,2,-1":  {2}, // non-ints and negatives dropped
	}
	for in, want := range cases {
		if got := parseScanDisplays(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseScanDisplays(%q) = %v, want %v", in, got, want)
		}
	}
}
