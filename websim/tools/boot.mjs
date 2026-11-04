// Node: boot the 3.4 DangerOS simulator headless on the JS JVM.
import { readFileSync } from "node:fs";
import { inflateRawSync } from "node:zlib";
import { Jar } from "../src/jvm/zip.js";
import { createVM, bootVM, runMain } from "../src/jvm/boot.js";
import { makeHost } from "../src/host.js";
import { writeFileSync } from "node:fs";
import { deflateSync } from "node:zlib";

let firstFrame = null;
const { host, natives: hostNatives } = makeHost({
  present: () => {
    if (firstFrame === null) firstFrame = Math.round(performance.now() - t0);
  },
});

const jars = [
  "public/vendor/websim.jar",
  "public/vendor/rt.jar",
  "public/vendor/library.jar",
  "public/vendor/simulator.jar",
].map((p) => new Jar(new Uint8Array(readFileSync(p)), (b) => new Uint8Array(inflateRawSync(b))));
const t0 = performance.now();
const vm = createVM({
  jars,
  log: (s) => console.error(s),
  natives: hostNatives,
  props: {
    "user.dir": "/state",
    "com.danger.username": "redanger",
    "com.danger.service_panel": "true",
    "com.danger.os_version": "3.4",
    "danger.storage.root": "/state/removable0",
    "com.danger.flash.settings_file": "/state/FLASH.Settings.out",
    "com.danger.ATInterface.ModemPort": "1",
  },
});
vm.debugNPE = !!process.env.DEBUG_NPE;
vm.vfs.mkdirs("/state/removable0");
vm.vfs.cwd = "/state";
for (const f of ["tzdb.dat", "currency.data", "calendars.properties"])
  vm.vfs.write("/jre/lib/" + f, new Uint8Array(readFileSync("public/vendor/jre-lib/" + f)));
for (const f of ["FLASH.Settings.out", "DangerM2SimulatorProperties"])
  vm.vfs.write("/state/" + f, new Uint8Array(readFileSync("public/vendor/state/" + f)));
const t = bootVM(vm);
vm.fastForward = true; // skip boot-time sleeps until the first frame
runMain(vm, t, "websim.Main");
// scripted keys: KEYS="2000:118,4000:40" (ms:awtKeyCode) press+release
for (const spec of (process.env.KEYS || "").split(",").filter(Boolean)) {
  const [ms, code] = spec.split(":").map(Number);
  setTimeout(() => {
    host.pushKey(401, code, 0xffff);
    host.pushKey(402, code, 0xffff);
  }, ms);
}
const limit = Number(process.env.SECS || 20) * 1000;
const loop = () => {
  const wait = vm.runSlice(50);
  const el = performance.now() - t0;
  if (wait === Infinity || el > limit) {
    console.error(`[boot] stopped after ${el.toFixed(0)} ms, ${vm.classes.size} classes, threads:`);
    for (const th of vm.threads) console.error(`   ${th.name} ${th.state} @ ${vm.where(th)}`);
    console.error(`[boot] ${host.frames} frames presented, first at ${firstFrame} ms`);
    if (process.env.TRACE) {
      const rt = jars[1];
      const names = [...vm.classes.keys()].filter(
        (n) => !n.startsWith("[") && !n.startsWith("#") && rt.has(n + ".class"),
      );
      writeFileSync(process.env.TRACE, names.join("\n") + "\n");
      console.error(`[boot] traced ${names.length} rt classes`);
    }
    if (host.screen) writeFileSync(process.env.PNG || "screen.png", png(host.screen));
    process.exit(0);
  }
  setTimeout(loop, Math.min(wait, 50));
};
loop();

function png({ pixels, width, height }) {
  const raw = Buffer.alloc((width * 3 + 1) * height);
  for (let y = 0; y < height; y++) {
    raw[y * (width * 3 + 1)] = 0;
    for (let x = 0; x < width; x++) {
      const v = pixels[y * width + x],
        o = y * (width * 3 + 1) + 1 + x * 3;
      raw[o] = (v >> 16) & 255;
      raw[o + 1] = (v >> 8) & 255;
      raw[o + 2] = v & 255;
    }
  }
  const crcT = Array.from({ length: 256 }, (_, n) => {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    return c >>> 0;
  });
  const crc = (b) => {
    let c = 0xffffffff;
    for (const x of b) c = crcT[(c ^ x) & 255] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  };
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const c = Buffer.alloc(4);
    c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}
