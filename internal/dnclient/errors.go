package dnclient

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrDNClientFailed is any failed invocation of the proprietary dnclient binary:
// the subprocess could not be started (binary missing, not executable) or it ran
// and exited non-zero. It carries EX_IOERR (74), the class for a local operation
// dn-tool could not complete — dn-tool itself is not buggy (70), the inputs were
// not rejected by a remote (65), and the child's own status is not a code from
// dn-tool's taxonomy, so it is reported as structured detail (exit_status)
// instead of leaking into the process exit code.
//
// The one command that does propagate the child's status is run, whose foreground
// daemon *is* the process outcome (design Req 5): it re-forces the code at its own
// outermost layer, and the *exec.ExitError stays reachable through this error's
// cause chain for that mapping.
var ErrDNClientFailed = terr.New("dnclient_failed", output.CodeIO,
	"check that dnclient is installed and executable (run `dn-tool install`), then inspect the dnclient output on stderr",
	"dnclient invocation failed")
