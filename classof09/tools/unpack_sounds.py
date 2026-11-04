#!/usr/bin/env python3
"""Write every sound in the pack as its own file, out/snd/<n>.mp3 (see FORMATS.md).

The app streams these by file name through the native MP3 player, so a scene's
voice track never has to be loaded into the Java heap. The copies inside the
pack stay as a fallback. Files whose contents haven't changed are left alone.

Usage: unpack_sounds.py build_dir
"""
import os, struct, sys

build = sys.argv[1]
index = open(os.path.join(build, 'classof09.c9p'), 'rb')
head = index.read(13)
if head[:4] != b'C9P3':
    sys.exit('not a C9P3 pack')
n_script, n_images, n_sounds, n_clips = struct.unpack('>HHHH', head[4:12])
names = [index.read(index.read(1)[0]).decode() for _ in range(head[12])]
n = n_script + n_images + n_sounds + n_clips
table = index.read(n * 9)
files = {}
out = os.path.join(build, 'snd')
os.makedirs(out, exist_ok=True)
written = 0
for snd in range(n_sounds):
    e = 1 + n_images + snd
    f, off, length = struct.unpack('>bii', table[e * 9:e * 9 + 9])
    if f not in files:
        files[f] = index if f == 0 else open(os.path.join(build, names[f]), 'rb')
    files[f].seek(off)
    data = files[f].read(length)
    path = os.path.join(out, f'{snd}.mp3')
    if os.path.exists(path) and open(path, 'rb').read() == data:
        continue
    open(path + '.tmp', 'wb').write(data)
    os.replace(path + '.tmp', path)
    written += 1
# drop files for sounds that no longer exist
for name in os.listdir(out):
    if name.endswith('.mp3') and int(name[:-4]) >= n_sounds:
        os.remove(os.path.join(out, name))
print(f'snd/: {n_sounds} sounds, {written} written')
