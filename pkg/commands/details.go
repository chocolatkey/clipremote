package commands

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
)

// This file defines the typed JSON "detail" bodies for every command. Field
// names use the exact, case-sensitive wire keys reverse-engineered from
// libClipStudioPaint.so. Objects are read by key on the host side, so field
// order is not significant for object details; positional arrays (Authenticate,
// DoGesture) and bare scalars (SetBrushSize, SetAlpha) are handled explicitly.

// ---------------------------------------------------------------------------
// Authenticate
// ---------------------------------------------------------------------------

// AuthResponse is the object body of an Authenticate reply (failure carries a
// non-empty AuthErrorReason; success may instead be a bare [true,false] array).
type AuthResponse struct {
	AuthErrorReason                  AuthErrorReason `json:"AuthErrorReason"`
	RemoteCommandSpecVersionOfServer string          `json:"RemoteCommandSpecVersionOfServer"`
	IsQuickAccessAvailable           bool            `json:"IsQuickAccessAvailable"`
}

// AuthResult is the resolved outcome of an Authenticate exchange.
type AuthResult struct {
	OK                   bool            // authenticated successfully
	Reason               AuthErrorReason // failure reason (empty when OK)
	ServerSpecVersion    string          // host's remote-command spec version
	QuickAccessAvailable bool            // host offers quick access
}

// ---------------------------------------------------------------------------
// TellHeartbeat
// ---------------------------------------------------------------------------

// HeartbeatRequest is the optional body of a TellHeartbeat. A bare keep-alive is
// sent with no body at all; this struct is only used to request an idle-timer
// reset.
type HeartbeatRequest struct {
	IdleTimerResetRequested bool `json:"IdleTimerResetRequested"`
}

// ---------------------------------------------------------------------------
// PreNotifyOfServerShutdown
// ---------------------------------------------------------------------------

// PreNotifyOfServerShutdownRequest is pushed by the host before it shuts down.
type PreNotifyOfServerShutdownRequest struct {
	IsManualShutdown bool `json:"IsManualShutdown"` // true = user-initiated, false = automatic
}

// ---------------------------------------------------------------------------
// GetServerSelectedTabKind / SetServerSelectedTabKind
// ---------------------------------------------------------------------------

// ServerSelectedTabKindResponse is the GetServerSelectedTabKind reply.
type ServerSelectedTabKindResponse struct {
	ServerSelectedTabKind TabKind `json:"ServerSelectedTabKind"`
}

// ---------------------------------------------------------------------------
// GetModifyKeyString
// ---------------------------------------------------------------------------

// ModifyKeyRequest asks the host for the modifier-key descriptions for the
// given pressed state.
type ModifyKeyRequest struct {
	ShiftPushed bool `json:"ShiftPushed"`
	CtrlPushed  bool `json:"CtrlPushed"`
	AltPushed   bool `json:"AltPushed"`
}

// ModifyKeyResponse is the GetModifyKeyString reply.
type ModifyKeyResponse struct {
	ShiftDescription string     `json:"ShiftDescription"`
	CtrlDescription  string     `json:"CtrlDescription"`
	AltDescription   string     `json:"AltDescription"`
	SystemKind       SystemKind `json:"SystemKind"`
}

// ---------------------------------------------------------------------------
// DoModeChange
// ---------------------------------------------------------------------------

// ModeChangeRequest drives the host's proxy keyboard (modifier press/release).
type ModeChangeRequest struct {
	FlagShift              bool              `json:"FlagShift"`
	FlagControl            bool              `json:"FlagControl"`
	FlagAlt                bool              `json:"FlagAlt"`
	ProxyKeyboardEventKind KeyboardEventKind `json:"ProxyKeyboardEventKind"`
}

// ---------------------------------------------------------------------------
// SetCurrentColor (3 discriminated variants)
// ---------------------------------------------------------------------------

