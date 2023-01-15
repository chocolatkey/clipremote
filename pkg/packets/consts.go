package packets

type PacketType byte

const (
	TypeClientCommand         PacketType = 0x01
	TypeServerResponseError   PacketType = 0x15
	TypeServerResponseSuccess PacketType = 0x06
)
