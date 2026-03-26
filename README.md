<p align="center">
  <img src="website/public/favicon.svg" width="80" height="80" alt="keytun logo" />
</p>

<h1 align="center">keytun</h1>

<p align="center">
  <strong>Think ngrok, but for keystrokes.</strong><br/>
  <sub>Let your colleague type into your terminal over a screenshare.</sub>
</p>

<p align="center">
  <a href="#install">Install</a> •
  <a href="#quick-start">Quick start</a> •
  <a href="#commands">Commands</a> •
  <a href="#security">Security</a> •
  <a href="https://keytun.com">Website</a>
</p>

---

```
Client (stdin) ──WS──▶ Relay (broker) ──WS──▶ Host (PTY)
                       ◀──────────────────────  (output back to client)
```

1. The **host** starts a session and gets a human-readable code (e.g. `keen-fox-42`)
2. The **client** joins using that code
3. Keystrokes flow through the relay to the host's terminal, output flows back
4. All data is encrypted end-to-end using X25519 key exchange + AES-256-GCM

## Install

### Shell (macOS / Linux)

```bash
curl -fsSL https://keytun.com/install.sh | sh
```

### Homebrew (macOS)

```bash
brew install gboston/tap/keytun
```

### From source

```bash
go install github.com/gboston/keytun@latest
```

## Quick start

```bash
# Terminal 1: Start the relay
keytun relay --port 8080

# Terminal 2: Host a session
keytun host --relay ws://localhost:8080/ws

# Terminal 3: Join the session (use the code from the host output)
keytun join keen-fox-42 --relay ws://localhost:8080/ws
```

## Commands

### `keytun relay`

Starts the WebSocket relay broker.

```
--port, -p    Port to listen on (default: 8080)
```

### `keytun host`

Hosts a session and shares a session code with your colleague.

```
--relay       Relay server URL (default: ws://localhost:8080/ws)
--mode        Injection mode: "terminal" or "system" (default: terminal)
--target      Target app name for system mode, e.g. "TextEdit" (macOS only)
```

**Terminal mode** spawns a PTY with your shell — the remote user sees and types into a full terminal session.

**System mode** injects keystrokes at the OS level into the focused application (macOS only).

### `keytun join <session-code>`

Joins an existing session. Press Escape twice to disconnect.

```
--relay       Relay server URL (default: ws://localhost:8080/ws)
```

## Security

All data between host and client is end-to-end encrypted. The relay only sees opaque ciphertext.

| Layer | Algorithm |
|-------|-----------|
| Key exchange | X25519 ECDH |
| Encryption | AES-256-GCM |
| Key derivation | HKDF-SHA256 |

The relay is a dumb pipe — it cannot read keystrokes or terminal output.

## Development

Requires [Go](https://go.dev/) 1.25+ and [just](https://github.com/casey/just) (install via `mise install`).

```bash
just build    # Compile binary to ./keytun
just test     # Run all tests
just clean    # Remove compiled binary
```

## License

MIT
