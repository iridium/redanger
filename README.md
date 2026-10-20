<p align="center">
   <img src="screenshots/logo.svg" alt="Logo" height=130 />
   <img src="screenshots/aim.png" alt="AIM Screenshot" />
</p>

*Re:Danger* is a revival project for Danger Hiptop and T-Mobile Sidekick devices. Since the original DangerOS servers were shut down over 15 years ago, these devices have been unable to get past the login screen. This is a clean-room reimplementation of that backend.

## Features

- [x] Server time sync
- [x] Discord bridge (Discord channels appear as AIM chat rooms with real-time two-way messaging)
- [x] Web browser (Kimba). Kind of works but very broken. Based on Chrome.
- [ ] Email support
- [ ] OSCAR support to connect to AIM servers
- [ ] Download Catalog
- [ ] Photo Album Sync
- [x] Development key generator for real devices (see [Developer mode](#developer-mode))

## Setup

Requires Go 1.25+.

```
cd server
cp .env.example .env   # don't forget to edit the file!
go run .
```

### Environment variables

| Variable           | Description                                                               |
| ------------------ | ------------------------------------------------------------------------- |
| `DISCORD_TOKEN`    | Discord bot token. If unset, the server runs in demo mode.                |
| `DISCORD_GUILD_ID` | The Discord server (guild) whose text channels are exposed as chat rooms. |

### Simulator

You'll need a copy of the [Hiptop SDK](https://archive.org/details/hiptop_sdk).

1. **Redirect the simulator to localhost.** The simulator connects to `disp.trial.danger.com` by default. Add the following line to your `/etc/hosts` file:

   ```
   127.0.0.1  disp.trial.danger.com
   ```

2. **Launch the simulator:**

   ```
   java "-Dcom.danger.username=redanger" "-Dcom.danger.service_panel=true" \
     -cp "libs/3.4/155053/library.jar;libs/3.4/simulator.jar" danger.Boot
   ```

3. **Disable the service redirector.** After the first launch, a file called `FLASH.Settings.out` will be created in the working directory. Add this line to the end of it (right before the `;;`) and restart the simulator:

   ```
   SVCREDIR:=0;;
   ```

4. **Log in.** A login prompt should appear and enter any password. If everything is working, the device clock will sync and you'll be connected like it's 2005!

## Developer mode

Danger let registered developers put their own phones into developer mode with a per-device *developer key*: a small config file, locked to one IMEI and signed by Danger. A phone in developer mode accepts apps over USB and turns on `htconsole` debugging. Danger stopped issuing keys long ago, so we recovered the signing key instead.

### The key

Developer keys are signed by the "Danger Developer Program" certificate (`swetland@danger.com`), a 512-bit RSA key that is a trusted root on every device we have checked (Hiptop 1, Hiptop 2 and Hiptop 3). We took the certificate from the Sidekick II factory OS images (`fac_*_prod_os*.sq`) in Danger's ServiceProgrammer, preserved on archive.org as [Danger Hiptop2 Firmware](https://archive.org/details/danger_hiptop2_firmware). Danger's sample `null-device-key.sq`, from the [developer.danger.com archive](https://archive.org/details/danger-hiptop), confirmed it is the key that signs developer keys. In October 2026 we factored its modulus with [CADO-NFS](https://gitlab.inria.fr/cado-nfs/cado-nfs), taking about 8.5 hours on 220 CPU cores (436 threads) across seven machines:

```
n = 11576385713948644711688861097066027618113758646736035007798668946285132955940594207257263351308819380417615195922932748143994263878857883771956785119149869
p = 104491666572898294264047880744514595625924491114579942567545554396922436316211
q = 110787645499676419929932206845243986470558771418524609420578927381066743957279
e = 65537
```

As a check, re-signing Danger's own `null-device-key.sq` with this key reproduces their original file byte for byte.

### Making a key

Open [`devkey.html`](devkey.html) in a browser. It runs entirely offline, with the key above prefilled. Pick your device, enter the phone's IMEI (dial `*#06#` or look in Settings > System Info) and click **Build and download**.

- **Hiptop 1 / Hiptop 2** get `devos_key_IMEI.sq`, Danger's original dev key format.
- **Hiptop 3** gets `devkey_IMEI.jar`, a signed recovery package that installs the same config.
- **Hiptop 3: remove the developer key** gets `devkey_undo.jar`, which takes a Hiptop 3 back out of developer mode.

The optional **dispatcher host** adds a `dispatcher` setting to the config, which should point the phone at a different service host (for example a Re:Danger server). It is untested.

### Installing it on a Hiptop 3

You need `htconsole` from the [Hiptop SDK](https://archive.org/details/hiptop_sdk) (`tools/linux/htconsole`). It is a 32-bit Linux binary from 2007 that expects the old `/proc/bus/usb` interface, which modern kernels no longer have. [`tools/htconsole.sh`](tools/htconsole.sh) recreates it inside a private mount namespace (nothing on your system changes) and runs htconsole with `sudo`:

```
tools/htconsole.sh /path/to/hiptop-sdk/tools/linux/htconsole
```

1. **Boot into recovery.** Power the phone on while holding **W**, and connect it over USB. In normal mode a production Hiptop 3 only shows up as USB storage, because htconsole's interface stays off until a developer key is installed. Recovery always has it.
2. **Flash the key** at the htconsole prompt:

   ```
   flash devkey_IMEI.jar
   ```

   A good install ends with `verified all 2 files in installation pkg` and `Install_Copy: /install/config/system-overrides.ipc /flash/config/system-overrides.ipc -> 0`.
3. **Reboot** normally (`reboot`). The boot log should show `Config: Override w/key IMEI found.` and `Config: DEVELOPER BUILD`, and htconsole now connects to the running system.
4. **Install an app** to prove it, for example the SDK's hello world:

   ```
   upload bundle hello.bndl
   ```

To undo it, pick **Hiptop 3: remove the developer key** in `devkey.html` (no IMEI needed), flash the resulting `devkey_undo.jar` from recovery the same way, and reboot. It replaces the override with one that has no IMEI lock, which the OS deletes on the next boot.

### Installing it on a Hiptop 1 / Hiptop 2

This is Danger's original procedure, which we have not tested ourselves. With htconsole connected to the running phone, run `flash devos_key_IMEI.sq` and reboot. Danger's developer wiki shows a good install printing `NEW FLASH IMAGE: 560 bytes` and `is_valid_signature: signature is valid`. To undo it, flash Danger's `null-device-key.sq`, a validly signed key with no IMEI lock.

### How it works

- **Hiptop 1 / 2 key (`.sq`):** a `GORD` file with an Adler-32 checksum, a `FILE` chunk holding an `IPC1` config message (`lock_to_imei_0 = IMEI`), and a `SIGN` chunk. The signature is a CMS SignedData in Microsoft Authenticode style, including Danger's malformed content-type OID `1.2.6.1.4.1.16172.1.2.1`.
- **Hiptop 3 recovery jar:** a zip with `META-INF/MANIFEST.MF`, an `INSTALL` script and `META-INF/SIGNATURE.BLK`, the same signature format over the manifest. Recovery runs `INSTALL` (`copy "config/system-overrides.ipc" "config/system-overrides.ipc";`) with the permissions granted by the signer's certificate. The Developer Program certificate only carries "Sign Developer Configuration File" (`1.3.6.1.4.1.16172.1.1.4`), so these jars can install the developer config and nothing else.
- **At boot** the OS merges `/flash/config/system-overrides.ipc` into the system config. If `lock_to_imei_0` matches the phone it enables developer mode; if the lock is missing it deletes the override.

## Protocol

The protocol is (sort of) documented in `docs/PROTOCOL.md`. Note that this documentation was AI-generated from decompiled sources and is very inaccurate in places.
The best way to understand the protocol is to read the decompiled sources and analyze what the device sends and receives.

## Attributions

This project is built with deep appreciation for the original Danger team. The Hiptop was ahead of its time, and the engineering behind it continues to inspire.

DangerOS is a trademark of Danger Inc. All other trademarks are the property of their respective owners.
