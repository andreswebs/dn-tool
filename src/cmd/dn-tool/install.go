package main

import (
	"context"
	"runtime"
	"time"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/dninstall"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// defaultInstallTimeout bounds the install command when DN_API_TIMEOUT is unset.
// It is more generous than the API-only deadlines (§2.3) because it must cover
// the dnclient binary download, not just a JSON call; an operator can override
// it with DN_API_TIMEOUT.
const defaultInstallTimeout = 60 * time.Second

// installAction is the wired install command: it resolves configuration and runs
// the install core. It is a resultAction, so withResult emits the Result and
// applies the exit-code / --assert-changed semantics.
func installAction(ctx context.Context, cmd *cli.Command) (output.Result, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return output.Result{}, err
	}
	return runInstall(ctx, cfg)
}

// runInstall is the testable core: it gates the OS/arch at this boundary (so
// non-install commands stay runnable on non-Linux dev hosts, design note
// dt-koaf), then runs the download/verify/place orchestration bounded by
// DN_API_TIMEOUT (or defaultInstallTimeout). The run command reuses it verbatim
// as its install step.
func runInstall(ctx context.Context, cfg *config.Config) (output.Result, error) {
	platform, err := dninstall.DetectPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return output.Result{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout(defaultInstallTimeout))
	defer cancel()

	client := api.New(cfg)
	return dninstall.Install(ctx, dninstall.InstallDeps{
		API:        client,
		HTTPClient: client.HTTPClient(),
		Platform:   platform,
	}, dninstall.InstallOptions{
		BinaryPath: dnclient.BinaryPath(cfg.ClientBinDir),
		Version:    cfg.ClientVersion,
	})
}
