# Changelog

All notable changes to keytun are documented in this file.

## [v0.3.0] — 2026-03-27

### Added

- Direct join links: host now prints a `https://keytun.com/s/<code>` URL for easy sharing
- Web client at `/s/` for joining sessions directly from the browser
- Cross-language crypto test vectors for JS/Go compatibility

## [v0.2.0] — 2026-03-26

### Changed

- Default relay URL is now `wss://relay.keytun.com/ws` — no `--relay` flag needed for host or join commands
- Website: added favicon, OG images, install script, system mode docs, and Firebase hosting
- License updated to AGPL-3.0
- Added Dockerfile for relay self-hosting
- Added README with install and usage instructions

### Fixed

- Use Node.js 22 in Firebase hosting CI workflows
- Use dedicated PAT for Homebrew tap push in GoReleaser

## [v0.1.1] — 2025-12-15

### Fixed

- Use dedicated PAT for Homebrew tap push in GoReleaser
- Adjusted GoReleaser config

## [v0.1.0] — 2025-12-14

### Added

- Initial release
- Terminal mode: host shares a PTY session, client types into it
- System mode: inject keystrokes at the OS level (macOS)
- WebSocket relay broker with in-memory session routing
- Human-readable session codes (e.g. `keen-fox-42`)
- End-to-end encryption (X25519 + AES-256-GCM)
- Double-Escape disconnect for clients
- Homebrew cask distribution via `gboston/tap`
