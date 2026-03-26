// ABOUTME: Entry point for the keytun CLI.
// ABOUTME: Delegates to cobra for subcommand routing (host, join, relay).
package main

import (
	"fmt"
	"os"

	"github.com/gboston/keytun/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
