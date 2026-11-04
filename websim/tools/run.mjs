// Node runner: node run.mjs <main class> [jar...]  (rt.jar is always first)
import { readFileSync } from "node:fs";
import { inflateRawSync } from "node:zlib";
import { Jar } from "../src/jvm/zip.js";
import { createVM, bootVM, runMain } from "../src/jvm/boot.js";

const [main, ...jarPaths] = process.argv.slice(2);
const jars = ["public/vendor/rt.jar", ...jarPaths].map(
  (p) => new Jar(new Uint8Array(readFileSync(p)), (b) => new Uint8Array(inflateRawSync(b))),
);
const vm = createVM({ jars, log: (s) => console.error(s) });
const t0 = performance.now();
const t = bootVM(vm);
console.error(
  `[run] booted JVM in ${(performance.now() - t0).toFixed(0)} ms, ${vm.classes.size} classes`,
);
runMain(vm, t, main);
const loop = () => {
  const wait = vm.runSlice(50);
  if (wait === Infinity) {
    console.error(
      `[run] done in ${(performance.now() - t0).toFixed(0)} ms, ${vm.classes.size} classes`,
    );
    return;
  }
  setTimeout(loop, wait);
};
loop();
