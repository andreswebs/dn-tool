package output

import (
	"errors"

	"github.com/andreswebs/dn-tool/internal/terr"
)

// Process exit codes, per ADR 0001 (the exit-code taxonomy). Codes 0 and 1 are
// result classes; 64-78 are the sysexits failure classes; 130/143 are the
// post-cleanup signal codes (128 + signum). Every code a command can produce is
// declared as data in exitcodes.go and asserted by a conformance test.
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

// ExitCodeFor maps err to its process exit code. A coded error (ADR 0002)
// carries its own; anything unclassified is an internal error, exit 70
// (EX_SOFTWARE), so a missing classification is loud instead of silently
// mislabeled.
//
// errors.As walks the chain outermost-first, so the outermost coded error wins.
// That is load-bearing for run: its signal override and its daemon-status
// passthrough both wrap an already-coded error to force the process exit status,
// and both rely on the outer code overriding the inner class.
func ExitCodeFor(err error) int {
	var coded terr.Coded
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return CodeInternal
}
