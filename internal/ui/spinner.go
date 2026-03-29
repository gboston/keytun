// ABOUTME: Animated terminal spinner for "waiting" states.
// ABOUTME: Runs in a goroutine and clears itself when stopped.
package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated spinner with a message.
type Spinner struct {
	w       io.Writer
	message string
	done    chan struct{}
	once    sync.Once
}

// NewSpinner creates and starts a spinner writing to w.
func NewSpinner(w io.Writer, message string) *Spinner {
	s := &Spinner{
		w:       w,
		message: message,
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *Spinner) run() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.done:
			// Clear the spinner line
			fmt.Fprintf(s.w, "\r\x1b[2K")
			return
		case <-ticker.C:
			frame := spinFrames[i%len(spinFrames)]
			fmt.Fprintf(s.w, "\r\x1b[2K%s %s", Cyan(frame), Dim(s.message))
			i++
		}
	}
}

// Stop halts the spinner and clears its line.
func (s *Spinner) Stop() {
	s.once.Do(func() { close(s.done) })
	// Small delay to let the clear happen before caller writes next line
	time.Sleep(10 * time.Millisecond)
}
