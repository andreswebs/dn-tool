package enroll

import (
	"context"
	"errors"
	"fmt"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/dnstate"
	"github.com/andreswebs/dn-tool/internal/output"
)

// API is the management-API seam the enrollment state machine depends on.
// *api.Client satisfies it; tests substitute a mock. It is the lookup
// (ListHosts), create (CreateHostAndEnrollmentCode) and delete (DeleteHost) the
// §2.4 cells need — the local-config-present no-op (row 1) calls none of them.
type API interface {
	ListHosts(ctx context.Context, networkID string) ([]api.Host, error)
	CreateHostAndEnrollmentCode(ctx context.Context, req api.CreateHostRequest) (*api.HostAndCode, error)
	DeleteHost(ctx context.Context, hostID string) error
}

// Deps are the injected collaborators for Enroll. ConfigRoot is the dnclient
// config root (default /etc/defined, supplied by the command layer; tests use a
// temp dir) probed for the "already enrolled" signal. DNClient runs the
// `dnclient enroll` hand-off once a remote record and code are obtained. Force
// is the operator's --force opt-in to delete a stale remote record and enroll
// afresh when an orphan is detected (§2.4 row 4).
type Deps struct {
	API        API
	ConfigRoot string
	DNClient   dnclient.Client
	Force      bool
}

// Enroll runs the §2.4 enrollment state machine over two truth sources: the
// local dnclient config and the remote API host record.
//
//   - Local config present → already enrolled: no changes, no API or dnclient
//     calls, Changed=false (exit 0, or exit 2 under --assert-changed).
//   - Local absent + remote absent → create the remote host record, obtain the
//     single-use enrollment code, and run `dnclient enroll` with it.
//   - Local absent + remote present → orphan: fail with guidance to re-run with
//     --force, unless Force is set, in which case delete the stale record by id
//     and then run the create path afresh.
//
// Any management-API error aborts before `dnclient enroll` runs, so a created
// record is never left behind without a started enrollment beyond the narrow
// case where the subprocess itself fails (the §2.5 enroll-path orphan).
func Enroll(ctx context.Context, cfg *config.Config, deps Deps) (output.Result, error) {
	// Role/port/static-address gates run first, so a misconfigured host fails
	// fast and creates no remote record (design Req 3).
	if err := validateRoles(cfg); err != nil {
		return output.Result{}, err
	}

	if dnstate.ConfigExists(deps.ConfigRoot, cfg.NetworkName) {
		return output.Result{Action: "enroll", Changed: false}, nil
	}

	in, err := newEnrollInput(cfg)
	if err != nil {
		return output.Result{}, err
	}

	// Is the enrollment name already taken remotely? findRemoteRecord hides the
	// no-name-filter list-and-match workaround (reference §4.2); a list failure
	// leaves presence unknown and aborts there rather than risk a duplicate.
	existing, err := findRemoteRecord(ctx, deps.API, in.Request.NetworkID, in.Request.Name)
	if err != nil {
		return output.Result{}, err
	}
	if existing != nil {
		// Orphan: a remote record bears the enrollment name but no local config
		// exists. Deleting it silently could disrupt a host legitimately enrolled
		// under the same name elsewhere, so default to failing with guidance and
		// gate the destructive recovery behind --force (§2.4 rows 3–4).
		if !deps.Force {
			return output.Result{}, fmt.Errorf("remote host record %q (id %s) already exists but no local config is present (orphaned enrollment); re-run with --force to delete the stale record and enroll afresh", existing.Name, existing.ID)
		}
		if err := deps.API.DeleteHost(ctx, existing.ID); err != nil {
			return output.Result{}, fmt.Errorf("deleting stale host record %s: %w", existing.ID, err)
		}
	}

	hc, err := deps.API.CreateHostAndEnrollmentCode(ctx, in.Request)
	if err != nil {
		// A Create-time duplicate on "name" means a record appeared after the
		// findRemoteRecord pre-check (a TOCTOU race, or the pre-check was skipped):
		// surface the §2.4 orphan guidance instead of a generic create error.
		// Under --force the pre-check already deleted any record it found, so a
		// duplicate here means one was recreated in the race window — the safe
		// response is the same retry guidance, not an auto-delete loop.
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.Has("ERR_DUPLICATE_VALUE", "name") {
			return output.Result{}, fmt.Errorf("remote host record %q already exists but no local config is present (orphaned enrollment); re-run with --force to delete the stale record and enroll afresh", in.Request.Name)
		}
		return output.Result{}, fmt.Errorf("creating host record: %w", err)
	}

	// The code is single-use and sensitive: revealed only here, handed straight
	// to the subprocess in-memory, never logged or persisted (Req 7 / SEC5).
	if err := deps.DNClient.Enroll(ctx, in.NetworkName, hc.EnrollmentCode.Code.Reveal()); err != nil {
		return output.Result{}, err
	}

	return output.Result{
		Action:  "enroll",
		Changed: true,
		HostID:  hc.Host.ID,
		Network: in.NetworkName,
	}, nil
}
