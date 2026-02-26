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
- [ ] Development key generator for real devices

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

## Protocol

The protocol is (sort of) documented in `docs/PROTOCOL.md`. Note that this documentation was AI-generated from decompiled sources and is very inaccurate in places.
The best way to understand the protocol is to read the decompiled sources and analyze what the device sends and receives.

## Attributions

This project is built with deep appreciation for the original Danger team. The Hiptop was ahead of its time, and the engineering behind it continues to inspire.

DangerOS is a trademark of Danger Inc. All other trademarks are the property of their respective owners.
