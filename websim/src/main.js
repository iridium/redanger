// Page side: the device frame, input, screen and audio. The simulator runs in worker.js.

// Hotspots, in the photo's pixel coordinates.
// k: AWT key code sent to the simulator; m: modifiers (2 = ctrl); ch: key char
const W = 480,
  H = 364;
const HOTSPOTS = [
  { label: "Menu (F7)", x: 44, y: 180, w: 36, h: 32, k: 118 },
  { label: "Jump (F8)", x: 42, y: 272, w: 38, h: 33, k: 119 },
  { label: "D-pad up", x: 22, y: 220, w: 38, h: 14, k: 38 },
  { label: "D-pad down", x: 22, y: 256, w: 38, h: 14, k: 40 },
  { label: "D-pad left", x: 8, y: 228, w: 18, h: 34, k: 37 },
  { label: "D-pad right", x: 56, y: 228, w: 18, h: 34, k: 39 },
  { label: "Cancel (Numpad 9)", x: 402, y: 175, w: 36, h: 32, k: 105 },
  { label: "End (Numpad 6)", x: 400, y: 208, w: 18, h: 20, k: 102 },
  { label: "Call (Numpad 4)", x: 400, y: 264, w: 18, h: 20, k: 100 },
  { label: "Done / back (F10)", x: 402, y: 284, w: 38, h: 26, k: 121 },
  // trackball: ball centered at (434, 243), r ~10; edges of the ring around it roll it
  { label: "Trackball up (F11)", x: 424, y: 224, w: 20, h: 9, k: 122, round: true },
  { label: "Trackball down (F12)", x: 424, y: 253, w: 20, h: 9, k: 123, round: true },
  { label: "Trackball left", x: 414, y: 233, w: 10, h: 20, k: 37, m: 2, round: true },
  { label: "Trackball right", x: 444, y: 233, w: 9, h: 20, k: 39, m: 2, round: true },
  { label: "Trackball click (F9)", x: 424, y: 233, w: 20, h: 20, k: 120, round: true },
];
// Keyboard: [label, center x, awtCode, char, width]; centers measured from the photo's key
// highlights (pitch ~26.5 px), one row per y.
const letter = (c, x) => [c, x, c.toUpperCase().charCodeAt(0), c];
// Keys a PC keyboard doesn't have are sent as Hiptop key codes (DangerOS's keymap: 80 is
// "," / "?", 81 is "@" / "~", 82 is "." / "/"), marked by a negative code.
const raw = (code) => -code;
const KEYROWS = [
  {
    y: 228,
    keys: "1234567890"
      .split("")
      .map((c, i) => [
        c,
        [120, 147, 174, 200.5, 227, 254, 281, 308, 335, 362][i],
        c.charCodeAt(0),
        c,
      ]),
  },
  {
    y: 252,
    keys: "qwertyuiop"
      .split("")
      .map((c, i) => letter(c, [119, 145, 172, 198, 224, 250, 276, 303, 329, 355][i])),
  },
  {
    y: 278,
    keys: [
      ..."asdfghjkl"
        .split("")
        .map((c, i) => letter(c, [129, 156, 181, 208, 234, 260, 287, 313, 340][i])),
      ["Del", 366, 8, "\b"],
    ],
  },
  {
    y: 304,
    keys: [
      ["Sym (F4)", 116, 115],
      ..."zxcvbnm".split("").map((c, i) => letter(c, [142, 169, 195, 221, 248, 274, 301][i])),
      ["?", 328, raw(80)],
      ["Enter", 360, 10, "\n", 26],
    ],
  },
  {
    y: 330,
    keys: [
      ["Shift", 135, 16],
      ["Alt", 162, 18],
      ["@", 188, raw(81)],
      ["Space", 241, 32, " ", 70],
      ["* /", 293, raw(82)],
      ["Alt", 320, 18],
      ["Shift", 347, 16],
    ],
  },
];
for (const r of KEYROWS)
  for (const [label, cx, k, ch, w = 22] of r.keys)
    HOTSPOTS.push({ label, x: cx - w / 2, y: r.y - 9.5, w, h: 19, k, ch });
