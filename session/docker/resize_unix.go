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
