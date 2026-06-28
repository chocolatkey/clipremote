package packets

// PacketType is the leading byte of every wire frame. It maps to CSP's internal
// EPWTcpRemoteCommandPacketType enum (1=client command, 2=success, 3=error)
// through the packed constant 0x150601 >> 8*(enum-1):
//
//	enum 1 (ClientCommand) -> 0x01
//	enum 2 (success/ACK)   -> 0x06
//	enum 3 (error/NAK)     -> 0x15
//
// CSP's command handlers return NONZERO on success and 0 on failure, and the
// dispatcher builds the reply type as (handler()==0)|2 — so a successful handler
// yields enum 2 (0x06) and a failed one yields enum 3 (0x15). This coincides with
// the conventional ASCII ACK=0x06 / NAK=0x15. Verified against
// libClipStudioPaint.so (the 0x150601 packing in PWTcpRemoteCommander::SendPacket,
// the type switch in DoCommonCommunicationLoop, and GetModifyKeyString::HandleCommand
// returning 1 on success) AND empirically against a live CSP host (Authenticate
// success replied 0x06/"Unknown", a stale-password failure replied 0x15/"PasswordMismatch").
//
// NOTE: CSP's *response handlers* key success on the reply DETAIL (e.g.
// AuthErrorReason), not on this type byte, so consumers should generally inspect
// the detail rather than treating an error-typed packet as a hard failure.
type PacketType byte

const (
	// TypeClientCommand is an outgoing command (or any peer-initiated command).
	TypeClientCommand PacketType = 0x01
	// TypeServerResponseSuccess is a success response to a command (CSP enum 2).
	TypeServerResponseSuccess PacketType = 0x06
	// TypeServerResponseError is a failure response to a command (CSP enum 3).
	TypeServerResponseError PacketType = 0x15
)

// IsResponse reports whether the packet type is a response (success or error)
// to a previously-sent command, as opposed to a peer-initiated command.
func (t PacketType) IsResponse() bool {
	return t == TypeServerResponseSuccess || t == TypeServerResponseError
}

func (t PacketType) String() string {
	switch t {
	case TypeClientCommand:
		return "command"
	case TypeServerResponseSuccess:
		return "success"
	case TypeServerResponseError:
		return "error"
	default:
		return "unknown"
	}
}
