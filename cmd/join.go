// ABOUTME: Cobra subcommand for `keytun join <session-code>`.
// ABOUTME: Connects to a host's session and forwards local keystrokes.
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/gbostoen/keytun/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var joinRelayURL string

var joinCmd = &cobra.Command{
	Use:   "join [session-code]",
	Short: "Join a keytun session and type into the remote terminal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionCode := args[0]

		c, err := client.New(joinRelayURL, sessionCode)
		if err != nil {
			return fmt.Errorf("failed to join session: %w", err)
		}
		defer c.Close()

		fmt.Printf("Connected to %s\n", sessionCode)
		fmt.Println("You are now typing into the remote terminal.")
		fmt.Println("Press Escape twice to disconnect.")
		fmt.Println()

		// Switch terminal to raw mode
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set raw mode: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		// Read stdin and send to relay, watching for double-Escape to disconnect
		esc := client.NewEscapeDetector(300 * time.Millisecond)
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				break
			}
			if n == 0 {
				continue
			}

			for i := 0; i < n; i++ {
				action := esc.Feed(buf[i])
				switch action {
				case client.Disconnect:
					fmt.Println("\r\nDisconnected.")
					return nil
				case client.EscapeHeld:
					// Wait for possible second escape
					continue
				case client.PassThrough:
					// If a previous escape timed out or was interrupted, flush it first
					if esc.HadPendingEscape() {
						if err := c.SendInput([]byte{0x1B}); err != nil {
							return fmt.Errorf("send failed: %w", err)
						}
					}
					if err := c.SendInput([]byte{buf[i]}); err != nil {
						return fmt.Errorf("send failed: %w", err)
					}
				}
			}

			// After processing all bytes, flush any timed-out escape
			if esc.Flush() {
				if err := c.SendInput([]byte{0x1B}); err != nil {
					return fmt.Errorf("send failed: %w", err)
				}
			}
		}

		return nil
	},
}

func init() {
	joinCmd.Flags().StringVar(&joinRelayURL, "relay", "ws://localhost:8080/ws", "relay server WebSocket URL")
}
