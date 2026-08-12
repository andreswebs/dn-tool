package run

import (
	"errors"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrSignalShutdown is the class for a shutdown initiated by SIGINT or SIGTERM.
// It is also the non-nil sentinel attached to a *clean* signal-triggered
// shutdown, so the 128+signum exit code has an error to carry: a graceful
// shutdown returns nil from runAndUnenroll, and a nil error cannot carry a code.
//
// Its class exit code is 143 (SIGTERM, the container-stop case), but the real
// code is per-invocation — 130 for SIGINT — and is supplied by exitErr. Both codes
// are declared in run's exit-code registry, which is where a consumer reads the
// variance; the hint is not a place to restate a code mapping (ADR 0002).
//
// It is a status rather than a failure (terr.NewStatus): a clean container stop
// is reported as the signal code and not as a failure (design Req 5), so the
// boundary writes no error envelope for it and stderr carries log records only.
// There is nothing to remediate, hence no hint.
var ErrSignalShutdown = terr.NewStatus("signal_shutdown", output.CodeSIGTERM, "",
	"terminated by signal")

// ErrDaemonExit is the class for the foreground dnclient daemon terminating on its
// own. Its class exit code is EX_SOFTWARE (70) — the code used when the daemon
// left no clean status (a non-exec failure, or a signal kill, whose ExitCode() is
// -1) and there is therefore nothing meaningful to propagate. When the daemon did
// exit with a status N, run propagates N verbatim (design Req 5) through exitErr.
// A non-zero daemon exit is a genuine failure of the wrapped process, so it keeps
// its error envelope, and its hint points the operator at the daemon's own
// diagnostics — the only place the reason for the exit exists.
var ErrDaemonExit = terr.New("daemon_exit", output.CodeInternal,
	"read the dnclient daemon's own output on stderr for why it stopped, then re-run `dn-tool run` once the cause is fixed",
	"the dnclient daemon exited")

// exitErr is the ADR 0002 per-instance form: a coded error whose exit code is
// computed per invocation (the signal's 128+signum, or the daemon's status N)
// while Code, Hint and identity delegate to its class sentinel. errors.Is against
// the class therefore still matches, and the boundary reads one interface.
type exitErr struct {
	class *terr.E // ErrSignalShutdown or ErrDaemonExit
	code  int     // the per-invocation code: 130/143, or the daemon's N
	cause error
}

var _ terr.Coded = (*exitErr)(nil)

// newExitErr pairs a class sentinel with the exit code this invocation must
// produce and the error being reported.
func newExitErr(class *terr.E, code int, cause error) *exitErr {
	return &exitErr{class: class, code: code, cause: cause}
}

func (e *exitErr) Code() string  { return e.class.Code() }
func (e *exitErr) Hint() string  { return e.class.Hint() }
func (e *exitErr) Unwrap() error { return e.cause }

// NonFailure forwards the class's kind, so the signal shutdown's status marker
// survives the per-invocation wrapper: without this the boundary would find the
// wrapped cause's classification instead and emit an envelope for a graceful stop.
func (e *exitErr) NonFailure() bool { return e.class.NonFailure() }

// ExitCode overrides the class's code with the per-invocation one.
func (e *exitErr) ExitCode() int { return e.code }

// Error reports the wrapped failure's own message, not the class label: this
// layer only supplies an exit code, so it must not prepend text to the daemon's
// or the shutdown's message.
func (e *exitErr) Error() string {
	if e.cause == nil {
		return e.class.Error()
	}
	return e.cause.Error()
}

// Is matches the class sentinel, so a caller can branch on the shutdown reason
// without knowing about this wrapper.
func (e *exitErr) Is(target error) bool { return errors.Is(e.class, target) }
