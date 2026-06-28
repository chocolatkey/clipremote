package clipremote_test

import (
	"encoding/hex"
	"testing"

	"github.com/chocolatkey/clipremote"
	"github.com/chocolatkey/clipremote/pkg/crypto"
)

// sampleShareURL builds a valid companion share URL the way CSP's QR does:
// "<ips>\t<port>\t<password>\t<generation>" obfuscated and hex-encoded.
func sampleShareURL() string {
	raw := []byte("192.168.1.50\t54312\tsecretpw\tG#1:2026")
	enc := make([]byte, len(raw))
	copy(enc, raw)
	crypto.ObfuscateRemoteParam(enc)
	return "https://companion.clip-studio.com/rc/en-us?s=" + hex.EncodeToString(enc)
}

func TestIsShareURL(t *testing.T) {
	if !clipremote.IsShareURL(sampleShareURL()) {
		t.Error("valid companion URL was rejected")
	}
	for _, bad := range []string{
		"",
		"not a url at all",
		"https://example.com/rc/en-us?s=abcd", // wrong host
		"https://companion.clip-studio.com/rc/en-us",      // no s param
		"https://companion.clip-studio.com/rc/en-us?s=zz", // s not hex
	} {
		if clipremote.IsShareURL(bad) {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}
