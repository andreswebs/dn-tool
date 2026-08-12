// Package command owns dn-tool's CLI surface behind a framework-free contract:
// Run(args, deps) int (ADR 0003). The urfave/cli wiring lives in this package's
// interior (root.go, commands.go) and appears in no exported identifier, so the
// framework is replaceable without renegotiating the contract that consumers and
// tests rely on.
package command

import (
	"bytes"
	"context"
	"io"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// Deps carries the process resources the CLI is allowed to touch: main injects
// the real ones, tests inject buffers and a fake environment. A field exists
// only because a test must fake it — Getenv backs both the DN_* configuration
// lookup and the DN_LOG_LEVEL logger level, and IsTerminal decides the log
// handler (text for a human, JSON for everything else), which no in-process test
// could otherwise exercise both ways.
type Deps struct {
	In         io.Reader
	Out, Err   io.Writer
	Getenv     func(string) string
	IsTerminal func(io.Writer) bool
}

// withDefaults replaces every unset field with an inert equivalent, so a
// partially populated Deps (a caller that only cares about the exit code) cannot
// nil-panic deep inside a command or fall back to the real process streams.
func (d Deps) withDefaults() Deps {
	if d.In == nil {
		d.In = bytes.NewReader(nil)
	}
	if d.Out == nil {
		d.Out = io.Discard
	}
	if d.Err == nil {
		d.Err = io.Discard
	}
	if d.Getenv == nil {
		d.Getenv = func(string) string { return "" }
	}
	if d.IsTerminal == nil {
		d.IsTerminal = func(io.Writer) bool { return false }
	}
	return d
}

// Run parses args (the command line without the program name), dispatches the
// matched command, and returns the process exit code. It owns the exit boundary,
// so main neither inspects errors nor calls os.Exit mid-command: every command
// returns, and deferred cleanup runs before the code is resolved.
// A failure before the flags are parsed — an unknown global flag — is reported
// by the error envelope alone, which is why there is no bootstrap logger to
// construct here: the envelope is the contract, and a log record repeating it
// would only duplicate envelope content (ADR 0004).
func Run(args []string, deps Deps) int {
	deps = deps.withDefaults()
	return exitCode(runRoot(context.Background(), args, deps), deps.Err)
}

// exitCode maps a command's returned error onto a process exit code and renders
// the failure as the JSON error envelope on w (ADR 0005). The code comes from the
// single boundary mapping (output.ExitCodeFor): a coded error carries its own, and
// an unclassified error is treated as a bug (CodeInternal), which the envelope
// reports as internal_error.
//
// A non-failure status (terr.Status: the --assert-changed no-op, a graceful signal
// shutdown) is the exception: its exit code is the whole contract, so the code is
// resolved exactly as for a failure but no envelope is written (ADR 0002, "the
// error value that carries the exit code prints nothing itself"). Emitting one
// would report ok:false on stderr about a command that succeeded, and — for a
// result-class exit — contradict the ok:true envelope already on stdout. The test
// is by declared class, never by the numeric code: 130/143 sit above the failure
// range yet are not failures, while a daemon passthrough status may be any value
// and is one.
func exitCode(err error, w io.Writer) int {
	if err == nil {
		return output.CodeOK
	}
	if !terr.IsNonFailure(err) {
		output.EmitError(w, err)
	}
	return output.ExitCodeFor(err)
}
