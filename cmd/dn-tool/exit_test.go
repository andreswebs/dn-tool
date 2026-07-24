package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// captureExit overrides the cli package globals so exitWithError records the
// resolved code and message instead of terminating the test process.
func captureExit(t *testing.T) (code *int, stderr *bytes.Buffer) {
	t.Helper()
	oldExiter := cli.OsExiter
	oldErrWriter := cli.ErrWriter
	t.Cleanup(func() {
		cli.OsExiter = oldExiter
		cli.ErrWriter = oldErrWriter
	})
	captured := -1
	code = &captured
	stderr = &bytes.Buffer{}
	cli.OsExiter = func(c int) { captured = c }
	cli.ErrWriter = stderr
	return code, stderr
}

func TestExitWithError_PlainErrorMapsToCodeInternal(t *testing.T) {
	code, stderr := captureExit(t)
	exitWithError(errors.New("boom"))
	if *code != output.CodeInternal {
		t.Errorf("exit code = %d, want %d", *code, output.CodeInternal)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), "boom")
	}
}

func TestExitWithError_ExitCoderHonored(t *testing.T) {
	code, _ := captureExit(t)
	exitWithError(output.ExitError(errors.New("no change"), output.CodeResult))
	if *code != output.CodeResult {
		t.Errorf("exit code = %d, want %d", *code, output.CodeResult)
	}
}

func TestExitWithError_WrappedExitCoderHonored(t *testing.T) {
	code, _ := captureExit(t)
	wrapped := output.ExitError(errors.New("inner"), output.CodeConfig)
	exitWithError(wrapped)
	if *code != output.CodeConfig {
		t.Errorf("wrapped ExitCoder: exit code = %d, want %d", *code, output.CodeConfig)
	}
}

// Behavior 1: a command returning nil yields no exit call (process exits 0).
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
	err := app.Run(context.Background(), []string{"stub"})
	if err == nil {
		t.Fatal("expected error from stub command")
	}
	if !cleaned {
		t.Error("deferred cleanup did not run; command must return, not os.Exit")
	}
	// Plain errors flow out of Run unmapped; main centralizes them to CodeInternal.
	if got := output.ResolveExitCode(err); got != output.CodeInternal {
		t.Errorf("ResolveExitCode(plain err) = %d, want %d", got, output.CodeInternal)
	}
}