// Edge buttons, barely visible front-on: the shoulder triggers on the body's top corners behind
// the screen, the volume rocker on the bottom edge under the left Shift/Alt, power on the bottom
// edge under the right Shift. No AWT key for most of them, so all go as Hiptop button codes
// (danger.app.Event).
HOTSPOTS.push(
  { label: "Left shoulder", x: 61, y: 146, w: 44, h: 18, k: raw(113) },
  { label: "Right shoulder", x: 375, y: 146, w: 44, h: 18, k: raw(114) },
  { label: "Volume up", x: 96, y: 351, w: 30, h: 13, k: raw(115) },
  { label: "Volume down", x: 126, y: 351, w: 30, h: 13, k: raw(116) },
  { label: "Power (Esc)", x: 352, y: 351, w: 40, h: 13, k: raw(100) },
);

const device = document.getElementById("device");
const pct = (v, of) => `${(v / of) * 100}%`;
for (const h of HOTSPOTS) {
  const b = document.createElement("button");
  b.className = "hot" + (h.round ? " round" : "");
  b.title = b.ariaLabel = h.label;
  Object.assign(b.style, {
    left: pct(h.x, W),
    top: pct(h.y, H),
    width: pct(h.w, W),
    height: pct(h.h, H),
  });
  const ch = h.ch ? h.ch.charCodeAt(0) : 0xffff;
  b.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    b.classList.add("down");
    send(401, h.k, ch, h.m || 0);
  });
  const up = () => {
    if (b.classList.contains("down")) {
      b.classList.remove("down");
      send(402, h.k, ch, h.m || 0);
    }
  };
  b.addEventListener("pointerup", up);
  b.addEventListener("pointerleave", up);
  b.addEventListener("keydown", (e) => {
    if (e.key !== " " && e.key !== "Enter") return;
    e.preventDefault();
    e.stopPropagation();
    send(401, h.k, ch, h.m || 0);
    send(402, h.k, ch, h.m || 0);
  });
  device.append(b);
}

const canvas = document.getElementById("screen");
const ctx = canvas.getContext("2d");
const logEl = document.getElementById("log");
const params = new URLSearchParams(location.search);
const flag = (name) => params.has(name) && !/^(0|false|no)$/i.test(params.get(name));
// ?autostart boots straight into an app instead of the launcher: bare, "true" or "classof09"
// mean Class of '09; anything else is a bundle name (e.g. ?autostart=apps.kbrowser).
const APP_ALIASES = {
  "": "com.redanger.classof09",
  true: "com.redanger.classof09",
  classof09: "com.redanger.classof09",
};
const autostart = params.get("autostart");
const autoboot = flag("autostart") ? (APP_ALIASES[autostart] ?? autostart) : null;

// The VM starts at once, or with ?defer only on the first click or key press: until then the
// page has loaded nothing but itself and the device photo (for embedding in other pages).
let worker = null;
const powerOn = document.getElementById("power-on");
// Compact mode shows the console on the screen while booting (index.html), until the first frame.
const bootLog = document.getElementById("bootlog");
function start() {
  if (worker) return;
  powerOn.hidden = true;
  bootLog.hidden = false;
  worker = new Worker(new URL("./worker.js", import.meta.url), { type: "module" });
  worker.onmessage = onWorkerMessage;
  worker.postMessage({ type: "init", vendor: new URL("vendor/", document.baseURI).href, autoboot });
}
if (flag("defer")) {
  powerOn.hidden = false;
  device.addEventListener("pointerdown", start);
  document.addEventListener("keydown", start, { once: true });
} else {
  start();
}
// AWT key events (401 pressed, 402 released); negative codes are raw Hiptop keys (501/502, see raw()).
const send = (id, code, ch = 0xffff, m = 0) =>
  worker?.postMessage(
    code < 0
      ? { type: "key", id: id + 100, code: -code, ch, mods: m }
      : { type: "key", id, code, ch, mods: m },
  );

