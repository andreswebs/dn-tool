package main

import (
	"context"
	"time"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/enroll"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// defaultEnrollTimeout bounds the enroll command's API and dnclient work when
// DN_API_TIMEOUT is unset (~30s per the §2.3 defaults). An operator can override
// it with DN_API_TIMEOUT.
const defaultEnrollTimeout = 30 * time.Second

// enrollAction is the wired enroll command: it resolves configuration (env-file
// beneath the live environment) and runs the orphan-aware state machine against
// the production config root, passing the --force opt-in. It is a resultAction,
// so withResult emits its Result and applies the exit-code / --assert-changed
// semantics.
func enrollAction(ctx context.Context, cmd *cli.Command) (output.Result, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return output.Result{}, err
	}
	return runEnroll(ctx, cfg, cfg.ClientConfigDir, cmd.Bool("force"))
}

// runEnroll is the testable core: it takes a resolved Config, the dnclient config
// root (injected so tests use a temp dir) and the --force flag, then runs the
// §2.4 state machine. The context is bounded by DN_API_TIMEOUT (or
// defaultEnrollTimeout). Required-parameter validation (DN_API_KEY/DN_NETWORK_ID/
// DN_ROLE_ID) lives inside the state machine's create path, so the
// already-enrolled no-op (§2.4 row 1) needs none of them.
func runEnroll(ctx context.Context, cfg *config.Config, configRoot string, force bool) (output.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout(defaultEnrollTimeout))
	defer cancel()

	return enroll.Enroll(ctx, cfg, enroll.Deps{
		API:        api.New(cfg),
		ConfigRoot: configRoot,
		DNClient:   dnclient.NewExecClient(dnclient.BinaryPath(cfg.ClientBinDir)),
		Force:      force,
	})
}
