//go:build !windows

package zellij

import (
	"claude-squad/log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"
)

// monitorWindowSize monitors and handles window resize events while attached.
func (z *ZellijSession) monitorWindowSize() {
	winchChan := make(chan os.Signal, 1)
	signal.Notify(winchChan, syscall.SIGWINCH)
	// Send initial SIGWINCH to trigger the first resize
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)

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

	// Debounce resize events
	z.wg.Add(2)
	debouncedWinch := make(chan os.Signal, 1)
	go func() {
		defer z.wg.Done()
		var resizeTimer *time.Timer
		for {
			select {
			case <-z.ctx.Done():
				return
			case <-winchChan:
				if resizeTimer != nil {
					resizeTimer.Stop()
				}
				resizeTimer = time.AfterFunc(50*time.Millisecond, func() {
					select {
					case debouncedWinch <- syscall.SIGWINCH:
					case <-z.ctx.Done():
					}
				})
			}
		}
	}()
	go func() {
		defer z.wg.Done()
		defer signal.Stop(winchChan)
		for {
			select {
			case <-z.ctx.Done():
				return
			case <-debouncedWinch:
				doUpdate()
			}
		}
	}()
}
