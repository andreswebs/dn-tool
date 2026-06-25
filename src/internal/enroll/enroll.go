// Package enroll builds the create-host request from configuration and runs the
// orphan-aware enrollment state machine (design §2.4).
package enroll

import (
	"fmt"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
)

// enrollInput is the validated create-path input for the §2.4 create cells
// (rows 2-4): the mapped create-host request plus the network name that drives
// the dnclient tun device and the emitted result. It is produced only after the
// row-1 already-enrolled no-op, so the no-op path needs none of the required
// params newEnrollInput validates. Concentrating the inputs here keeps the state
// machine reading from one value instead of re-deriving them from cfg.
type enrollInput struct {
	Request     api.CreateHostRequest
	NetworkName string
}

// newEnrollInput validates and maps the create-path config into an enrollInput.
// It wraps buildCreateRequest (the required-param checks + request mapping) and
// carries the network name through, so the create path never reads cfg again.
func newEnrollInput(cfg *config.Config) (enrollInput, error) {
	req, err := buildCreateRequest(cfg)
	if err != nil {
		return enrollInput{}, err
	}
	return enrollInput{Request: req, NetworkName: cfg.NetworkName}, nil
}

// buildCreateRequest maps the resolved config onto the create-host request body
// (api.CreateHostRequest, reference §4.1) for the common host path, after
// validating the three parameters dn-tool requires for any enrollment: the
// management API key (DN_API_KEY), the network ID (DN_NETWORK_ID), and the role
// ID (DN_ROLE_ID). A missing one fails with an error naming the first such
// parameter, in that order.
//
// The tun device name (matching DN_NETWORK_NAME) is deliberately absent from the
// body: the v2 create-host endpoint has no such field. The network name drives
// the tun device at `dnclient enroll` time, not in this POST.
//
// A lighthouse is enrolled with the lighthouse role plus its static addresses; a
// relay with the relay role; both carry their configured listen port. The listen
// port is passed through verbatim — 0 (the plain-host default) means auto-select
// per the API. The role rules themselves (mutual exclusion, lighthouse needs a
// static address, lighthouse/relay need a non-zero port) are enforced by
// validateRoles before this runs, so this maps already-valid input.
func buildCreateRequest(cfg *config.Config) (api.CreateHostRequest, error) {
	switch {
	case cfg.APIKey == "":
		return api.CreateHostRequest{}, missingParam("DN_API_KEY")
	case cfg.NetworkID == "":
		return api.CreateHostRequest{}, missingParam("DN_NETWORK_ID")
	case cfg.RoleID == "":
		return api.CreateHostRequest{}, missingParam("DN_ROLE_ID")
	}

	req := api.CreateHostRequest{
		Name:       cfg.Hostname,
		NetworkID:  cfg.NetworkID,
		RoleID:     cfg.RoleID,
		Tags:       cfg.Tags,
		ListenPort: cfg.ListenPort,
	}
	if cfg.IPAddress != "" {
		req.IPAddresses = []string{cfg.IPAddress}
	}
	if cfg.IsLighthouse {
		req.IsLighthouse = true
		req.StaticAddresses = cfg.StaticAddrs
	}
	if cfg.IsRelay {
		req.IsRelay = true
	}
	return req, nil
}

func missingParam(name string) error {
	return fmt.Errorf("missing required configuration: %s", name)
}
