// ABOUTME: PTYInjector delivers keystrokes by writing to a pseudo-terminal.
// ABOUTME: Spawns the user's shell and exposes the PTY fd for output reading.
package inject

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/creack/pty"
)

// PTYInjector injects keystrokes into a pseudo-terminal running the user's shell.
type PTYInjector struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// NewPTY creates a PTYInjector by spawning the user's shell in a new PTY.
func NewPTY() (*PTYInjector, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell)
	// Set argv[0] to "-shellname" so the shell starts as a login shell,
	// which sources profile/rc files (where aliases are defined).
	cmd.Args[0] = "-" + filepath.Base(shell)
	cmd.Env = os.Environ()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &PTYInjector{ptmx: ptmx, cmd: cmd}, nil
}

// Inject writes raw keystroke bytes to the PTY.
func (p *PTYInjector) Inject(data []byte) error {
	_, err := p.ptmx.Write(data)
	return err
}

// HasOutput returns true because PTY produces readable output.
func (p *PTYInjector) HasOutput() bool {
	return true
}

// OutputFd returns the PTY master file descriptor for reading output.
func (p *PTYInjector) OutputFd() *os.File {
	return p.ptmx
}

// PTY returns the PTY master file descriptor for direct I/O (e.g. stdin copy).
func (p *PTYInjector) PTY() *os.File {
	return p.ptmx
}

// Cmd returns the shell command process.
func (p *PTYInjector) Cmd() *exec.Cmd {
	return p.cmd
}

// ResizePTY sets the PTY window size.
func (p *PTYInjector) ResizePTY(rows, cols uint16) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// Close shuts down the PTY and kills the shell process.
func (p *PTYInjector) Close() error {
	p.ptmx.Close()
	p.cmd.Process.Kill()
	// Wait returns an error for killed processes; that's expected.
	p.cmd.Wait()
	return nil
}
