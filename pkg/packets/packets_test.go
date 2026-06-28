package packets

import (
	"bytes"
	"testing"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/protocol"
)

func TestPacketTypeBytes(t *testing.T) {
	// CSP handlers return nonzero on success, so via (handler()==0)|2 the success
	// reply is enum 2 (0x06) and the error reply is enum 3 (0x15) — the
	// conventional ACK/NAK. Verified empirically against a live host.
	if TypeClientCommand != 0x01 {
		t.Errorf("TypeClientCommand = %#x, want 0x01", byte(TypeClientCommand))
	}
	if TypeServerResponseSuccess != 0x06 {
		t.Errorf("TypeServerResponseSuccess = %#x, want 0x06", byte(TypeServerResponseSuccess))
	}
	if TypeServerResponseError != 0x15 {
		t.Errorf("TypeServerResponseError = %#x, want 0x15", byte(TypeServerResponseError))
	}
}

func TestFrameRoundTrip(t *testing.T) {
	detail := []byte(`{"ShiftPushed":false,"CtrlPushed":true,"AltPushed":false}`)

	var buf bytes.Buffer
	if err := WriteFrame(&buf, TypeServerResponseSuccess, commands.GetModifyKeyString, 7, detail); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	// Frame must begin with the type byte and a '$', and end with the terminator.
	raw := buf.Bytes()
	if raw[0] != byte(TypeServerResponseSuccess) {
		t.Fatalf("leading byte = %#x", raw[0])
	}
	if raw[len(raw)-1] != protocol.CommandTerminator {
		t.Fatalf("missing terminator")
	}

	var sc ServerCommand
	if err := sc.Parse(raw); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sc.Type != TypeServerResponseSuccess {
		t.Errorf("Type = %s", sc.Type)
	}
	if sc.IsError() {
		t.Errorf("IsError() = true for a success packet")
	}
	if sc.Command != commands.GetModifyKeyString {
		t.Errorf("Command = %q", sc.Command)
	}
	if sc.Serial != 7 {
		t.Errorf("Serial = %d", sc.Serial)
	}
	if string(sc.RawDetail) != string(detail) {
		t.Errorf("RawDetail = %q, want %q", sc.RawDetail, detail)
	}

	var got commands.ModifyKeyRequest
	if err := sc.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.CtrlPushed != true || got.ShiftPushed || got.AltPushed {
		t.Errorf("decoded detail = %+v", got)
	}
}

func TestFrameBinaryTail(t *testing.T) {
	// detail = JSON + 0x0B + base64 pixel tail (as in PreviewWebtoonFromClient).
	json := `{"Operation":"ReadPreviewBlock"}`
	detail := append([]byte(json), protocol.DetailSeparator)
	detail = append(detail, []byte("QUJDREVG")...) // base64("ABCDEF")

	var buf bytes.Buffer
	if err := WriteFrame(&buf, TypeServerResponseSuccess, commands.PreviewWebtoonFromClient, 1, detail); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	var sc ServerCommand
	if err := sc.Parse(buf.Bytes()); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(sc.RawDetail) != json {
		t.Errorf("RawDetail = %q, want %q", sc.RawDetail, json)
	}
	if string(sc.Data) != "QUJDREVG" {
		t.Errorf("Data = %q", sc.Data)
	}
}

func TestFrameEmptyDetail(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, TypeServerResponseSuccess, commands.GetQuickAccessData, 0, nil); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	var sc ServerCommand
	if err := sc.Parse(buf.Bytes()); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sc.RawDetail) != 0 {
		t.Errorf("RawDetail = %q, want empty", sc.RawDetail)
	}
	if sc.Detail != nil {
		t.Errorf("Detail = %v, want nil", sc.Detail)
	}
}

func TestMarshalDetailBareScalar(t *testing.T) {
	// SetBrushSize / SetAlpha encode bare scalars, not objects.
	b, err := MarshalDetail(42.5)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "42.5" {
		t.Errorf("MarshalDetail(42.5) = %q", b)
	}
	b, err = MarshalDetail(50)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "50" {
		t.Errorf("MarshalDetail(50) = %q", b)
	}
	if b, _ := MarshalDetail(nil); b != nil {
		t.Errorf("MarshalDetail(nil) = %q, want nil", b)
	}
}
