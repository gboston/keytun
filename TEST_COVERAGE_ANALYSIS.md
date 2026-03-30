# Test Coverage Analysis

**Date:** 2026-03-30

## Overview

The codebase has **3,999 lines of test code** covering **3,010 lines of source** across 12 test files. The internal packages have strong coverage, but the `cmd/` and `internal/ui/` packages are entirely untested.

## Coverage by Package

| Package | Source Lines | Test Lines | Test Files | Coverage |
|---------|-------------|------------|------------|----------|
| `internal/relay` | 450 | 1,028 | 1 | Excellent |
| `internal/host` | 783 | 928 | 1 | Excellent |
| `internal/client` | 412 | 775 | 2 | Excellent |
| `internal/crypto` | 127 | 399 | 2 | Excellent |
| `internal/protocol` | 36 | 250 | 1 | Excellent |
| `internal/inject` | 357 | 142+ | 3 | Good |
| `internal/session` | 78 | 48 | 1 | Good |
| `internal/integration` | — | 429 | 1 | E2E suite |
| **`cmd/`** | **577** | **0** | **0** | **None** |
| **`internal/ui/`** | **190** | **0** | **0** | **None** |

## Recommended Improvements (Priority Order)

### 1. `cmd/join.go` — Pure functions with zero tests (HIGH)

Three functions are easily unit-testable with no dependencies on terminals or network:

- **`backoffDelay()`** (line 211) — Exponential backoff with jitter. Test that delay grows exponentially, respects the max cap, and jitter stays within bounds. This is critical for reconnection reliability.
- **`isSessionGone()`** (line 200) — String-matching error classifier. Test with matching and non-matching error messages.
- **`isPasswordError()`** (line 206) — String-matching error classifier. Same approach.

*Effort: Low. These are pure functions that take simple inputs and return simple outputs.*

### 2. `cmd/host.go` — Formatting helpers with zero tests (HIGH)

- **`formatDuration()`** (line 69) — Converts `time.Duration` to `"1h2m3s"` format. Test boundary cases: zero, sub-second, exactly 1 hour, etc.
- **`formatBytes()`** (line 83) — Converts byte counts to human-readable strings. Test boundaries: 0, 1023, 1024, large values.

*Effort: Low. Pure functions, trivially testable.*

### 3. `internal/ui/` — Untested UI package (MEDIUM)

Four source files (190 lines total) with no tests:

- **`color.go`** — ANSI color formatting (`Green()`, `Bold()`, `Dim()`, etc.). Test that output contains correct escape codes and that `NO_COLOR` env var disables colors.
- **`box.go`** — `Box()` renders bordered boxes to a writer. Test output format with known inputs.
- **`spinner.go`** — Animated spinner. Test `Start()`/`Stop()` lifecycle (write to a `bytes.Buffer` instead of stdout).
- **`clipboard.go`** — `CopyToClipboard()`. Harder to test (OS-dependent), but can verify the function doesn't panic when clipboard tools are missing.

*Effort: Medium. Requires writing to buffers and checking ANSI output.*

### 4. `internal/crypto` — Concurrency safety (MEDIUM)

The crypto package has excellent functional coverage but no concurrency tests. Since `Session.Encrypt()` mutates a nonce counter, concurrent calls could produce nonce reuse (a critical AES-GCM vulnerability).

- **Test:** Launch N goroutines calling `Encrypt()` simultaneously. Verify all ciphertexts are unique and all decrypt correctly.
- **Test:** Concurrent `Encrypt()` + `Decrypt()` to check for data races.

*Effort: Medium. Run with `-race` flag.*

### 5. `internal/relay` — Error and edge-case paths (MEDIUM)

The relay has excellent happy-path coverage but some gaps:

- **Stale limiter cleanup** — `sweepStaleLimiters()` runs on a timer. Test that expired limiters are actually removed.
- **Concurrent host disconnect during client write** — Race between session cleanup and message routing.
- **`realIP()` with X-Forwarded-For** — Test parsing of `X-Forwarded-For` header with multiple IPs, empty values, and spoofed headers.
- **`CloseAllSessions()` with active writers** — Verify graceful shutdown doesn't panic when messages are in-flight.

*Effort: Medium.*

### 6. `internal/session` — Collision and distribution (LOW)

Current tests verify format and uniqueness across 100 samples. Missing:

- **Statistical test** — Generate 10,000 codes and verify roughly uniform distribution across adjectives and nouns.
- **Thread safety** — Concurrent `Generate()` calls (uses `math/rand` global source).

*Effort: Low.*

### 7. `internal/host` — Edge cases in multi-client management (LOW)

The host package is well-tested, but some scenarios are missing:

- **Rapid client join/leave cycles** — Multiple clients connecting and disconnecting in quick succession.
- **Key exchange timeout/failure** — What happens when a client connects but never completes key exchange?
- **`ReadOutputUntil()` timeout path** — Only the success path is tested in integration tests.
- **Output broadcasting when encryption fails for one client** — Verify other clients still receive output.

*Effort: Medium.*

### 8. `cmd/join.go` — `runInputLoop()` integration (LOW)

The input loop is the main client-side logic and is completely untested. It's harder to test (requires terminal raw mode), but could be refactored for testability:

- Extract the byte-processing loop (escape detection + send) into a function that takes an `io.Reader` and a `SendInput` interface, making it testable without a real terminal.
- Test: escape detection triggers disconnect return, connection loss mid-send returns `loopConnectionLost`, stdin EOF returns `loopStdinError`.

*Effort: High. Requires minor refactoring for dependency injection.*

## Summary

| Priority | Area | Effort | Impact |
|----------|------|--------|--------|
| HIGH | `cmd/join.go` pure functions | Low | Prevents reconnection bugs |
| HIGH | `cmd/host.go` formatters | Low | Prevents display bugs |
| MEDIUM | `internal/ui/` package | Medium | 190 lines fully untested |
| MEDIUM | `internal/crypto` concurrency | Medium | Prevents nonce-reuse vulnerability |
| MEDIUM | `internal/relay` edge cases | Medium | Prevents production crashes |
| LOW | `internal/session` distribution | Low | Nice to have |
| LOW | `internal/host` edge cases | Medium | Defensive coverage |
| LOW | `cmd/join.go` input loop | High | Requires refactoring |
