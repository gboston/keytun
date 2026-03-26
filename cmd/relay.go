// ABOUTME: Cobra subcommand for `keytun relay`.
// ABOUTME: Starts the WebSocket relay server that brokers host/client connections.
package cmd

import (
	"fmt"
	"net"
	"net/http"

	"github.com/gbostoen/keytun/internal/relay"
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

		fmt.Printf("keytun relay listening on %s\n", addr)
		return http.Serve(ln, mux)
	},
}

func init() {
	relayCmd.Flags().IntVarP(&relayPort, "port", "p", 8080, "port to listen on")
}
