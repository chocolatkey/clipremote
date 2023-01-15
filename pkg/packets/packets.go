package packets

import (
	"bufio"
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

type ClientCommand struct {
	Command  commands.Command
	Serial   Serial
	Detail   interface{} // Has to be JSON serializable
	Callback ClientCommandCallback
}

// Example: tcp_remote_command_protocol_version=1.0$command=Authenticate$serial=0$detail=["G#1:2022.12","cdaebaecfcd893d3b6fdaac9e682c2bcfdaa87f184c7a0f7b7d3a38cd7a7f9a1d5debc9ffcefb9aa89","899bb0fbffedbcf9"]
func (p ClientCommand) String() string {
	var detailStr string
	if p.Detail != nil {
		jbin, err := json.Marshal(p.Detail)
		if len(jbin) == 0 {
			panic(err)
		}
		detailStr = string(jbin)
	}

	return fmt.Sprintf(
		"$tcp_remote_command_protocol_version=1.0"+string(protocol.CommandParamSeparator)+
			"command=%s"+string(protocol.CommandParamSeparator)+
			"serial=%d"+string(protocol.CommandParamSeparator)+
			"detail=%s"+string(protocol.CommandParamSeparator[0]),
		p.Command, p.Serial, detailStr,
	)
}

func (p ClientCommand) Write(w io.Writer) error {
	b := bufio.NewWriterSize(w, 8192)
	err := b.WriteByte(byte(TypeClientCommand))
	if err != nil {
		return errors.Wrap(err, "failed writing command packet type")
	}
	logrus.Debugln("sending", p.String())
	_, err = b.WriteString(p.String())
	if err != nil {
		return errors.Wrap(err, "failed writing command packet body")
	}
	err = b.WriteByte(protocol.CommandTerminator) // Packets are terminated with NUL
	if err != nil {
		return errors.Wrap(err, "failed writing command packet terminator")
	}
	b.Flush()
	return nil
}

type ServerCommand struct {
	Type    PacketType
	Command commands.Command
	Serial  Serial
	Detail  interface{} // Has to be JSON serializable
	Data    []byte      // Raw data
}

func (p ServerCommand) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})
	result["command"] = string(p.Command)
	result["serial"] = p.Serial

	switch p.Type {
	case TypeServerResponseSuccess:
		result["type"] = "success"
	case TypeServerResponseError:
		result["type"] = "error"
	case TypeClientCommand:
		result["type"] = "command"
	}

	if p.Detail != nil {
		result["detail"] = p.Detail
	}

	if p.Data != nil {
		result["data"] = string(p.Data)
	}

	return json.Marshal(result)
}

func (p *ServerCommand) Parse(data []byte) error {
	if len(data) < 72 {
		return errors.New("server returned packet that is too short")
	}
	logrus.Debugln("receiving", string(data))

	if data[0] != byte(TypeServerResponseSuccess) &&
		data[0] != byte(TypeServerResponseError) &&
		data[0] != byte(TypeClientCommand) {
		return errors.New(
			fmt.Sprintf("server returned unknown packet type %x", data[0]),
		)
	}

	p.Type = PacketType(data[0])

	if data[1] != '$' {
		return errors.New("server returned malformed packet")
	}
	if data[len(data)-1] != protocol.CommandTerminator {
		return errors.New("server returned malformed packet")
	}

	frags := bytes.SplitN(data[2:len(data)-2], protocol.CommandParamSeparator, 4)
	if len(frags) != 4 {
		return errors.New("server returned packet with invalid amount of fragments")
	}

	// tcp_remote_command_protocol_version=1.0
	if !bytes.Equal(frags[0], []byte("tcp_remote_command_protocol_version=1.0")) {
		return errors.New("server returned packet with invalid protocol version " + string(frags[0]))
	}

	// command=XXX
	if !bytes.HasPrefix(frags[1], []byte("command=")) {
		return errors.New("server returned packet with invalid command part " + string(frags[1]))
	}
	p.Command = commands.Command(frags[1][8:])

	// serial=XXX
	if !bytes.HasPrefix(frags[2], []byte("serial=")) {
		return errors.New("server returned packet with invalid serial part " + string(frags[2]))
	}
	serial, err := strconv.ParseUint(string(frags[2][7:]), 10, 32)
	if err != nil {
		return errors.Wrap(err, "server returned packet with invalid serial number "+string(frags[2]))
	}
	p.Serial = Serial(serial)

	// detail=XXX
	if !bytes.HasPrefix(frags[3], []byte("detail=")) {
		return errors.New("server returned packet with invalid detail part " + string(frags[3]))
	}
	detailFrags := bytes.SplitN(frags[3][7:], []byte{protocol.DetailSeparator}, 2)
	if len(detailFrags[0]) > 2 {
		err = json.Unmarshal(detailFrags[0], &p.Detail)
		if err != nil {
			return errors.Wrap(err, "server returned packet with invalid detail JSON "+string(detailFrags[0]))
		}
		if len(detailFrags) > 1 {
			p.Data = detailFrags[1]
		}
	}
	return nil
}