// SetColorRGBRequest sets an RGB color. It MUST NOT carry ColorSpaceKind or
// ColorIndex — the host detects RGB by the absence of a string ColorSpaceKind.
type SetColorRGBRequest struct {
	IsColorTransparent bool `json:"IsColorTransparent"`
	RGBColorR          int  `json:"RGBColorR"` // 0-255
	RGBColorG          int  `json:"RGBColorG"` // 0-255
	RGBColorB          int  `json:"RGBColorB"` // 0-255
}

// SetColorHLSRequest sets an HLS color (channels are raw 32-bit, not 0-255).
type SetColorHLSRequest struct {
	ColorIndex         int            `json:"ColorIndex"`
	IsColorTransparent bool           `json:"IsColorTransparent"`
	ColorSpaceKind     ColorSpaceKind `json:"ColorSpaceKind"` // must be ColorSpaceHLS
	HLSColorH          int            `json:"HLSColorH"`
	HLSColorL          int            `json:"HLSColorL"`
	HLSColorS          int            `json:"HLSColorS"`
}

// SetColorHSVRequest sets an HSV color (channels are raw 32-bit, not 0-255).
type SetColorHSVRequest struct {
	ColorIndex         int            `json:"ColorIndex"`
	IsColorTransparent bool           `json:"IsColorTransparent"`
	ColorSpaceKind     ColorSpaceKind `json:"ColorSpaceKind"` // must be ColorSpaceHSV
	HSVColorH          int            `json:"HSVColorH"`
	HSVColorS          int            `json:"HSVColorS"`
	HSVColorV          int            `json:"HSVColorV"`
}

// ---------------------------------------------------------------------------
// SetColorSelectionModel
// ---------------------------------------------------------------------------

// ColorSelectionModelRequest sets the color-circle model (HSV/HLS).
type ColorSelectionModelRequest struct {
	ColorSpaceKind ColorSpaceKind `json:"ColorSpaceKind"`
}

// ---------------------------------------------------------------------------
// DoNavigator
// ---------------------------------------------------------------------------

// NavigatorRequest triggers a canvas navigation action (zoom/rotate/etc).
type NavigatorRequest struct {
	CommandType NavigatorCommandType `json:"CommandType"`
}

// ---------------------------------------------------------------------------
// DoGesture (10-element positional array — order is load-bearing)
// ---------------------------------------------------------------------------

// GestureDetail builds the positional DoGesture detail array:
// [type, p1x, p1y, p2x, p2y, factorA, factorB, flag, subKind, seq].
func GestureDetail(g GestureCommandType, p1x, p1y, p2x, p2y int, factorA, factorB float64, flag bool, subKind int, seq int64) []interface{} {
	return []interface{}{string(g), p1x, p1y, p2x, p2y, factorA, factorB, flag, subKind, seq}
}

// ---------------------------------------------------------------------------
// DoQuickAccess (3 discriminated variants)
// ---------------------------------------------------------------------------

// DoQuickAccessToolRequest activates a tool by UUID.
type DoQuickAccessToolRequest struct {
	ItemType     ToolBarItemType `json:"ItemType"` // ToolBarItemTool
	ItemToolUuid string          `json:"ItemToolUuid"`
}

// DoQuickAccessCommandRequest invokes a named command.
type DoQuickAccessCommandRequest struct {
	ItemType        ToolBarItemType `json:"ItemType"` // ToolBarItemCommand
	ItemCommandType string          `json:"ItemCommandType"`
	ItemCommandName string          `json:"ItemCommandName"`
}

// DoQuickAccessDrawColorRequest sets the draw color.
type DoQuickAccessDrawColorRequest struct {
	ItemType                   ToolBarItemType `json:"ItemType"` // ToolBarItemDrawColor
	ItemDrawColorR             int             `json:"ItemDrawColorR"`
	ItemDrawColorG             int             `json:"ItemDrawColorG"`
	ItemDrawColorB             int             `json:"ItemDrawColorB"`
	ItemDrawColorIsTransparent bool            `json:"ItemDrawColorIsTransparent"`
}

