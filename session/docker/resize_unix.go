//go:build !windows

package docker

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// handleResize monitors terminal size changes and resizes the PTY.
func (d *DockerSession) handleResize() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)

	// Apply initial resize immediately, BEFORE waiting for signals
	if d.ptmx != nil {
		width, height, err := term.GetSize(int(os.Stdin.Fd()))
		if err == nil {
			winsize := &pty.Winsize{Rows: uint16(height), Cols: uint16(width)}
			if err := pty.Setsize(d.ptmx, winsize); err == nil {
				d.termBuffer.Resize(height, width)
			}
		}
	}

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ch:
			if d.ptmx != nil {
				width, height, _ := term.GetSize(int(os.Stdin.Fd()))
				pty.Setsize(d.ptmx, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)})
				d.termBuffer.Resize(height, width)
			}
		}
	}
}
