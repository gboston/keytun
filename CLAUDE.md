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

**Message flow:** Client reads stdin byte-by-byte → encrypts (AES-256-GCM) → base64-encodes → sends `input` message → relay routes to host → host decrypts → writes to PTY. PTY output flows back the same way as `output` messages. The relay only sees opaque ciphertext.

### Key packages

- **`cmd/`** — Cobra CLI subcommands (`host`, `join`, `relay`, `version`). Each wires flags and delegates to the corresponding `internal/` package.
- **`internal/host/`** — Spawns a PTY (or system-level injector on macOS), multiplexes local stdin with remote input from relay. Manages per-client crypto sessions.
- **`internal/client/`** — Connects to relay, reads stdin byte-by-byte, encrypts and sends keystrokes. Double-Escape disconnects. Auto-reconnects with exponential backoff.
- **`internal/relay/`** — HTTP server that upgrades `/ws` to WebSocket. Maintains an in-memory session map pairing host↔client connections and routing messages between them. Per-IP rate limiting on joins.
- **`internal/protocol/`** — JSON message envelope (`Message` struct with `type`, `session`, `client_id`, `data`, `message`, `event`, `cols`, `rows` fields).
- **`internal/crypto/`** — End-to-end encryption: X25519 key exchange, HKDF-SHA256 key derivation, AES-256-GCM authenticated encryption. The relay never sees plaintext.
- **`internal/inject/`** — Keystroke injection backends: PTY mode (all platforms) and system mode (macOS only, via CoreGraphics). Defines the `Injector` interface.
- **`internal/session/`** — Generates human-readable session codes in `adjective-noun-NN` format from embedded wordlists.
- **`internal/integration/`** — End-to-end tests using an in-process relay (httptest) to verify keystroke flow, disconnect notifications, and control character preservation.

## Protocol

Ten message types: `host_register`, `host_registered`, `client_join`, `session_joined`, `key_exchange`, `input`, `output`, `resize`, `peer_event`, `error`. See `internal/protocol/messages.go` for definitions.

## Dependencies

Go 1.25.0 with: `spf13/cobra` (CLI), `gorilla/websocket` (WebSocket), `creack/pty` (PTY), `golang.org/x/term` (raw mode), `golang.org/x/time` (rate limiting).

## Website

The `website/` directory contains an Astro.js documentation site (separate Node.js toolchain, not part of the Go build).

## Browser automation

`agent-browser` is available system-wide for browser automation tasks (screenshots, navigation, interaction).

```bash
agent-browser open <url>          # Navigate to URL
agent-browser screenshot <path>   # Take a screenshot
agent-browser scroll down <px>    # Scroll
agent-browser eval <js>           # Run JavaScript
agent-browser snapshot            # Accessibility tree with refs
agent-browser click <selector>    # Click element
```

Use this to visually verify website changes against the local dev server at `http://localhost:4321`.
