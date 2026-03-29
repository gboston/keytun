#!/usr/bin/env bash
# ABOUTME: Generates a keytun demo .cast file and converts to GIF.
# ABOUTME: Builds the cast programmatically — no TTY or tmux needed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CAST_FILE="$REPO_ROOT/demo.cast"
GIF_FILE="$REPO_ROOT/demo.gif"

# Build fresh and read version from source
echo "==> Building keytun..."
(cd "$REPO_ROOT" && go build -o keytun .)
KEYTUN_VERSION="keytun $(sed -n 's/.*Version = "\(.*\)"/\1/p' "$REPO_ROOT/cmd/version.go")"

rm -f "$CAST_FILE" "$GIF_FILE"

echo "==> Generating demo cast ($KEYTUN_VERSION)..."

CAST_FILE="$CAST_FILE" KEYTUN_VERSION="$KEYTUN_VERSION" python3 << 'PYEOF'
import json, os

COLS = 100
ROWS = 28
CAST_FILE = os.environ["CAST_FILE"]
KEYTUN_VERSION = os.environ.get("KEYTUN_VERSION", "keytun dev")

# ANSI codes
RESET = "\033[0m"
DIM = "\033[2m"
GREEN = "\033[32m"
CYAN = "\033[36m"
GRAY = "\033[90m"
BGREEN = "\033[1;32m"
BCYAN = "\033[1;36m"
BYELLOW = "\033[1;33m"
CLEAR = "\033[2J\033[H"

HLINE, VLINE, TJOIN = "─", "│", "┬"

def move(r, c):
    return f"\033[{r};{c}H"

events = []
t = 0.0

def out(text, delay=0.0):
    global t
    t += delay
    events.append((round(t, 4), "o", text))

def type_text(row, col, text, delay=0.045):
    for i, ch in enumerate(text):
        out(move(row, col + i) + ch, delay)

def pause(s):
    global t
    t += s

MID = 50

def draw_frame():
    s = move(1, 1)
    ll = " HOST "
    rl = " CLIENT "
    s += f"{GRAY}{HLINE*2}{ll}{HLINE*(MID-len(ll)-4)}{TJOIN}{HLINE*2}{rl}{HLINE*(COLS-MID-len(rl)-3)}{RESET}"
    for r in range(2, ROWS + 1):
        s += move(r, MID) + f"{GRAY}{VLINE}{RESET}"
    return s

def hp(row):
    return move(row, 2) + f"{BGREEN}${RESET} "

def hl(row, text):
    return move(row, 2) + text

def cl(row, text):
    return move(row, MID + 2) + text

# ── Draw frame ──
out(CLEAR)
out(draw_frame())
pause(0.3)
out(hp(2))
out(move(2, MID + 2) + f"{BCYAN}${RESET} ")
pause(0.6)

# ── Host types command ──
type_text(2, 6, "keytun host")
pause(0.4)
out(hl(3, f"{DIM}{KEYTUN_VERSION}{RESET}"))
pause(0.3)
out(hl(4, f"Session:  {BYELLOW}keen-fox-42{RESET}"))
pause(0.1)
out(hl(5, f"Join:     {CYAN}https://keytun.com/s/keen-fox-42{RESET}"))
pause(0.2)
out(hl(6, f"{DIM}Waiting for client...{RESET}"))
pause(2.0)

# ── Client types join command ──
type_text(2, MID + 6, "keytun join keen-fox-42")
pause(0.6)
out(cl(3, f"{DIM}Connected to keen-fox-42{RESET}"))
pause(0.2)
out(cl(4, f"{DIM}You are now typing into the{RESET}"))
out(cl(5, f"{DIM}remote terminal.{RESET}"))
pause(0.1)
out(cl(6, f"{DIM}Press Escape twice to disconnect.{RESET}"))
pause(0.3)

# Host: client connected
out(hl(8, f"{GREEN}✓ Client connected{RESET} {DIM}(end-to-end encrypted){RESET}"))
pause(0.4)
out(hp(10))
pause(1.2)

# ── Magic: client types, both panes show input and output ──

def type_both(host_row, client_row, text, delay=0.045):
    for i, ch in enumerate(text):
        out(move(client_row, MID + 2 + i) + ch, delay)
        out(move(host_row, 6 + i) + ch)

hr = 10  # host row cursor
cr = 8   # client row cursor

# Command 1: greeting
type_both(hr, cr, "echo 'Hello from the other side!'")
cr += 1
pause(0.4)
out(hl(hr + 1, "Hello from the other side!"))
out(cl(cr, "Hello from the other side!"))
cr += 1
pause(0.2)
out(hp(hr + 2))
pause(1.2)

# Command 2: whoami
hr += 2
type_both(hr, cr, "whoami")
cr += 1
pause(0.4)
out(hl(hr + 1, "glenn"))
out(cl(cr, "glenn"))
cr += 1
pause(0.2)
out(hp(hr + 2))
pause(1.2)

# Command 3: punchline
hr += 2
type_both(hr, cr, "echo 'No screen control needed.'")
cr += 1
pause(0.4)
out(hl(hr + 1, "No screen control needed."))
out(cl(cr, "No screen control needed."))
cr += 1
pause(0.2)
out(hp(hr + 2))
pause(2.5)

# ── Write cast file ──
with open(CAST_FILE, "w") as f:
    header = {
        "version": 2,
        "width": COLS,
        "height": ROWS,
        "timestamp": 1711700000,
        "env": {"SHELL": "/bin/zsh", "TERM": "xterm-256color"},
    }
    f.write(json.dumps(header) + "\n")
    for ts, etype, data in events:
        f.write(json.dumps([ts, etype, data]) + "\n")

print(f"  {len(events)} events, {round(t, 1)}s total")
PYEOF

echo "==> Cast file: $CAST_FILE"

# Convert to GIF
if command -v agg &>/dev/null; then
    echo "==> Converting to GIF..."
    agg "$CAST_FILE" "$GIF_FILE" \
        --font-size 14 \
        --speed 1 \
        --idle-time-limit 2
    echo "==> Done: $GIF_FILE"
else
    echo "==> Install agg to convert to GIF: brew install agg"
fi
