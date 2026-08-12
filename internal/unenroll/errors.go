package unenroll

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrMissingAPIKey reports the absence of DN_API_KEY, which unenroll requires
// before attempting unenrollment (design Req 4 / §2.3). It is checked before
// ReadHostID so a keyless unenroll fails with this actionable error rather than
// a not-enrolled one. The module owns this precondition, symmetric with enroll's
// own in-module key check.
//
// It carries EX_CONFIG (78) because a missing variable is an incomplete
// environment: no remote call is attempted, so nothing about the API or the local
// state is implicated.
var ErrMissingAPIKey = terr.New("missing_api_key", output.CodeConfig,
	"set DN_API_KEY to a management API key with host-delete scope",
	"DN_API_KEY is required to unenroll")

// ErrLocalState is a failure reading the local dnclient state unenroll needs (the
// recorded remote host id). It carries EX_IOERR (74): reading
// <config root>/<network>/dnclient.yml is a local filesystem operation, and its
// failure modes — absent, unreadable, unparsable, missing the field — are all
// local. The specific cause (including dnstate.ErrNotEnrolled) stays matchable
// through the wrapped chain.
var ErrLocalState = terr.New("local_state_error", output.CodeIO,
	"check DN_CLIENT_CONFIG_DIR and that the per-network dnclient.yml is readable by this user",
	"unreadable local dnclient state")