let image = null;
function onWorkerMessage(e) {
  const m = e.data;
  if (m.type === "frame") {
    bootLog.hidden = true;
    if (!image || image.width !== m.width || image.height !== m.height) {
      canvas.width = m.width;
      canvas.height = m.height;
      image = ctx.createImageData(m.width, m.height);
    }
    const d = image.data,
      p = m.pixels;
    for (let i = 0, j = 0; i < p.length; i++, j += 4) {
      const v = p[i];
      d[j] = (v >> 16) & 255;
      d[j + 1] = (v >> 8) & 255;
      d[j + 2] = v & 255;
      d[j + 3] = 255;
    }
    ctx.putImageData(image, 0, 0);
  } else if (m.type === "audio") {
    audioMessage(m);
  } else if (m.type === "log") {
    const atEnd = logEl.scrollTop + logEl.clientHeight >= logEl.scrollHeight - 4;
    logEl.append(m.text + "\n");
    if (logEl.childNodes.length > 2000) logEl.firstChild.remove();
    if (atEnd) logEl.scrollTop = logEl.scrollHeight;
    if (!bootLog.hidden) {
      bootLog.append(m.text + "\n");
      if (bootLog.childNodes.length > 40) bootLog.firstChild.remove();
    }
  }
}

// DangerOS audio streams (MP3 etc.), decoded and played with Web Audio.
let actx = null,
  master = null;
const streams = new Map(); // id -> {buffer, pending, src, gain}
// Volume: the slider's 0–100 is squared into gain, which sounds closer to even steps.
const volume = document.getElementById("volume");
try {
  volume.value = localStorage.getItem("websim.volume") ?? volume.value;
} catch {}
const gainFor = () => (volume.value / 100) ** 2;
const ctxNow = () => {
  if (!actx) {
    actx = new AudioContext();
    master = actx.createGain();
    master.gain.value = gainFor();
    master.connect(actx.destination);
  }
  return actx;
};
// Browsers block audio before a user gesture. Until the first click or key on the page, streams
// only keep time (see startSrc); on unlock each one still playing starts where it would be by now.
let unlocked = false;
const unlock = () => {
  if (unlocked) return;
  unlocked = true;
  ctxNow()
    .resume()
    .then(() => {
      for (const [id, s] of streams)
        if (s.silent) startSrc(id, s, performance.now() - s.silent.at, s.silent.loop);
    });
};
document.addEventListener("pointerdown", unlock, true);
document.addEventListener("keydown", unlock, true);
volume.addEventListener("input", () => {
  if (master) master.gain.value = gainFor();
  try {
    localStorage.setItem("websim.volume", volume.value);
  } catch {}
});

window.websimAudio = { streams, context: () => actx }; // for debugging from devtools
const audioEvent = (id, event, extra = {}) =>
  worker?.postMessage({ type: "audio-event", id, event, ...extra });
const stopSrc = (s) => {
  if (s.silent) {
    clearTimeout(s.silent.timer);
    s.silent = null;
  }
  if (!s.src) return;
  const src = s.src;
  s.src = null;
  try {
    src.stop();
  } catch {}
};
const startSrc = (id, s, offsetMs, loop) => {
  stopSrc(s);
  if (actx.state !== "running") {
    // no sound yet: report it playing and ended on time so the game carries on
    const lenMs = Math.round(s.buffer.duration * 1000);
    const off = Math.max(0, offsetMs) % Math.max(1, lenMs);
    const silent = { at: performance.now() - off, loop, timer: 0 };
    if (!loop)
      silent.timer = setTimeout(() => {
        if (s.silent === silent) {
          s.silent = null;
          audioEvent(id, "ended");
        }
      }, lenMs - off);
    s.silent = silent;
    audioEvent(id, "playing", { lenMs, offsetMs: off });
    return;
  }
  const src = actx.createBufferSource();
  src.buffer = s.buffer;
  src.loop = loop;
  src.connect(s.gain);
  src.onended = () => {
    if (s.src === src) {
      s.src = null;
      audioEvent(id, "ended");
    }
  };
  const lenMs = Math.round(s.buffer.duration * 1000);
  const off = Math.max(0, offsetMs) % Math.max(1, lenMs);
  src.start(0, off / 1000);
  s.src = src;
  audioEvent(id, "playing", { lenMs, offsetMs: off });
};
const audioMessage = (m) => {
  const ctx = ctxNow();
  let s = streams.get(m.id);
  if (m.op === "load") {
    if (s) stopSrc(s);
    s = { buffer: null, pending: null, src: null, silent: null, gain: ctx.createGain() };
    s.gain.connect(master);
    streams.set(m.id, s);
    ctx.decodeAudioData(m.data.buffer).then(
      (buf) => {
        s.buffer = buf;
        if (s.pending) {
          const [o, l] = s.pending;
          s.pending = null;
          startSrc(m.id, s, o, l);
        }
      },
      () => audioEvent(m.id, "error"),
    );
  } else if (!s) {
    return;
  } else if (m.op === "play") {
    if (s.buffer) startSrc(m.id, s, m.offsetMs, m.loop);
    else s.pending = [m.offsetMs, m.loop];
  } else if (m.op === "stop") {
    s.pending = null;
    stopSrc(s);
  } else if (m.op === "volume") {
    s.gain.gain.value = Math.max(0, Math.min(1, m.volume / 100));
  } else if (m.op === "free") {
    s.pending = null;
    stopSrc(s);
    s.gain.disconnect();
    streams.delete(m.id);
  }
};

