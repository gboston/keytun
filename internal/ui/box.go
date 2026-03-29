// ABOUTME: Box-drawing utility for framing terminal output.
// ABOUTME: Renders lines inside a Unicode box with consistent padding.
package ui

import (
	"fmt"
	"io"
	"strings"
)

// Box renders lines inside a Unicode box to the given writer.
// Each line is a pre-formatted string (may contain ANSI codes).
// visibleLengths provides the visible (non-ANSI) length of each line
// so padding is calculated correctly.
func Box(w io.Writer, lines []string, visibleLengths []int) {
	// Find max visible width
	maxWidth := 0
	for _, vl := range visibleLengths {
		if vl > maxWidth {
			maxWidth = vl
		}
	}

	// Add padding on each side
	innerWidth := maxWidth + 2

	top := "┌" + strings.Repeat("─", innerWidth) + "┐"
	bottom := "└" + strings.Repeat("─", innerWidth) + "┘"

	fmt.Fprintln(w, Dim(top))
	for i, line := range lines {
		pad := innerWidth - visibleLengths[i] - 1
		if pad < 0 {
			pad = 0
		}
		fmt.Fprintf(w, "%s %s%s%s\n", Dim("│"), line, strings.Repeat(" ", pad), Dim("│"))
	}
	fmt.Fprintln(w, Dim(bottom))
}
