package main

import (
	"context"
	"time"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/unenroll"
	"github.com/urfave/cli/v3"
)

// defaultUnenrollTimeout bounds the unenroll API work when DN_API_TIMEOUT is
// unset. It is the binary-side counterpart to the module's TimeoutStopSec
// (design §2.5, finding D5) and must stay strictly shorter so the binary always
// wins the service-stop race; ~10s per the §2.3 defaults.
const defaultUnenrollTimeout = 10 * time.Second

// unenrollAction is the wired unenroll command: it resolves configuration
// (env-file beneath the live environment) and runs the unenroll against the
// production config root. It is a resultAction, so withResult emits its Result
// and applies the exit-code / --assert-changed semantics.
func unenrollAction(ctx context.Context, cmd *cli.Command) (output.Result, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return output.Result{}, err
	}
	return runUnenroll(ctx, cfg, cfg.ClientConfigDir)
}

// runUnenroll is the testable core: it takes a resolved Config and the dnclient
// config root (injected so tests use a temp dir) and performs the bounded
// remote-delete-then-local-removal. The API key is required up front. The
// context is bounded by DN_API_TIMEOUT (or defaultUnenrollTimeout) so the binary
// always returns within the surrounding service-stop deadline; the §2.5
// local/remote invariant (no local removal until the delete succeeds or the
// record is already absent) is enforced inside unenroll.Unenroll.
func runUnenroll(ctx context.Context, cfg *config.Config, configRoot string) (output.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout(defaultUnenrollTimeout))
	defer cancel()

	return unenroll.Unenroll(ctx, unenroll.Input{NetworkName: cfg.NetworkName, APIKey: cfg.APIKey}, unenroll.Deps{
		API:        api.New(cfg),
		ConfigRoot: configRoot,
	})
}
