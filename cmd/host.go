// ABOUTME: Cobra subcommand for `keytun host`.
// ABOUTME: Starts a session, creates an injector, and muxes local + remote input.
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/gboston/keytun/internal/host"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/session"
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

func runTerminalMode(code string) error {
	fmt.Printf("keytun %s\n", Version)
	fmt.Printf("Session: %s\n", code)
	fmt.Printf("Join:    https://keytun.com/s/%s\n", code)
	fmt.Println("Waiting for client... (share the link with your colleague)")
	fmt.Println()

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

	// Switch local terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Copy local stdin to PTY — blocks until stdin closes or PTY dies
	io.Copy(inj.PTY(), os.Stdin)

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

	fmt.Printf("keytun %s (system mode)\n", Version)
	fmt.Printf("Session: %s\n", code)
	fmt.Printf("Join:    https://keytun.com/s/%s\n", code)
	if hostTarget != "" {
		fmt.Printf("Keystrokes will be injected into %s.\n", hostTarget)
	} else {
		fmt.Println("Keystrokes will be injected into the focused app.")
	}
	fmt.Println("Press Ctrl+C to stop.")

	// Block until SIGINT/SIGTERM or host done
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		fmt.Println("\nStopped.")
	case <-h.Done():
		fmt.Println("Session ended.")
	}

	return nil
}

func init() {
	hostCmd.Flags().StringVar(&hostRelayURL, "relay", "wss://relay.keytun.com/ws", "relay server WebSocket URL")
	hostCmd.Flags().StringVar(&hostMode, "mode", "terminal", "injection mode: terminal (PTY) or system (OS-level)")
	hostCmd.Flags().StringVar(&hostTarget, "target", "", "target app name for system mode (e.g. \"TextEdit\")")
}
