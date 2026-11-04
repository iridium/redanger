#!/bin/sh
# Build classof09.c9p from your own copy of the game. Everything runs in podman.
# Usage: tools/build.sh "/path/to/Class_Of_09-1.2"     (output: classof09/out/, app icons: classof09/app/assets/)
# Env: ONLY=images|sounds|movies, JOBS=n (passed to build_pack.py)
set -e
here=$(cd "$(dirname "$0")" && pwd)
game=$(realpath "${1:?usage: build.sh <game dir containing game/>}")/game
out=$here/../out
mkdir -p "$out"
podman image exists classof09-conv || podman build -t classof09-conv "$here"
[ -d "$here/unrpyc" ] || git clone -q https://github.com/CensoredUsername/unrpyc.git "$here/unrpyc"
git -C "$here/unrpyc" checkout -q 3ae8334
run() { podman run --rm --userns=keep-id -e ONLY -e JOBS -v "$game":/g:ro -v "$out":/o -v "$here":/t:ro classof09-conv "$@"; }
if [ ! -f "$out/rpy/screens.rpy" ]; then
  mkdir -p "$out/rpy" && cp "$game/script.rpyc" "$game/screens.rpyc" "$out/rpy/"
  run python /t/unrpyc/unrpyc.py /o/rpy >/dev/null
fi
mkdir -p "$here/../app/assets"
podman run --rm --userns=keep-id -v "$game":/g:ro -v "$here/../app/assets":/a -v "$here":/t:ro classof09-conv python /t/make_icons.py /g /a
run python /t/compile_script.py /o/rpy/script.rpy /g/images /o
run python /t/build_pack.py /g /o
