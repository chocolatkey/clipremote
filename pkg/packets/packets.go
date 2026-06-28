package packets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/protocol"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ClientCommandCallback func(*ServerCommand, error)

// MarshalDetail encodes a command detail body to its on-wire JSON bytes. A nil
// detail produces an empty body (used by commands with no request body). Bare
// scalars (e.g. a float64 for SetBrushSize) and arrays (Authenticate, DoGesture)
// encode to their natural JSON form.
func MarshalDetail(detail any) ([]byte, error) {
	if detail == nil {
		return nil, nil
	}
	if b, ok := detail.([]byte); ok { // already-encoded detail
		return b, nil
	}
	return json.Marshal(detail)
}

// WriteFrame writes one wire frame:
//
//	<type>$tcp_remote_command_protocol_version=1.0\x1e$command=<name>\x1e$serial=<n>\x1e$detail=<detail>\x1e\x00
//
// detail is the raw (already-encoded) detail body and may be empty. It is used
// for outgoing commands (type 0x01) and for ACK responses to peer-initiated
// commands (success 0x15 / error 0x06).
func WriteFrame(w io.Writer, typ PacketType, command commands.Command, serial Serial, detail []byte) error {
	var buf bytes.Buffer
	buf.Grow(96 + len(detail))
	buf.WriteByte(byte(typ))
	buf.WriteString("$tcp_remote_command_protocol_version=1.0")
	buf.Write(protocol.CommandParamSeparator) // 0x1e '$'
	buf.WriteString("command=")
	buf.WriteString(string(command))
	buf.Write(protocol.CommandParamSeparator)
	buf.WriteString("serial=")
	buf.WriteString(strconv.FormatUint(uint64(serial), 10))
	buf.Write(protocol.CommandParamSeparator)
	buf.WriteString("detail=")
	buf.Write(detail)
	buf.WriteByte(protocol.CommandParamSeparator[0]) // trailing 0x1e
	buf.WriteByte(protocol.CommandTerminator)        // 0x00
	_, err := w.Write(buf.Bytes())
	return err
}

type ClientCommand struct {
	Command  commands.Command
	Serial   Serial
	Detail   any // JSON-serializable, or a pre-encoded []byte, or nil for no body
	Callback ClientCommandCallback
}

// Example: tcp_remote_command_protocol_version=1.0$command=Authenticate$serial=0$detail=["G#1:2022.12","cdae...","899b..."]
func (p ClientCommand) String() string {
	detail, _ := MarshalDetail(p.Detail)
	return fmt.Sprintf(
		"$tcp_remote_command_protocol_version=1.0"+string(protocol.CommandParamSeparator)+
			"command=%s"+string(protocol.CommandParamSeparator)+
			"serial=%d"+string(protocol.CommandParamSeparator)+
			"detail=%s"+string(protocol.CommandParamSeparator[0]),
		p.Command, p.Serial, detail,
	)
}

func (p ClientCommand) Write(w io.Writer) error {
	detail, err := MarshalDetail(p.Detail)
	if err != nil {
		return errors.Wrap(err, "failed marshaling command detail")
	}
	logrus.Debugln("sending", p.String())
	if err := WriteFrame(w, TypeClientCommand, p.Command, p.Serial, detail); err != nil {
		return errors.Wrap(err, "failed writing command frame")
	}
	return nil
}

type ServerCommand struct {
	Type      PacketType
	Command   commands.Command
	Serial    Serial
	Detail    any    // parsed JSON detail (object/array/scalar)
	RawDetail []byte // raw JSON detail bytes (before any binary tail)
	Data      []byte // raw bytes after the 0x0B detail/binary separator (e.g. base64 preview pixels)
}

func (p ServerCommand) MarshalJSON() ([]byte, error) {
	result := make(map[string]any)
	result["command"] = string(p.Command)
	result["serial"] = p.Serial
	result["type"] = p.Type.String()

	if p.Detail != nil {
		result["detail"] = p.Detail
	}
	if p.Data != nil {
		result["data"] = string(p.Data)
	}
	return json.Marshal(result)
}

// IsError reports whether this is an error response.
func (p ServerCommand) IsError() bool { return p.Type == TypeServerResponseError }

// Decode unmarshals the JSON detail body into v.
func (p ServerCommand) Decode(v any) error {
	if len(p.RawDetail) == 0 {
		return nil
	}
	return json.Unmarshal(p.RawDetail, v)
}

func (p *ServerCommand) Parse(data []byte) error {
	logrus.Debugln("receiving", string(data))
	if len(data) < 4 {
		return errors.New("server returned packet that is too short")
	}

	if data[0] != byte(TypeServerResponseSuccess) &&
		data[0] != byte(TypeServerResponseError) &&
		data[0] != byte(TypeClientCommand) {
		return fmt.Errorf("server returned unknown packet type %#x", data[0])
	}
	p.Type = PacketType(data[0])

	if data[1] != '$' {
		return errors.New("server returned malformed packet (missing leading $)")
	}
	if data[len(data)-1] != protocol.CommandTerminator {
		return errors.New("server returned malformed packet (missing terminator)")
	}

	// Strip leading "<type>$" and trailing "\x1e\x00".
	frags := bytes.SplitN(data[2:len(data)-2], protocol.CommandParamSeparator, 4)
	if len(frags) != 4 {
		return errors.New("server returned packet with invalid number of fragments")
	}

	if !bytes.Equal(frags[0], []byte("tcp_remote_command_protocol_version=1.0")) {
		return errors.New("server returned packet with invalid protocol version " + string(frags[0]))
	}
	if !bytes.HasPrefix(frags[1], []byte("command=")) {
		return errors.New("server returned packet with invalid command part " + string(frags[1]))
	}
	p.Command = commands.Command(frags[1][8:])

	if !bytes.HasPrefix(frags[2], []byte("serial=")) {
		return errors.New("server returned packet with invalid serial part " + string(frags[2]))
	}
	serial, err := strconv.ParseUint(string(frags[2][7:]), 10, 32)
	if err != nil {
		return errors.Wrap(err, "server returned packet with invalid serial number "+string(frags[2]))
	}
	p.Serial = Serial(serial)

	if !bytes.HasPrefix(frags[3], []byte("detail=")) {
		return errors.New("server returned packet with invalid detail part " + string(frags[3]))
	}
	// Detail may be "<json>" or "<json>\x0b<binary tail>".
	detailFrags := bytes.SplitN(frags[3][7:], []byte{protocol.DetailSeparator}, 2)
	p.RawDetail = detailFrags[0]
	if len(detailFrags[0]) > 0 {
		if err := json.Unmarshal(detailFrags[0], &p.Detail); err != nil {
			return errors.Wrap(err, "server returned packet with invalid detail JSON "+string(detailFrags[0]))
		}
	}
	if len(detailFrags) > 1 {
		p.Data = detailFrags[1]
	}
	return nil
}
