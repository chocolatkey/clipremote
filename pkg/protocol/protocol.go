package protocol

import "time"

var ReconnectionRequest = []byte("{{(([[reconnection request marker]]))}}\r\n")

const DetailSeparator = byte(0x0b)
const CommandTerminator = byte(0x00)

var CommandParamSeparator = []byte{0x1e, '$'}

const HeartbeatTimeout = time.Second * 3 // Seems to work
