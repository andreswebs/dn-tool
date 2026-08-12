package logctx

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

// The logger travels on the context, so orchestration constructs it once and
// every layer below retrieves the same instance.
func TestWithFrom_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelDebug, false)

	ctx := With(context.Background(), logger)

	if got := From(ctx); got != logger {
		t.Errorf("From returned %p, want the logger stored by With (%p)", got, logger)
	}
}

// From falls back to slog.Default() so a domain unit test that never sets up
// logging still runs; the fallback is the only slog.Default() call in the tree.
func TestFrom_FallsBackToDefault(t *testing.T) {
	if got := From(context.Background()); got != slog.Default() {
		t.Errorf("From(bare ctx) = %p, want slog.Default() (%p)", got, slog.Default())
	}
}

// A nil logger must not displace the fallback: With(ctx, nil) would otherwise
// hand callers a nil *slog.Logger and panic at the first record.
func TestWith_NilLoggerKeepsFallback(t *testing.T) {
	ctx := With(context.Background(), nil)

	if got := From(ctx); got != slog.Default() {
		t.Errorf("From after With(nil) = %p, want slog.Default() (%p)", got, slog.Default())
	}
}
