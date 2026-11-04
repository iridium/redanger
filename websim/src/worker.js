// Runs the JVM + DangerOS in a worker. Talks to index.html with postMessage:
//   in:  {type: "init", vendor: absolute URL of public/vendor/, autoboot: bundle name or null}, {type: "key", id, code, ch, mods}
//   out: {type: "frame", width, height, pixels}, {type: "log", text}, {type: "audio", ...}

import { Jar } from "./jvm/zip.js";
import { inflate } from "./jvm/inflate.js";
import { CHUNK } from "./chunks.js";
import { createVM, bootVM, runMain } from "./jvm/boot.js";
import { makeHost } from "./host.js";

const log = (text) => postMessage({ type: "log", text });

class Pack {
  constructor(bytes) {
    this.entries = new Map();
    const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    const dec = new TextDecoder();
    for (let p = 0; p < bytes.length;) {
      const nl = dv.getUint32(p, true);
      const name = dec.decode(bytes.subarray(p + 4, p + 4 + nl));
      p += 4 + nl;
      const len = dv.getUint32(p, true);
      this.entries.set(name, bytes.subarray(p + 4, p + 4 + len));
      p += 4 + len;
    }
  }
  get(name) {
    return this.entries.get(name) || null;
  }
  has(name) {
    return this.entries.has(name);
  }
}

// Reads [pos, pos+len) of a lazily loaded file, published as chunk files (see chunks.js) when
// larger than one chunk. Synchronous XHR is fine in a worker; recent chunks are cached.
function chunkReader(url, size) {
  const chunks = new Map();
  const chunk = (i) => {
    let b = chunks.get(i);
    if (b) return b;
    const xhr = new XMLHttpRequest();
    xhr.open("GET", size > CHUNK ? `${url}.${i}` : url, false);
    xhr.responseType = "arraybuffer";
    xhr.send();
    if (xhr.status !== 200) throw new Error(`${url} chunk ${i}: ${xhr.status}`);
    b = new Uint8Array(xhr.response);
    if (chunks.size >= 24) chunks.delete(chunks.keys().next().value);
    chunks.set(i, b);
    return b;
  };
  return (pos, len) => {
    const out = new Uint8Array(len);
    for (let n = 0; n < len;) {
      const i = Math.floor((pos + n) / CHUNK),
        o = (pos + n) % CHUNK;
      const b = chunk(i);
      const k = Math.min(len - n, b.length - o);
      out.set(b.subarray(o, o + k), n);
      n += k;
    }
    return out;
  };
}

// JDK classes missing from the pack, read out of rt.jar on demand: its central directory once,
// then each class's bytes.
class RemoteJar {
  constructor(url, size) {
    this.read = chunkReader(url, size);
    this.size = size;
    this.dir = null; // name -> {local, csize, method}
    this.cache = new Map();
  }
  directory() {
    if (this.dir) return this.dir;
    this.dir = new Map();
    const tailLen = Math.min(this.size, 65558);
    const tail = this.read(this.size - tailLen, tailLen);
    const tv = new DataView(tail.buffer);
    let e = tail.length - 22;
    while (e >= 0 && tv.getUint32(e, true) !== 0x06054b50) e--;
    const count = tv.getUint16(e + 10, true),
      cdSize = tv.getUint32(e + 12, true),
      cdOff = tv.getUint32(e + 16, true);
    const cd = this.read(cdOff, cdSize);
    const dv = new DataView(cd.buffer);
    const dec = new TextDecoder();
    for (let p = 0, i = 0; i < count; i++) {
      const nlen = dv.getUint16(p + 28, true),
        elen = dv.getUint16(p + 30, true),
        clen = dv.getUint16(p + 32, true);
      this.dir.set(dec.decode(cd.subarray(p + 46, p + 46 + nlen)), {
        method: dv.getUint16(p + 10, true),
        csize: dv.getUint32(p + 20, true),
        local: dv.getUint32(p + 42, true),
      });
      p += 46 + nlen + elen + clen;
    }
    return this.dir;
  }
  get(name) {
    if (!name.endsWith(".class") || !/^(java|javax|sun|jdk|com\/sun)\//.test(name)) return null;
    if (this.cache.has(name)) return this.cache.get(name);
    const e = this.directory().get(name);
    let data = null;
    if (e) {
      const head = this.read(e.local, 30);
      const hv = new DataView(head.buffer);
      const start = e.local + 30 + hv.getUint16(26, true) + hv.getUint16(28, true);
      const raw = this.read(start, e.csize);
      data = e.method === 0 ? raw : inflate(raw).out;
      console.warn("[websim] class outside pack:", name);
    }
    this.cache.set(name, data);
    return data;
  }
  has(name) {
    return !!this.get(name);
  }
}

async function fetchBytes(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`${url}: ${r.status}`);
  return new Uint8Array(await r.arrayBuffer());
}

async function gunzip(bytes) {
  const s = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"));
  return new Uint8Array(await new Response(s).arrayBuffer());
}

async function loadJar(url) {
  const jar = new Jar(await fetchBytes(url), () => {
    throw new Error("jar entry not pre-inflated");
  });
  await jar.inflateAll();
  return jar;
}

// The simulator's files live in IndexedDB.
function openDB() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open("websim", 1);
    req.onupgradeneeded = () => req.result.createObjectStore("files");
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function loadFiles(db) {
  return new Promise((resolve) => {
    const out = new Map();
    const tx = db.transaction("files").objectStore("files").openCursor();
    tx.onsuccess = () => {
      const c = tx.result;
      if (!c) return resolve(out);
      out.set(c.key, c.value);
      c.continue();
    };
    tx.onerror = () => resolve(out);
  });
}

