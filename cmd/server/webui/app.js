"use strict";
/*
 * clipremote web UI — drives every CSP companion-mode command through the
 * server's /request bridge, shows live host pushes over /api/events (SSE),
 * polls /api/status for health, and renders the webtoon preview via /preview.
 *
 * No build step, no framework: the command catalog below is plain data and the
 * forms are generated from it.
 */

// ---------------------------------------------------------------------------
// Wire enums (mirrors pkg/commands/enums.go)
// ---------------------------------------------------------------------------
const TAB_KINDS = ["Invalid", "QuickAccess", "TouchGesture", "ColorCircle", "SubView",
  "ColorMixing", "WebtoonPreview", "ModeChange", "Setting", "Promotion"];
const COLOR_SPACES = ["HSV", "HLS"];
const KEY_EVENTS = ["KeyDown", "KeyUp", "Unknown"];
const ITEM_TYPES = ["Tool", "Command", "DrawColor", "Invalid"];
const NAVIGATOR = ["None", "ZoomIn", "ZoomOut", "PixelSize", "Fitting", "WholeSize",
  "RotateLeft", "RotateRight", "Rotate0", "ReverseHorz", "ReverseVert", "ResetPosition"];
const GESTURES = [
  ["B", "Begin (down)"], ["E", "End (up)"], ["BM", "Begin move"], ["M", "Move"], ["EM", "End move"],
  ["BS", "Begin swipe"], ["S", "Swipe"], ["ES", "End swipe"], ["HT", "Hover touch"],
  ["BR", "Begin rotate"], ["R", "Rotate"], ["ER", "End rotate"],
  ["B2T", "Begin 2-finger tap"], ["2T", "2-finger tap"], ["E2T", "End 2-finger tap"],
  ["BLP", "Begin long press"], ["LP", "Long press"], ["ELP", "End long press"],
];
const ALL_COMMANDS = [
  "Authenticate", "TellHeartbeat", "PreNotifyOfServerShutdown", "GetServerSelectedTabKind",
  "SetServerSelectedTabKind", "GetModifyKeyString", "DoModeChange", "SetCurrentColor",
  "SetColorSelectionModel", "SetBrushSize", "SetAlpha", "DoGesture", "DoNavigator",
  "GetQuickAccessData", "GetQuickAccessItemIcon", "DoQuickAccess", "PreviewWebtoonFromClient",
  "PreviewWebtoonFromServer", "SyncQuickAccessUIState", "SyncColorCircleUIState",
  "SyncColorMixUIState", "SyncGesturePadUIState", "SyncSubViewUIState", "SyncSettingUIState",
];

// Move/pan gesture families for the canvas pad.
const PAN_FAMILIES = {
  Move: { begin: "BM", mid: "M", end: "EM" },
  Swipe: { begin: "BS", mid: "S", end: "ES" },
};

let gestureSeq = 0; // monotonic gesture stream counter

// ---------------------------------------------------------------------------
// Tiny DOM helpers
// ---------------------------------------------------------------------------
const $ = (sel, root = document) => root.querySelector(sel);
function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}
const div = (cls, text) => el("div", cls, text);
const span = (cls, text) => el("span", cls, text);
function button(cls, text) { const b = el("button", cls, text); b.type = "button"; return b; }
function esc(s) {
  return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}
