// ABOUTME: Cross-platform clipboard copy utility.
// ABOUTME: Uses pbcopy on macOS, xclip/xsel on Linux; fails silently.
package ui

import (
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard attempts to copy text to the system clipboard.
// Returns true if successful, false otherwise. Never errors loudly.
func CopyToClipboard(text string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return false
		}
	default:
		return false
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}