// ---------------------------------------------------------------------------
// Quick-access toolbar shared structs
// ---------------------------------------------------------------------------

// ToolBarItemID identifies a quick-access toolbar item.
type ToolBarItemID struct {
	ItemIDType        ToolBarItemType `json:"ItemIDType"`
	ItemIDCommandType string          `json:"ItemIDCommandType"`
	ItemIDCommandName string          `json:"ItemIDCommandName"`
	ItemIDToolUuid    string          `json:"ItemIDToolUuid"`
}

// QuickAccessItem is a leaf of a quick-access item set.
type QuickAccessItem struct {
	ToolBarItemID
	ItemShowName               string `json:"ItemShowName"`
	ItemIsEnabled              bool   `json:"ItemIsEnabled"`
	ItemIsChecked              bool   `json:"ItemIsChecked"`
	ItemDrawColorR             int    `json:"ItemDrawColorR"`
	ItemDrawColorG             int    `json:"ItemDrawColorG"`
	ItemDrawColorB             int    `json:"ItemDrawColorB"`
	ItemIsDrawColorTransparent bool   `json:"ItemIsDrawColorTransparent"`
}

// QuickAccessItemSet is a named set of items, nested rows -> columns -> items.
type QuickAccessItemSet struct {
	ItemSetName string                `json:"ItemSetName"`
	ItemSetUuid string                `json:"ItemSetUuid"`
	ItemSet     [][][]QuickAccessItem `json:"ItemSet"`
}

// QuickAccessViewInfo carries the current set indices.
type QuickAccessViewInfo struct {
	ViewInfoCurrentSetIndex          int `json:"ViewInfoCurrentSetIndex"`
	ViewInfoRemoteControllerSetIndex int `json:"ViewInfoRemoteControllerSetIndex"`
}

// GetQuickAccessDataResponse is the GetQuickAccessData reply.
type GetQuickAccessDataResponse struct {
	ToolBarData     []QuickAccessItemSet `json:"ToolBarData"`
	ToolBarViewInfo QuickAccessViewInfo  `json:"ToolBarViewInfo"`
}

// QuickAccessItemIcon is the GetQuickAccessItemIcon reply: the echoed item ID
// plus icon fields (present only when the host found an icon). The pixels are a
// hex-encoded RGBA string inside the JSON, not an appended binary tail.
type QuickAccessItemIcon struct {
	ToolBarItemID
	ItemIconWidth                  int    `json:"ItemIconWidth,omitempty"`
	ItemIconHeight                 int    `json:"ItemIconHeight,omitempty"`
	ItemIconPixelDataRGBAHexString string `json:"ItemIconPixelDataRGBAHexString,omitempty"`
	ItemIconColorIndex             int    `json:"ItemIconColorIndex,omitempty"`
	ItemIsUserIcon                 bool   `json:"ItemIsUserIcon,omitempty"`
}

// HasIcon reports whether icon pixel data is present.
func (i *QuickAccessItemIcon) HasIcon() bool {
	return i.ItemIconWidth > 0 && i.ItemIconHeight > 0 && i.ItemIconPixelDataRGBAHexString != ""
}

// DecodeImage decodes ItemIconPixelDataRGBAHexString into an RGBA image
// (width*height*4 bytes of hex-encoded RGBA).
func (i *QuickAccessItemIcon) DecodeImage() (*image.RGBA, error) {
	if !i.HasIcon() {
		return nil, fmt.Errorf("commands: item icon has no pixel data")
	}
	raw, err := hex.DecodeString(i.ItemIconPixelDataRGBAHexString)
	if err != nil {
		return nil, fmt.Errorf("commands: decode icon hex: %w", err)
	}
	if need := i.ItemIconWidth * i.ItemIconHeight * 4; len(raw) < need {
		return nil, fmt.Errorf("commands: icon needs %d bytes for %dx%d, got %d", need, i.ItemIconWidth, i.ItemIconHeight, len(raw))
	}
	img := image.NewRGBA(image.Rect(0, 0, i.ItemIconWidth, i.ItemIconHeight))
	copy(img.Pix, raw)
	return img, nil
}

