# Clip Studio Paint Remote

(Initial research done manually, completed the rest of the commands using Claude)

A Go reimplementation of the **client (smartphone) side** of Clip Studio Paint's
["companion mode"](https://tips.clip-studio.com/en-us/articles/5995) — the
protocol normally used when you scan the in-app QR code to remote-control CSP on
a PC from a phone.

The wire protocol, all **24 commands**, their JSON detail schemas, the auth
crypto, and the packet framing were reverse-engineered from CSP's symboled
Android binary (`libClipStudioPaint.so`).

## Quick start (web UI + HTTP bridge)

1. In CSP, click **Connect to smartphone**. A QR code appears.
2. Start the bridge — either way it authenticates and listens on
   `http://localhost:8089`:
   - **Auto-detect (recommended)** — leave the QR dialog visible and run:
     ```sh
     go run ./cmd/server
     ```
     It repeatedly screenshots every display (the process is made per-monitor
     DPI-aware so high-DPI/scaled monitors capture cleanly), finds the CSP
     companion QR code, decodes it, prints the host it will connect to, and
     connects. It waits up to 90s for the dialog to appear. (Use
     `go run ./cmd/server scan` to force this even if you normally pass a URL.)
   - **Manual** — decode the QR to a URL yourself (e.g. screenshot it and paste
     into <https://qr-code-scanner.net/#paste>) and pass it:
     ```sh
     go run ./cmd/server "https://companion.clip-studio.com/rc/en-us?s=XXXX"
     ```
3. Open <http://localhost:8089> in any modern browser for the **web UI**, or
   drive the HTTP endpoints directly (below).

> Auto-detect reads whatever QR is on your screen, so as a safety check it
> refuses a scanned QR that points to a non-LAN address (a decoy QR could
> otherwise redirect this password-bearing connection). Override with
> `CLIPREMOTE_ALLOW_PUBLIC=1` if your host is genuinely off-LAN.

**Environment variables:**

| Variable | Effect |
| --- | --- |
| `CLIPREMOTE_DEBUG` | Any value logs every protocol frame (the password is redacted). |
| `CLIPREMOTE_SCAN_DISPLAY` | Limit auto-detect to display index(es), e.g. `0` or `0,2`. |
| `CLIPREMOTE_ALLOW_PUBLIC` | Allow a scanned QR pointing off the LAN. |

### Web UI

The server embeds a zero-build web UI (plain HTML/CSS/JS, compiled into the
binary via `go:embed`) served from `/`. It exposes:

- a form for **every command** and variant (driven by the wire schemas), plus a
  raw command console;
- a **live connection-status** pill (polls `/api/status`) and an activity log
  that streams host pushes over Server-Sent Events (`/api/events`);
- the **webtoon preview** image with client-side zoom/pan and one-click PNG
  save (scan gallery → pick canvas → **Load full canvas**). CSP refuses a single
  preview block over ~2.6 MP, so large canvases are fetched as multiple
  sub-blocks (≤ a configurable pixel budget) and stitched into the full image;
- **canvas movement** — navigator quick-actions (zoom/fit/rotate/flip/reset), a
  nudge D-pad, and a drag-pan surface that synthesize `DoGesture` streams.

### HTTP endpoints

The web UI is built on these; you can also call them directly:

- `GET|POST /request?command=<name>&detail=<json>` → the response packet as JSON.
- `GET /preview?...&format=png` → a single webtoon preview block as PNG (or BMP).
- `GET /canvas?canvas_index=<n>&format=png` → an **entire** webtoon canvas, tiled
  and reassembled server-side into one image (gallery id + size auto-discovered).
  Optional `max_block_pixels`, `concurrency`, and explicit
  `gallery_identification_number`/`canvas_width`/`canvas_height`.
- `GET /api/status` → `{alive, remoteAddr, specVersion, quickAccessAvailable}`.
- `GET /api/events` → SSE stream of host-pushed commands.
- `GET /healthz` → `200 ok` / `503` for liveness checks.

### Shell examples

```sh
# any command + JSON detail -> JSON response
curl 'http://localhost:8089/request?command=GetModifyKeyString&detail={"ShiftPushed":false,"CtrlPushed":false,"AltPushed":false}'
curl 'http://localhost:8089/request?command=DoNavigator&detail={"CommandType":"ZoomIn"}'
curl 'http://localhost:8089/request?command=SetBrushSize&detail=42'      # bare scalar detail
curl 'http://localhost:8089/request?command=SetCurrentColor&detail={"IsColorTransparent":false,"RGBColorR":255,"RGBColorG":0,"RGBColorB":0}'

# single webtoon preview block -> PNG image
curl 'http://localhost:8089/preview?format=png&gallery_identification_number=1&canvas_index=0&block_index=0&block_left=0&block_top=0&block_right=690&block_bottom=1024' -o block.png

# entire webtoon canvas, tiled + reassembled server-side -> one PNG
curl 'http://localhost:8089/canvas?canvas_index=2&format=png' -o canvas.png
```

## Library usage

```go
client, password, err := clipremote.ConnectFromURL(shareURL)
if err != nil { log.Fatal(err) }
if _, err := client.Authenticate(password); err != nil { log.Fatal(err) }

ctx := context.Background()
_ = client.DoNavigator(ctx, commands.NavigatorZoomIn)
_ = client.SetCurrentColorRGB(ctx, 0x33, 0x66, 0xff, false)
_ = client.SetBrushSize(ctx, 24.0)
mods, _ := client.GetModifyKeyString(ctx, false, true, false) // -> Ctrl/Alt/Shift descriptions + host OS

// Read a whole webtoon canvas, tiled under CSP's per-block limit and reassembled
// into one *image.RGBA (nil opts = defaults: 2.4 MP blocks, 4-way concurrency):
img, _ := client.PreviewWebtoonReadCanvas(ctx, galleryID, canvasIndex, width, height, nil)

// React to host pushes (the client auto-ACKs them):
client.OnPreNotifyOfServerShutdown(func(e commands.PreNotifyOfServerShutdownRequest) { /* ... */ })
client.OnSyncColorCircleUIState(func(s *commands.ColorCircleUIState) { /* ... */ })
```

Every command has a typed method; `SendCommandSync` / `SendCommand` remain
available for raw access.

## Commands

| Controller → host | Host → controller (handled + auto-ACKed) |
| --- | --- |
| Authenticate, TellHeartbeat, GetModifyKeyString, GetServerSelectedTabKind, SetServerSelectedTabKind, DoModeChange, SetCurrentColor, SetColorSelectionModel, SetBrushSize, SetAlpha, DoGesture, DoNavigator, DoQuickAccess, GetQuickAccessData, GetQuickAccessItemIcon, PreviewWebtoonFromClient, PreviewWebtoonFromServer | PreNotifyOfServerShutdown, SyncColorCircleUIState, SyncColorMixUIState, SyncSubViewUIState, SyncGesturePadUIState, SyncSettingUIState, SyncQuickAccessUIState |

## Protocol notes

- **Framing**: `<type>$tcp_remote_command_protocol_version=1.0\x1e$command=<name>\x1e$serial=<n>\x1e$detail=<detail>\x1e\x00`.
- **Packet types**: client command `0x01`, success/ACK `0x06`, error/NAK `0x15`.
  CSP's response handlers determine success from the reply **detail** (e.g.
  `AuthErrorReason`), not the type byte — the Authenticate reply is even `0x06`
  on success regardless.
- **Auth**: detail is `[generation, enc(password), enc(newPassword)]`; `enc` is a
  repeating-key XOR (`B6 D5 92 C4 A7 83 E1`) hex-encoded. A fresh password is
  rotated in for reconnects.
- **Detail bodies** are UTF-8 JSON; a couple are bare scalars (`SetBrushSize`,
  `SetAlpha`) or positional arrays (`Authenticate`, `DoGesture`). Outgoing detail
  may not contain control bytes `0x00-0x06`, `0x0E-0x1F`, or `0x7F`, so binary
  payloads (webtoon preview pixels) are base64-encoded after a `0x0B` separator.

## Packages

- `clipremote` — `Client`, connection bootstrap, typed command methods, host-push handlers.
- `pkg/commands` — command names, enums, and typed request/response detail bodies.
- `pkg/packets` — packet types and frame encode/decode.
- `pkg/protocol` — framing constants.
- `pkg/crypto` — the auth / share-parameter obfuscation.
- `pkg/preview` — webtoon preview RGB decoding.
- `pkg/qrscan` — screenshots displays and decodes the companion QR for auto-detect.
