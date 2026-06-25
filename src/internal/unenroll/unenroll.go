// Package unenroll removes a host from the network: it deletes the remote host
// record then the local dnclient configuration, in that order (design §2.5).
package unenroll

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnstate"
	"github.com/andreswebs/dn-tool/internal/output"
)

// Input is the validated request data for Unenroll: the network whose host
// record to remove, and the management API key required before any remote call.
// It is the narrow alternative to a whole *config.Config — unenroll reads only
// these two fields.
type Input struct {
	NetworkName string
	APIKey      config.Secret
}

// ErrMissingAPIKey reports the absence of DN_API_KEY, which unenroll requires
// before attempting unenrollment (design Req 4 / §2.3). It is checked before
// ReadHostID so a keyless unenroll fails with this actionable error rather than
// a not-enrolled one. The module owns this precondition, symmetric with enroll's
// own in-module key check.
var ErrMissingAPIKey = errors.New("DN_API_KEY is required to unenroll")

// HostDeleter deletes a remote host record. *api.Client satisfies it; tests
// substitute a fake. DeleteHost reports a 404 as nil (idempotent already-absent
// success), so this layer cannot distinguish 2xx from 404 — both leave the
// remote record gone.
type HostDeleter interface {
	DeleteHost(ctx context.Context, hostID string) error
}

// Deps are the injected collaborators. ConfigRoot is the dnclient config root
// (default /etc/defined, supplied by the command layer; tests use a temp dir).
type Deps struct {
	API        HostDeleter
	ConfigRoot string
}

// unenrollFailureAdvisory is appended to a delete-failure error so the operator
// is not surprised by the retained-local outcome. It states the §2.5 invariant
// consequence plainly: the remote record may still exist, the local config is
// deliberately kept, and the host therefore stays enrolled and resumes on the
// next boot. No orphan (local-absent / remote-present) is produced, so no
// --force recovery is needed — a later unenroll can still complete the delete.
const unenrollFailureAdvisory = "remote record may persist, local config retained, host remains enrolled and will resume on next boot"

// Unenroll requires the API key first (Req 4: a key before any remote call),
// then reads the remote host ID from the local config, deletes the remote
// record, and only then removes the local network config directory. ctx is
// already bounded by DN_API_TIMEOUT at the command layer.
//
// The local config is removed strictly after a successful (or idempotent-404)
// delete, so a delete failure retains it and keeps the local/remote invariant
// (design §2.5): no orphan is produced.
func Unenroll(ctx context.Context, in Input, deps Deps) (output.Result, error) {
	if in.APIKey == "" {
		return output.Result{}, ErrMissingAPIKey
	}

	hostID, err := dnstate.ReadHostID(deps.ConfigRoot, in.NetworkName)
	if err != nil {
		return output.Result{}, err
	}

	if err := deps.API.DeleteHost(ctx, hostID); err != nil {
		return output.Result{}, fmt.Errorf("deleting remote host %s: %w; %s", hostID, err, unenrollFailureAdvisory)
	}

	networkDir := filepath.Join(deps.ConfigRoot, in.NetworkName)
	if err := os.RemoveAll(networkDir); err != nil {
		return output.Result{}, fmt.Errorf("removing local config %s: %w", networkDir, err)
	}

	return output.Result{
		Action:  "unenroll",
		Changed: true,
		HostID:  hostID,
		Network: in.NetworkName,
	}, nil
}
