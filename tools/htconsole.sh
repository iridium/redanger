#!/usr/bin/env bash
# Run Danger's htconsole on a modern Linux kernel.
#
# htconsole talks to the hiptop through the old usbfs layout (/proc/bus/usb/BBB/DDD and
# /proc/bus/usb/devices), which Linux removed around 3.5. This recreates that layout inside a
# private mount namespace from /dev/bus/usb and debugfs, so nothing changes on the host and it
# all disappears when htconsole exits. Needs root for the mounts and the USB device nodes.
#
# usage: tools/htconsole.sh /path/to/hiptop-sdk/tools/linux/htconsole
#        HTCONSOLE=/path/to/htconsole tools/htconsole.sh
HT=${1:-$HTCONSOLE}
if [ -z "$HT" ] || [ ! -x "$HT" ]; then
    echo "usage: $0 /path/to/htconsole   (the Linux htconsole from the Hiptop SDK, tools/linux/htconsole)" >&2
    exit 1
fi
HT=$(realpath "$HT")

exec sudo unshare -m bash -c '
    set -e
    mount --make-rprivate /
    mount -t tmpfs none /proc/bus
    mkdir /proc/bus/usb
    for b in /dev/bus/usb/*; do
        mkdir "/proc/bus/usb/${b##*/}"
        mount --bind "$b" "/proc/bus/usb/${b##*/}"
    done
    mountpoint -q /sys/kernel/debug || mount -t debugfs none /sys/kernel/debug
    touch /proc/bus/usb/devices
    mount --bind /sys/kernel/debug/usb/devices /proc/bus/usb/devices
    cd "$1"
    export TERM=xterm   # its old static ncurses cannot read modern entries like xterm-256color
    exec "$2"' _ "$PWD" "$HT"
