package dnclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForSocket_ReturnsWhenSocketAlreadyExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnclient.defined.sock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("creating fake socket: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitForSocket(ctx, path); err != nil {
		t.Errorf("WaitForSocket(existing) = %v, want nil", err)
	}
}

func TestWaitForSocket_ReturnsWhenSocketAppearsLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnclient.defined.sock")

	go func() {
		time.Sleep(150 * time.Millisecond)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Errorf("creating fake socket: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitForSocket(ctx, path); err != nil {
		t.Errorf("WaitForSocket(appears later) = %v, want nil", err)
	}
}

func TestWaitForSocket_TimesOutWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := WaitForSocket(ctx, path)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("WaitForSocket(absent) = %v, want context.DeadlineExceeded", err)
	}
}

func TestWaitForSocket_CancelReturnsErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never.sock")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WaitForSocket(ctx, path)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("WaitForSocket(cancelled) = %v, want context.Canceled", err)
	}
}
