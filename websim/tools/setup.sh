#!/bin/sh
# Populates public/vendor/ (gitignored) with everything the simulator loads:
#   library.jar, simulator.jar   DangerOS 3.4 from the SDK (references/, not redistributable)
#   rt.jar, rt-pack.dat        OpenJDK 8 class library, and the subset DangerOS boots with
#   jre-lib/                      tzdb.dat etc. from the same JRE
#   websim.jar                    our AWT shim + host bridge (shim/build.sh)
#   state/                        seed FLASH.Settings.out + simulator prefs
#   apps/, card/                  preinstalled apps and the SD card (Class of '09 + its data)
# Needs podman with the localhost/redanger-sdk image (OpenJDK 8), see references/sdk/README.md.
set -e
here=$(cd "$(dirname "$0")/.." && pwd)
repo=$(cd "$here/.." && pwd)
v=$here/public/vendor
sdk=$repo/references/sdk
mkdir -p "$v/apps" "$v/jre-lib" "$v/state"

ln -sfn "$sdk/hiptop/x/hiptop-sdk/libs/3.4/155053/library.jar" "$v/library.jar"
ln -sfn "$sdk/hiptop/x/hiptop-sdk/libs/3.4/simulator.jar" "$v/simulator.jar"
ln -sfn "$sdk/state-3.4/removable0" "$v/card"
[ -f "$repo/classof09/app/work/classof09.jar" ] && ln -sfn "$repo/classof09/app/work/classof09.jar" "$v/apps/classof09.jar"
for f in FLASH.Settings.out DangerM2SimulatorProperties; do cp "$sdk/state-3.4/$f" "$v/state/$f"; done

if [ ! -f "$v/rt.jar" ]; then
  podman run --rm --userns=keep-id -v "$v":/o localhost/redanger-sdk sh -c \
    'j=/usr/lib/jvm/java-8-openjdk/jre/lib; cp $j/rt.jar /o/ && cp $j/tzdb.dat $j/currency.data $j/calendars.properties /o/jre-lib/'
fi

"$here/shim/build.sh"
cd "$here"
node tools/pack.mjs
node tools/card.mjs
