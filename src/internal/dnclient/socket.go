package dnclient

import (
	"context"
	"os"
	"time"
)

// socketPollInterval is how often WaitForSocket re-checks for the control socket.
const socketPollInterval = 100 * time.Millisecond

// WaitForSocket blocks until a file exists at path or ctx is done, polling every
// socketPollInterval. The daemon (`dnclient run`) creates its control socket
// there once it is listening, and `dnclient enroll` connects to that socket — so
// run waits here for the daemon to come up before enrolling. It returns ctx.Err()
// if the socket does not appear before ctx is cancelled or its deadline passes.
func WaitForSocket(ctx context.Context, path string) error {
	ticker := time.NewTicker(socketPollInterval)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
