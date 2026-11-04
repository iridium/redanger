# websim

The Hiptop (Sidekick 3, DangerOS 3.4) simulator in the browser. A small JVM written in
JavaScript (`src/jvm/`) runs the original OS bytecode from the SDK; only the hardware layer
underneath (screen, keys, files, audio) is rebuilt, as JS natives (`src/host.js`) and a
minimal AWT in Java (`shim/`). No networking.

```sh
npm install
npm run setup     # fills public/vendor/ (gitignored) from references/ and OpenJDK 8, needs podman
npm run dev       # http://localhost:5173
npm run build     # static site in dist/ (~210 MB, ~390 files, none over 8 MB)
```

- `src/main.js` is the page: device frame (`public/skin/sidekick.webp`), hotspots, keyboard,
  Web Audio. Keys a PC keyboard lacks (`@`, `* /`, `?`, and the shoulder, volume and power buttons, which have hotspots on the photo's edges) are sent as raw Hiptop key codes. `src/worker.js` runs the VM in a Web Worker and keeps the device's files in
  IndexedDB.
- DataStores (settings, messages, the game's save) are written to `/state/datastores/` two
  seconds after any change (`shim/src/websim/StoreSaver.java`, prompted by a native on
  `DataStore.updateLastModified`), since the simulator itself only writes them at power-off;
  from there they go to IndexedDB with the rest of `/state` and load again at boot.
- The modem is answered in memory (`shim/src/danger/system/SimModemInterfaceImpl.java`) so the
  SMS stack starts and Text Messaging can load its inbox.
- Class of '09 is preinstalled: `public/vendor/apps/classof09.jar` plus the SD card
  (`public/vendor/card/`), read lazily in chunks.
- Headless: `npm run boot` boots in Node and writes `screen.png` (env `SECS`, `KEYS`,
  `TRACE`, `DEBUG_NPE`); `npm test` runs `tools/test/Hello.java` (build its jar with javac 8).
- The SD card and `rt.jar` are published as 1 MiB chunk files (`movies.c9p.0`, `.1`, …; see
  `src/chunks.js`), fetched whole as they're read, so any static host works: no range requests,
  nothing over Cloudflare Pages' 25 MiB file limit. The dev server serves the same URLs.
- Deploy to Cloudflare Pages: `npx wrangler pages deploy dist --project-name <name>`.

## Embedding

`?compact` (or `?compact=true`) renders the device on a transparent background, scaled to
fill the window, for use in an iframe. Under it is one row of controls (volume,
`[reset]`, and `[simulator]`, which opens the full page in a new tab); the logo, size picker, button
outlines toggle and console are left out. Until the first frame, the boot console is shown on the device's screen
instead. The device is 480 × 364 plus about 36 px for the controls, so give the frame an aspect
ratio of about 480 / 400:

```html
<iframe src="https://redanger.x86.cx/?compact" style="border:0; width:600px; aspect-ratio:480/400"
        allow="autoplay"></iframe>
```

Keyboard input reaches the simulator once the iframe has focus (a click on the device gives it).

Browsers block sound until the first click or key press in the page; anything already playing
then starts where it would be by now.

`?autostart` boots straight into Class of '09 instead of the launcher (`?autostart=classof09`
is the same; any other value is taken as a bundle name, e.g. `?autostart=apps.kbrowser`).
It combines with compact: `?compact&autostart`.

`?defer` loads nothing but the page and the device photo until the first click or key press
(the screen shows a power button), so an embed costs ~100 KB until someone uses it. That click
also unlocks audio. For an embed: `?compact&autostart&defer`.

## The SD card

Files are fetched lazily, a chunk at a time, only when DangerOS or the game reads them. The
game's data files are mounted under dot names (`.movies.c9p`, `snd/.12.mp3`, as
`classof09/tools/sync.sh` now writes them to a real card) because DangerOS's music library scan
reads the head and tail of every non-dot file on the card at boot; with dot names a boot reads 3
files instead of 218.