// Browser key -> AWT key code (java.awt.event.KeyEvent VK_*).
const VK = {
  Enter: 10,
  Backspace: 8,
  Tab: 9,
  Escape: 27,
  Space: 32,
  ShiftLeft: 16,
  ShiftRight: 16,
  ControlLeft: 17,
  ControlRight: 17,
  AltLeft: 18,
  AltRight: 18,
  ArrowLeft: 37,
  ArrowUp: 38,
  ArrowRight: 39,
  ArrowDown: 40,
  Home: 36,
  End: 35,
  PageUp: 33,
  PageDown: 34,
  Insert: 155,
  Delete: 127,
  Comma: 44,
  Minus: 45,
  Period: 46,
  Slash: 47,
  Semicolon: 59,
  Equal: 61,
  BracketLeft: 91,
  Backslash: 92,
  BracketRight: 93,
  Quote: 222,
  Backquote: 192,
};
const awtCode = (e) => {
  if (VK[e.code] !== undefined) return VK[e.code];
  let m = /^Key([A-Z])$/.exec(e.code);
  if (m) return m[1].charCodeAt(0);
  m = /^Digit(\d)$/.exec(e.code);
  if (m) return 48 + +m[1];
  m = /^F(\d+)$/.exec(e.code);
  if (m) return 111 + +m[1];
  m = /^Numpad(\d)$/.exec(e.code);
  if (m) return 96 + +m[1];
  return 0;
};
const awtChar = (e) => {
  if (e.key.length === 1) return e.key.charCodeAt(0);
  return { Enter: 10, Backspace: 8, Tab: 9, Escape: 27, Delete: 127 }[e.key] ?? 0xffff;
};
const mods = (e) =>
  (e.shiftKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.altKey ? 8 : 0);
const onKey = (e) => {
  if (e.target.closest && e.target.closest("select, input, textarea")) return;
  const code = awtCode(e);
  if (!code) return;
  if ((e.ctrlKey || e.metaKey) && /^(KeyR|KeyL|KeyT|KeyW|KeyQ)$/.test(e.code)) return; // browser shortcuts
  if (e.code === "F12" && e.shiftKey) return;
  e.preventDefault();
  send(e.type === "keydown" ? 401 : 402, code, awtChar(e), mods(e));
};
document.addEventListener("keydown", onKey);
document.addEventListener("keyup", onKey);

canvas.addEventListener(
  "wheel",
  (e) => {
    e.preventDefault();
    const code = e.deltaY < 0 ? 122 : 123;
    send(401, code);
    send(402, code);
  },
  { passive: false },
);

const scaleSel = document.getElementById("scale");
try {
  scaleSel.value = localStorage.getItem("websim.k") || scaleSel.value;
} catch {}
const applyScale = () => {
  document.getElementById("stage").style.setProperty("--k", scaleSel.value);
  try {
    localStorage.setItem("websim.k", scaleSel.value);
  } catch {}
};
scaleSel.addEventListener("change", applyScale);
applyScale();
document
  .getElementById("showhot")
  .addEventListener("change", (e) => device.classList.toggle("show-hot", e.target.checked));

document.getElementById("reset").addEventListener("click", () => {
  if (!confirm("Erase the simulator's saved settings and datastores?")) return;
  worker?.terminate();
  const req = indexedDB.deleteDatabase("websim");
  req.onsuccess = req.onerror = req.onblocked = () => location.reload();
});
