// Host side of the simulator: natives that connect DangerOS's screen, keys and pixel
// loops to the page (or to Node for headless runs).

import { BLOCK, JObject } from "./jvm/jvm.js";
import { inflate, Incomplete } from "./jvm/inflate.js";

export function makeHost({ present = () => {}, decodeImage = null, audio = () => {} } = {}) {
  const keys = []; // pending {id, code, char, mods}
  let waiter = null; // thread blocked in nextKeyEvent
  let storesChanged = false; // a DataStore changed since websim.StoreSaver last looked

  // Web Audio streams, played by the page. The page reports back with host.audioEvent().
  const streams = new Map(); // id -> {playing, done, startAt, offset, lenMs, loop}
  let nextStream = 1;
  const now = () => performance.now();
  const position = (s) => {
    if (!s.playing) return s.offset;
    const p = Math.round(now() - s.startAt);
    return s.lenMs ? (s.loop ? p % s.lenMs : Math.min(p, s.lenMs)) : p;
  };
  // Whole file from the VFS without keeping lazily mounted files in memory.
  const fileBytes = (vfs, path) => {
    const p = vfs.norm(path);
    const d = vfs.files.get(p);
    if (d) return d.slice();
    const r = vfs.remote.get(p);
    return r ? r.read(0, r.size) : null;
  };

  const host = {
    screen: null, // {pixels: Int32Array, width, height}
    frames: 0,
    audioEvent(m) {
      const s = streams.get(m.id);
      if (!s) return;
      if (m.event === "playing") {
        s.lenMs = m.lenMs;
        s.startAt = now() - m.offsetMs;
      } else if (m.event === "ended") {
        s.offset = position(s);
        s.playing = false;
        s.done = true;
      } else if (m.event === "error") {
        s.playing = false;
        s.done = true;
      }
    },
    pushKey(id, code, ch, mods = 0) {
      keys.push([id, code, ch, mods]);
      if (waiter) {
        waiter.state = "blocked";
        waiter.blockedOn = null;
        waiter = null;
      }
    },
  };

  const backBuffer = (vm) => {
    const ss = vm.classes.get("danger/ui/SimScreen");
    const aw = ss && ss.statics.mAwtScreen;
    return aw && aw.mBackBuffer;
  };

  const natives = {
    "danger/ui/SimScreen.off2On()V": (t) => {
      const img = backBuffer(t.jvm);
      if (!img) return;
      host.screen = { pixels: img.pixels.a, width: img.width, height: img.height };
      host.frames++;
      t.jvm.fastForward = false; // booted: timers run in real time from here on
      present(host.screen);
    },
    // The simulator starts with the screen closed over the keyboard ("Flip to dial"); the device
    // photo is open, so the hinge always reads open.
    "danger/ui/AWTScreen.getHingeState()Z": () => 1,
    "danger/ui/WebEventPump.keyboard()Ldanger/ui/SimKeyboard;": (t) => {
      const aw = t.jvm.classes.get("danger/ui/AWTScreen");
      const s = aw && aw.statics.sAWTScreen;
      return s ? s.mKeyboard : null;
    },
    "websim/WebHost.nextKeyEvent()[I": (t) => {
      const take = () => {
        if (!keys.length) {
          t.state = "waiting";
          t.wakeAt = Infinity;
          waiter = t;
          return BLOCK;
        }
        const arr = t.jvm.newArray("[I", 4);
        arr.a.set(keys.shift());
        return arr;
      };
      t.resume = take;
      const r = take();
      if (r !== BLOCK) t.resume = null;
      return r;
    },
    // The simulator's clock starts at 0, DangerOS's epoch (2000-01-01 UTC), and only the
    // service's time sync sets it; start it at the browser's time instead. setDate (Settings,
    // Date & Time) still moves it, as getDate counts from whatever base that set.
    "danger/system/SimHardwareImpl.getDate()I": (t, hw) => {
      const vm = t.jvm,
        kb = fieldKey(vm, "danger/system/SimHardwareImpl", "mBaseSeconds");
      const kd = fieldKey(vm, "danger/system/SimHardwareImpl", "mDeltaSeconds");
      const now = Math.floor(Date.now() / 1000);
      if (hw[kd] === 0) {
        hw[kd] = now;
        hw[kb] = now - DANGER_EPOCH;
      }
      return (hw[kb] + (now - hw[kd])) | 0;
    },
    // DangerOS's time zone: the one chosen in Settings, else (where the service would have
    // configured one) the browser's, else Pacific as before.
    "danger/internal/TimeZone.getDefault()Ldanger/internal/TimeZone;": (t) => {
      const vm = t.jvm,
        c = vm.loadClass("danger/internal/TimeZone");
      if (c.statics.sDefault) return c.statics.sDefault;
      const st = vm.need(t, "danger/system/Settings");
      vm.ensureInitSync(t, st);
      const get = c.methods.get("getTimeZone(I)Ldanger/internal/TimeZone;");
      const id = vm.invokeSync(t, st.methods.get("getIntValue(Ljava/lang/String;I)I"), [
        st.statics.TIME_ZONE_ID,
        0,
      ]);
      const tz =
        (id && vm.invokeSync(t, get, [id])) ||
        vm.invokeSync(t, get, [browserZone()]) ||
        vm.invokeSync(t, get, [5]);
      c.statics.sDefault = tz;
      return tz;
    },
    // every DataStore edit (add, insert, remove, set data or flags) ends here; noted so
    // websim.StoreSaver writes the stores out, since the simulator only does at power-off
    "danger/app/DataStore.updateLastModified()V": (t, ds) => {
      ds[fieldKey(t.jvm, "danger/app/DataStore", "mLastModified")] = BigInt(Date.now());
      storesChanged = true;
    },
    "websim/WebHost.storesChanged()Z": () => {
      const c = storesChanged;
      storesChanged = false;
      return c ? 1 : 0;
    },
    "websim/WebHost.decodeImage([B[I)[I": (t, data, wh) => {
      if (!decodeImage) return null;
      const bytes = new Uint8Array(data.a.buffer, data.a.byteOffset, data.a.length).slice();
      let result;
      let done = false;
      decodeImage(bytes).then(
        (r) => {
          result = r;
          done = true;
          t.wakeAt = 0;
        },
        () => {
          result = null;
          done = true;
          t.wakeAt = 0;
        },
      );
      t.state = "sleeping";
      t.wakeAt = Infinity;
      t.resume = () => {
        if (!done) {
          t.state = "sleeping";
          t.wakeAt = Infinity;
          return BLOCK;
        }
        if (!result) return null;
        wh.a[0] = result.width;
        wh.a[1] = result.height;
        const arr = t.jvm.newArray("[I", result.width * result.height);
        arr.a.set(result.pixels);
        return arr;
      };
      return BLOCK;
    },
    "danger/audio/SimAudioManagerImpl.createStream(ZI)Ldanger/audio/Stream;": (t) => {
      const vm = t.jvm;
      const c = vm.loadClass("danger/audio/WebStream");
      vm.ensureInitSync(t, c);
      const o = new JObject(c);
      vm.invokeSync(t, c.methods.get("<init>()V"), [o]);
      return o;
    },
    // The simulator's codec check only knows MIDI/RMF/AIFF/WAV magic numbers, so every MP3 was
    // "unsupported". Devices report MP3 as type 9 (file-streamable); everything else goes through
    // the simulator's own table (SimAudioManagerImpl.sMimes) as before.
    "danger/audio/SimAudioManagerImpl.isCodecSupported([B)I": (t, self, data) => {
      if (!data || data.a.length < 4) return 0;
      const b = data.a;
      if (
        (b[0] === 0x49 && b[1] === 0x44 && b[2] === 0x33) ||
        ((b[0] & 0xff) === 0xff && (b[1] & 0xe0) === 0xe0)
      )
        return 9;
      const magic = (b[0] + (b[1] << 8) + (b[2] << 16) + (b[3] << 24)) | 0;
      const mimes = t.jvm.loadClass("danger/audio/SimAudioManagerImpl").statics.sMimes;
      for (const m of mimes ? mimes.a : []) if (m && m.mMagic === magic) return m.mType;
      return 0;
    },
    "danger/audio/SimAudioManagerImpl.getCodecMimeType(I)Ljava/lang/String;": (t, self, type) => {
      if (type === 9) return t.jvm.intern("audio/mpeg");
      const mimes = t.jvm.loadClass("danger/audio/SimAudioManagerImpl").statics.sMimes;
      for (const m of mimes ? mimes.a : []) if (m && m.mType === type) return m.mMime;
      return null;
    },
    // SonicBOOM: play songs like a device does (a Stream), not with the simulator's silent stand-in
    "apps/mediaplayer/MediaPlayer.playableForMusicFile(Lapps/mediaplayer/MusicFile;)Ldanger/audio/PlayableAudio;":
      (t, self, mf) => {
        const vm = t.jvm;
        const c = vm.need(t, "apps/mediaplayer/WebPlayables"); // in websim.jar (shim/build.sh)
        vm.ensureInitSync(t, c);
        return vm.callFromNative(
          t,
          c.methods.get("forFile(Lapps/mediaplayer/MusicFile;)Ldanger/audio/PlayableAudio;"),
          [mf],
        );
      },
    "danger/audio/WebStream.create()I": () => {
      const id = nextStream++;
      streams.set(id, {
        playing: false,
        done: false,
        startAt: 0,
        offset: 0,
        lenMs: 0,
        loop: false,
      });
      return id;
    },
    "danger/audio/WebStream.loadFile(ILjava/lang/String;)Z": (t, id, path) => {
      const data = fileBytes(t.jvm.vfs, t.jvm.jsstr(path));
      if (!data) return 0;
      audio({ op: "load", id, data });
      return 1;
    },
    "danger/audio/WebStream.loadBytes(I[B)Z": (t, id, b) => {
      audio({
        op: "load",
        id,
        data: new Uint8Array(b.a.buffer, b.a.byteOffset, b.a.length).slice(),
      });
      return 1;
    },
    "danger/audio/WebStream.play(IIZ)V": (t, id, offsetMs, loop) => {
      const s = streams.get(id);
      if (!s) return;
      if (offsetMs < 0) offsetMs = s.offset; // resume
      Object.assign(s, {
        playing: true,
        done: false,
        offset: offsetMs,
        startAt: now() - offsetMs,
        loop: !!loop,
      });
      audio({ op: "play", id, offsetMs, loop: !!loop });
    },
    "danger/audio/WebStream.halt(IZ)V": (t, id, pause) => {
      const s = streams.get(id);
      if (!s) return;
      s.offset = position(s);
      s.playing = false;
      if (!pause) {
        s.done = true;
        s.offset = 0;
      }
      audio({ op: "stop", id });
    },
    "danger/audio/WebStream.setGain(II)V": (t, id, v) => audio({ op: "volume", id, volume: v }),
    "danger/audio/WebStream.free(I)V": (t, id) => {
      streams.delete(id);
      audio({ op: "free", id });
    },
    "danger/audio/WebStream.state(I)[I": (t, id) => {
      const s = streams.get(id);
      const arr = t.jvm.newArray("[I", 4);
      if (s) arr.a.set([s.playing ? 1 : 0, s.done ? 1 : 0, position(s), s.lenMs]);
      else arr.a.set([0, 1, 0, 0]);
      return arr;
    },
    // The simulator turns a Bitmap into an AWT image with getPoint/setRGB per pixel, interpreted;
    // the game's frame buffer goes through it on every paint. Same conversion, in JS.
    "danger/ui/SimPen.bitmapToImage(Ldanger/ui/Bitmap;)Ljava/awt/Image;": (t, bmp) => {
      const vm = t.jvm;
      const bc = vm.need(t, "java/awt/image/BufferedImage");
      vm.ensureInitSync(t, bc);
      const w = bmp[fieldKey(vm, "danger/ui/Bitmap", "mWidth")],
        h = bmp[fieldKey(vm, "danger/ui/Bitmap", "mHeight")];
      const img = new JObject(bc);
      vm.invokeSync(t, bc.methods.get("<init>(III)V"), [img, w, h, 2]);
      const out = img[fieldKey(vm, "java/awt/image/BufferedImage", "pixels")].a;
      if (!bitmapPixels(vm, bmp, w, h, out)) {
        const gp = vm.virtualMethod(bmp.cls, "getPoint(II)I");
        for (let y = 0; y < h; y++)
          for (let x = 0; x < w; x++) out[y * w + x] = vm.invokeSync(t, gp, [bmp, x, y]);
      }
      return img;
    },
    // The simulator's PNG decoder unfilters rows a byte at a time, interpreted; nearly all of
    // boot's theme and splash-screen loading is spent here. Same loop as PNG.InflateNormal (a
    // filter byte and a row at a time from the Inflater), in JS. Interlaced PNGs stay in Java.
    "danger/ui/PNG.InflateNormal()V": (t, png) => {
      const vm = t.jvm;
      const k = (n) => fieldKey(vm, "danger/ui/PNG", n);
      const inf = png[k("inflater")];
      const ik = (n) => fieldKey(vm, "java/util/zip/Inflater", n);
      const buf = inf[ik("buf")],
        ioff = inf[ik("off")],
        ilen = inf[ik("len")];
      let data;
      try {
        // zlib stream: skip the 2-byte header, as the Inflater natives do
        data =
          ilen > 2
            ? inflate(new Uint8Array(buf.a.buffer, buf.a.byteOffset + ioff + 2, ilen - 2)).out
            : new Uint8Array(0);
      } catch (e) {
        if (!(e instanceof Incomplete)) vm.throwNew(t, "java/lang/RuntimeException", e.message);
        data = new Uint8Array(0); // an Inflater that never finishes yields nothing
      }
      inf[ik("off")] = ioff + ilen;
      inf[ik("len")] = 0;
      inf[ik("finished")] = 1;
      const stride = png[k("stride")],
        bpp = png[k("bpp")],
        cs = png[k("colorspace")];
      const out = png[k("out")].a; // Int8Array
      let offset = png[k("offset")];
      const row = new Uint8Array(stride),
        prev = new Uint8Array(stride);
      let pos = 0;
      while (pos < data.length) {
        const filter = (data[pos++] << 24) >> 24; // a Java byte
        const n = Math.min(stride, data.length - pos);
        if (n === 0) break;
        row.set(data.subarray(pos, pos + n));
        pos += n;
        if (cs === 259) {
          if (offset + stride > out.length)
            vm.throwNew(t, "java/lang/ArrayIndexOutOfBoundsException", String(out.length));
          for (let i = 0; i < stride; i++) out[offset + i] = 255 - ((row[i] << 24) >> 24);
        } else if (filter <= 4) {
          unfilter(filter, row, prev, stride, bpp);
          if (offset + stride > out.length)
            vm.throwNew(t, "java/lang/ArrayIndexOutOfBoundsException", String(out.length));
          out.set(new Int8Array(row.buffer, 0, stride), offset);
          if (cs === 770) {
            // RGBA -> ARGB
            for (let i = offset; i < offset + stride; i += 4) {
              const a = out[i + 3];
              out[i + 3] = out[i + 2];
              out[i + 2] = out[i + 1];
              out[i + 1] = out[i];
              out[i] = a;
            }
          } else if (cs === 772) {
            // 16-bit RGBA -> ARGB
            for (let i = offset; i < offset + stride; i += 8) {
              const a0 = out[i + 6],
                a1 = out[i + 7];
              out.copyWithin(i + 2, i, i + 6);
              out[i] = a0;
              out[i + 1] = a1;
            }
          }
        } else {
          vm.log(`PNG: inflate_normal: invalid line filter ${filter}`);
        }
        offset += n;
        prev.set(row);
      }
      png[k("offset")] = offset;
    },
    "websim/Raster.fill([IIIIIIIIF)V": (t, px, stride, x0, y0, x1, y1, argb, rule, alpha) => {
      const p = px.a;
      const a = Math.trunc(((argb >>> 24) & 255) * alpha);
      if (rule === 2 || a >= 255) {
        const v = rule === 2 ? argb : argb | 0xff000000;
        for (let y = y0; y < y1; y++) p.fill(v, y * stride + x0, y * stride + x1);
        return;
      }
      if (rule === 1) {
        for (let y = y0; y < y1; y++) p.fill(0, y * stride + x0, y * stride + x1);
        return;
      }
      if (a <= 0) return;
      for (let y = y0; y < y1; y++)
        for (let i = y * stride + x0, e = y * stride + x1; i < e; i++) p[i] = blend(p[i], argb, a);
    },
    "websim/Raster.blit([IIZIIII[IIIIIIIIIIIF)V": (
      t,
      src,
      sstride,
      srcAlpha,
      sx,
      sy,
      sw,
      sh,
      dst,
      dstride,
      dx,
      dy,
      dw,
      dh,
      cx0,
      cy0,
      cx1,
      cy1,
      rule,
      alpha,
    ) => {
      const s = src.a,
        d = dst.a;
      const y0 = Math.max(cy0, dy),
        y1 = Math.min(cy1, dy + dh);
      const x0 = Math.max(cx0, dx),
        x1 = Math.min(cx1, dx + dw);
      for (let y = y0; y < y1; y++) {
        const srow = (sy + Math.trunc(((y - dy) * sh) / dh)) * sstride + sx;
        for (let x = x0; x < x1; x++) {
          let v = s[srow + Math.trunc(((x - dx) * sw) / dw)];
          if (!srcAlpha) v |= 0xff000000;
          const a = Math.trunc(((v >>> 24) & 255) * alpha);
          const i = y * dstride + x;
          d[i] =
            rule === 2
              ? v
              : rule === 1
                ? 0
                : a >= 255
                  ? v | 0xff000000
                  : a <= 0
                    ? d[i]
                    : blend(d[i], v, a);
        }
      }
    },
  };
  return { host, natives };
}