// ---------------------------------------------------------------------------
// SyncQuickAccessUIState
// ---------------------------------------------------------------------------

// SyncQuickAccessRequest optionally asks the host to push an updated set.
type SyncQuickAccessRequest struct {
	UpdateTargetSetIndex int             `json:"UpdateTargetSetIndex"`
	UpdateTargetItemIDs  []ToolBarItemID `json:"UpdateTargetItemIDs"`
}

// QuickAccessUIState is a (best-effort) decode of a SyncQuickAccessUIState push.
// The body is polymorphic; consumers should branch on ServerSelectedTabKind
// (tab-only fast path) and SyncTargetType. SetToolBarData is left raw.
type QuickAccessUIState struct {
	ServerSelectedTabKind   TabKind         `json:"ServerSelectedTabKind,omitempty"`
	CanExecuteSetterCommand bool            `json:"CanExecuteSetterCommand,omitempty"`
	IsManipulating          bool            `json:"IsManipulating,omitempty"`
	UpdateTargetSetIndex    int             `json:"UpdateTargetSetIndex,omitempty"`
	DisabledItemIDs         []ToolBarItemID `json:"DisabledItemIDs,omitempty"`
	CheckedItemIDs          []ToolBarItemID `json:"CheckedItemIDs,omitempty"`
	SyncTargetType          SyncTargetType  `json:"SyncTargetType,omitempty"`
	SetIndex                int             `json:"SetIndex,omitempty"`
	SetName                 string          `json:"SetName,omitempty"`
	SetToolBarData          json.RawMessage `json:"SetToolBarData,omitempty"`
	ItemName                string          `json:"ItemName,omitempty"`
	ItemArrayIndex          int             `json:"ItemArrayIndex,omitempty"`
	ItemIndex               int             `json:"ItemIndex,omitempty"`
	// Embedded item ID for SyncTargetType CommandIcon/ToolIconWithName/ActionCommandName.
	ItemIDType        ToolBarItemType `json:"ItemIDType,omitempty"`
	ItemIDCommandType string          `json:"ItemIDCommandType,omitempty"`
	ItemIDCommandName string          `json:"ItemIDCommandName,omitempty"`
	ItemIDToolUuid    string          `json:"ItemIDToolUuid,omitempty"`
}

// ---------------------------------------------------------------------------
// SyncColorCircleUIState
// ---------------------------------------------------------------------------

// ColorCircleUIState is the color-circle UI state push/poll reply. When
// ServerSelectedTabKind is set, it is the tab-only fast path and the rest is
// absent. Otherwise the active color block is chosen by ColorSelectionModel.
// Color channels are 32-bit values (decode as int64), never floats.
type ColorCircleUIState struct {
	ServerSelectedTabKind     TabKind        `json:"ServerSelectedTabKind,omitempty"`
	CanExecuteSetterCommand   bool           `json:"CanExecuteSetterCommand,omitempty"`
	IsManipulating            bool           `json:"IsManipulating,omitempty"`
	IsToolBrushSizeAvailable  bool           `json:"IsToolBrushSizeAvailable,omitempty"`
	IsToolAlphaAvailable      bool           `json:"IsToolAlphaAvailable,omitempty"`
	CurrentToolBrushSize      float64        `json:"CurrentToolBrushSize,omitempty"`
	CurrentToolAlphaPercent   int64          `json:"CurrentToolAlphaPercent,omitempty"`
	LengthUnitKind            LengthUnitKind `json:"LengthUnitKind,omitempty"`
	CurrentColorIndex         int64          `json:"CurrentColorIndex,omitempty"`
	IsCurrentColorTransparent bool           `json:"IsCurrentColorTransparent,omitempty"`
	PrevColorIndex            int64          `json:"PrevColorIndex,omitempty"`
	ColorSelectionModel       ColorSpaceKind `json:"ColorSelectionModel,omitempty"`
	HSVColorMainH             int64          `json:"HSVColorMainH,omitempty"`
	HSVColorMainS             int64          `json:"HSVColorMainS,omitempty"`
	HSVColorMainV             int64          `json:"HSVColorMainV,omitempty"`
	HSVColorSubH              int64          `json:"HSVColorSubH,omitempty"`
	HSVColorSubS              int64          `json:"HSVColorSubS,omitempty"`
	HSVColorSubV              int64          `json:"HSVColorSubV,omitempty"`
	HLSColorMainH             int64          `json:"HLSColorMainH,omitempty"`
	HLSColorMainL             int64          `json:"HLSColorMainL,omitempty"`
	HLSColorMainS             int64          `json:"HLSColorMainS,omitempty"`
	HLSColorSubH              int64          `json:"HLSColorSubH,omitempty"`
	HLSColorSubL              int64          `json:"HLSColorSubL,omitempty"`
	HLSColorSubS              int64          `json:"HLSColorSubS,omitempty"`
}

