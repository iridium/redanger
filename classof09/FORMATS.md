# Class of '09 for Sidekick 3: data formats

All game data lives in `/removable0/classof09/` on the miniSD card, read with
`RandomAccessFile`. It's an index file plus segment files, split by how often they change, so
a typical update only means copying the small index (`tools/sync.sh` skips unchanged files). Integers are
big-endian unless noted. Pixels are 16-bit `RGB16_BGR` (colour space 265, the M2
screen format: `b5 g6 r5`, blue in the high bits). The engine converts palettes at
load time if the screen reports `RGB16` (9).

The game's 1920×1080 virtual screen is scaled to the screen's height: 284×160. Sprites
are placed on that virtual stage as in Ren'Py, and the 240×160 screen shows its middle
240 px (22 px are cropped off each side). The dialogue box overlays the bottom. Movies
are not cropped: they're 240×136, centred vertically.

## C9Z: block compression

This is the [LZ4 block format](https://github.com/lz4/lz4/blob/dev/doc/lz4_Block_format.md)
with no frame header. Each block is stored as `rawLen:u32 compLen:u32 data[compLen]`.
It was chosen because the decoder is about 40 lines of Java and does only byte copies.

## C9P: pack

```
classof09.c9p (index):
"C9P3"
nScript:u16=1 nImages:u16 nSounds:u16 nClips:u16 nFiles:u8
files:   nFiles × { len:u8 name }        file 0 is the index itself (empty name)
entries: (nScript + nImages + nSounds + nClips) × { file:u8 offset:u32 length:u32 }
blobs stored in the index...
```

| File | Holds | Size |
|---|---|---|
| `classof09.c9p` | the index, the script, small UI sounds | ~0.3 MB |
| `images.c9p` | C9I images | ~3 MB |
| `audio.c9p` | ambience and menu music | ~2.5 MB |
| `voice.c9p` | scene voice tracks | ~38 MB |
| `movies.c9p` | C9V movies and their audio | ~53 MB |

- Entry 0 is the script.
- Image entries are C9I or C9V blobs; the engine tells them apart by magic, and a movie is shown like an image.
- Sound entries (ambience, music, effects, movie audio, scene voice tracks) are MP3 files.
- Clip entries: none at the moment (nClips = 0).
- A movie's audio is the sound with the same name as the movie image.
- Segment files are plain concatenations of their blobs; offsets are from the start of the file.
- **Hidden names:** on the card every data file has a leading dot (`.classof09.c9p`, `.movies.c9p`, `snd/.<n>.mp3`). DangerOS's music library scan reads the head and tail of every non-dot file on the card, so plain names cost hundreds of probes at boot and list the clips as songs. The app also accepts the plain names (older cards). The app's log (`.log.txt`) and `sync.sh`'s `.hashes.txt` are dot files for the same reason; the simulator's scan even lists unrecognized files as songs.
- **Sound files:** every sound is also copied to the card as `snd/<n>.mp3` (`tools/unpack_sounds.py`, run by `sync.sh`). The app streams these by file name through the native MP3 player, so no clip has to fit in the Java heap; the copies in the pack are the fallback.

## C9S: script

```
"C9S1" startPc:u32
strings:  n:u16 { len:u16 utf8 }      dialogue and menu text
chars:    n:u16 { len:u16 utf8 }      speaker names
images:   n:u16 { len:u16 utf8 }      image names (index = pack image index)
sounds:   n:u16 { len:u16 utf8 }      sound names (index = pack sound index)
atl:      n:u16 { len:u16 program }
endings:  n:u8 { id:u8 sender:u16 preview:u16 body:u16 }   the ending tracker's texts (strings)
bonus:    pc:u32 tip:u16           scene_0999's entry (unlocked by all endings) and the tracker's tip text (not shown)
code:     len:u32 bytecode
auto:     n:u16 { start:u32 ambient:u16 fast:u8 stop:u32 nChoices:u8 choices[nChoices]:u8 }
```

`auto` is Auto Play's route, worked out by `tools/auto_plan.py`. Each segment restarts at
the label `start` with that ambience (0xFFFF = none). If `fast`, it fast-forwards to the
first menu, then plays normally, taking `choices` at the menus it meets. It ends when it
reaches `stop` (0xFFFFFFFF = none: it runs to a RETURN, i.e. an ending). Identical menus
count as one. A flavor menu (every option reaches the same endings) plays its options back
to back up to where they rejoin; a branch menu has each option played to its end. A path
stops at a scene that has already played or at a menu already explored.

Bytecode ops (pc = byte offset into code):

| op | name | operands | meaning |
|----|------|----------|---------|
| 1 | SCENE | img:u16 layer:u8 atl:u16 | clear layer; show img (0xFFFF = none) |
| 2 | SHOW | tag:u16 img:u16 layer:u8 atl:u16 | show/replace tag; atl 0xFFFF keeps the running transform |
| 3 | HIDE | tag:u16 layer:u8 | |
| 4 | PLAY | chan:u8 snd:u16 fadeinMs:u16 | channel 0 = ambient (loops) |
| 5 | STOP | chan:u8 fadeoutMs:u16 | |
| 6 | VOICE | snd:u16 | start the scene's voice track; it plays straight through |
| 7 | SAY | char:u8 str:u16 beginMs:u32 endMs:u32 | show the line; it ends when the voice track reaches endMs (re-seek to beginMs if off by > 0.6 s; skipping seeks to endMs) |
| 8 | MENU | n:u8 { str:u16 pc:u32 } | |
| 9 | JUMP | pc:u32 | |
| 10 | ENDING | id:u8 | record that ending `id` was reached; a new one unlocks its message and notifies on return to the title |
| 11 | PAUSE | ms:u32 | |
| 12 | RETURN | | back to the main menu |
| 13 | LABEL | | scene start; the engine autosaves here |
| 14 | TEXT | sender:u16 body:u16 | an in-story text arrives; the engine ignores it (the scene's art shows the text) |

ATL programs are a list of instructions ended by 0:

| op | name | operands |
|----|------|----------|
| 1 | SET | n:u8 { prop:u8 value:s16 } |
| 2 | LINEAR | ms:u16 n:u8 { prop:u8 value:s16 } |
| 3 | PAUSE | ms:u16 |
| 4 | PARALLEL | n:u8 { len:u16 program } |
| 5 | IMAGE | img:u16 |

Props: 0 xalign, 1 yalign, 2 alpha, 3 zoom, 4 xzoom (all ×1000), 5 rotate (degrees ×10).
The placement follows Ren'Py: `x = xalign × (stageW − w·|xzoom·zoom|)`, and the same for y.
New sprites start at xalign 0.5, yalign 1.0.

## C9I: image

```
"C9I1" w:u16 h:u16 flags:u8 ncolors:u16 palette[ncolors]:u16 indices:C9Z(w*h bytes)
```

- **flags:** bit 0 means palette index 0 is transparent; bit 1 means solid (fill the whole stage with palette[0]).
- **Encoding:** images are scaled by 160/1080, then quantised to ≤256 colours per image with light Floyd–Steinberg dithering.
- **Alpha:** 1-bit (threshold 50%).

## Audio: MP3

All audio is MP3, played by the device's native decoder (`danger.audio.Stream.loadFromFile`), so the CPU only reads bytes off the card.

- **Format:** MPEG-2 Layer III, 22.05 kHz mono, constant bitrate. Voice, ambience and movie audio are 32 kbps; menu music is 48 kbps.
- **Tags:** no ID3 or Xing headers.
- **Voice tracks:** each scene's track stays whole and plays continuously, like the original, because it carries room tone between lines. Text is timed to the track's playback position, using the script's `setWait(begin, end)` marks.
- **Ambience:** trimmed to ≤ 20 s, crossfaded at the loop seam, and played looping.

## C9V: video

```
"C9V2" | "C9VA" w:u16 h:u16 fpsX100:u16 nFrames:u32 frames...
frame: C9Z(payload)
payload: modes[ceil(nBlocks/4)]          2 bits per 4x4 block, MSB first
         nSolid:u16 nTwo:u16 nEight:u16
         solid[nSolid]  { c:u16 }
         two[nTwo]      { c0:u16 c1:u16 mask:u16 }
         eight[nEight]  { (c0:u16 c1:u16) x 4 quadrants, mask:u16 }
```

- **Block modes:** 0 = skip (keep the previous frame), 1 = solid, 2 = two colours, 3 = eight colours.
- **Masks:** a set bit picks c1, and pixel 0 is the MSB. In mode 3 each 2×2 quadrant (TL, TR, BL, BR) has its own colour pair and 4 mask bits, in that order.
- **Encoder:** picks the cheapest mode whose squared error is under a threshold, so this is MS Video 1-style block coding.
- **Decoding:** only bit tests and stores. Two-colour blocks keep subtitle text sharp.
- **Alpha (`C9VA`):** movies with a WebM alpha channel (overlays such as the pill pop) mark clear pixels with the transparent key `0x0020`. A fully clear block is a solid key; a partly clear block uses mode 3 with the key as `c0` in each quadrant that has clear pixels. The engine skips key pixels, so an overlay only costs its visible part. Blocks are only skipped when their clear/visible pattern is unchanged.
- **Format:** 8 fps at 240×136, centred vertically on the 160 px screen (about 55 KB/s).
- **Audio:** the MP3 sound with the same name as the movie, played whole alongside it.
