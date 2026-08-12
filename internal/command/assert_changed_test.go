package command

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// runAssertChanged drives a stub command wrapped by withResult, with
// --assert-changed optionally set, returning the resolved process exit code and
// the bytes written to stdout. It mirrors production exactly: the framework's own
// exit handling is neutralized, so whatever the action returns reaches the same
// exit boundary Run uses.
func runAssertChanged(t *testing.T, assertChanged bool, res output.Result, actionErr error) (code int, stdout string) {
	t.Helper()
	var out, errOut bytes.Buffer
	app := &cli.Command{
		Name:      rootName,
		Writer:    &out,
		ErrWriter: &errOut,
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
	neutralize(app)
	args := []string{rootName}
	if assertChanged {
		args = append(args, "--assert-changed")
	}
	args = append(args, "stub")
	return exitCode(app.Run(context.Background(), args), &errOut), out.String()
}

// Behavior 1: no change + --assert-changed → the result code (exit 1).
func TestAssertChanged_NoOpExitsResult(t *testing.T) {
	code, stdout := runAssertChanged(t, true, output.Result{Action: "enroll", Changed: false}, nil)
	if code != output.CodeResult {
		t.Errorf("exit code = %d, want %d", code, output.CodeResult)
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

// Behavior 4: a command error takes precedence over the Changed flag even with
// --assert-changed set and a no-op result. The result code (1) is a result
// class and must never collide with a failure: an unclassified error maps to
// CodeInternal (70), never to the assert no-op result code.
func TestAssertChanged_FailureIgnoresResultCodeWithFlag(t *testing.T) {
	code, _ := runAssertChanged(t, true, output.Result{Action: "enroll", Changed: false}, errors.New("boom"))
	if code != output.CodeInternal {
		t.Errorf("exit code = %d, want %d", code, output.CodeInternal)
	}
	if code == output.CodeResult {
		t.Error("a failure must never surface the assert no-op result code")
	}
}
