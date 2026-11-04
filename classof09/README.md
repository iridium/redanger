# Class of '09 for the Sidekick 3

A port of *Class of '09* (Ren'Py, 1080p) to the T-Mobile Sidekick 3 (Hiptop 3 / M2: 240×160 screen, DangerOS 3.4, ARM9).
It is a small Java engine that replays the game's script, art, voice acting and movies from custom compressed formats on the miniSD card.

No game content is in this repository. You build the data pack from your own copy of the game.

## Layout

- `app/`: the DangerOS app (Java 1.3, SDK 3.4). It contains the script VM, the ATL animation interpreter, the software compositor, the C9I/C9V decoders, and native MP3 playback.
- `FORMATS.md`: the pack, script, image, audio and video formats.
- `tools/`: the asset pipeline, which runs in podman. `build.sh` fetches [unrpyc](https://github.com/CensoredUsername/unrpyc) at a pinned commit to decompile `script.rpyc`. `compile_script.py` turns the script into C9S bytecode, and `build_pack.py` encodes the art, audio and movies and writes the pack.
- `out/` (gitignored): the generated pack.

## Building

1. **Data pack.** Needs a copy of the game; it runs in podman. It writes the pack files to `out/`, about 98 MB in total, split into an index plus images, audio, voice and movies files.

   ```bash
   classof09/tools/build.sh "/path/to/Class of '09/Class_Of_09-1.2"
   ```

2. **App.** Built with the SDK 3.4 helper from `references/sdk`. It writes `app/work/classof09.bndl` and `app/work/classof09.jar`.

   ```bash
   SDK_VERSION=3.4 references/sdk/build-app.sh classof09/app
   ```

3. **Device.** With the card mounted over USB, run the command below. It copies the pack to `/classof09/` on the card, skipping files that haven't changed: after the first copy, most updates only copy the 0.3 MB index. Install the `.bndl` with `htconsole`, which needs a developer key on the device.

   ```bash
   classof09/tools/sync.sh /run/media/$USER/<card>
   ```

## Simulator

Install the pack on the sim's virtual card with `classof09/tools/sync.sh references/sdk/state-3.4/removable0`, then run:

```bash
SDK_VERSION=3.4 SIM_JAVA_OPTS=-Dcom.danger.autoboot=com.redanger.classof09 references/sdk/sim.sh classof09/app/work/classof09.jar
```

The sim has no sound on Linux, because its audio needs a native library the SDK only ships for Windows.

## Controls

- **Advance, skip the line, or choose:** trackball click, or Enter in the sim.
- **Move through menus:** trackball up/down, or the arrow keys in the sim.
- **Back to the title:** the Back key, or Home in the sim. The game autosaves at the start of every scene, and Continue resumes there.
- **Auto-lock:** while a scene plays or Auto Play runs, the game window blocks the keyguard (`Window.blocksKeyGuard()`, the same override the registration screens use), so the phone doesn't auto-lock. On the title and Messages screens it locks as usual. The screen itself stays on while the game is open.
- **Sim key map:** Jump is F8, Menu F7, Back F10/Home and wheel click F9/End. `references/sdk/simkey.sh` sends these by name, e.g. `simkey.sh jump down click`.

## How the port maps Ren'Py

- **Dialogue:** each line is a `speak()` with `setWait(begin, end)` marks into a per-scene voice track. The track plays continuously through the device's native MP3 decoder, and each line advances when playback reaches its end mark, as in the original. Skipping seeks the track forward instead of stopping it.
- **Animation:** `show`/`scene` transforms (linear, pause, parallel, align, zoom, xzoom flip, alpha, rotate) run on a small ATL interpreter. Statements chain on scheduled times, so long moves stay in sync with the voice.
- **Graphics:** art is scaled to the screen's 160 px height (a 284×160 stage), and the middle 240 px are shown. It's composited in software into a `ShortBitmap` in the screen's native RGB16_BGR. Movies keep their full width and are centred vertically.
- **Movies** (splash, opening, 15 endings) use a block codec built for cheap decoding (see FORMATS.md). Their audio is one MP3, played alongside.
- **Texts:**
  - **Story texts** (Crispin, Coach Colby, Kylar) show the game's phone pictures, as in the original.
  - **Ending texts:** reaching a new ending raises a real Sidekick notification when you're returned to the title: the system title bar scrolls "New message from …" with the text alert sound.
  - **Messages:** an ending's text goes into the phone's real Text Messaging inbox, and Messages on the title menu opens Text Messaging. That means writing to the SMS app's DataStore, which third-party apps aren't allowed to open, so it goes through reflection (`Sms.java`). The browser simulator allows it; a real phone refuses, and the game then keeps the ending tracker as its own inbox on the title menu (sender and preview per row, and a full-screen reader). The original draws it on a PDA graphic; here the phone's own screen is the phone. The bonus video, once every ending is found, is at the end of the Endings list.
- **Auto Play** (title menu) walks the whole choice tree on its own. It takes each menu's options in turn, plays each path through its ending, and returns to the title (with the ending's notification) between paths. Then it backs up to the latest menu with an untried option and replays that scene up to it. After all 102 options it plays the hidden outro (scene_0999). Progress is saved: "Auto Play (resume x/y)" continues a run, and a run interrupted (by leaving the game, say) picks up when you come back. Any key on the title between paths, or Back during a scene, stops it.
- **Not ported:** the original's desktop-file ending easter eggs, music/ambience fades (they cut instead), and the pause menu (Back goes to the title).
