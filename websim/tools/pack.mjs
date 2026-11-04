// Builds public/vendor/rt-pack.dat (gzipped; not .gz, or servers add Content-Encoding): the JDK classes DangerOS uses (from a boot trace,
// tools/rt-trace.txt, plus whole core packages), so the browser doesn't fetch them one by one.
// Anything else is read lazily out of rt.jar with HTTP range requests.
import { readFileSync, writeFileSync } from "node:fs";
import { inflateRawSync, gzipSync } from "node:zlib";
import { Jar } from "../src/jvm/zip.js";

const rt = new Jar(
  new Uint8Array(readFileSync("public/vendor/rt.jar")),
  (b) => new Uint8Array(inflateRawSync(b)),
);
const traced = new Set(
  readFileSync("tools/rt-trace.txt", "utf8")
    .split("\n")
    .filter(Boolean)
    .map((n) => n + ".class"),
);
const core =
  /^(java\/(lang|util|io|text|math)\/[^/]+\.class|sun\/util\/(calendar|locale\/provider)\/[^/]+\.class|sun\/nio\/cs\/(UTF_8|ISO_8859_1|US_ASCII|StreamEncoder|StreamDecoder)[^/]*\.class)$/;
const chunks = [];
let n = 0,
  bytes = 0;
const u32 = (v) => {
  const b = Buffer.alloc(4);
  b.writeUInt32LE(v);
  return b;
};
for (const name of rt.entries.keys()) {
  if (!name.endsWith(".class")) continue;
  if (!traced.has(name) && !core.test(name)) continue;
  const data = rt.get(name);
  const nb = Buffer.from(name);
  chunks.push(u32(nb.length), nb, u32(data.length), Buffer.from(data));
  n++;
  bytes += data.length;
}
const out = gzipSync(Buffer.concat(chunks), { level: 9 });
writeFileSync("public/vendor/rt-pack.dat", out);
writeFileSync(
  "public/vendor/rt.json",
  JSON.stringify({ size: readFileSync("public/vendor/rt.jar").length }),
);
console.log(
  `packed ${n} classes, ${(bytes / 1e6).toFixed(1)} MB -> ${(out.length / 1e6).toFixed(1)} MB gz`,
);
