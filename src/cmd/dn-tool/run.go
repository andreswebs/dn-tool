package main

import (
	"context"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/run"
	"github.com/urfave/cli/v3"
)

// runAction is the wired run command: it resolves configuration, then runs the
// container/pipeline lifecycle against the production config root. run is a
// long-running foreground command, so it is a plain cli action (not a
// resultAction): its outcome is the daemon's termination, which run.Lifecycle
// returns as an output.ExitError carrying the daemon's exit code for main's
// exitWithError to honor. --assert-changed does not apply.
func runAction(ctx context.Context, cmd *cli.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	return runRun(ctx, cfg, cfg.ClientConfigDir)
}

// runRun is the testable core: it runs the install -> enroll -> foreground
// daemon -> unenroll-on-termination lifecycle (design Req 5 / §2.7). The config
// root is injected so tests use a temp dir.
func runRun(ctx context.Context, cfg *config.Config, configRoot string) error {
	return run.Lifecycle(ctx, cfg, productionRunDeps(cfg, configRoot))
}

// productionRunDeps assembles the lifecycle dependencies from the standalone
// command cores, so run reuses install/enroll/unenroll verbatim rather than
// forking their logic (the internal/run design intent). enroll runs without
// --force (the container path has no such flag), and the daemon execs the
// dnclient binary install placed under DN_CLIENT_BIN_DIR. WaitReady polls the
// daemon's control socket (DN_CLIENT_SOCKET) so enroll — which connects to it —
// only runs once the daemon is up. UnenrollTimeout mirrors the unenroll command's
// bound so a configured DN_API_TIMEOUT is honored on shutdown rather than capped
// by run's default.
func productionRunDeps(cfg *config.Config, configRoot string) run.Deps {
	return run.Deps{
		Install: func(ctx context.Context) (output.Result, error) {
			return runInstall(ctx, cfg)
		},
		Daemon: dnclient.NewExecClient(dnclient.BinaryPath(cfg.ClientBinDir)),
		WaitReady: func(ctx context.Context) error {
			return dnclient.WaitForSocket(ctx, cfg.ClientSocket)
		},
		Enroll: func(ctx context.Context) (output.Result, error) {
			return runEnroll(ctx, cfg, configRoot, false)
		},
		Unenroll: func(ctx context.Context) (output.Result, error) {
			return runUnenroll(ctx, cfg, configRoot)
		},
		UnenrollTimeout: cfg.Timeout(defaultUnenrollTimeout),
	}
}
