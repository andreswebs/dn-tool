package enroll

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrMissingRequiredConfig is an enrollment attempted without one of the three
// parameters any enrollment needs (DN_API_KEY, DN_NETWORK_ID, DN_ROLE_ID). It
// carries EX_CONFIG (78) because the environment is incomplete; it is raised
// before any remote call, so no host record exists to reconcile.
var ErrMissingRequiredConfig = terr.New("missing_required_config", output.CodeConfig,
	"set the DN_* variable named in the message before enrolling",
	"missing required configuration")

// ErrRoleConflict is a lighthouse/relay configuration that violates the design
// Req 3 rules: the two roles together, a lighthouse with no static address, or a
// lighthouse/relay with no listen port. It carries EX_CONFIG (78) for the same
// reason as any other bad DN_* combination, and is raised before the create call
// so a misconfigured host never leaves a remote record behind.
var ErrRoleConflict = terr.New("role_conflict", output.CodeConfig,
	"fix the DN_IS_LIGHTHOUSE / DN_IS_RELAY / DN_STATIC_ADDRESSES / DN_LISTEN_PORT combination named in the message",
	"invalid host role configuration")

// ErrOrphanedEnrollment is the §2.4 orphan: a remote host record bears the
// enrollment name but no local dnclient config exists. It carries EX_CONFIG (78),
// not EX_SOFTWARE (70): dn-tool is working correctly and the state it found is a
// deployment fact the operator resolves — by re-running with --force, or by
// enrolling under a different DN_HOSTNAME. It is not EX_DATAERR (65) either,
// because the API accepted every request; only the local/remote pairing is wrong.
var ErrOrphanedEnrollment = terr.New("orphaned_enrollment", output.CodeConfig,
	"re-run with --force to delete the stale remote record and enroll afresh, or enroll under a different DN_HOSTNAME",
	"orphaned enrollment")