function nowStr() {
  const d = new Date();
  const p = (n, w = 2) => String(n).padStart(w, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`;
}
function hexToRgb(hex) {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex || "");
  if (!m) return { r: 0, g: 0, b: 0 };
  return { r: parseInt(m[1], 16), g: parseInt(m[2], 16), b: parseInt(m[3], 16) };
}
function rgbToHex(r, g, b) {
  const h = (n) => Math.max(0, Math.min(255, n | 0)).toString(16).padStart(2, "0");
  return "#" + h(r) + h(g) + h(b);
}

// ---------------------------------------------------------------------------
// JSON pretty-printer with light syntax highlighting
// ---------------------------------------------------------------------------
function jsonHTML(value) {
  const json = esc(JSON.stringify(value, null, 2));
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (m) => {
      let cls = "n";
      if (/^"/.test(m)) cls = /:$/.test(m) ? "k" : "s";
      else if (/true|false|null/.test(m)) cls = "b";
      return `<span class="${cls}">${m}</span>`;
    }
  );
}

// ---------------------------------------------------------------------------
// Activity log
// ---------------------------------------------------------------------------
const logEl = () => $("#log");
function addLog(dir, title, body) {
  const li = el("li", "dir-" + dir);
  const head = div("lh");
  head.append(span("lcmd", title));
  head.append(span("lt", nowStr()));
  li.append(head);
  if (body != null) {
    const b = div("lbody");
    b.innerHTML = typeof body === "string" ? esc(body) : jsonHTML(body);
    li.append(b);
  }
  const list = logEl();
  list.append(li);
  while (list.childElementCount > 300) list.removeChild(list.firstChild);
  if ($("#activity-autoscroll").checked) list.scrollTop = list.scrollHeight;
}

// ---------------------------------------------------------------------------
// Server API
// ---------------------------------------------------------------------------
async function request(command, detail) {
  const body = new URLSearchParams();
  body.set("command", command);
  if (detail !== null && detail !== undefined) body.set("detail", JSON.stringify(detail));
  addLog("out", "→ " + command, detail === null || detail === undefined ? "(no detail)" : detail);

  const res = await fetch("/request", {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body,
  });
  const text = await res.text();
  if (!res.ok) {
    addLog("err", "✗ " + command, text || res.status + " " + res.statusText);
    throw new Error(text || res.status + " " + res.statusText);
  }
  const resp = JSON.parse(text);
  const isErr = resp.type === "error";
  addLog(isErr ? "err" : "ok", (isErr ? "✗ " : "← ") + command, resp.detail ?? "(empty)");
  return resp;
}

async function fetchStatus() {
  const res = await fetch("/api/status", { cache: "no-store" });
  if (!res.ok) throw new Error("status " + res.status);
  return res.json();
}

// ---------------------------------------------------------------------------
// Result rendering
// ---------------------------------------------------------------------------
function showResult(resultEl, kind, html) {
  resultEl.innerHTML = "";
  resultEl.classList.add("show");
  const tagClass = kind === "err" ? "err" : kind === "info" ? "info" : "ok";
  const label = kind === "err" ? "error" : kind === "info" ? "info" : "success";
  resultEl.append(Object.assign(div("rtag " + tagClass), { textContent: label }));
  const wrap = div();
  wrap.innerHTML = html;
  resultEl.append(wrap);
}
function showResponse(resultEl, resp) {
  const kind = resp.type === "error" ? "err" : "ok";
  let html = `<div class="preview-meta">serial ${resp.serial} · type ${esc(resp.type)}${resp.data ? " · data " + resp.data.length + "B" : ""}</div>`;
  html += `<pre class="json">${resp.detail !== undefined ? jsonHTML(resp.detail) : "(empty body)"}</pre>`;
  showResult(resultEl, kind, html);
}

// Generic submit for schema cards.
async function runCard(card, form, resultEl, btn) {
  let detail = null;
  try {
    detail = card.build ? card.build(getValues(form, card.fields || [])) : null;
  } catch (e) {
    showResult(resultEl, "err", esc(e.message));
    return;
  }
  btn.disabled = true;
  try {
    const resp = await request(card.command, detail);
    if (card.render) card.render(resp, resultEl, detail);
    else showResponse(resultEl, resp);
  } catch (e) {
    showResult(resultEl, "err", esc(e.message));
  } finally {
    btn.disabled = false;
  }
}

// ---------------------------------------------------------------------------
// Field rendering
// ---------------------------------------------------------------------------
function renderField(f) {
  if (f.type === "info") {
    const wrap = div("field");
    wrap.append(Object.assign(div("hint"), { innerHTML: f.html || esc(f.text || "") }));
    return wrap;
  }
  const wrap = div("field" + (f.type === "check" ? " row" : ""));
  const id = "f_" + Math.random().toString(36).slice(2);
  const label = el("label", null, f.label);
  label.htmlFor = id;

  let input;
  if (f.type === "check") {
    input = el("input");
    input.type = "checkbox";
    if (f.def) input.checked = true;
  } else if (f.type === "select") {
    input = el("select");
    (f.options || []).forEach((o) => {
      const val = Array.isArray(o) ? o[0] : o;
      const lab = Array.isArray(o) ? o[1] : o;
      const opt = el("option", null, lab);
      opt.value = val;
      if (val === f.def) opt.selected = true;
      input.append(opt);
    });
  } else if (f.type === "color") {
    input = el("input");
    input.type = "color";
    input.value = f.def || "#3aa0ff";
  } else {
    input = el("input");
    input.type = f.type === "num" ? "number" : "text";
    if (f.def != null) input.value = f.def;
    if (f.placeholder) input.placeholder = f.placeholder;
    if (f.type === "num") {
      if (f.min != null) input.min = f.min;
      if (f.max != null) input.max = f.max;
      input.step = f.step != null ? f.step : f.int ? 1 : "any";
    }
  }
  input.id = id;
  input.name = f.name;

  if (f.type === "check") {
    wrap.append(label, input);
  } else {
    wrap.append(label, input);
  }
  if (f.hint) wrap.append(Object.assign(div("hint"), { innerHTML: esc(f.hint) }));
  return wrap;
}

function getValues(form, fields) {
  const v = {};
  for (const f of fields) {
    if (f.type === "info") continue;
    const node = form.querySelector(`[name="${f.name}"]`);
    if (!node) continue;
    if (f.type === "check") v[f.name] = node.checked;
    else if (f.type === "color") v[f.name] = hexToRgb(node.value);
    else if (f.type === "num") {
      const raw = node.value;
      if (raw === "") { v[f.name] = f.def != null ? f.def : 0; continue; }
      const n = Number(raw);
      if (Number.isNaN(n)) throw new Error(`${f.label} must be a number`);
      v[f.name] = f.int ? Math.trunc(n) : n;
    } else v[f.name] = node.value;
  }
  return v;
}

// ---------------------------------------------------------------------------
// Command catalog
// ---------------------------------------------------------------------------
const GROUPS = [
  { id: "status", title: "Connection & Status" },
  { id: "view", title: "View & Tabs" },
  { id: "color", title: "Color" },
  { id: "tool", title: "Tool Parameters" },
  { id: "canvas", title: "Canvas Navigation" },
  { id: "quick", title: "Quick Access" },
  { id: "preview", title: "Webtoon Preview" },
  { id: "sync", title: "UI State Sync" },
  { id: "raw", title: "Raw Console" },
];

const CARDS = [
  // ---- Connection & status ----
  { group: "status", key: "session", title: "Session", custom: sessionCard, wide: false },
  {
    group: "status", key: "modkeys", command: "GetModifyKeyString",
    title: "Modifier key strings",
    desc: "Ask the host how it labels Shift/Ctrl/Alt and which OS it is.",
    fields: [
      { name: "ShiftPushed", label: "Shift pushed", type: "check" },
      { name: "CtrlPushed", label: "Ctrl pushed", type: "check" },
      { name: "AltPushed", label: "Alt pushed", type: "check" },
    ],
    build: (v) => ({ ShiftPushed: v.ShiftPushed, CtrlPushed: v.CtrlPushed, AltPushed: v.AltPushed }),
    submitLabel: "Get",
  },
  {
    group: "status", key: "heartbeat", command: "TellHeartbeat",
    title: "Heartbeat",
    desc: "The server already pings once per second; send a manual keep-alive here.",
    fields: [{ name: "idle", label: "Request idle-timer reset", type: "check" }],
    build: (v) => (v.idle ? { IdleTimerResetRequested: true } : null),
    submitLabel: "Ping",
  },
  {
    group: "status", key: "shutdownnotify", command: "PreNotifyOfServerShutdown",
    title: "Pre-notify shutdown",
    desc: "Normally pushed by the host; you can also send it. Watch the log for incoming ones.",
    fields: [{ name: "IsManualShutdown", label: "Manual shutdown", type: "check" }],
    build: (v) => ({ IsManualShutdown: v.IsManualShutdown }),
    submitLabel: "Notify",
  },

  // ---- View & tabs ----
  {
    group: "view", key: "gettab", command: "GetServerSelectedTabKind",
    title: "Get selected tab",
    desc: "Which remote tab the host currently shows.",
    build: () => null, submitLabel: "Get",
  },
  {
    group: "view", key: "settab", command: "SetServerSelectedTabKind",
    title: "Notify selected tab",
    desc: "Tells the host the controller switched tab. CSP tracks this in-process, so the wire body is intentionally empty.",
    fields: [{ name: "tab", label: "Tab (for your reference)", type: "select", options: TAB_KINDS, def: "QuickAccess" }],
    build: () => null, submitLabel: "Notify",
  },
  {
    group: "view", key: "modechange", command: "DoModeChange",
    title: "Proxy modifier keys",
    desc: "Press or release Shift/Ctrl/Alt on the host (e.g. hold Shift while drawing a line).",
    fields: [
      { name: "FlagShift", label: "Shift", type: "check" },
      { name: "FlagControl", label: "Control", type: "check" },
      { name: "FlagAlt", label: "Alt", type: "check" },
      { name: "ProxyKeyboardEventKind", label: "Event", type: "select", options: KEY_EVENTS, def: "KeyDown" },
    ],
    build: (v) => ({ FlagShift: v.FlagShift, FlagControl: v.FlagControl, FlagAlt: v.FlagAlt, ProxyKeyboardEventKind: v.ProxyKeyboardEventKind }),
    submitLabel: "Send",
  },

  // ---- Color ----
  {
    group: "color", key: "color-rgb", command: "SetCurrentColor",
    title: "Set color · RGB",
    desc: "Set the host's current color from an RGB picker.",
    fields: [
      { name: "rgb", label: "Color", type: "color", def: "#ff3b30" },
      { name: "IsColorTransparent", label: "Transparent", type: "check" },
    ],
    build: (v) => ({ IsColorTransparent: v.IsColorTransparent, RGBColorR: v.rgb.r, RGBColorG: v.rgb.g, RGBColorB: v.rgb.b }),
    submitLabel: "Apply",
  },
  {
    group: "color", key: "color-hsv", command: "SetCurrentColor",
    title: "Set color · HSV",
    desc: "Raw 32-bit HSV channels (not 0–255).",
    fields: [
      { name: "ColorIndex", label: "Color index", type: "num", int: true, def: 0 },
      { name: "IsColorTransparent", label: "Transparent", type: "check" },
      { name: "HSVColorH", label: "H", type: "num", int: true, def: 0 },
      { name: "HSVColorS", label: "S", type: "num", int: true, def: 0 },
      { name: "HSVColorV", label: "V", type: "num", int: true, def: 0 },
    ],
    build: (v) => ({ ColorIndex: v.ColorIndex, IsColorTransparent: v.IsColorTransparent, ColorSpaceKind: "HSV", HSVColorH: v.HSVColorH, HSVColorS: v.HSVColorS, HSVColorV: v.HSVColorV }),
    submitLabel: "Apply",
  },
  {
    group: "color", key: "color-hls", command: "SetCurrentColor",
    title: "Set color · HLS",
    desc: "Raw 32-bit HLS channels (not 0–255).",
    fields: [
      { name: "ColorIndex", label: "Color index", type: "num", int: true, def: 0 },
      { name: "IsColorTransparent", label: "Transparent", type: "check" },
      { name: "HLSColorH", label: "H", type: "num", int: true, def: 0 },
      { name: "HLSColorL", label: "L", type: "num", int: true, def: 0 },
      { name: "HLSColorS", label: "S", type: "num", int: true, def: 0 },
    ],
    build: (v) => ({ ColorIndex: v.ColorIndex, IsColorTransparent: v.IsColorTransparent, ColorSpaceKind: "HLS", HLSColorH: v.HLSColorH, HLSColorL: v.HLSColorL, HLSColorS: v.HLSColorS }),
    submitLabel: "Apply",
  },
  {
    group: "color", key: "colormodel", command: "SetColorSelectionModel",
    title: "Color selection model",
    desc: "Switch the color circle between HSV and HLS.",
    fields: [{ name: "ColorSpaceKind", label: "Model", type: "select", options: COLOR_SPACES, def: "HSV" }],
    build: (v) => ({ ColorSpaceKind: v.ColorSpaceKind }),
    submitLabel: "Set",
  },

  // ---- Tool parameters ----
  {
    group: "tool", key: "brush", command: "SetBrushSize",
    title: "Brush size",
    desc: "Bare numeric detail; the host rejects values ≤ 0.",
    fields: [{ name: "size", label: "Size", type: "num", def: 24, min: 0.1, step: 0.1 }],
    build: (v) => v.size,
    submitLabel: "Set",
  },
  {
    group: "tool", key: "alpha", command: "SetAlpha",
    title: "Tool opacity",
    desc: "Bare integer percent (0–100).",
    fields: [{ name: "alpha", label: "Opacity %", type: "num", int: true, def: 100, min: 0, max: 100 }],
    build: (v) => v.alpha,
    submitLabel: "Set",
  },

  // ---- Canvas navigation ----
  { group: "canvas", key: "move", title: "Move around the canvas", custom: canvasMoveCard, wide: true },
  {
    group: "canvas", key: "navigator", command: "DoNavigator",
    title: "Navigator (all actions)",
    desc: "Every navigator action, including the ones not on the quick bar above.",
    fields: [{ name: "CommandType", label: "Action", type: "select", options: NAVIGATOR, def: "ZoomIn" }],
    build: (v) => ({ CommandType: v.CommandType }),
    submitLabel: "Do",
  },
  {
    group: "canvas", key: "gesture-raw", command: "DoGesture",
    title: "Gesture (raw)",
    desc: "Full 10-element positional gesture array for experimentation.",
    fields: [
      { name: "type", label: "Type", type: "select", options: GESTURES, def: "BM" },
      { name: "p1x", label: "p1 x", type: "num", int: true, def: 1000 },
      { name: "p1y", label: "p1 y", type: "num", int: true, def: 1000 },
      { name: "p2x", label: "p2 x", type: "num", int: true, def: 1000 },
      { name: "p2y", label: "p2 y", type: "num", int: true, def: 1000 },
      { name: "factorA", label: "factor A", type: "num", def: 0 },
      { name: "factorB", label: "factor B", type: "num", def: 0 },
      { name: "flag", label: "flag", type: "check" },
      { name: "subKind", label: "sub kind", type: "num", int: true, def: 0 },
      { name: "seq", label: "seq", type: "num", int: true, def: 0, hint: "Increase by 1 per gesture in a stream." },
    ],
    build: (v) => [v.type, v.p1x, v.p1y, v.p2x, v.p2y, v.factorA, v.factorB, v.flag, v.subKind, v.seq],
    submitLabel: "Send",
  },

  // ---- Quick access ----
  {
    group: "quick", key: "qa-data", command: "GetQuickAccessData",
    title: "Get quick-access data",
    desc: "The full toolbar layout (sets → rows → columns → items).",
    build: () => null, render: renderQuickAccessData, submitLabel: "Fetch",
  },
  {
    group: "quick", key: "qa-tool", command: "DoQuickAccess",
    title: "Activate tool (UUID)",
    fields: [{ name: "ItemToolUuid", label: "Tool UUID", type: "text", placeholder: "00000000-0000-…" }],
    build: (v) => ({ ItemType: "Tool", ItemToolUuid: v.ItemToolUuid }),
    submitLabel: "Activate",
  },
  {
    group: "quick", key: "qa-cmd", command: "DoQuickAccess",
    title: "Invoke command",
    fields: [
      { name: "ItemCommandType", label: "Command type", type: "text", placeholder: "e.g. MenuCommand" },
      { name: "ItemCommandName", label: "Command name", type: "text", placeholder: "e.g. Undo" },
    ],
    build: (v) => ({ ItemType: "Command", ItemCommandType: v.ItemCommandType, ItemCommandName: v.ItemCommandName }),
    submitLabel: "Invoke",
  },
  {
    group: "quick", key: "qa-color", command: "DoQuickAccess",
    title: "Set draw color",
    fields: [
      { name: "rgb", label: "Color", type: "color", def: "#000000" },
      { name: "ItemDrawColorIsTransparent", label: "Transparent", type: "check" },
    ],
    build: (v) => ({ ItemType: "DrawColor", ItemDrawColorR: v.rgb.r, ItemDrawColorG: v.rgb.g, ItemDrawColorB: v.rgb.b, ItemDrawColorIsTransparent: v.ItemDrawColorIsTransparent }),
    submitLabel: "Apply",
  },
  {
    group: "quick", key: "qa-icon", command: "GetQuickAccessItemIcon",
    title: "Get item icon",
    desc: "Fetch the icon bitmap for one toolbar item.",
    fields: [
      { name: "ItemIDType", label: "Item type", type: "select", options: ITEM_TYPES, def: "Command" },
      { name: "ItemIDCommandType", label: "Command type", type: "text" },
      { name: "ItemIDCommandName", label: "Command name", type: "text" },
      { name: "ItemIDToolUuid", label: "Tool UUID", type: "text" },
    ],
    build: (v) => ({ ItemIDType: v.ItemIDType, ItemIDCommandType: v.ItemIDCommandType, ItemIDCommandName: v.ItemIDCommandName, ItemIDToolUuid: v.ItemIDToolUuid }),
    render: renderItemIcon,
    submitLabel: "Fetch",
  },
  {
    group: "quick", key: "qa-sync", command: "SyncQuickAccessUIState",
    title: "Sync quick-access state",
    desc: "Ask the host to (re)push quick-access UI state for a set.",
    fields: [{ name: "UpdateTargetSetIndex", label: "Set index", type: "num", int: true, def: 0 }],
    build: (v) => ({ UpdateTargetSetIndex: v.UpdateTargetSetIndex, UpdateTargetItemIDs: [] }),
    submitLabel: "Sync",
  },

  // ---- Preview ----
  { group: "preview", key: "preview", title: "Live preview", custom: previewCard, wide: true },
  {
    group: "preview", key: "prev-reset-gallery", command: "PreviewWebtoonFromServer",
    title: "Reset preview gallery",
    desc: "Host-side preview op: clears the gallery.",
    build: () => ({ Operation: "ResetGallery" }),
    submitLabel: "Reset",
  },
  {
    group: "preview", key: "prev-reset-canvas", command: "PreviewWebtoonFromServer",
    title: "Reset preview canvas",
    fields: [{ name: "CanvasIndex", label: "Canvas index", type: "num", int: true, def: 0 }],
    build: (v) => ({ Operation: "ResetCanvas", CanvasIndex: v.CanvasIndex }),
    submitLabel: "Reset",
  },

  // ---- UI state sync ----
  { group: "sync", key: "sync-circle", command: "SyncColorCircleUIState", title: "Color circle state", build: () => null, submitLabel: "Poll" },
  { group: "sync", key: "sync-mix", command: "SyncColorMixUIState", title: "Color mix state", build: () => null, submitLabel: "Poll" },
  { group: "sync", key: "sync-subview", command: "SyncSubViewUIState", title: "Sub-view state", build: () => null, submitLabel: "Poll" },
  { group: "sync", key: "sync-setting", command: "SyncSettingUIState", title: "Setting tab state", build: () => null, submitLabel: "Poll" },
  {
    group: "sync", key: "sync-gesture", command: "SyncGesturePadUIState",
    title: "Gesture pad state",
    desc: "Sends an empty navigator array and returns the host's recomputed pad state + localized hints.",
    build: () => [], submitLabel: "Poll",
  },

  // ---- Raw console ----
  { group: "raw", key: "raw", title: "Raw command console", custom: rawConsoleCard, wide: true },
];

// ---------------------------------------------------------------------------
// Custom cards
// ---------------------------------------------------------------------------
function sessionCard(cardEl) {
  cardEl.append(Object.assign(div("desc"),
    { innerHTML: "The server authenticated this session on launch (<code>Authenticate</code> consumes the QR password — one QR = one login). Health refreshes automatically." }));
  const meta = div("preview-meta");
  meta.id = "session-meta";
  meta.textContent = "checking…";
  cardEl.append(meta);
  const row = div("actions");
  const b = button("btn ghost sm", "Check now");
  b.addEventListener("click", refreshStatus);
  row.append(b);
  cardEl.append(row);
}

function canvasMoveCard(cardEl) {
  cardEl.append(Object.assign(div("desc"),
    { innerHTML: "Pan, zoom, rotate and flip the host canvas. Navigator actions are exact; the pan pad and D-pad synthesize <code>DoGesture</code> streams." }));

  // Navigator quick bar
  const navBar = div("btn-grid");
  const navButtons = [
    ["ZoomIn", "Zoom +"], ["ZoomOut", "Zoom −"], ["PixelSize", "100%"], ["Fitting", "Fit"], ["WholeSize", "Whole"],
    ["RotateLeft", "⟲ Rotate"], ["RotateRight", "⟳ Rotate"], ["Rotate0", "Rot 0°"],
    ["ReverseHorz", "Flip H"], ["ReverseVert", "Flip V"], ["ResetPosition", "Reset pos"],
  ];
  navButtons.forEach(([cmd, label]) => {
    const b = button("btn ghost sm", label);
    b.addEventListener("click", () => request("DoNavigator", { CommandType: cmd }).catch(() => {}));
    navBar.append(b);
  });
  cardEl.append(navBar);

  // Pan settings
  const settings = div("fields");
  settings.style.margin = "14px 0";
  settings.append(renderField({ name: "family", label: "Pan gesture", type: "select", options: ["Move", "Swipe"], def: "Move" }));
  settings.append(renderField({ name: "step", label: "D-pad step (canvas px)", type: "num", int: true, def: 150 }));
  settings.append(renderField({ name: "scale", label: "Pad sensitivity", type: "num", def: 2, step: 0.5 }));
  settings.append(renderField({ name: "invert", label: "Invert direction", type: "check" }));
  cardEl.append(settings);
  const cfg = () => ({
    family: settings.querySelector('[name="family"]').value,
    step: Number(settings.querySelector('[name="step"]').value) || 150,
    scale: Number(settings.querySelector('[name="scale"]').value) || 1,
    sign: settings.querySelector('[name="invert"]').checked ? -1 : 1,
  });

  const wrap = div("move-wrap");

  // D-pad
  const dpadCol = div();
  dpadCol.append(Object.assign(div("hint"), { textContent: "Nudge" }));
  const dpad = div("dpad");
  const ANCHOR = { x: 3000, y: 3000 };
  async function pan(dx, dy) {
    const c = cfg();
    const fam = PAN_FAMILIES[c.family];
    const to = { x: ANCHOR.x + dx * c.step * c.scale * c.sign, y: ANCHOR.y + dy * c.step * c.scale * c.sign };
    try {
      await sendGesture(fam.begin, ANCHOR, ANCHOR);
      await sendGesture(fam.mid, to, ANCHOR);
      await sendGesture(fam.end, to, to);
    } catch { /* logged in request() */ }
  }
  const cells = [
    null, ["↑", 0, -1], null,
    ["←", -1, 0], ["•", 0, 0], ["→", 1, 0],
    null, ["↓", 0, 1], null,
  ];
  cells.forEach((cell) => {
    if (!cell) { dpad.append(Object.assign(button("btn ghost spacer"), {})); return; }
    const [label, dx, dy] = cell;
    const b = button("btn ghost", label);
    if (dx === 0 && dy === 0) b.addEventListener("click", () => request("DoNavigator", { CommandType: "ResetPosition" }).catch(() => {}));
    else b.addEventListener("click", () => pan(dx, dy));
    dpad.append(b);
  });
  dpadCol.append(dpad);
  wrap.append(dpadCol);

  // Drag pad
  const padCol = div();
  padCol.append(Object.assign(div("hint"), { textContent: "Drag to pan" }));
  const pad = div("padzone");
  pad.append(span(null, "drag here"));
  const nub = div("nub");
  pad.append(nub);
  padCol.append(pad);
  wrap.append(padCol);
  cardEl.append(wrap);

  // Drag-pad pointer handling (throttled Move sends).
  let dragging = false, last = 0, start = null;
  function padPoint(ev) {
    const r = pad.getBoundingClientRect();
    return { px: ev.clientX - r.left, py: ev.clientY - r.top };
  }
  pad.addEventListener("pointerdown", async (ev) => {
    dragging = true;
    pad.setPointerCapture(ev.pointerId);
    pad.classList.add("active");
    start = padPoint(ev);
    nub.style.display = "block";
    nub.style.left = start.px + "px";
    nub.style.top = start.py + "px";
    const fam = PAN_FAMILIES[cfg().family];
    await sendGesture(fam.begin, ANCHOR, ANCHOR).catch(() => {});
  });
  pad.addEventListener("pointermove", (ev) => {
    if (!dragging) return;
    const p = padPoint(ev);
    nub.style.left = p.px + "px";
    nub.style.top = p.py + "px";
    const t = performance.now();
    if (t - last < 55) return; // throttle to ~18 fps
    last = t;
    const c = cfg();
    const to = { x: ANCHOR.x + (p.px - start.px) * c.scale * c.sign, y: ANCHOR.y + (p.py - start.py) * c.scale * c.sign };
    const fam = PAN_FAMILIES[c.family];
    sendGesture(fam.mid, to, ANCHOR).catch(() => {});
  });
  function endDrag(ev) {
    if (!dragging) return;
    dragging = false;
    pad.classList.remove("active");
    nub.style.display = "none";
    const p = padPoint(ev);
    const c = cfg();
    const to = { x: ANCHOR.x + (p.px - start.px) * c.scale * c.sign, y: ANCHOR.y + (p.py - start.py) * c.scale * c.sign };
    const fam = PAN_FAMILIES[c.family];
    sendGesture(fam.end, to, to).catch(() => {});
  }
  pad.addEventListener("pointerup", endDrag);
  pad.addEventListener("pointercancel", endDrag);
}

function sendGesture(type, p1, p2, factorA = 0, factorB = 0, flag = false, subKind = 0) {
  const detail = [type, p1.x | 0, p1.y | 0, p2.x | 0, p2.y | 0, factorA, factorB, flag, subKind, gestureSeq++];
  return request("DoGesture", detail);
}

// CSP refuses a single preview block larger than ~2.62 MP (it times out), so
// large canvases must be fetched as multiple sub-blocks and stitched. We tile
// under a configurable pixel budget and cap any single block side to a value CSP
// is known to accept.
const PREVIEW_BLOCK_BUDGET = 2400000; // px per block (safely under CSP's ~2.62 MP)
const PREVIEW_MAX_BLOCK_SIDE = 2000; // px; keep each block dimension within known-good range
// Browser <canvas> backing-store limits — only downscale (losing resolution) when
// the canvas genuinely can't be held at full size. 32767 is the cross-browser max
// dimension (Firefox; Chrome/Edge allow more), so normal webtoons stitch 1:1.
const PREVIEW_MAX_CANVAS_SIDE = 32767;
const PREVIEW_MAX_CANVAS_AREA = 100000000; // px backing-store cap (~400 MB RGBA)

// planTiles splits a W×H canvas into blocks whose area ≤ budget and whose sides ≤
// PREVIEW_MAX_BLOCK_SIDE, row-major from the top-left.
function planTiles(W, H, budget) {
  budget = Math.max(50000, Math.floor(budget) || PREVIEW_BLOCK_BUDGET);
  const tileW = Math.max(1, Math.min(W, PREVIEW_MAX_BLOCK_SIDE, budget));
  const tileH = Math.max(1, Math.min(H, PREVIEW_MAX_BLOCK_SIDE, Math.floor(budget / tileW)));
  const tiles = [];
  for (let top = 0; top < H; top += tileH) {
    const bottom = Math.min(H, top + tileH);
    for (let left = 0; left < W; left += tileW) {
      tiles.push({ left, top, right: Math.min(W, left + tileW), bottom });
    }
  }
  return tiles;
}

function previewCard(cardEl) {
  cardEl.append(Object.assign(div("desc"),
    { innerHTML: "Live preview comes from CSP's <b>webtoon preview</b>. Scan the gallery, pick a canvas, then load. <b>Load full canvas</b> tiles the request into blocks (CSP rejects single blocks over ~2.6 MP) and stitches them. Requires a document with webtoon preview available." }));

  const state = { galleryId: 1, sizes: [], canvasW: 0, canvasH: 0, view: { s: 1, tx: 0, ty: 0 }, drag: null, lastLoad: null, cancel: false };

  const grid = div("preview-grid");
  const controls = div("preview-controls");

  const mk = (f) => { const w = renderField(f); controls.append(w); return w.querySelector("[name]"); };
  const inMax = mk({ name: "max", label: "Gallery max length", type: "num", int: true, def: 4096 });
  const scanRow = div("actions");
  const scanBtn = button("btn", "Scan gallery");
  scanRow.append(scanBtn);
  controls.append(scanRow);

  const inGallery = mk({ name: "gid", label: "Gallery ID", type: "num", int: true, def: 1 });
  const selCanvas = mk({ name: "canvas", label: "Canvas", type: "select", options: [["0", "canvas 0"]], def: "0" });
  const inBudget = mk({ name: "budget", label: "Max block pixels", type: "num", int: true, def: PREVIEW_BLOCK_BUDGET, hint: "Lower this if blocks still time out." });
  const inConc = mk({ name: "conc", label: "Parallel blocks", type: "num", int: true, def: 4, min: 1, max: 8, hint: "Concurrent block requests (lower if CSP struggles)." });

  const fullRow = div("actions");
  const fullBtn = button("btn", "Load full canvas");
  const cancelBtn = button("btn ghost sm", "Cancel");
  cancelBtn.style.display = "none";
  const saveBtn = button("btn accent2", "Save PNG");
  saveBtn.disabled = true;
  fullRow.append(fullBtn, cancelBtn, saveBtn);
  controls.append(fullRow);

  const progress = div("preview-meta", "");
  controls.append(progress);

  // Single-block (manual) loader — advanced.
  const inBlockIdx = mk({ name: "bidx", label: "Block index", type: "num", int: true, def: 0 });
  const inL = mk({ name: "left", label: "Left", type: "num", int: true, def: 0 });
  const inT = mk({ name: "top", label: "Top", type: "num", int: true, def: 0 });
  const inR = mk({ name: "right", label: "Right", type: "num", int: true, def: 690 });
  const inB = mk({ name: "bottom", label: "Bottom", type: "num", int: true, def: 1024 });
  const blockRow = div("actions");
  const blockBtn = button("btn ghost sm", "Load single block");
  blockRow.append(blockBtn);
  controls.append(blockRow);

  const auto = div("field row");
  const autoChk = el("input"); autoChk.type = "checkbox"; autoChk.name = "auto";
  auto.append(el("label", null, "Auto-refresh"), autoChk);
  controls.append(auto);
  const inEvery = mk({ name: "every", label: "Interval (s)", type: "num", def: 3, min: 0.5, step: 0.5 });

  grid.append(controls);

  // Stage
  const stage = div("preview-stage");
  const canvas = el("canvas");
  canvas.style.display = "none";
  const empty = div("preview-empty", "No preview loaded yet. Scan the gallery, pick a canvas, then Load full canvas.");
  stage.append(canvas, empty);
  const stageWrap = div();
  stageWrap.append(stage);
  const meta = div("preview-meta", "");
  stageWrap.append(meta);
  grid.append(stageWrap);
  cardEl.append(grid);

  function applyView() {
    canvas.style.transform = `translate(${state.view.tx}px, ${state.view.ty}px) scale(${state.view.s})`;
    canvas.style.transformOrigin = "center center";
  }
  function fitToStage(displayW) {
    const fit = Math.min(1, (stage.clientWidth - 24) / displayW);
    state.view = { s: fit > 0 ? fit : 1, tx: 0, ty: 0 };
    applyView();
  }
  function showCanvas() { canvas.style.display = "block"; empty.style.display = "none"; saveBtn.disabled = false; }
  function setBusy(busy) {
    fullBtn.disabled = busy; blockBtn.disabled = busy; scanBtn.disabled = busy;
    cancelBtn.style.display = busy ? "" : "none";
  }

  // fetchBlockImage requests one preview block (PNG) and returns it as an Image.
  async function fetchBlockImage(block, blockIndex) {
    const params = new URLSearchParams({
      format: "png",
      gallery_identification_number: inGallery.value || "1",
      canvas_index: selCanvas.value || "0",
      block_index: String(blockIndex),
      block_left: String(block.left),
      block_top: String(block.top),
      block_right: String(block.right),
      block_bottom: String(block.bottom),
    });
    const res = await fetch("/preview?" + params.toString());
    if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
    return blobToImage(await res.blob());
  }

  async function scan() {
    setBusy(true);
    try {
      const sync = await request("PreviewWebtoonFromClient", { Operation: "SyncPreview" });
      const gal = await request("PreviewWebtoonFromClient", { Operation: "UpdateGallery", MaxLength: Number(inMax.value) || 4096 });
      const d = gal.detail || {};
      const sd = sync.detail || {};
      const gid = d.GalleryIdentificationNumber ?? sd.GalleryIdentificationNumber ?? state.galleryId;
      state.galleryId = gid;
      inGallery.value = gid;
      const sizes = d.CanvasSizeArray || [];
      state.sizes = sizes;
      selCanvas.innerHTML = "";
      const count = d.CanvasCount || sizes.length || 0;
      if (count === 0) {
        meta.textContent = "No canvases reported — open a document and ensure webtoon preview is available in CSP.";
      } else {
        for (let i = 0; i < count; i++) {
          const sz = sizes[i] || {};
          const opt = el("option", null, `canvas ${i}${sz.CanvasWidth ? ` — ${sz.CanvasWidth}×${sz.CanvasHeight}` : ""}`);
          opt.value = String(i);
          selCanvas.append(opt);
        }
        meta.textContent = `gallery ${gid} · ${count} canvas(es)`;
        await applyCanvasBounds();
      }
    } catch (e) {
      meta.textContent = "scan failed: " + e.message;
    } finally {
      setBusy(false);
    }
  }

  async function applyCanvasBounds() {
    const i = Number(selCanvas.value) || 0;
    let sz = state.sizes[i];
    // Confirm/refresh the size for this canvas.
    try {
      const r = await request("PreviewWebtoonFromClient", { Operation: "UpdateCanvas", CanvasIndex: i });
      if (r.detail && r.detail.CanvasWidth) sz = { CanvasWidth: r.detail.CanvasWidth, CanvasHeight: r.detail.CanvasHeight };
    } catch { /* ignore */ }
    if (sz && sz.CanvasWidth) {
      state.canvasW = sz.CanvasWidth;
      state.canvasH = sz.CanvasHeight;
      inL.value = 0; inT.value = 0; inR.value = sz.CanvasWidth; inB.value = sz.CanvasHeight;
    }
  }

  // loadFull tiles the whole selected canvas into blocks and stitches them.
  async function loadFull() {
    const W = state.canvasW, H = state.canvasH;
    if (!W || !H) { meta.textContent = "scan and pick a canvas first."; return; }

    const budget = Number(inBudget.value) || PREVIEW_BLOCK_BUDGET;
    const tiles = planTiles(W, H, budget);

    // Downscale the stitched output if the canvas is too large for the browser.
    const scale = Math.min(1, PREVIEW_MAX_CANVAS_SIDE / W, PREVIEW_MAX_CANVAS_SIDE / H,
      Math.sqrt(PREVIEW_MAX_CANVAS_AREA / (W * H)));
    const dispW = Math.max(1, Math.round(W * scale));
    const dispH = Math.max(1, Math.round(H * scale));
    canvas.width = dispW;
    canvas.height = dispH;
    const ctx = canvas.getContext("2d");
    // No interpolation across block edges — avoids faint seam lines when downscaling
    // (and is a no-op at scale=1, where blocks are copied 1:1).
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, dispW, dispH);
    showCanvas();
    fitToStage(dispW);

    state.lastLoad = loadFull;
    state.cancel = false;
    setBusy(true);
    const concurrency = Math.max(1, Math.min(8, Number(inConc.value) || 4));
    addLog("out", "→ preview (tiled)", { canvas: `${W}×${H}`, tiles: tiles.length, budget, concurrency, scale: scale.toFixed(3) });
    let next = 0, done = 0, failed = 0;
    const total = tiles.length;
    const t0 = performance.now();
    // Worker pool: each worker pulls the next tile index and draws it into its own
    // region, so completion order doesn't matter (drawImage is synchronous and the
    // 2D context is single-threaded, so the concurrent draws don't race).
    async function worker() {
      while (!state.cancel) {
        const i = next++;
        if (i >= total) return;
        const t = tiles[i];
        // Snap each block's destination edges to whole pixels so neighbours share
        // an exact boundary — no sub-pixel gap (seam line) even at fractional scale.
        const dx = Math.round(t.left * scale), dy = Math.round(t.top * scale);
        const dw = Math.round(t.right * scale) - dx, dh = Math.round(t.bottom * scale) - dy;
        try {
          const img = await fetchBlockImage(t, i);
          ctx.drawImage(img, 0, 0, img.naturalWidth, img.naturalHeight, dx, dy, dw, dh);
          done++;
        } catch (e) {
          failed++;
          addLog("err", "✗ preview block", `block ${i} [${t.left},${t.top},${t.right},${t.bottom}]: ${e.message}`);
          ctx.fillStyle = "rgba(248,113,113,0.18)"; // mark the failed region
          ctx.fillRect(dx, dy, dw, dh);
        }
        const seen = done + failed;
        progress.textContent = `loaded ${seen}/${total} (${Math.round((seen / total) * 100)}%)…`;
      }
    }
    await Promise.all(Array.from({ length: concurrency }, () => worker()));
    if (state.cancel) progress.textContent = `cancelled at ${done + failed}/${total} blocks`;
    setBusy(false);
    const secs = ((performance.now() - t0) / 1000).toFixed(1);
    const scaleNote = scale < 1
      ? ` · downscaled to ${Math.round(scale * 100)}% (${dispW}×${dispH}, save is this size) — exceeds browser canvas limits`
      : " · full resolution";
    const failNote = failed ? ` · ${failed} block(s) failed — try a lower Max block pixels` : "";
    meta.textContent = `${W}×${H}px · ${tiles.length} blocks · ${secs}s${scaleNote}${failNote}`;
    progress.textContent = state.cancel ? progress.textContent : `done: ${done}/${tiles.length} blocks`;
    addLog(failed ? "err" : "ok", "← preview (tiled)", `${done}/${tiles.length} blocks, ${secs}s`);
  }

  // loadBlock loads exactly the manual bounds as a single block.
  async function loadBlock() {
    state.lastLoad = loadBlock;
    setBusy(true);
    progress.textContent = "";
    try {
      const block = {
        left: Number(inL.value) || 0, top: Number(inT.value) || 0,
        right: Number(inR.value) || 0, bottom: Number(inB.value) || 0,
      };
      addLog("out", "→ preview", block);
      const img = await fetchBlockImage(block, Number(inBlockIdx.value) || 0);
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;
      canvas.getContext("2d").drawImage(img, 0, 0);
      showCanvas();
      fitToStage(img.naturalWidth);
      meta.textContent = `${img.naturalWidth}×${img.naturalHeight}px (single block)`;
      addLog("ok", "← preview", `${img.naturalWidth}×${img.naturalHeight}`);
    } catch (e) {
      meta.textContent = "load failed: " + e.message;
      addLog("err", "✗ preview", e.message);
    } finally {
      setBusy(false);
    }
  }

  function save() {
    if (canvas.style.display === "none") return;
    canvas.toBlob((blob) => {
      if (!blob) return;
      const a = el("a");
      a.href = URL.createObjectURL(blob);
      a.download = `clipstudio-preview-c${selCanvas.value || 0}-${state.canvasW}x${state.canvasH}.png`;
      document.body.append(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 4000);
    }, "image/png");
  }

  // Client-side zoom/pan of the loaded preview.
  stage.addEventListener("wheel", (ev) => {
    if (canvas.style.display === "none") return;
    ev.preventDefault();
    const f = ev.deltaY < 0 ? 1.1 : 1 / 1.1;
    state.view.s = Math.max(0.02, Math.min(20, state.view.s * f));
    applyView();
  }, { passive: false });
  stage.addEventListener("pointerdown", (ev) => {
    if (canvas.style.display === "none") return;
    state.drag = { x: ev.clientX, y: ev.clientY, tx: state.view.tx, ty: state.view.ty };
    stage.setPointerCapture(ev.pointerId);
  });
  stage.addEventListener("pointermove", (ev) => {
    if (!state.drag) return;
    state.view.tx = state.drag.tx + (ev.clientX - state.drag.x);
    state.view.ty = state.drag.ty + (ev.clientY - state.drag.y);
    applyView();
  });
  const endStageDrag = () => { state.drag = null; };
  stage.addEventListener("pointerup", endStageDrag);
  stage.addEventListener("pointercancel", endStageDrag);

  // Auto-refresh re-runs whichever load mode was used last.
  let timer = null;
  function syncTimer() {
    if (timer) { clearInterval(timer); timer = null; }
    if (autoChk.checked) {
      const every = Math.max(500, (Number(inEvery.value) || 3) * 1000);
      timer = setInterval(() => { if (state.lastLoad && !fullBtn.disabled) state.lastLoad(); }, every);
    }
  }
  autoChk.addEventListener("change", syncTimer);
  inEvery.addEventListener("change", syncTimer);
  previewAutoReload = () => { if (autoChk.checked && state.lastLoad && !fullBtn.disabled) state.lastLoad(); };

  scanBtn.addEventListener("click", scan);
  selCanvas.addEventListener("change", applyCanvasBounds);
  fullBtn.addEventListener("click", loadFull);
  blockBtn.addEventListener("click", loadBlock);
  cancelBtn.addEventListener("click", () => { state.cancel = true; });
  saveBtn.addEventListener("click", save);
}

let previewAutoReload = () => {};

function blobToImage(blob) {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => { resolve(img); setTimeout(() => URL.revokeObjectURL(url), 4000); };
    img.onerror = () => { URL.revokeObjectURL(url); reject(new Error("image decode failed")); };
    img.src = url;
  });
}

function rawConsoleCard(cardEl) {
  cardEl.append(div("desc", "Send any command with a hand-written JSON detail (or a bare scalar / array)."));
  const form = div("fields");
  const cmdWrap = renderField({ name: "command", label: "Command", type: "select", options: ALL_COMMANDS, def: "GetModifyKeyString" });
  form.append(cmdWrap);
  const detailWrap = div("field");
  detailWrap.append(el("label", null, "Detail (JSON, or leave empty)"));
  const ta = el("textarea");
  ta.name = "detail";
  ta.rows = 5;
  ta.placeholder = '{"CommandType":"ZoomIn"}   ·   42   ·   ["BM",1000,1000,1000,1000,0,0,false,0,0]';
  detailWrap.append(ta);
  form.append(detailWrap);
  cardEl.append(form);

  const actions = div("actions");
  const send = button("btn", "Send");
  actions.append(send);
  cardEl.append(actions);
  const result = div("result");
  cardEl.append(result);

  send.addEventListener("click", async () => {
    const command = cmdWrap.querySelector("[name]").value;
    let detail = null;
    const raw = ta.value.trim();
    if (raw !== "") {
      try { detail = JSON.parse(raw); }
      catch (e) { showResult(result, "err", "detail is not valid JSON: " + esc(e.message)); return; }
    }
    send.disabled = true;
    try {
      const resp = await request(command, detail);
      showResponse(result, resp);
    } catch (e) {
      showResult(result, "err", esc(e.message));
    } finally {
      send.disabled = false;
    }
  });
}

// ---------------------------------------------------------------------------
// Special response renderers
// ---------------------------------------------------------------------------
function renderQuickAccessData(resp, resultEl) {
  const d = resp.detail;
  if (!d || !Array.isArray(d.ToolBarData)) { showResponse(resultEl, resp); return; }
  resultEl.innerHTML = "";
  resultEl.classList.add("show");
  resultEl.append(div("rtag ok", "success"));
  const info = d.ToolBarViewInfo || {};
  resultEl.append(Object.assign(div("preview-meta"), { textContent: `current set ${info.ViewInfoCurrentSetIndex ?? "?"} · remote set ${info.ViewInfoRemoteControllerSetIndex ?? "?"}` }));
  d.ToolBarData.forEach((set) => {
    const box = div("qa-set");
    box.append(div("qa-name", set.ItemSetName || set.ItemSetUuid || "(unnamed set)"));
    const items = div("qa-items");
    const flat = [];
    (set.ItemSet || []).forEach((rows) => (rows || []).forEach((cols) => (cols || []).forEach((it) => flat.push(it))));
    if (flat.length === 0) items.append(span("qa-chip", "(empty)"));
    flat.forEach((it) => {
      const chip = span("qa-chip" + (it.ItemIsChecked ? " checked" : "") + (it.ItemIsEnabled ? "" : " disabled"),
        it.ItemShowName || it.ItemIDCommandName || it.ItemIDToolUuid || it.ItemIDType || "?");
      items.append(chip);
    });
    box.append(items);
    resultEl.append(box);
  });
}

function renderItemIcon(resp, resultEl) {
  const d = resp.detail || {};
  if (resp.type === "error" || !d.ItemIconPixelDataRGBAHexString || !d.ItemIconWidth) {
    showResponse(resultEl, resp);
    return;
  }
  resultEl.innerHTML = "";
  resultEl.classList.add("show");
  resultEl.append(div("rtag ok", "success"));
  const w = d.ItemIconWidth, h = d.ItemIconHeight;
  const hex = d.ItemIconPixelDataRGBAHexString;
  const bytes = new Uint8Array(w * h * 4);
  for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
  const cv = el("canvas", "iconcanvas");
  cv.width = w; cv.height = h;
  cv.style.width = Math.min(96, w * 2) + "px";
  cv.getContext("2d").putImageData(new ImageData(new Uint8ClampedArray(bytes), w, h), 0, 0);
  resultEl.append(Object.assign(div("preview-meta"), { textContent: `${w}×${h}${d.ItemIsUserIcon ? " · user icon" : ""}` }));
  resultEl.append(cv);
}

// ---------------------------------------------------------------------------
// Card factory + layout
// ---------------------------------------------------------------------------
function makeCard(card) {
  const node = div("card" + (card.wide ? " wide" : ""));
  node.id = "card-" + card.key;
  const h = el("h3");
  h.append(document.createTextNode(card.title));
  if (card.command) h.append(span("cmd-tag", card.command));
  node.append(h);

  if (card.custom) { if (card.desc) node.append(div("desc", card.desc)); card.custom(node); return node; }

  if (card.desc) node.append(div("desc", card.desc));
  const form = el("form", "fields");
  (card.fields || []).forEach((f) => form.append(renderField(f)));
  const actions = div("actions");
  const btn = button("btn", card.submitLabel || "Send");
  actions.append(btn);
  const result = div("result");
  const submit = (e) => { e.preventDefault(); runCard(card, form, result, btn); };
  btn.addEventListener("click", submit);
  form.addEventListener("submit", submit);
  node.append(form, actions, result);
  return node;
}

function buildUI() {
  const main = $("#main");
  const nav = $("#sidenav");
  GROUPS.forEach((g) => {
    const sec = el("section", "group");
    sec.id = "sec-" + g.id;
    sec.append(el("h2", null, g.title));
    const cards = div("cards");
    CARDS.filter((c) => c.group === g.id).forEach((c) => cards.append(makeCard(c)));
    sec.append(cards);
    main.append(sec);

    const a = el("a", null, g.title);
    a.href = "#sec-" + g.id;
    a.dataset.target = "sec-" + g.id;
    nav.append(a);
  });

  // Scroll-spy for the side nav.
  const links = [...nav.querySelectorAll("a")];
  const obs = new IntersectionObserver((entries) => {
    entries.forEach((en) => {
      if (en.isIntersecting) {
        links.forEach((l) => l.classList.toggle("active", l.dataset.target === en.target.id));
      }
    });
  }, { rootMargin: "-40% 0px -55% 0px" });
  GROUPS.forEach((g) => obs.observe($("#sec-" + g.id)));
}

// ---------------------------------------------------------------------------
// Status + events
// ---------------------------------------------------------------------------
function setPill(kind, text) {
  const pill = $("#status-pill");
  pill.className = "pill " + kind;
  $("#status-text").textContent = text;
}
async function refreshStatus() {
  try {
    const s = await fetchStatus();
    setPill(s.alive ? "ok" : "bad", s.alive ? "connected" : "disconnected");
    $("#status-addr").textContent = s.remoteAddr || "—";
    $("#status-spec").textContent = s.specVersion || "—";
    $("#status-qa").textContent = s.quickAccessAvailable ? "yes" : "no";
    const sm = $("#session-meta");
    if (sm) sm.textContent = `${s.alive ? "alive" : "down"} · host ${s.remoteAddr || "?"} · spec ${s.specVersion || "?"} · quick-access ${s.quickAccessAvailable ? "yes" : "no"}`;
  } catch (e) {
    setPill("bad", "no bridge");
    const sm = $("#session-meta");
    if (sm) sm.textContent = "cannot reach bridge: " + e.message;
  }
}

function startEvents() {
  let es;
  try { es = new EventSource("/api/events"); }
  catch { return; }
  es.onmessage = (ev) => {
    let m;
    try { m = JSON.parse(ev.data); } catch { return; }
    handlePush(m);
  };
  es.onerror = () => { /* EventSource auto-reconnects via the server's retry hint */ };
}

function handlePush(m) {
  addLog("in", "⇐ " + m.event, m.data);
  if (m.event === "PreNotifyOfServerShutdown") {
    showBanner(`Host announced shutdown${m.data && m.data.IsManualShutdown ? " (manual)" : ""}.`, "warn");
  }
  if (m.event === "PreviewWebtoonFromServer") {
    previewAutoReload();
  }
}

let bannerTimer = null;
function showBanner(text, kind) {
  let b = $("#banner");
  if (!b) {
    b = div("");
    b.id = "banner";
    b.style.cssText = "margin:0 0 18px;padding:12px 16px;border-radius:10px;font-weight:600;";
    $("#main").prepend(b);
  }
  b.style.background = kind === "warn" ? "rgba(251,191,36,0.15)" : "rgba(58,160,255,0.15)";
  b.style.color = kind === "warn" ? "#fbbf24" : "#3aa0ff";
  b.textContent = text;
  if (bannerTimer) clearTimeout(bannerTimer);
  bannerTimer = setTimeout(() => b.remove(), 12000);
}

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------
function wireChrome() {
  $("#activity-clear").addEventListener("click", () => (logEl().innerHTML = ""));
  const toggle = $("#activity-toggle");
  toggle.addEventListener("click", () => $("#activity").classList.toggle("open"));
}

document.addEventListener("DOMContentLoaded", () => {
  buildUI();
  wireChrome();
  refreshStatus();
  setInterval(refreshStatus, 2500);
  startEvents();
});
