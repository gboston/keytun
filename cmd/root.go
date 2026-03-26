// ABOUTME: Root cobra command and Execute entrypoint for the keytun CLI.
// ABOUTME: Registers host, join, and relay subcommands.
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "keytun",
	Short: "Lightweight keyboard tunnel for developers",
	Long:  "Think ngrok, but for keystrokes. Let a remote colleague type into your terminal over a screenshare.",
}

func init() {
	rootCmd.AddCommand(hostCmd)
	rootCmd.AddCommand(joinCmd)
	rootCmd.AddCommand(relayCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
