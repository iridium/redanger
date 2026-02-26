# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Re:Danger is a clean-room reimplementation of the DangerOS server (Danger Hiptop / T-Mobile Sidekick backend). Devices connect directly to port 25025 via TCP (no redirector). The protocol spec is in `docs/PROTOCOL.md`.

## Build & Run

All Go code lives in `server/`.

```bash
# Build
cd server && go build -o redanger .

# Run
cd server && go run .

# Test
cd server && go test ./...

# Hot-reload development (uses Air)
cd server && go tool air
```
