//go:build windows

package zellij

import (
	"claude-squad/log"
	"os"
	"time"

	"golang.org/x/term"
)

// monitorWindowSize on Windows uses polling instead of SIGWINCH (which doesn't exist on Windows).
// Note: Zellij does not officially support Windows, so this is primarily a fallback stub.
func (z *ZellijSession) monitorWindowSize() {
	everyN := log.NewEvery(60 * time.Second)

	doUpdate := func() {
		cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
		if err != nil {
			if everyN.ShouldLog() {
				log.ErrorLog.Printf("failed to get terminal size: %v", err)
			}
		} else {
			if err := z.SetDetachedSize(cols, rows); err != nil {
				if everyN.ShouldLog() {
					log.ErrorLog.Printf("failed to update window size: %v", err)
				}
			}
		}
	}
	// Set initial size
	defer doUpdate()

	// Poll for window size changes on Windows
	z.wg.Add(1)
	go func() {
		defer z.wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var lastCols, lastRows int
		for {
			select {
			case <-z.ctx.Done():
				return
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
				if err == nil && (cols != lastCols || rows != lastRows) {
					lastCols, lastRows = cols, rows
					doUpdate()
				}
			}
		}
	}()
}
