#!/bin/sh
# Compiles the AWT shim + host bridge into public/vendor/websim.jar (in podman, OpenJDK 8).
set -e
cd "$(dirname "$0")/.."
cp -L public/vendor/library.jar public/vendor/simulator.jar shim/
podman run --rm --userns=keep-id -v "$PWD/shim":/s -w /s localhost/redanger-sdk sh -c '
  rm -rf out && mkdir out &&
  javac -nowarn -source 1.8 -target 1.8 -encoding UTF-8 -cp library.jar:simulator.jar -d out $(find src -name "*.java") &&
  cd out && jar cf ../websim.jar .'
mv shim/websim.jar public/vendor/websim.jar
rm -f shim/library.jar shim/simulator.jar
echo built public/vendor/websim.jar
