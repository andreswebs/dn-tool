package output

import (
	"errors"

	"github.com/urfave/cli/v3"
)

// Canonical process exit codes shared by every command (design Req 8). Code 2 is
// reserved for the --assert-changed no-op (a distinct, non-error status) and must
// never be reused for failures.
const (
	CodeOK         = 0 // success: a change was made, or a no-op without --assert-changed
	CodeError      = 1 // failure
	CodeNoOpAssert = 2 // no-op while --assert-changed is set
)

var _ cli.ExitCoder = (*exitError)(nil)

// exitError pairs an error with a process exit code while preserving the wrapped
// chain, so callers can still use errors.Is/As across it.
type exitError struct {
	err  error
	code int
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) ExitCode() int { return e.code }
func (e *exitError) Unwrap() error { return e.err }

// ResolveExitCode maps a command's returned error to a process exit code: nil →
// CodeOK, an error carrying an exit code (a cli.ExitCoder) → that code, and any
// other error → CodeError.
func ResolveExitCode(err error) int {
	if err == nil {
		return CodeOK
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return CodeError
}

// ExitError wraps err as a cli.ExitCoder carrying code, so a command can signal a
// specific process exit status by returning the error rather than calling
// os.Exit — deferred cleanup still runs. A nil err yields a true nil interface.
func ExitError(err error, code int) cli.ExitCoder {
	if err == nil {
		return nil
	}
	return &exitError{err: err, code: code}
}
