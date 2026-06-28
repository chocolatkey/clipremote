package clipremote

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/packets"
	"github.com/chocolatkey/clipremote/pkg/preview"
	"github.com/sirupsen/logrus"
)

// This file provides idiomatic, typed methods for every companion-mode command.
// Outgoing methods take a context.Context (use context.Background() if you don't
// need cancellation); host-pushed commands are surfaced via the On* registrars.

// callVoid sends a command whose reply body is unused and reports only success.
func (c *Client) callVoid(ctx context.Context, command commands.Command, detail any) error {
	_, err := c.call(ctx, command, detail)
	return err
}

// callDecode sends a command and decodes its reply detail into v.
func (c *Client) callDecode(ctx context.Context, command commands.Command, detail, v any) error {
	scp, err := c.call(ctx, command, detail)
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	return scp.Decode(v)
}

// ---------------------------------------------------------------------------
// View / tab state
// ---------------------------------------------------------------------------

// GetModifyKeyString asks the host for the modifier-key descriptions and OS.
func (c *Client) GetModifyKeyString(ctx context.Context, shift, ctrl, alt bool) (*commands.ModifyKeyResponse, error) {
	var resp commands.ModifyKeyResponse
	err := c.callDecode(ctx, commands.GetModifyKeyString,
		commands.ModifyKeyRequest{ShiftPushed: shift, CtrlPushed: ctrl, AltPushed: alt}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetServerSelectedTabKind returns the host's currently selected remote tab.
func (c *Client) GetServerSelectedTabKind(ctx context.Context) (commands.TabKind, error) {
	var resp commands.ServerSelectedTabKindResponse
	if err := c.callDecode(ctx, commands.GetServerSelectedTabKind, nil, &resp); err != nil {
		return commands.TabInvalid, err
	}
	return resp.ServerSelectedTabKind, nil
}

// SetServerSelectedTabKind notifies the host of the controller's selected tab.
// Note: the binary does not serialize the tab kind (it is tracked in-process);
// the wire body is empty. The argument is accepted for API clarity.
func (c *Client) SetServerSelectedTabKind(ctx context.Context, _ commands.TabKind) error {
	return c.callVoid(ctx, commands.SetServerSelectedTabKind, nil)
}

// ClearServerSelectedTabKind clears the host's selected tab (empty-body command,
// byte-identical to SetServerSelectedTabKind).
func (c *Client) ClearServerSelectedTabKind(ctx context.Context) error {
	return c.callVoid(ctx, commands.SetServerSelectedTabKind, nil)
}

// DoModeChange presses/releases proxy modifier keys on the host.
func (c *Client) DoModeChange(ctx context.Context, shift, control, alt bool, kind commands.KeyboardEventKind) error {
	return c.callVoid(ctx, commands.DoModeChange, commands.ModeChangeRequest{
		FlagShift:              shift,
		FlagControl:            control,
		FlagAlt:                alt,
		ProxyKeyboardEventKind: kind,
	})
}

// ---------------------------------------------------------------------------
// Color
// ---------------------------------------------------------------------------

// SetCurrentColorRGB sets the host's current color from RGB (0-255).
func (c *Client) SetCurrentColorRGB(ctx context.Context, r, g, b uint8, transparent bool) error {
	return c.callVoid(ctx, commands.SetCurrentColor, commands.SetColorRGBRequest{
		IsColorTransparent: transparent,
		RGBColorR:          int(r),
		RGBColorG:          int(g),
		RGBColorB:          int(b),
	})
}

// SetCurrentColorHLS sets the host's current color from raw 32-bit HLS channels.
func (c *Client) SetCurrentColorHLS(ctx context.Context, colorIndex, h, l, s int, transparent bool) error {
	return c.callVoid(ctx, commands.SetCurrentColor, commands.SetColorHLSRequest{
		ColorIndex:         colorIndex,
		IsColorTransparent: transparent,
		ColorSpaceKind:     commands.ColorSpaceHLS,
		HLSColorH:          h,
		HLSColorL:          l,
		HLSColorS:          s,
	})
}

// SetCurrentColorHSV sets the host's current color from raw 32-bit HSV channels.
func (c *Client) SetCurrentColorHSV(ctx context.Context, colorIndex, h, s, v int, transparent bool) error {
	return c.callVoid(ctx, commands.SetCurrentColor, commands.SetColorHSVRequest{
		ColorIndex:         colorIndex,
		IsColorTransparent: transparent,
		ColorSpaceKind:     commands.ColorSpaceHSV,
		HSVColorH:          h,
		HSVColorS:          s,
		HSVColorV:          v,
	})
}

// SetColorSelectionModel sets the color-circle model (HSV/HLS).
func (c *Client) SetColorSelectionModel(ctx context.Context, kind commands.ColorSpaceKind) error {
	return c.callVoid(ctx, commands.SetColorSelectionModel, commands.ColorSelectionModelRequest{ColorSpaceKind: kind})
}

// ---------------------------------------------------------------------------
// Tool parameters
// ---------------------------------------------------------------------------

// SetBrushSize sets the current tool's brush size. The wire detail is a bare
// JSON number; the host rejects values <= 0.
func (c *Client) SetBrushSize(ctx context.Context, brushSize float64) error {
	return c.callVoid(ctx, commands.SetBrushSize, brushSize)
}

// SetAlpha sets the current tool's opacity percent (bare JSON integer).
func (c *Client) SetAlpha(ctx context.Context, alphaPercent int) error {
	return c.callVoid(ctx, commands.SetAlpha, alphaPercent)
}

// ---------------------------------------------------------------------------
// Canvas navigation / gestures
// ---------------------------------------------------------------------------

// DoGesture sends a gesture-pad command. Coordinates are sent as given (the
// controller is responsible for any display scaling); the detail is a 10-element
// positional array.
func (c *Client) DoGesture(ctx context.Context, gesture commands.GestureCommandType, p1, p2 image.Point, factorA, factorB float64, flag bool, subKind int, seq int64) error {
	detail := commands.GestureDetail(gesture, p1.X, p1.Y, p2.X, p2.Y, factorA, factorB, flag, subKind, seq)
	return c.callVoid(ctx, commands.DoGesture, detail)
}

// DoNavigator triggers a canvas navigation action (zoom/rotate/fit/etc).
func (c *Client) DoNavigator(ctx context.Context, cmd commands.NavigatorCommandType) error {
	return c.callVoid(ctx, commands.DoNavigator, commands.NavigatorRequest{CommandType: cmd})
}

// ---------------------------------------------------------------------------
// Quick access
// ---------------------------------------------------------------------------

// DoQuickAccessTool activates a tool by UUID.
func (c *Client) DoQuickAccessTool(ctx context.Context, toolUUID string) error {
	return c.callVoid(ctx, commands.DoQuickAccess, commands.DoQuickAccessToolRequest{
		ItemType:     commands.ToolBarItemTool,
		ItemToolUuid: toolUUID,
	})
}

// DoQuickAccessCommand invokes a named command.
func (c *Client) DoQuickAccessCommand(ctx context.Context, commandType, commandName string) error {
	return c.callVoid(ctx, commands.DoQuickAccess, commands.DoQuickAccessCommandRequest{
		ItemType:        commands.ToolBarItemCommand,
		ItemCommandType: commandType,
		ItemCommandName: commandName,
	})
}

// DoQuickAccessDrawColor sets the draw color via the quick-access bar.
func (c *Client) DoQuickAccessDrawColor(ctx context.Context, r, g, b uint8, transparent bool) error {
	return c.callVoid(ctx, commands.DoQuickAccess, commands.DoQuickAccessDrawColorRequest{
		ItemType:                   commands.ToolBarItemDrawColor,
		ItemDrawColorR:             int(r),
		ItemDrawColorG:             int(g),
		ItemDrawColorB:             int(b),
		ItemDrawColorIsTransparent: transparent,
	})
}

// GetQuickAccessData returns the quick-access toolbar layout and view state.
func (c *Client) GetQuickAccessData(ctx context.Context) (*commands.GetQuickAccessDataResponse, error) {
	var resp commands.GetQuickAccessDataResponse
	if err := c.callDecode(ctx, commands.GetQuickAccessData, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetQuickAccessItemIcon fetches the icon for a quick-access item.
func (c *Client) GetQuickAccessItemIcon(ctx context.Context, item commands.ToolBarItemID) (*commands.QuickAccessItemIcon, error) {
	var resp commands.QuickAccessItemIcon
	if err := c.callDecode(ctx, commands.GetQuickAccessItemIcon, item, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Webtoon preview (controller -> host)
// ---------------------------------------------------------------------------

// PreviewBlock is a decoded ReadPreviewBlock reply: the response metadata plus
// the decoded RGB image.
type PreviewBlock struct {
	Response commands.PreviewWebtoonResponse
	Image    *image.RGBA
}

// PreviewWebtoonSyncPreview requests preview synchronization.
func (c *Client) PreviewWebtoonSyncPreview(ctx context.Context) (*commands.PreviewWebtoonResponse, error) {
	var resp commands.PreviewWebtoonResponse
	err := c.callDecode(ctx, commands.PreviewWebtoonFromClient,
		commands.PreviewSyncRequest{Operation: commands.WebtoonOpSyncPreview}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PreviewWebtoonUpdateGallery requests the gallery metadata (canvas sizes).
func (c *Client) PreviewWebtoonUpdateGallery(ctx context.Context, maxLength int) (*commands.PreviewWebtoonResponse, error) {
	var resp commands.PreviewWebtoonResponse
	err := c.callDecode(ctx, commands.PreviewWebtoonFromClient,
		commands.PreviewUpdateGalleryRequest{Operation: commands.WebtoonOpUpdateGallery, MaxLength: maxLength}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PreviewWebtoonUpdateCanvas requests a single canvas's dimensions.
func (c *Client) PreviewWebtoonUpdateCanvas(ctx context.Context, canvasIndex int) (*commands.PreviewWebtoonResponse, error) {
	var resp commands.PreviewWebtoonResponse
	err := c.callDecode(ctx, commands.PreviewWebtoonFromClient,
		commands.PreviewUpdateCanvasRequest{Operation: commands.WebtoonOpUpdateCanvas, CanvasIndex: canvasIndex}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PreviewWebtoonReadBlock reads a preview block and decodes its pixels. The
// pixel tail is base64-encoded RGB (3 bytes/px); dimensions come from block.
func (c *Client) PreviewWebtoonReadBlock(ctx context.Context, galleryID, canvasIndex, blockIndex int, block image.Rectangle) (*PreviewBlock, error) {
	req := commands.PreviewReadBlockRequest{
		Operation:                   commands.WebtoonOpReadPreviewBlock,
		GalleryIdentificationNumber: galleryID,
		CanvasIndex:                 canvasIndex,
		BlockIndex:                  blockIndex,
		BlockLeft:                   block.Min.X,
		BlockTop:                    block.Min.Y,
		BlockRight:                  block.Max.X,
		BlockBottom:                 block.Max.Y,
	}
	scp, err := c.call(ctx, commands.PreviewWebtoonFromClient, req)
	if err != nil {
		return nil, err
	}

	out := &PreviewBlock{}
	if err := scp.Decode(&out.Response); err != nil {
		return nil, fmt.Errorf("decode preview response: %w", err)
	}
	if len(scp.Data) > 0 {
		rgb, err := decodePreviewRGB(scp.Data)
		if err != nil {
			return nil, fmt.Errorf("decode preview pixels: %w", err)
		}
		w := block.Dx()
		h := block.Dy()
		img, err := preview.Decode(rgb, w, h)
		if err != nil {
			return nil, err
		}
		out.Image = img
	}
	return out, nil
}

// decodePreviewRGB decodes CSP's base64 RGB pixel string, tolerating both
// padded and unpadded base64.
func decodePreviewRGB(data []byte) ([]byte, error) {
	s := strings.TrimSpace(string(data))
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// ---------------------------------------------------------------------------
// Webtoon preview (host-side operations)
// ---------------------------------------------------------------------------

// PreviewWebtoonResetGallery resets the preview gallery.
func (c *Client) PreviewWebtoonResetGallery(ctx context.Context) error {
	return c.callVoid(ctx, commands.PreviewWebtoonFromServer,
		commands.PreviewResetGalleryRequest{Operation: commands.WebtoonOpResetGallery})
}

// PreviewWebtoonResetCanvas resets a single preview canvas.
func (c *Client) PreviewWebtoonResetCanvas(ctx context.Context, canvasIndex int) error {
	return c.callVoid(ctx, commands.PreviewWebtoonFromServer,
		commands.PreviewResetCanvasRequest{Operation: commands.WebtoonOpResetCanvas, CanvasIndex: canvasIndex})
}

// PreviewWebtoonChangeNotification notifies of changed preview rectangles.
func (c *Client) PreviewWebtoonChangeNotification(ctx context.Context, galleryID int64, canvasIndex int, rects []commands.WebtoonPreviewRect) error {
	return c.callVoid(ctx, commands.PreviewWebtoonFromServer, commands.PreviewChangeNotificationRequest{
		Operation:                   commands.WebtoonOpChangeNotification,
		GalleryIdentificationNumber: galleryID,
		CanvasIndex:                 canvasIndex,
		RectArray:                   rects,
	})
}

// ---------------------------------------------------------------------------
// UI-state sync polls (controller -> host)
// ---------------------------------------------------------------------------

// SyncColorCircleUIState polls the color-circle UI state.
func (c *Client) SyncColorCircleUIState(ctx context.Context) (*commands.ColorCircleUIState, error) {
	var resp commands.ColorCircleUIState
	if err := c.callDecode(ctx, commands.SyncColorCircleUIState, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncColorMixUIState polls the color-mix UI state.
func (c *Client) SyncColorMixUIState(ctx context.Context) (*commands.ColorRGBUIState, error) {
	var resp commands.ColorRGBUIState
	if err := c.callDecode(ctx, commands.SyncColorMixUIState, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncSubViewUIState polls the sub-view UI state.
func (c *Client) SyncSubViewUIState(ctx context.Context) (*commands.ColorRGBUIState, error) {
	var resp commands.ColorRGBUIState
	if err := c.callDecode(ctx, commands.SyncSubViewUIState, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncSettingUIState polls the setting tab's selected-tab state.
func (c *Client) SyncSettingUIState(ctx context.Context) (commands.TabKind, error) {
	var resp commands.ServerSelectedTabKindResponse
	if err := c.callDecode(ctx, commands.SyncSettingUIState, nil, &resp); err != nil {
		return commands.TabInvalid, err
	}
	return resp.ServerSelectedTabKind, nil
}

// SyncGesturePadUIState sends the controller's navigator data (bare array) and
// returns the host's recomputed gesture-pad state.
func (c *Client) SyncGesturePadUIState(ctx context.Context, nav []commands.NavigatorData) (*commands.GesturePadUIState, error) {
	if nav == nil {
		nav = []commands.NavigatorData{}
	}
	var resp commands.GesturePadUIState
	if err := c.callDecode(ctx, commands.SyncGesturePadUIState, nav, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncQuickAccessUIState asks the host to push quick-access UI state for a set.
func (c *Client) SyncQuickAccessUIState(ctx context.Context, updateTargetSetIndex int, itemIDs []commands.ToolBarItemID) (*commands.QuickAccessUIState, error) {
	if itemIDs == nil {
		itemIDs = []commands.ToolBarItemID{}
	}
	var resp commands.QuickAccessUIState
	err := c.callDecode(ctx, commands.SyncQuickAccessUIState,
		commands.SyncQuickAccessRequest{UpdateTargetSetIndex: updateTargetSetIndex, UpdateTargetItemIDs: itemIDs}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// PreNotifyOfServerShutdown notifies the peer of an impending shutdown.
func (c *Client) PreNotifyOfServerShutdown(ctx context.Context, isManual bool) error {
	return c.callVoid(ctx, commands.PreNotifyOfServerShutdown,
		commands.PreNotifyOfServerShutdownRequest{IsManualShutdown: isManual})
}

// ---------------------------------------------------------------------------
// Host-pushed command handlers (server -> controller)
// ---------------------------------------------------------------------------

// OnServerCommand registers a raw handler for a host-pushed command. The handler
// may return a reply detail; returning ok=false makes the auto-ACK an error.
func (c *Client) OnServerCommand(command commands.Command, fn func(*packets.ServerCommand)) {
	c.Handle(command, func(pkt *packets.ServerCommand) ([]byte, bool) {
		fn(pkt)
		return nil, true
	})
}

// decodePush decodes a pushed command into v, logging (but ACKing) on failure.
func decodePush[T any](pkt *packets.ServerCommand, fn func(*T)) ([]byte, bool) {
	var v T
	if err := pkt.Decode(&v); err != nil {
		logrus.Warnf("failed decoding pushed %s: %v", pkt.Command, err)
		return nil, true
	}
	fn(&v)
	return nil, true
}

// OnPreNotifyOfServerShutdown is called when the host announces a shutdown.
func (c *Client) OnPreNotifyOfServerShutdown(fn func(commands.PreNotifyOfServerShutdownRequest)) {
	c.Handle(commands.PreNotifyOfServerShutdown, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, func(v *commands.PreNotifyOfServerShutdownRequest) { fn(*v) })
	})
}

// OnSyncColorCircleUIState is called when the host pushes color-circle UI state.
func (c *Client) OnSyncColorCircleUIState(fn func(*commands.ColorCircleUIState)) {
	c.Handle(commands.SyncColorCircleUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, fn)
	})
}

// OnSyncColorMixUIState is called when the host pushes color-mix UI state.
func (c *Client) OnSyncColorMixUIState(fn func(*commands.ColorRGBUIState)) {
	c.Handle(commands.SyncColorMixUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, fn)
	})
}

// OnSyncSubViewUIState is called when the host pushes sub-view UI state.
func (c *Client) OnSyncSubViewUIState(fn func(*commands.ColorRGBUIState)) {
	c.Handle(commands.SyncSubViewUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, fn)
	})
}

// OnSyncGesturePadUIState is called when the host pushes gesture-pad UI state.
func (c *Client) OnSyncGesturePadUIState(fn func(*commands.GesturePadUIState)) {
	c.Handle(commands.SyncGesturePadUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, fn)
	})
}

// OnSyncSettingUIState is called when the host pushes setting UI state.
func (c *Client) OnSyncSettingUIState(fn func(commands.TabKind)) {
	c.Handle(commands.SyncSettingUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, func(v *commands.ServerSelectedTabKindResponse) { fn(v.ServerSelectedTabKind) })
	})
}

// OnSyncQuickAccessUIState is called when the host pushes quick-access UI state.
func (c *Client) OnSyncQuickAccessUIState(fn func(*commands.QuickAccessUIState)) {
	c.Handle(commands.SyncQuickAccessUIState, func(pkt *packets.ServerCommand) ([]byte, bool) {
		return decodePush(pkt, fn)
	})
}

// OnPreviewWebtoonFromServer is called when the host pushes a preview operation.
func (c *Client) OnPreviewWebtoonFromServer(fn func(*packets.ServerCommand)) {
	c.OnServerCommand(commands.PreviewWebtoonFromServer, fn)
}
