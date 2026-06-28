package protocol

import "time"

var ReconnectionRequest = []byte("{{(([[reconnection request marker]]))}}\r\n")

const DetailSeparator = byte(0x0b)
const CommandTerminator = byte(0x00)

var CommandParamSeparator = []byte{0x1e, '$'}

// HeartbeatInterval is how often the client sends a TellHeartbeat. CSP's
// URRemoteCommandToTellHeartbeat::StartClientSideTimer arms a 1000 ms periodic
// timer, so the app heartbeats once per second.
const HeartbeatInterval = time.Second