const DANGER_EPOCH = 946684800; // 2000-01-01 00:00 UTC in Unix seconds

// The browser's time zone as one of DangerOS's (danger/internal/TimeZone IDs). Zones with
// daylight saving go by name, as DangerOS applies its own DST rules; the rest by their current
// UTC offset, to a fixed "GMT±N" zone.
const ZONES = [
  [
    2,
    /^America\/(New_York|Detroit|Toronto|Montreal|Nassau|Indiana\/.*|Kentucky\/.*)$|^US\/(Eastern|Michigan)$/,
  ],
  [3, /^America\/(Chicago|Winnipeg|Menominee|North_Dakota\/.*)$|^US\/Central$/],
  [4, /^America\/(Denver|Edmonton|Boise|Yellowknife)$|^US\/Mountain$/],
  [8, /^America\/Phoenix$|^US\/Arizona$/],
  [5, /^America\/(Los_Angeles|Vancouver|Tijuana)$|^US\/Pacific$/],
  [6, /^America\/(Anchorage|Juneau|Sitka|Nome|Yakutat|Metlakatla)$|^US\/Alaska$/],
  [7, /^America\/Adak$/],
  [10, /^Pacific\/Honolulu$|^US\/Hawaii$/],
  [1, /^America\/(Halifax|Glace_Bay|Moncton|Goose_Bay|Thule)$|^Atlantic\/Bermuda$/],
  [79, /^America\/Puerto_Rico$/],
  [40, /^America\/St_Johns$/],
  [70, /^America\/(Regina|Swift_Current)$/],
  [39, /^America\/Santiago$/],
  [43, /^Europe\/(London|Dublin|Belfast|Guernsey|Isle_of_Man|Jersey)$/],
  [72, /^Europe\/Lisbon$|^Atlantic\/(Canary|Faroe|Madeira)$/],
  [
    44,
    /^Europe\/(Amsterdam|Andorra|Belgrade|Berlin|Bratislava|Brussels|Budapest|Copenhagen|Gibraltar|Ljubljana|Luxembourg|Madrid|Malta|Monaco|Oslo|Paris|Prague|Rome|San_Marino|Sarajevo|Skopje|Stockholm|Tirane|Vaduz|Vatican|Vienna|Warsaw|Zagreb|Zurich|Podgorica)$/,
  ],
  [
    45,
    /^Europe\/(Athens|Bucharest|Helsinki|Kiev|Kyiv|Riga|Sofia|Tallinn|Vilnius|Chisinau|Mariehamn)$|^Asia\/(Nicosia|Beirut)$/,
  ],
  [46, /^Africa\/Cairo$/],
  [47, /^Asia\/(Jerusalem|Tel_Aviv)$/],
  [64, /^Australia\/(Sydney|Melbourne|Canberra|ACT|NSW|Victoria)$/],
  [65, /^Australia\/(Hobart|Tasmania)$/],
  [62, /^Australia\/(Adelaide|Broken_Hill|South)$/],
  [78, /^Australia\/Lord_Howe$/],
  [68, /^Pacific\/Auckland$|^NZ$/],
  [80, /^Pacific\/Chatham$/],
];
// minutes east of UTC -> a fixed-offset zone (named ones where DangerOS has no plain GMT±N)
const OFFSETS = {
  "-720": 12,
  "-660": 11,
  "-600": 10,
  "-540": 6,
  "-480": 5,
  "-420": 8,
  "-360": 18,
  "-300": 19,
  "-240": 20,
  "-210": 40,
  "-180": 21,
  "-120": 22,
  "-60": 23,
  0: 24,
  60: 25,
  120: 26,
  180: 27,
  210: 50,
  240: 28,
  270: 52,
  300: 54,
  330: 55,
  345: 56,
  360: 30,
  390: 58,
  420: 31,
  480: 32,
  540: 33,
  570: 63,
  600: 34,
  630: 78,
  660: 67,
  720: 36,
  765: 80,
  780: 37,
};
function browserZone() {
  let name = "";
  try {
    name = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {}
  for (const [id, re] of ZONES) if (re.test(name)) return id;
  return OFFSETS[-new Date().getTimezoneOffset()] || 24;
}

const fieldKeys = new Map();
function fieldKey(vm, cls, name) {
  const k = cls + "." + name;
  let key = fieldKeys.get(k);
  if (key === undefined) fieldKeys.set(k, (key = vm.loadClass(cls).fields.get(name).key));
  return key;
}

// ARGB pixels of a ShortBitmap or IntBitmap into out, as their getPoint() returns them
// (danger/ui/ShortBitmap, IntBitmap); false for any other bitmap or colorspace.
function bitmapPixels(vm, bmp, w, h, out) {
  const cls = bmp.cls.name;
  if (cls !== "danger/ui/ShortBitmap" && cls !== "danger/ui/IntBitmap") return false;
  const src = bmp[fieldKey(vm, "danger/ui/Bitmap", "mPixels")];
  const cs = bmp[fieldKey(vm, "danger/ui/Bitmap", "mColorSpace")];
  const n = w * h;
  if (!src || src.a.length < n) return false;
  const s = src.a;
  if (cls === "danger/ui/IntBitmap") {
    if (cs === 774)
      out.set(s.subarray(0, n)); // RGB32_BGRA
    else if (cs === 773) {
      // RGB32_BGRx
      const tr = bmp[fieldKey(vm, "danger/ui/IntBitmap", "mTransparentColorXRGB")];
      for (let i = 0; i < n; i++) {
        const p = s[i] & 0xffffff;
        out[i] = p === tr ? p : p | 0xff000000;
      }
    } else return false;
    return true;
  }
  const tr = bmp[fieldKey(vm, "danger/ui/Bitmap", "mTransparentColor")];
  let px;
  switch (cs) {
    case 265:
      px = (p) => {
        const q = ((p & 31) << 19) | ((p & 2016) << 5) | ((p & 63488) >> 8);
        return (p === tr ? 0 : 0xff000000) | q | ((q & 14680288) >> 5) | ((q & 49152) >> 6);
      };
      break;
    case 521:
      px = (p) => {
        // RGB16_RGB_LE
        const q = ((p & 63488) << 8) | ((p & 2016) << 5) | ((p & 31) << 3);
        return (p === tr ? 0 : 0xff000000) | q | ((q & 14680288) >> 5) | ((q & 49152) >> 6);
      };
      break;
    // the rest store their 16 bits byte-swapped
    case 9:
      px = (p) => {
        p = ((p & 65280) >> 8) | ((p & 255) << 8);
        const q = ((p & 63488) << 8) | ((p & 2016) << 5) | ((p & 31) << 3);
        return (p === tr ? 0 : 0xff000000) | q | ((q & 14680288) >> 5) | ((q & 49152) >> 6);
      };
      break;
    case 776:
      px = (p) => {
        // RGB16_RGB_444
        p = (((p & 65280) >> 8) | ((p & 255) << 8)) & 4095;
        const q = ((p & 3840) << 8) | ((p & 240) << 4) | (p & 15);
        return (p === tr ? 0 : 0xff000000) | q | (q << 4);
      };
      break;
    case 780:
      px = (p) => {
        // RGB16_ARGB_4444
        p = ((p & 65280) >> 8) | ((p & 255) << 8);
        const q = ((p & 61440) << 12) | ((p & 3840) << 8) | ((p & 240) << 4) | (p & 15);
        return q | (q << 4);
      };
      break;
    case 781:
      px = (p) => {
        // RGB16_RGB_555
        p = (((p & 65280) >> 8) | ((p & 255) << 8)) & 32767;
        const q = ((p & 31744) << 9) | ((p & 992) << 6) | ((p & 31) << 3);
        return (p === tr ? 0 : 0xff000000) | q | ((q & 14737632) >> 5);
      };
      break;
    case 782:
      px = (p) => {
        // RGB16_ARGB_1555
        p = ((p & 65280) >> 8) | ((p & 255) << 8);
        const q = ((p & 31744) << 9) | ((p & 992) << 6) | ((p & 31) << 3);
        return ((p & 32768) === 0 ? 0 : 0xff000000) | q | ((q & 14737632) >> 5);
      };
      break;
    default:
      return false;
  }
  for (let i = 0; i < n; i++) out[i] = px(s[i] & 0xffff);
  return true;
}

// PNG row filters, in place (danger/ui/PNG.line_expand_*); 0 or a negative filter leaves the row.
function unfilter(filter, row, prev, stride, bpp) {
  switch (filter) {
    case 1:
      for (let i = bpp; i < stride; i++) row[i] += row[i - bpp];
      break;
    case 2:
      for (let i = 0; i < stride; i++) row[i] += prev[i];
      break;
    case 3:
      for (let i = 0; i < bpp; i++) row[i] += prev[i] >> 1;
      for (let i = bpp; i < stride; i++) row[i] += (prev[i] + row[i - bpp]) >> 1;
      break;
    case 4:
      for (let i = 0; i < bpp; i++) row[i] += prev[i];
      for (let i = bpp; i < stride; i++) {
        const a = row[i - bpp],
          b = prev[i],
          c = prev[i - bpp];
        const p = a + b - c,
          pa = Math.abs(p - a),
          pb = Math.abs(p - b),
          pc = Math.abs(p - c);
        row[i] += pa > pb || pa > pc ? (pb <= pc ? b : c) : a;
      }
      break;
  }
}

function blend(d, s, a) {
  const ia = 255 - a;
  const r = Math.trunc((((s >> 16) & 255) * a + ((d >> 16) & 255) * ia) / 255);
  const g = Math.trunc((((s >> 8) & 255) * a + ((d >> 8) & 255) * ia) / 255);
  const b = Math.trunc(((s & 255) * a + (d & 255) * ia) / 255);
  return 0xff000000 | (r << 16) | (g << 8) | b;
}