// ---------------------------------------------------------------------------
// SyncGesturePadUIState
// ---------------------------------------------------------------------------

// NavigatorData is one navigator/gesture-pad button's enable/check state.
type NavigatorData struct {
	Type      NavigatorCommandType `json:"Type"`
	IsEnabled bool                 `json:"IsEnabled"`
	IsChecked bool                 `json:"IsChecked"`
}

// GestureHintState is one gesture-pad hint row. Name is a runtime-localized
// label provided by the host.
type GestureHintState struct {
	Name              string `json:"Name"`
	IsEnabled         bool   `json:"IsEnabled"`
	SwipeToMouseWheel bool   `json:"SwipeToMouseWheel"`
}

// GesturePadUIState is the SyncGesturePadUIState reply object.
type GesturePadUIState struct {
	ServerSelectedTabKind   TabKind            `json:"ServerSelectedTabKind,omitempty"`
	CanExecuteSetterCommand bool               `json:"CanExecuteSetterCommand,omitempty"`
	IsManipulating          bool               `json:"IsManipulating,omitempty"`
	NavigatorDataArray      []NavigatorData    `json:"NavigatorDataArray,omitempty"`
	GestureHintArray        []GestureHintState `json:"GestureHintArray,omitempty"`
}

// ---------------------------------------------------------------------------
// SyncColorMixUIState / SyncSubViewUIState (identical RGB shape)
// ---------------------------------------------------------------------------

// ColorRGBUIState is the shared shape of SyncColorMixUIState and
// SyncSubViewUIState replies (channels are 0-255 for SubView, 32-bit RGB for
// ColorMix). ServerSelectedTabKind is the tab-only fast path.
type ColorRGBUIState struct {
	ServerSelectedTabKind     TabKind `json:"ServerSelectedTabKind,omitempty"`
	CanExecuteSetterCommand   bool    `json:"CanExecuteSetterCommand,omitempty"`
	IsManipulating            bool    `json:"IsManipulating,omitempty"`
	CurrentColorRed           int     `json:"CurrentColorRed,omitempty"`
	CurrentColorGreen         int     `json:"CurrentColorGreen,omitempty"`
	CurrentColorBlue          int     `json:"CurrentColorBlue,omitempty"`
	IsCurrentColorTransparent bool    `json:"IsCurrentColorTransparent,omitempty"`
}

// ---------------------------------------------------------------------------
// Webtoon preview shared structs
// ---------------------------------------------------------------------------

// CanvasSize is one canvas's dimensions in a preview gallery.
type CanvasSize struct {
	CanvasWidth  int `json:"CanvasWidth"`
	CanvasHeight int `json:"CanvasHeight"`
}

// WebtoonPreviewRect is a preview block rectangle.
type WebtoonPreviewRect struct {
	BlockLeft   int `json:"BlockLeft"`
	BlockTop    int `json:"BlockTop"`
	BlockRight  int `json:"BlockRight"`
	BlockBottom int `json:"BlockBottom"`
}

