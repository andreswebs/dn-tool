package command

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// Behavior 1: a command returning nil yields no error, so Run reports success.
func TestRun_SuccessCommandReturnsNil(t *testing.T) {
	app := &cli.Command{
		Name:   "stub",
		Writer: &bytes.Buffer{},
		Action: func(_ context.Context, _ *cli.Command) error { return nil },
	}
	if err := app.Run(context.Background(), []string{"stub"}); err != nil {
		t.Errorf("success command returned error: %v", err)
	}
}

// Behavior 4: commands return errors (never os.Exit), so deferred cleanup runs
// on the error path.
func TestRun_DeferredCleanupRunsOnErrorPath(t *testing.T) {
	cleaned := false
	app := &cli.Command{
		Name:      "stub",
		Writer:    &bytes.Buffer{},
		ErrWriter: &bytes.Buffer{},
		Action: func(_ context.Context, _ *cli.Command) error {
			defer func() { cleaned = true }()
			return errors.New("fail after cleanup")
		},
	}
	neutralize(app)
	err := app.Run(context.Background(), []string{"stub"})
	if err == nil {
		t.Fatal("expected error from stub command")
	}
	if !cleaned {
		t.Error("deferred cleanup did not run; command must return, not os.Exit")
	}
	// Plain errors flow out of the framework unmapped; the exit boundary in Run
	// centralizes them to CodeInternal.
	if got := output.ExitCodeFor(err); got != output.CodeInternal {
		t.Errorf("ExitCodeFor(plain err) = %d, want %d", got, output.CodeInternal)
	}
}
