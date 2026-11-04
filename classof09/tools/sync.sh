#!/bin/sh
# Copy the pack to a memory card, skipping files that are already up to date.
# Usage: tools/sync.sh <card root>     e.g. tools/sync.sh /run/media/$USER/SIDEKICK
# The card keeps classof09/.hashes.txt (sha1 of each pack file as last copied),
# so unchanged files (usually movies.c9p and voice.c9p) aren't copied again.
# It also writes each sound as snd/<n>.mp3 (the app streams these by name) and
# copies those the same way.
# On the card every data file gets a leading dot (.movies.c9p, snd/.12.mp3): the
# media player's library scan skips dot files, so it doesn't probe the game data.
set -e
here=$(cd "$(dirname "$0")" && pwd)
out=$here/../out
dest=${1:?usage: sync.sh <card root>}/classof09
mkdir -p "$dest/snd"
touch "$dest/.hashes.txt"
python3 "$here/unpack_sounds.py" "$out"
copied=0
for f in "$out"/*.c9p "$out"/snd/*.mp3; do
  plain=${f#"$out"/}
  case $plain in */*) name=${plain%/*}/.${plain##*/} ;; *) name=.$plain ;; esac
  sum=$(sha1sum "$f" | cut -d' ' -f1)
  if grep -qx "$sum  $name" "$dest/.hashes.txt" && [ -f "$dest/$name" ]; then
    echo "  up to date  $name"
    continue
  fi
  echo "  copying     $name ($(du -h "$f" | cut -f1))"
  cp "$f" "$dest/$name.tmp" && mv "$dest/$name.tmp" "$dest/$name"
  grep -vx ".*  $name" "$dest/.hashes.txt" > "$dest/.hashes.new" || true
  echo "$sum  $name" >> "$dest/.hashes.new"
  mv "$dest/.hashes.new" "$dest/.hashes.txt"
  copied=1
done
[ "$copied" = 1 ] && sync
echo done