// ---------------------------------------------------------------------------
// PreviewWebtoonFromClient (controller -> host; ReadPreviewBlock returns binary)
// ---------------------------------------------------------------------------

// PreviewSyncRequest is the SyncPreview operation.
type PreviewSyncRequest struct {
	Operation WebtoonOperationFromClient `json:"Operation"`
}

// PreviewUpdateCanvasRequest is the UpdateCanvas operation.
type PreviewUpdateCanvasRequest struct {
	Operation   WebtoonOperationFromClient `json:"Operation"`
	CanvasIndex int                        `json:"CanvasIndex"`
}

// PreviewUpdateGalleryRequest is the UpdateGallery operation.
type PreviewUpdateGalleryRequest struct {
	Operation WebtoonOperationFromClient `json:"Operation"`
	MaxLength int                        `json:"MaxLength"`
}

// PreviewReadBlockRequest is the ReadPreviewBlock operation. The reply carries a
// base64-encoded RGB pixel tail (see the client's preview helpers).
type PreviewReadBlockRequest struct {
	Operation                   WebtoonOperationFromClient `json:"Operation"`
	GalleryIdentificationNumber int                        `json:"GalleryIdentificationNumber"`
	CanvasIndex                 int                        `json:"CanvasIndex"`
	BlockLeft                   int                        `json:"BlockLeft"`
	BlockTop                    int                        `json:"BlockTop"`
	BlockRight                  int                        `json:"BlockRight"`
	BlockBottom                 int                        `json:"BlockBottom"`
	BlockIndex                  int                        `json:"BlockIndex"`
}

// PreviewWebtoonResponse is the union of all PreviewWebtoonFromClient replies.
// Inspect Operation to know which fields are populated.
type PreviewWebtoonResponse struct {
	Operation                   WebtoonOperationFromClient `json:"Operation,omitempty"`
	MaxLength                   int                        `json:"MaxLength,omitempty"`
	CanvasCount                 int                        `json:"CanvasCount,omitempty"`
	CanvasIndex                 int                        `json:"CanvasIndex,omitempty"`
	CanvasWidth                 int                        `json:"CanvasWidth,omitempty"`
	CanvasHeight                int                        `json:"CanvasHeight,omitempty"`
	CanvasSizeArray             []CanvasSize               `json:"CanvasSizeArray,omitempty"`
	GalleryIdentificationNumber int                        `json:"GalleryIdentificationNumber,omitempty"`
	BlockLeft                   int                        `json:"BlockLeft,omitempty"`
	BlockTop                    int                        `json:"BlockTop,omitempty"`
	BlockRight                  int                        `json:"BlockRight,omitempty"`
	BlockBottom                 int                        `json:"BlockBottom,omitempty"`
	BlockIndex                  int                        `json:"BlockIndex,omitempty"`
}

// ---------------------------------------------------------------------------
// PreviewWebtoonFromServer (host-side preview ops)
// ---------------------------------------------------------------------------

// PreviewResetGalleryRequest is the ResetGallery operation.
type PreviewResetGalleryRequest struct {
	Operation WebtoonOperationFromServer `json:"Operation"`
}

// PreviewResetCanvasRequest is the ResetCanvas operation.
type PreviewResetCanvasRequest struct {
	Operation   WebtoonOperationFromServer `json:"Operation"`
	CanvasIndex int                        `json:"CanvasIndex"`
}

// PreviewChangeNotificationRequest is the ChangeNotification operation (the wire
// Operation string is "ChangeNotification").
type PreviewChangeNotificationRequest struct {
	Operation                   WebtoonOperationFromServer `json:"Operation"`
	GalleryIdentificationNumber int64                      `json:"GalleryIdentificationNumber"`
	CanvasIndex                 int                        `json:"CanvasIndex"`
	RectArray                   []WebtoonPreviewRect       `json:"RectArray"`
}
