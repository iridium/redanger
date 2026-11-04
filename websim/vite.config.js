import {
  closeSync,
  copyFileSync,
  createReadStream,
  existsSync,
  mkdirSync,
  openSync,
  readdirSync,
  readSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { join, relative, resolve } from "node:path";
import { defineConfig } from "vite";
import { CHUNK, isLazy } from "./src/chunks.js";

// public/vendor/ links to the SDK jars and SD card under references/. Vite would copy those
// links as links, so the static build copies public/ itself with links resolved, cutting the
// lazily read files into chunks (see src/chunks.js).
function copyResolved(from, to, root) {
  const st = statSync(from);
  if (st.isDirectory()) {
    mkdirSync(to, { recursive: true });
    for (const name of readdirSync(from)) copyResolved(join(from, name), join(to, name), root);
  } else if (isLazy(relative(root, from)) && st.size > CHUNK) {
    const fd = openSync(from, "r");
    const buf = Buffer.alloc(CHUNK);
    for (let i = 0, pos = 0; pos < st.size; i++, pos += CHUNK) {
      const n = readSync(fd, buf, 0, CHUNK, pos);
      writeFileSync(`${to}.${i}`, buf.subarray(0, n));
    }
    closeSync(fd);
  } else {
    copyFileSync(from, to);
  }
}

const copyPublicResolved = {
  name: "copy-public-resolved",
  apply: "build",
  closeBundle() {
    copyResolved(resolve("public"), resolve("dist"), resolve("public"));
  },
};

// The dev server serves the same chunk URLs by slicing the original files.
const serveChunks = {
  name: "serve-chunks",
  configureServer(server) {
    server.middlewares.use((req, res, next) => {
      const m = /^\/(vendor\/.+)\.(\d+)$/.exec(decodeURIComponent((req.url || "").split("?")[0]));
      const file = m && resolve("public", m[1]);
      if (!m || !isLazy(m[1]) || !existsSync(file)) return next();
      const start = Number(m[2]) * CHUNK,
        end = Math.min(statSync(file).size, start + CHUNK) - 1;
      if (start > end) return next();
      res.setHeader("Content-Type", "application/octet-stream");
      res.setHeader("Content-Length", end - start + 1);
      createReadStream(file, { start, end }).pipe(res);
    });
  },
};

export default defineConfig({
  base: "./",
  publicDir: "public",
  build: { copyPublicDir: false, target: "es2022" },
  worker: { format: "es" },
  server: { fs: { strict: false } },
  plugins: [copyPublicResolved, serveChunks],
});
