# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Keytun?

Keytun is a lightweight CLI tool that lets remote colleagues type into your terminal over a screenshare — think ngrok for keystrokes. A WebSocket relay broker connects a **host** (who shares their terminal via PTY) with a **client** (who sends keystrokes).

## Build & Test Commands

Uses [just](https://github.com/casey/just) as task runner (install via `mise install`).

```bash
just build          # Compile binary to ./keytun
just test           # Run all tests (go test ./... -v)
just clean          # Remove compiled binary

go test ./internal/relay/ -v -run TestMessageRouting   # Run a single test
```

## Running

```bash
./keytun relay --port 8080              # Start relay server
./keytun host --relay ws://localhost:8080/ws   # Host a session (spawns PTY)
./keytun join <session-code> --relay ws://localhost:8080/ws  # Join as client
```

## Architecture

The system has three actors connected via WebSocket:

```
Client (stdin) ──WS──▶ Relay (broker) ──WS──▶ Host (PTY)
                       ◀──────────────────────  (output back to client)
```

**Message flow:** Client reads stdin byte-by-byte → base64-encodes → sends `input` message → relay routes to host → host writes to PTY. PTY output flows back the same way as `output` messages.

### Key packages

- **`cmd/`** — Cobra CLI subcommands (`host`, `join`, `relay`). Each wires flags and delegates to the corresponding `internal/` package.
- **`internal/host/`** — Spawns a PTY (`creack/pty`), sets terminal to raw mode, multiplexes local stdin with remote input from relay.
- **`internal/client/`** — Connects to relay, reads stdin byte-by-byte, sends keystrokes as base64 JSON. Double-Escape disconnects.
- **`internal/relay/`** — HTTP server that upgrades `/ws` to WebSocket. Maintains an in-memory session map pairing host↔client connections and routing messages between them.
- **`internal/protocol/`** — JSON message envelope (`Message` struct with `type`, `session`, `data`, `event`, `message` fields). All I/O data is base64-encoded.
- **`internal/session/`** — Generates human-readable session codes in `adjective-noun-NN` format from embedded wordlists.
- **`internal/integration/`** — End-to-end tests using an in-process relay (httptest) to verify keystroke flow, disconnect notifications, and control character preservation.

## Protocol

Seven message types: `host_register`, `client_join`, `input`, `output`, `error`, `peer_event`, `session_joined`. See `internal/protocol/messages.go` for definitions.

## Dependencies

Go 1.25.0 with: `spf13/cobra` (CLI), `gorilla/websocket` (WebSocket), `creack/pty` (PTY), `golang.org/x/term` (raw mode).

## Website

The `website/` directory contains an Astro.js documentation site (separate Node.js toolchain, not part of the Go build).
