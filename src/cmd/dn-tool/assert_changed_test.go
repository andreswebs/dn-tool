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

// runAssertChanged drives a stub command wrapped by withResult through the app,
// with --assert-changed optionally set, returning the resolved process exit code
// and the bytes written to stdout. It mirrors production exactly: cli auto-exits
// (via cli.OsExiter) inside Run when the action returns an ExitCoder, and main's
// exitWithError maps any remaining non-nil error afterward. captureExit records
// the OsExiter code (sentinel -1 = never called = success).
func runAssertChanged(t *testing.T, assertChanged bool, res output.Result, actionErr error) (code int, stdout string) {
	t.Helper()
	captured, _ := captureExit(t)
	var out bytes.Buffer
	app := &cli.Command{
		Name:   "dn-tool",
		Writer: &out,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "assert-changed"},
		},
		Commands: []*cli.Command{
			{
				Name: "stub",
				Action: withResult(func(_ context.Context, _ *cli.Command) (output.Result, error) {
					return res, actionErr
				}),
			},
		},
	}
	args := []string{"dn-tool"}
	if assertChanged {
		args = append(args, "--assert-changed")
	}
	args = append(args, "stub")
	if err := app.Run(context.Background(), args); err != nil {
		exitWithError(err)
	}
	if *captured == -1 {
		return output.CodeOK, out.String()
	}
	return *captured, out.String()
}

// Behavior 1: no change + --assert-changed → exit 2.
func TestAssertChanged_NoOpExitsTwo(t *testing.T) {
	code, stdout := runAssertChanged(t, true, output.Result{Action: "enroll", Changed: false}, nil)
	if code != output.CodeNoOpAssert {
		t.Errorf("exit code = %d, want %d", code, output.CodeNoOpAssert)
	}
	// The result is still emitted on stdout before the no-op status is signalled,
	// so a pipeline can read the outcome even when asserting no change.
	if !strings.Contains(stdout, `"changed":false`) {
		t.Errorf("stdout = %q, want it to contain the result object", stdout)
	}
}

// Behavior 2: a change + --assert-changed → exit 0.
func TestAssertChanged_ChangeExitsZero(t *testing.T) {
	code, _ := runAssertChanged(t, true, output.Result{Action: "enroll", Changed: true}, nil)
	if code != output.CodeOK {
		t.Errorf("exit code = %d, want %d", code, output.CodeOK)
	}
}

// Behavior 3: no change without --assert-changed → exit 0 (the no-op is only a
// distinct status when the operator asked for the assertion).
func TestAssertChanged_NoOpWithoutFlagExitsZero(t *testing.T) {
	code, _ := runAssertChanged(t, false, output.Result{Action: "enroll", Changed: false}, nil)
	if code != output.CodeOK {
		t.Errorf("exit code = %d, want %d", code, output.CodeOK)
	}
}

// Behavior 4: a command error → exit 1 even with --assert-changed set and a
// no-op result. 2 is reserved for the assert-changed no-op and must never
// collide with a failure; the error path takes precedence over the Changed flag.
func TestAssertChanged_FailureExitsOneWithFlag(t *testing.T) {
	code, _ := runAssertChanged(t, true, output.Result{Action: "enroll", Changed: false}, errors.New("boom"))
	if code != output.CodeError {
		t.Errorf("exit code = %d, want %d", code, output.CodeError)
	}
}
