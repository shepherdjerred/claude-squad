//go:build windows

package docker

import (
	"os"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// handleResize monitors terminal size changes using polling on Windows.
func (d *DockerSession) handleResize() {
	var lastWidth, lastHeight int

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if d.ptmx != nil {
				width, height, _ := term.GetSize(int(os.Stdin.Fd()))
				if width != lastWidth || height != lastHeight {
					lastWidth, lastHeight = width, height
					pty.Setsize(d.ptmx, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)})
					d.termBuffer.Resize(height, width)
				}
			}
		}
	}
}
