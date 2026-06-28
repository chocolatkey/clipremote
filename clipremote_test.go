package clipremote

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/crypto"
	"github.com/chocolatkey/clipremote/pkg/packets"
)

func TestDecodeConfig(t *testing.T) {
	const (
		wantPort = uint16(6789)
		wantPass = "s3cr3t"
		wantGen  = "G#1:2024.01"
	)
	plain := "192.168.0.5,10.0.0.2\t6789\t" + wantPass + "\t" + wantGen
	raw := []byte(plain)
	crypto.ObfuscateRemoteParam(raw) // XOR is its own inverse
	url := "https://companion.clip-studio.com/rc/en-us?s=" + hex.EncodeToString(raw)

	ips, port, pass, gen, err := DecodeConfig(url)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if len(ips) != 2 || ips[0] != "192.168.0.5" || ips[1] != "10.0.0.2" {
		t.Errorf("ips = %v", ips)
	}
	if port != wantPort {
		t.Errorf("port = %d, want %d", port, wantPort)
	}
	if pass != wantPass {
		t.Errorf("password = %q", pass)
	}
	if gen != wantGen {
		t.Errorf("generation = %q", gen)
	}
}

func TestDecodeConfigRejectsBadHost(t *testing.T) {
	if _, _, _, _, err := DecodeConfig("https://evil.example.com/rc/en-us?s=00"); err == nil {
		t.Error("expected error for wrong host")
	}
}

func TestParseAuthResult(t *testing.T) {
	cases := []struct {
		name   string
		typ    packets.PacketType
		detail string
		wantOK bool
		reason commands.AuthErrorReason
	}{
		{"success bool array", packets.TypeServerResponseSuccess, "[true,false]", true, commands.AuthErrorNone},
		{"success empty", packets.TypeServerResponseSuccess, "", true, commands.AuthErrorNone},
		{"password mismatch", packets.TypeServerResponseSuccess,
			`{"AuthErrorReason":"PasswordMismatch","RemoteCommandSpecVersionOfServer":"1.0","IsQuickAccessAvailable":true}`,
			false, commands.AuthErrorPasswordMismatch},
		{"unknown reason is success", packets.TypeServerResponseSuccess,
			`{"AuthErrorReason":"Unknown"}`, true, commands.AuthErrorUnknown},
		{"error packet type", packets.TypeServerResponseError, "", false, commands.AuthErrorNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scp := &packets.ServerCommand{Type: tc.typ, RawDetail: []byte(tc.detail)}
			res := parseAuthResult(scp)
			if res.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v", res.OK, tc.wantOK)
			}
			if res.Reason != tc.reason {
				t.Errorf("Reason = %q, want %q", res.Reason, tc.reason)
			}
		})
	}
}

func TestGestureDetailShape(t *testing.T) {
	detail := commands.GestureDetail(commands.GestureBeginSwipe, 10, 20, 30, 40, 1.5, 0.25, true, 2, 999)
	b, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	want := `["BS",10,20,30,40,1.5,0.25,true,2,999]`
	if string(b) != want {
		t.Errorf("gesture detail = %s, want %s", b, want)
	}
}

func TestAuthHexMatchesBinary(t *testing.T) {
	// authHex = repeating-key XOR (key B6 D5 92 C4 A7 83 E1) + lowercase hex,
	// matching URRemoteControlAuthUtility::EncryptStringWithKey.
	got := authHex([]byte("AB"))
	// 'A'(0x41)^0xB6 = 0xF7, 'B'(0x42)^0xD5 = 0x97
	if got != "f797" {
		t.Errorf("authHex(\"AB\") = %q, want %q", got, "f797")
	}
}
