// ABOUTME: Cobra subcommand for `keytun join <session-code>`.
// ABOUTME: Connects to a host's session and forwards local keystrokes with auto-reconnect.
package cmd

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/gboston/keytun/internal/client"
	"github.com/gboston/keytun/internal/ui"
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

		const (
			initialDelay = 500 * time.Millisecond
			maxDelay     = 15 * time.Second
			multiplier   = 2.0
			jitter       = 0.25
		)

		attempt := 0
		firstConnect := true

		for {
			c, err := client.New(joinRelayURL, sessionCode)
			if err != nil {
				if isSessionGone(err) {
					if firstConnect {
						return fmt.Errorf("failed to join session: %w", err)
					}
					fmt.Fprintf(os.Stderr, "\r\n%s\r\n", ui.Yellow("[keytun] session ended (host disconnected)"))
					return nil
				}
				attempt++
				delay := backoffDelay(attempt, initialDelay, maxDelay, multiplier, jitter)
				fmt.Fprintf(os.Stderr, "%s\n", ui.Yellowf("[keytun] connection failed, retrying in %s... (attempt %d)", delay.Round(time.Millisecond), attempt))
				time.Sleep(delay)
				continue
			}

			attempt = 0

			// Display decrypted terminal output from the host
			c.SetOnOutput(func(data []byte) {
				os.Stdout.Write(data)
			})

			if firstConnect {
				fmt.Printf("%s %s\n", ui.Green("Connected to"), ui.Bold(ui.Green(sessionCode)))
				fmt.Println(ui.Dim("You are now typing into the remote terminal."))
				fmt.Println(ui.Dim("Press Escape twice to disconnect."))
				fmt.Println()
				firstConnect = false
			} else {
				fmt.Fprintf(os.Stderr, "\r\n%s\r\n", ui.Greenf("[keytun] reconnected to %s", sessionCode))
			}

			reason := runInputLoop(c)
			c.Close()

			switch reason {
			case loopDisconnect:
				return nil
			case loopConnectionLost:
				attempt++
				delay := backoffDelay(attempt, initialDelay, maxDelay, multiplier, jitter)
				fmt.Fprintf(os.Stderr, "\r\n%s\r\n", ui.Yellowf("[keytun] connection lost, reconnecting in %s... (attempt %d)", delay.Round(time.Millisecond), attempt))
				time.Sleep(delay)
				continue
			case loopStdinError:
				return nil
			}
		}
	},
}

type loopExitReason int

const (
	loopDisconnect    loopExitReason = iota // user pressed Esc×2
	loopConnectionLost                      // send failed or connection closed
	loopStdinError                          // stdin read error
)

// runInputLoop sets the terminal to raw mode and forwards stdin to the relay.
// Returns the reason the loop exited.
func runInputLoop(c *client.Client) loopExitReason {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set raw mode: %v\n", err)
		return loopStdinError
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	esc := client.NewEscapeDetector(300 * time.Millisecond)
	buf := make([]byte, 256)

	// Use a channel for stdin reads so we can select on connection loss
	type stdinRead struct {
		data []byte
		err  error
	}
	stdinCh := make(chan stdinRead, 1)

	go func() {
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				stdinCh <- stdinRead{nil, err}
				return
			}
			if n > 0 {
				copied := make([]byte, n)
				copy(copied, buf[:n])
				stdinCh <- stdinRead{copied, nil}
			}
		}
	}()

	for {
		select {
		case <-c.Done():
			return loopConnectionLost

		case read := <-stdinCh:
			if read.err != nil {
				return loopStdinError
			}

			for i := 0; i < len(read.data); i++ {
				action := esc.Feed(read.data[i])
				switch action {
				case client.Disconnect:
					term.Restore(int(os.Stdin.Fd()), oldState)
					fmt.Println("\nDisconnected.")
					return loopDisconnect
				case client.EscapeHeld:
					continue
				case client.PassThrough:
					if esc.HadPendingEscape() {
						if err := c.SendInput([]byte{0x1B}); err != nil {
							return loopConnectionLost
						}
					}
					if err := c.SendInput([]byte{read.data[i]}); err != nil {
						return loopConnectionLost
					}
				}
			}

			if esc.Flush() {
				if err := c.SendInput([]byte{0x1B}); err != nil {
					return loopConnectionLost
				}
			}
		}
	}
}

// isSessionGone returns true if the error indicates the session no longer exists.
func isSessionGone(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "session not found") || strings.Contains(msg, "no such session")
}

// backoffDelay calculates an exponential backoff delay with jitter.
func backoffDelay(attempt int, initial, max time.Duration, mult, jitterFrac float64) time.Duration {
	delay := float64(initial) * math.Pow(mult, float64(attempt-1))
	if delay > float64(max) {
		delay = float64(max)
	}
	// Apply jitter: +/- jitterFrac
	jitterRange := delay * jitterFrac
	delay += (rand.Float64()*2 - 1) * jitterRange
	return time.Duration(delay)
}

func init() {
	joinCmd.Flags().StringVar(&joinRelayURL, "relay", "wss://relay.keytun.com/ws", "relay server WebSocket URL")
}
