package clipremote

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/packets"
)

// fakeHost is a minimal in-process CSP companion-mode host used to exercise the
// full client stack (framing, auth, response matching, and the auto-ACK of a
// host-initiated command).
func TestIntegrationRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)

	const authReply = `{"AuthErrorReason":"","RemoteCommandSpecVersionOfServer":"1.2.3","IsQuickAccessAvailable":true}`
	acks := make(chan *packets.ServerCommand, 4)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		pushed := false
		for {
			data, err := r.ReadBytes(0x00)
			if err != nil {
				return
			}
			var sc packets.ServerCommand
			if err := sc.Parse(data); err != nil {
				t.Errorf("host: parse client frame: %v (%q)", err, data)
				return
			}
			if sc.Type == packets.TypeClientCommand {
				var reply []byte
				if sc.Command == commands.Authenticate {
					reply = []byte(authReply)
				}
				// Success is 0x15 (CSP's inverted convention).
				if err := packets.WriteFrame(conn, packets.TypeServerResponseSuccess, sc.Command, sc.Serial, reply); err != nil {
					return
				}
				// Once the client issues SetBrushSize, push a host-initiated
				// command and confirm the client ACKs it.
				if sc.Command == commands.SetBrushSize && !pushed {
					pushed = true
					_ = packets.WriteFrame(conn, packets.TypeClientCommand, commands.SyncColorMixUIState, 1000,
						[]byte(`{"CurrentColorRed":10,"CurrentColorGreen":20,"CurrentColorBlue":30}`))
				}
			} else {
				// A response from the client = the ACK to our push.
				acks <- &sc
			}
		}
	}()

	client, err := Connect([]string{"127.0.0.1"}, port, "G#1:test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	pushCh := make(chan *commands.ColorRGBUIState, 1)
	client.OnSyncColorMixUIState(func(s *commands.ColorRGBUIState) { pushCh <- s })

	res, err := client.Authenticate("password")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !res.OK {
		t.Fatalf("auth not OK: %+v", res)
	}
	if res.ServerSpecVersion != "1.2.3" || !res.QuickAccessAvailable {
		t.Errorf("auth result = %+v", res)
	}
	if !client.Alive() {
		t.Error("client should be alive after auth")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.SetBrushSize(ctx, 12.5); err != nil {
		t.Fatalf("SetBrushSize: %v", err)
	}

	// The host pushed SyncColorMixUIState; the client must decode it and ACK.
	select {
	case s := <-pushCh:
		if s.CurrentColorRed != 10 || s.CurrentColorGreen != 20 || s.CurrentColorBlue != 30 {
			t.Errorf("pushed state = %+v", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive pushed SyncColorMixUIState")
	}

	select {
	case ack := <-acks:
		if ack.Type != packets.TypeServerResponseSuccess {
			t.Errorf("ACK type = %s, want success", ack.Type)
		}
		if ack.Command != commands.SyncColorMixUIState {
			t.Errorf("ACK command = %q", ack.Command)
		}
		if ack.Serial != 1000 {
			t.Errorf("ACK serial = %d, want 1000", ack.Serial)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not ACK the host-initiated command")
	}
}
