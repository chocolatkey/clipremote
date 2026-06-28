package commands

// All of CSP companion-mode's command enums are serialized on the wire as their
// STRING form (the int backing value is in-process only), so each is modeled as
// a Go string type whose constants are the exact wire strings. They therefore
// marshal/unmarshal as plain JSON strings with no custom (Un)MarshalJSON.
//
// Verified against libClipStudioPaint.so (the Android app, symboled).

// TabKind is CSP's ERemoteControllerTabKind. Wire key: "ServerSelectedTabKind".
type TabKind string

const (
	TabInvalid        TabKind = "Invalid" // 0 — no-match / suppresses replies
	TabQuickAccess    TabKind = "QuickAccess"
	TabTouchGesture   TabKind = "TouchGesture"
	TabColorCircle    TabKind = "ColorCircle"
	TabSubView        TabKind = "SubView"
	TabColorMixing    TabKind = "ColorMixing"
	TabWebtoonPreview TabKind = "WebtoonPreview"
	TabModeChange     TabKind = "ModeChange"
	TabSetting        TabKind = "Setting"
	TabPromotion      TabKind = "Promotion" // 9 — added in CSP 5.0.4
)

// ColorSpaceKind is CSP's EURRemoteControllerColorSpaceKind. Wire keys:
// "ColorSpaceKind" (SetColorSelectionModel) / "ColorSelectionModel" (Sync*).
type ColorSpaceKind string

const (
	ColorSpaceUnknown ColorSpaceKind = "Unknown" // 0 — no match
	ColorSpaceHSV     ColorSpaceKind = "HSV"
	ColorSpaceHLS     ColorSpaceKind = "HLS"
)

// LengthUnitKind is CSP's brush-size unit. Wire key: "LengthUnitKind".
type LengthUnitKind string

const (
	LengthUnitPx      LengthUnitKind = "px"
	LengthUnitCm      LengthUnitKind = "cm"
	LengthUnitMm      LengthUnitKind = "mm"
	LengthUnitIn      LengthUnitKind = "in"
	LengthUnitPc      LengthUnitKind = "pc"
	LengthUnitPt      LengthUnitKind = "pt"
	LengthUnitQ       LengthUnitKind = "q"
	LengthUnitPtAnglo LengthUnitKind = "pt_anglo"
	LengthUnitPtDidot LengthUnitKind = "pt_didot"
	LengthUnitUnknown LengthUnitKind = "Unknown" // -1 — encode fallback / no match
)

// ToolBarItemType is CSP's RemoteControllerToolBarItemType.
// Wire key: "ItemType" (DoQuickAccess) or "ItemIDType" (toolbar item IDs).
type ToolBarItemType string

const (
	ToolBarItemInvalid   ToolBarItemType = "Invalid"
	ToolBarItemCommand   ToolBarItemType = "Command"
	ToolBarItemDrawColor ToolBarItemType = "DrawColor"
	ToolBarItemTool      ToolBarItemType = "Tool"
)

// AuthErrorReason is the failure reason reported by an Authenticate response.
// Empty (or "Unknown") means success. Wire key: "AuthErrorReason".
type AuthErrorReason string

const (
	AuthErrorNone             AuthErrorReason = ""
	AuthErrorVersionMismatch  AuthErrorReason = "VersionMismatch"
	AuthErrorPasswordMismatch AuthErrorReason = "PasswordMismatch"
	AuthErrorServerUnready    AuthErrorReason = "ServerUnready"
	AuthErrorUnknown          AuthErrorReason = "Unknown" // server switch-default; treated as success/none
)

// IsFailure reports whether the reason indicates a real authentication failure.
// "" and "Unknown" both map to success in the binary.
func (r AuthErrorReason) IsFailure() bool {
	return r != AuthErrorNone && r != AuthErrorUnknown
}

// SystemKind is the host OS reported by GetModifyKeyString. Wire key: "SystemKind".
type SystemKind string

const (
	SystemUnknown SystemKind = "Unknown"
	SystemWindows SystemKind = "Windows"
	SystemMacOSX  SystemKind = "MacOSX"
	SystemIOS     SystemKind = "iOS"
	SystemAndroid SystemKind = "Android"
)

// NavigatorCommandType is CSP's ENavigatorCommandType (DoNavigator).
// Wire key: "CommandType" (DoNavigator) or "Type" (NavigatorData).
type NavigatorCommandType string

