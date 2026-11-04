// Writes public/vendor/card-manifest.json: the files of the simulated SD card (public/vendor/card/)
// with their sizes. The browser mounts them lazily and reads them with HTTP range requests.
//
// Class of '09's data files are mounted under dot names (.movies.c9p, snd/.12.mp3), as
// classof09/tools/sync.sh puts them on a real card: the media player's library scan skips dot
// files, so it doesn't fetch hundreds of game files at boot. URLs keep the plain names, since
// many static servers refuse to serve dot files. The card's sync bookkeeping and log are left out.
import { readdirSync, statSync, writeFileSync } from "node:fs";

const SKIP = new Set([
  "classof09/hashes.txt",
  "classof09/log.txt",
  "classof09/.hashes.txt",
  "classof09/.log.txt",
]);
const hide = (rel) =>
  /^classof09\/(.*\/)?[^/.][^/]*\.(c9p|mp3)$/.test(rel) ? rel.replace(/([^/]+)$/, ".$1") : rel;

const files = [];
const walk = (dir, rel) => {
  for (const name of readdirSync(dir)) {
    const p = dir + "/" + name,
      r = rel ? rel + "/" + name : name;
    const st = statSync(p);
    if (st.isDirectory()) walk(p, r);
    else if (!SKIP.has(r)) files.push({ path: hide(r), url: r, size: st.size });
  }
};
walk("public/vendor/card", "");
writeFileSync("public/vendor/card-manifest.json", JSON.stringify(files));
console.log(
  `${files.length} card files, ${(files.reduce((a, f) => a + f.size, 0) / 1e6).toFixed(1)} MB`,
);
