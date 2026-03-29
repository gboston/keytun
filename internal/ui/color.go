// ABOUTME: ANSI color helpers for terminal output.
// ABOUTME: Provides simple wrappers that apply color codes to strings.
package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// isTTY reports whether stdout is connected to a terminal.
// Colors are suppressed when piping to a file or another process.
var isTTY = term.IsTerminal(int(os.Stdout.Fd()))

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	cyan    = "\x1b[36m"
	boldRed = "\x1b[1;31m"
)

func wrap(code, s string) string {
	if !isTTY {
		return s
	}
	return code + s + reset
}

// Bold returns s in bold.
func Bold(s string) string { return wrap(bold, s) }

// Dim returns s in dim/faint.
func Dim(s string) string { return wrap(dim, s) }

// Green returns s in green.
func Green(s string) string { return wrap(green, s) }

// Yellow returns s in yellow.
func Yellow(s string) string { return wrap(yellow, s) }

// Red returns s in red.
func Red(s string) string { return wrap(red, s) }

// Cyan returns s in cyan.
func Cyan(s string) string { return wrap(cyan, s) }

// BoldRed returns s in bold red.
func BoldRed(s string) string { return wrap(boldRed, s) }

// Greenf formats and returns the result in green.
func Greenf(format string, a ...any) string { return Green(fmt.Sprintf(format, a...)) }

// Yellowf formats and returns the result in yellow.
func Yellowf(format string, a ...any) string { return Yellow(fmt.Sprintf(format, a...)) }

// Redf formats and returns the result in red.
func Redf(format string, a ...any) string { return Red(fmt.Sprintf(format, a...)) }