const (
	NavigatorNone          NavigatorCommandType = "None"
	NavigatorZoomIn        NavigatorCommandType = "ZoomIn"
	NavigatorZoomOut       NavigatorCommandType = "ZoomOut"
	NavigatorPixelSize     NavigatorCommandType = "PixelSize"
	NavigatorFitting       NavigatorCommandType = "Fitting"
	NavigatorWholeSize     NavigatorCommandType = "WholeSize"
	NavigatorRotateLeft    NavigatorCommandType = "RotateLeft"
	NavigatorRotateRight   NavigatorCommandType = "RotateRight"
	NavigatorRotate0       NavigatorCommandType = "Rotate0"
	NavigatorReverseHorz   NavigatorCommandType = "ReverseHorz"
	NavigatorReverseVert   NavigatorCommandType = "ReverseVert"
	NavigatorResetPosition NavigatorCommandType = "ResetPosition"
)

// KeyboardEventKind is CSP's EKeyboardEventKind (DoModeChange).
// Wire key: "ProxyKeyboardEventKind".
type KeyboardEventKind string

const (
	KeyboardEventUnknown KeyboardEventKind = "Unknown"
	KeyboardEventKeyUp   KeyboardEventKind = "KeyUp"
	KeyboardEventKeyDown KeyboardEventKind = "KeyDown"
)

// GestureCommandType is CSP's EGestureCommandType — the terse code at index 0
// of a DoGesture positional array.
type GestureCommandType string

const (
	GestureInvalid           GestureCommandType = ""    // encode out-of-range -> "U"
	GestureHoverTouch        GestureCommandType = "HT"  // 1
	GestureBegin             GestureCommandType = "B"   // 2
	GestureEnd               GestureCommandType = "E"   // 3
	GestureBeginSwipe        GestureCommandType = "BS"  // 4
	GestureSwipe             GestureCommandType = "S"   // 5
	GestureEndSwipe          GestureCommandType = "ES"  // 6
	GestureBeginMove         GestureCommandType = "BM"  // 7
	GestureMove              GestureCommandType = "M"   // 8
	GestureEndMove           GestureCommandType = "EM"  // 9
	GestureBeginRotate       GestureCommandType = "BR"  // 10 — doubles NOT scale-divided
	GestureRotate            GestureCommandType = "R"   // 11 — doubles NOT scale-divided
	GestureEndRotate         GestureCommandType = "ER"  // 12 — doubles NOT scale-divided
	GestureBeginTwoFingerTap GestureCommandType = "B2T" // 13
	GestureTwoFingerTap      GestureCommandType = "2T"  // 14
	GestureEndTwoFingerTap   GestureCommandType = "E2T" // 15
	GestureBeginPressAndTap  GestureCommandType = "BPT" // 16
	GesturePressAndTap       GestureCommandType = "PT"  // 17
	GestureEndPressAndTap    GestureCommandType = "EPT" // 18
	GestureBeginLongPress    GestureCommandType = "BLP" // 19
	GestureLongPress         GestureCommandType = "LP"  // 20
	GestureEndLongPress      GestureCommandType = "ELP" // 21
)

// WebtoonOperationFromClient is the "Operation" discriminator for the
// PreviewWebtoonFromClient command.
type WebtoonOperationFromClient string

const (
	WebtoonOpUpdateGallery    WebtoonOperationFromClient = "UpdateGallery"
	WebtoonOpUpdateCanvas     WebtoonOperationFromClient = "UpdateCanvas"
	WebtoonOpReadPreviewBlock WebtoonOperationFromClient = "ReadPreviewBlock"
	WebtoonOpSyncPreview      WebtoonOperationFromClient = "SyncPreview"
)

// WebtoonOperationFromServer is the "Operation" discriminator for the
// PreviewWebtoonFromServer command. Note the change op's wire string is
// "ChangeNotification" even though the C++ method is PreviewChangeNotification.
type WebtoonOperationFromServer string

const (
	WebtoonOpResetGallery       WebtoonOperationFromServer = "ResetGallery"
	WebtoonOpResetCanvas        WebtoonOperationFromServer = "ResetCanvas"
	WebtoonOpChangeNotification WebtoonOperationFromServer = "ChangeNotification"
)

// SyncTargetType is the per-element discriminator inside a
// SyncQuickAccessUIState reply. Wire key: "SyncTargetType".
type SyncTargetType string

const (
	SyncTargetInvalid           SyncTargetType = "Invalid"
	SyncTargetSet               SyncTargetType = "Set"
	SyncTargetSetName           SyncTargetType = "SetName"
	SyncTargetCommandIcon       SyncTargetType = "CommandIcon"
	SyncTargetToolIconWithName  SyncTargetType = "ToolIconWithName"
	SyncTargetActionCommandName SyncTargetType = "ActionCommandName"
	SyncTargetDrawColorName     SyncTargetType = "DrawColorName"
)