let V = ""; // public/vendor/ URL, sent by the page
let AUTOBOOT = null; // bundle to boot into instead of the launcher, sent by the page
let pending = null; // latest frame waiting to be posted

async function main() {
  const [websim, pack, library, simulator, classof09, card, rt] = await Promise.all([
    loadJar(V + "websim.jar"),
    fetchBytes(V + "rt-pack.dat")
      .then(gunzip)
      .then((b) => new Pack(b)),
    loadJar(V + "library.jar"),
    loadJar(V + "simulator.jar"),
    loadJar(V + "apps/classof09.jar"), // preinstalled app: its /application.dat is the app bundle
    fetch(V + "card-manifest.json").then((r) => (r.ok ? r.json() : [])),
    fetch(V + "rt.json").then((r) => r.json()),
  ]);

  const { host, natives } = makeHost({
    audio: (m) => postMessage({ type: "audio", ...m }, m.data ? [m.data.buffer] : []),
    present: (screen) => {
      if (!pending) setTimeout(postFrame, 16);
      pending = screen;
    },
    decodeImage: async (bytes) => {
      const bmp = await createImageBitmap(new Blob([bytes]));
      const c = new OffscreenCanvas(bmp.width, bmp.height);
      const g = c.getContext("2d");
      g.drawImage(bmp, 0, 0);
      const d = g.getImageData(0, 0, bmp.width, bmp.height).data;
      const px = new Int32Array(bmp.width * bmp.height);
      for (let i = 0; i < px.length; i++)
        px[i] = (d[i * 4 + 3] << 24) | (d[i * 4] << 16) | (d[i * 4 + 1] << 8) | d[i * 4 + 2];
      return { width: bmp.width, height: bmp.height, pixels: px };
    },
  });
  self.onmessage = (e) => {
    const m = e.data;
    if (m.type === "key") host.pushKey(m.id, m.code, m.ch, m.mods);
    else if (m.type === "audio-event") host.audioEvent(m);
  };

  let line = "";
  const out = (s) => {
    line += s;
    const parts = line.split("\n");
    line = parts.pop();
    for (const p of parts) {
      console.log(p);
      log(p);
    }
  };
  const vm = createVM({
    jars: [websim, pack, library, simulator, classof09, new RemoteJar(V + "rt.jar", rt.size)],
    natives,
    stdout: out,
    stderr: out,
    log: (s) => {
      console.log(s);
      log(s);
    },
    props: {
      "user.dir": "/state",
      "com.danger.username": "redanger",
      "com.danger.service_panel": "true",
      "com.danger.os_version": "3.4",
      "danger.storage.root": "/state/removable0",
      "com.danger.flash.settings_file": "/state/FLASH.Settings.out",
      "com.danger.ATInterface.ModemPort": "1", // in-memory modem (shim SimModemInterfaceImpl), so SMS loads
      ...(AUTOBOOT ? { "com.danger.autoboot": AUTOBOOT } : {}),
    },
  });

  // Files: JRE data, the seeded simulator state, then whatever IndexedDB has saved.
  const vfs = vm.vfs;
  vfs.cwd = "/state";
  vfs.mkdirs("/state/removable0");
  // The SD card: mounted lazily from vendor/card/ (Class of '09's data packs live there).
  for (const f of card) {
    const url = V + "card/" + (f.url || f.path).split("/").map(encodeURIComponent).join("/");
    vfs.mount("/state/removable0/" + f.path, f.size, chunkReader(url, f.size));
  }
  for (const f of ["tzdb.dat", "currency.data", "calendars.properties"])
    vfs.write("/jre/lib/" + f, await fetchBytes(V + "jre-lib/" + f));
  let db = null;
  try {
    db = await openDB();
  } catch (e) {
    log("[websim] no IndexedDB, state will not persist");
  }
  const saved = db ? await loadFiles(db) : new Map();
  for (const f of ["FLASH.Settings.out", "DangerM2SimulatorProperties"]) {
    if (!saved.has("/state/" + f)) vfs.write("/state/" + f, await fetchBytes(V + "state/" + f));
  }
  for (const [p, data] of saved) vfs.write(p, data);
  if (db) {
    const dirty = new Set();
    let timer = null;
    vfs.onChange = (p) => {
      if (!p.startsWith("/state/")) return;
      dirty.add(p);
      clearTimeout(timer);
      timer = setTimeout(() => {
        const tx = db.transaction("files", "readwrite").objectStore("files");
        for (const q of dirty) {
          const d = vfs.read(q);
          if (d) tx.put(d, q);
          else tx.delete(q);
        }
        dirty.clear();
      }, 1000);
    };
  }

  const t = bootVM(vm);
  vm.fastForward = true; // skip boot-time sleeps until the first frame
  runMain(vm, t, "websim.Main");
  const t0 = performance.now();
  let booted = false;
  const tick = () => {
    const wait = vm.runSlice(25);
    if (!booted && host.frames) {
      booted = true;
      console.log(`[websim] first frame after ${((performance.now() - t0) / 1000).toFixed(1)} s`);
    }
    if (vm.exited) return; // powered off
    setTimeout(tick, wait === Infinity ? 50 : Math.min(wait, 25));
  };
  tick();
}

function postFrame() {
  const s = pending;
  pending = null;
  if (!s) return;
  const pixels = new Int32Array(s.pixels);
  postMessage({ type: "frame", width: s.width, height: s.height, pixels }, [pixels.buffer]);
}

self.onmessage = (e) => {
  if (e.data.type !== "init") return;
  V = e.data.vendor;
  AUTOBOOT = e.data.autoboot || null;
  main().catch((err) => {
    console.error(err);
    log(String(err.stack || err));
  });
};
