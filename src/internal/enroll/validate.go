package enroll

import (
	"fmt"

	"github.com/andreswebs/dn-tool/internal/config"
)

// validateRoles enforces the lighthouse/relay configuration rules (design Req 3)
// before any remote API call, so a misconfigured host fails fast and creates no
// remote record. Returns nil for a plain host (neither role set).
//
// Rules, in check order:
//   - lighthouse and relay are mutually exclusive;
//   - a lighthouse requires at least one static address (DN_STATIC_ADDRESSES);
//   - a lighthouse or relay requires a non-zero listen port (DN_LISTEN_PORT).
func validateRoles(cfg *config.Config) error {
	if cfg.IsLighthouse && cfg.IsRelay {
		return fmt.Errorf("host configured as both lighthouse (DN_IS_LIGHTHOUSE) and relay (DN_IS_RELAY); these are mutually exclusive")
	}

	if cfg.IsLighthouse && len(cfg.StaticAddrs) == 0 {
		return fmt.Errorf("lighthouse requires at least one static address (DN_STATIC_ADDRESSES)")
	}

	if (cfg.IsLighthouse || cfg.IsRelay) && cfg.ListenPort == 0 {
		return fmt.Errorf("lighthouse/relay requires a non-zero listen port (DN_LISTEN_PORT)")
	}

	return nil
}
