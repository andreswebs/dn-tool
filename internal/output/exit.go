package output

import (
	"errors"

	"github.com/urfave/cli/v3"
)

// Process exit codes for the whole CLI family, per ADR 0001 (the exit-code
// taxonomy). Codes 0 and 1 are result classes; 64-78 are the sysexits failure
// classes; 130/143 are the post-cleanup signal codes (128 + signum). Every code
// a command can produce is declared as data in exitcodes.go and asserted by a
// conformance test.
const (
	CodeOK          = 0   // success, clean
	CodeResult      = 1   // recoverable result (e.g. the --assert-changed no-op); not a failure
	CodeUsage       = 64  // EX_USAGE: the CLI surface was misused (bad flags, unknown command)
	CodeData        = 65  // EX_DATAERR: input/payload data rejected (API 4xx, checksum mismatch)
	CodeUnavailable = 69  // EX_UNAVAILABLE: a dependency was unreachable (API transport error / HTTP 5xx)
	CodeInternal    = 70  // EX_SOFTWARE: an unclassified error, treated as a bug
	CodeIO          = 74  // EX_IOERR: a local input/output failure
	CodeConfig      = 78  // EX_CONFIG: a configuration error (bad/missing DN_* vars, env-file, platform)
	CodeSIGINT      = 130 // terminated by SIGINT after graceful shutdown (128 + 2)
	CodeSIGTERM     = 143 // terminated by SIGTERM after graceful shutdown (128 + 15)
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
// other error → CodeInternal (an unclassified failure is treated as a bug).
func ResolveExitCode(err error) int {
	if err == nil {
		return CodeOK
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return CodeInternal
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

// Classify assigns a taxonomy exit code to err at a package boundary, unless err
// already carries an exit code — the first (innermost) class assigned wins, so
// wrapping again at an outer layer never overrides a more specific inner class
// (ADR 0001's registry-as-data invariant; mirrors exitWithError's ExitCoder
// check). A nil err is returned unchanged. This is the boundary counterpart to
// ExitError: ExitError forces a code, Classify defers to any code already set.
func Classify(err error, code int) error {
	if err == nil {
		return nil
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		return err
	}
	return &exitError{err: err, code: code}
}
