// ABOUTME: Cobra subcommand for `keytun relay`.
// ABOUTME: Starts the WebSocket relay server that brokers host/client connections.
package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gboston/keytun/internal/relay"
	"github.com/spf13/cobra"
)

var relayPort int

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Start the keytun relay server",
	RunE: func(cmd *cobra.Command, args []string) error {
		r := relay.New()
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", r.HandleWebSocket)

		addr := fmt.Sprintf(":%d", relayPort)

		// Check if the port is already in use before starting
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("port %d is already in use — is another process listening on it?", relayPort)
		}

		srv := &http.Server{Handler: mux}

		// Listen for termination signals and shut down gracefully,
		// giving active WebSocket sessions time to finish.
		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			fmt.Println("\nshutting down relay...")
			r.CloseAllSessions()
			shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			srv.Shutdown(shutCtx)
		}()

		fmt.Printf("keytun relay listening on %s\n", addr)
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

func init() {
	relayCmd.Flags().IntVarP(&relayPort, "port", "p", 8080, "port to listen on")
}
