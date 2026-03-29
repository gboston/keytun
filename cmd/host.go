// ABOUTME: Cobra subcommand for `keytun host`.
// ABOUTME: Starts a session, creates an injector, and muxes local + remote input.
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gboston/keytun/internal/host"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/session"
	"github.com/gboston/keytun/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var hostRelayURL string
var hostMode string
var hostTarget string

var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "Start a keytun session and share the code with your colleague",
	RunE: func(cmd *cobra.Command, args []string) error {
		code := session.Generate()

		switch hostMode {
		case "terminal":
			return runTerminalMode(code)
		case "system":
			return runSystemMode(code)
		default:
			return fmt.Errorf("unknown mode %q (use 'terminal' or 'system')", hostMode)
		}
	},
}

func printSessionBox(code string) {
	joinURL := fmt.Sprintf("https://keytun.com/s/%s", code)
	clipNote := ""
	if ui.CopyToClipboard(joinURL) {
		clipNote = ui.Dim(" (copied to clipboard)")
	}

	lines := []string{
		fmt.Sprintf("%s  %s", ui.Dim("Session:"), ui.Bold(ui.Green(code))),
		fmt.Sprintf("%s     %s%s", ui.Dim("Join:"), ui.Cyan(joinURL), clipNote),
	}
	visible := []int{
		len("Session:  ") + len(code),
		len("Join:     ") + len(joinURL),
	}
	if clipNote != "" {
		visible[1] += len(" (copied to clipboard)")
	}
	fmt.Println()
	ui.Box(os.Stdout, lines, visible)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

func printSessionSummary(stats host.SessionStats) {
	fmt.Println()
	fmt.Println(ui.Dim("─── session ended ───"))
	fmt.Printf("  %s %s\n", ui.Dim("Duration:"), formatDuration(stats.Duration))
	fmt.Printf("  %s %s\n", ui.Dim("Input:"), formatBytes(stats.InputBytes))
	fmt.Printf("  %s %d join(s), %d disconnect(s)\n", ui.Dim("Clients:"), stats.TotalJoins, stats.TotalLeaves)
	fmt.Println(ui.Dim("─────────────────────"))
}

func runTerminalMode(code string) error {
	fmt.Printf("%s %s\n", ui.Bold("keytun"), ui.Dim(Version))
	printSessionBox(code)

	inj, err := inject.NewPTY()
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	defer inj.Close()

	h, err := host.New(hostRelayURL, code, inj, os.Stdout)
	if err != nil {
		return fmt.Errorf("failed to start host: %w", err)
	}
	defer h.Close()

	// Show a spinner while waiting for the first client to join
	spinner := ui.NewSpinner(os.Stdout, "Waiting for client... (share the link with your colleague)")
	go func() {
		<-h.ClientJoined()
		spinner.Stop()
	}()

	// Handle window size changes — resize the PTY and notify the client
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
				inj.ResizePTY(ws.Rows, ws.Cols)
				h.UpdateTermSize(ws.Cols, ws.Rows)
			}
		}
	}()
	// Set initial size
	if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
		inj.ResizePTY(ws.Rows, ws.Cols)
		h.UpdateTermSize(ws.Cols, ws.Rows)
	}

	// Open /dev/tty for terminal state management. We use a separate fd
	// because os.Stdin gets closed to unblock the read loop on PTY exit,
	// and term.Restore would fail on the closed fd.
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()
	ttyFd := int(tty.Fd())

	// Switch local terminal to raw mode
	oldState, err := term.MakeRaw(ttyFd)
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}

	// When the shell process exits (e.g. user types "exit"), close stdin
	// so the read loop below unblocks immediately.
	go func() {
		<-inj.Done()
		os.Stdin.Close()
	}()

	// Copy local stdin to PTY. Before a client joins, Ctrl+C (0x03) exits
	// keytun cleanly instead of passing through to the PTY shell.
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		select {
		case <-h.ClientJoined():
			// client connected — pass all bytes through normally
		default:
			// no client yet — check for Ctrl+C
			for i := 0; i < n; i++ {
				if buf[i] == 0x03 {
					// Close host first so goroutines stop writing to stdout,
					// then restore the terminal to avoid garbled output.
					stats := h.Stats()
					h.Close()
					h.ClearTerminalTitle()
					term.Restore(ttyFd, oldState)
					printSessionSummary(stats)
					return nil
				}
			}
		}
		if _, err := inj.PTY().Write(buf[:n]); err != nil {
			break
		}
	}

	// Close host first so goroutines stop writing to stdout,
	// then restore the terminal to avoid garbled output.
	stats := h.Stats()
	h.Close()
	h.ClearTerminalTitle()
	term.Restore(ttyFd, oldState)
	printSessionSummary(stats)
	return nil
}

func runSystemMode(code string) error {
	var inj *inject.SystemInjector
	var err error
	if hostTarget != "" {
		fmt.Printf("Looking up app %q...\n", hostTarget)
		inj, err = inject.NewSystemWithTarget(hostTarget)
	} else {
		inj, err = inject.NewSystem()
	}
	if err != nil {
		return fmt.Errorf("failed to start system injector: %w", err)
	}
	defer inj.Close()

	h, err := host.New(hostRelayURL, code, inj, os.Stdout)
	if err != nil {
		return fmt.Errorf("failed to start host: %w", err)
	}
	defer h.Close()

	fmt.Printf("%s %s %s\n", ui.Bold("keytun"), ui.Dim(Version), ui.Dim("(system mode)"))
	printSessionBox(code)
	if hostTarget != "" {
		fmt.Printf("%s Keystrokes will be injected into %s.\n", ui.Dim("▸"), ui.Bold(hostTarget))
	} else {
		fmt.Printf("%s Keystrokes will be injected into the focused app.\n", ui.Dim("▸"))
	}

	// Show a spinner while waiting for the first client to join
	spinner := ui.NewSpinner(os.Stdout, "Waiting for client... (share the link with your colleague)")
	go func() {
		<-h.ClientJoined()
		spinner.Stop()
	}()

	fmt.Println(ui.Dim("Press Ctrl+C to stop."))

	// Block until SIGINT/SIGTERM or host done
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-h.Done():
	}

	stats := h.Stats()
	printSessionSummary(stats)
	return nil
}

func init() {
	hostCmd.Flags().StringVar(&hostRelayURL, "relay", "wss://relay.keytun.com/ws", "relay server WebSocket URL")
	hostCmd.Flags().StringVar(&hostMode, "mode", "terminal", "injection mode: terminal (PTY) or system (OS-level)")
	hostCmd.Flags().StringVar(&hostTarget, "target", "", "target app name for system mode (e.g. \"TextEdit\")")
}
