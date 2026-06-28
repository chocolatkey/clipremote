// Package commands enumerates every Clip Studio Paint "companion mode" remote
// command and the typed request/response detail bodies that travel with them.
//
// The full command set (24) and every JSON detail schema were reverse-engineered
// from the symboled Android binary libClipStudioPaint.so. Each wire command "X"
// is implemented in CSP by the class Planeswalker::Urza::URRemoteCommandToX.
package commands

// Command is the wire name carried in the "$command=" field of a packet.
type Command string

const (
	// --- Connection lifecycle ---

	// Authenticate logs in. Detail is a 3-element array
	// [generation(plain), enc(currentPassword), enc(newPassword)].
	Authenticate Command = "Authenticate"
	// TellHeartbeat is the symmetric keep-alive.
	TellHeartbeat Command = "TellHeartbeat"
	// PreNotifyOfServerShutdown is pushed by the host before it closes.
	PreNotifyOfServerShutdown Command = "PreNotifyOfServerShutdown"

	// --- View / tab state ---

	GetServerSelectedTabKind Command = "GetServerSelectedTabKind"
	SetServerSelectedTabKind Command = "SetServerSelectedTabKind"
	GetModifyKeyString       Command = "GetModifyKeyString"
	DoModeChange             Command = "DoModeChange"

	// --- Color ---

	SetCurrentColor        Command = "SetCurrentColor"
	SetColorSelectionModel Command = "SetColorSelectionModel"

	// --- Tool parameters ---

	SetBrushSize Command = "SetBrushSize"
	SetAlpha     Command = "SetAlpha"

	// --- Canvas navigation / gestures ---

	DoGesture   Command = "DoGesture"
	DoNavigator Command = "DoNavigator"

	// --- Quick access toolbar ---

	GetQuickAccessData     Command = "GetQuickAccessData"
	GetQuickAccessItemIcon Command = "GetQuickAccessItemIcon"
	DoQuickAccess          Command = "DoQuickAccess"

	// --- Webtoon preview ---

	PreviewWebtoonFromClient Command = "PreviewWebtoonFromClient"
	PreviewWebtoonFromServer Command = "PreviewWebtoonFromServer"

	// --- UI-state sync (host -> controller pushes) ---

	SyncQuickAccessUIState Command = "SyncQuickAccessUIState"
	SyncColorCircleUIState Command = "SyncColorCircleUIState"
	SyncColorMixUIState    Command = "SyncColorMixUIState"
	SyncGesturePadUIState  Command = "SyncGesturePadUIState"
	SyncSubViewUIState     Command = "SyncSubViewUIState"
	SyncSettingUIState     Command = "SyncSettingUIState"
)

// All lists every command in the protocol.
var All = []Command{
	Authenticate, TellHeartbeat, PreNotifyOfServerShutdown,
	GetServerSelectedTabKind, SetServerSelectedTabKind, GetModifyKeyString, DoModeChange,
	SetCurrentColor, SetColorSelectionModel,
	SetBrushSize, SetAlpha,
	DoGesture, DoNavigator,
	GetQuickAccessData, GetQuickAccessItemIcon, DoQuickAccess,
	PreviewWebtoonFromClient, PreviewWebtoonFromServer,
	SyncQuickAccessUIState, SyncColorCircleUIState, SyncColorMixUIState,
	SyncGesturePadUIState, SyncSubViewUIState, SyncSettingUIState,
}

// serverInitiated are the commands the host normally pushes to the controller.
// A controller mainly receives these (and must reply with an ACK). It can also
// receive a response to any command it sent, regardless of this set.
var serverInitiated = map[Command]bool{
	PreviewWebtoonFromServer:  true,
	PreNotifyOfServerShutdown: true,
	SyncQuickAccessUIState:    true,
	SyncColorCircleUIState:    true,
	SyncColorMixUIState:       true,
	SyncGesturePadUIState:     true,
	SyncSubViewUIState:        true,
	SyncSettingUIState:        true,
}

// IsServerInitiated reports whether c is normally pushed by the host to the
// controller (as opposed to being initiated by the controller).
func IsServerInitiated(c Command) bool { return serverInitiated[c] }
