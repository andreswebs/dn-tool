// Command dn-tool enrolls and unenrolls Linux hosts in a defined.net Managed
// Nebula network, wrapping the defined.net REST API and the dnclient daemon.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/version"
	"github.com/urfave/cli/v3"
)

// loadConfig resolves the dn-tool configuration for a command: the live
// environment overlaid on an optional --env-file (design §2.3 precedence). It is
// the shared bootstrap each command uses before contacting the API.
func loadConfig(cmd *cli.Command) (*config.Config, error) {
	return config.LoadWithEnvFile(cmd.String("env-file"), os.Getenv)
}

// errNoChange is the non-nil sentinel carried by the --assert-changed no-op exit.
// output.ExitError(nil, …) collapses to a true nil interface (the typed-nil
// guard), so signalling the distinct code-2 status needs a real error to attach.
var errNoChange = errors.New("no change made")

// resultAction is a command action that yields a machine-readable output.Result
// alongside its error, so the wrapper can both emit the result and read its
// Changed flag for --assert-changed.
type resultAction func(context.Context, *cli.Command) (output.Result, error)

// withResult adapts a result-producing action into a cli action: it writes the
// Result as one JSON object to stdout, then — when --assert-changed is set and
// the command made no change — returns the distinct, non-error exit-2 status.
// A command error propagates unchanged (exit 1) regardless of --assert-changed;
// code 2 is never produced for failures.
func withResult(action resultAction) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		res, err := action(ctx, cmd)
		if err != nil {
			return err
		}
		if werr := output.WriteResult(cmd.Root().Writer, res); werr != nil {
			return werr
		}
		if cmd.Bool("assert-changed") && !res.Changed {
			return output.ExitError(errNoChange, output.CodeNoOpAssert)
		}
		return nil
	}
}

// logOptions derives the logger configuration from the parsed command: the
// --log-text global flag selects plain-text output, and the level still comes
// from DN_LOG_LEVEL in the live environment (env-file precedence for the level
// is later command-wiring's concern).
func logOptions(cmd *cli.Command) output.LogOptions {
	return output.LogOptions{
		Level: os.Getenv("DN_LOG_LEVEL"),
		Text:  cmd.Bool("log-text"),
	}
}

func newApp() *cli.Command {
	return &cli.Command{
		Name:    "dn-tool",
		Usage:   "enroll and unenroll Linux hosts in a defined.net Managed Nebula network",
		Version: version.Current(),
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			slog.SetDefault(output.NewLogger(os.Stderr, logOptions(cmd)))
			return ctx, nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "env-file",
				Usage:   "load configuration from key-value `FILE`",
				Sources: cli.EnvVars("DN_ENV_FILE"),
			},
			&cli.BoolFlag{
				Name:  "assert-changed",
				Usage: "exit non-zero (distinct, non-error) when the command makes no change",
			},
			&cli.BoolFlag{
				Name:  "log-text",
				Usage: "emit human-readable text logs instead of JSON",
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "install",
				Usage:  "download and verify dnclient into the binary directory",
				Action: withResult(installAction),
			},
			{
				Name:  "enroll",
				Usage: "create the remote host record and run dnclient enroll",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "delete a stale remote host record and enroll afresh",
					},
				},
				Action: withResult(enrollAction),
			},
			{
				Name:   "unenroll",
				Usage:  "delete the remote host record and remove local config",
				Action: withResult(unenrollAction),
			},
			{
				Name:   "run",
				Usage:  "install, enroll, then run dnclient; unenroll on termination",
				Action: runAction,
			},
			{
				Name:      "write-config",
				Usage:     "persist the current environment configuration to a 0600 file",
				ArgsUsage: "<path>",
				Action:    withResult(writeConfigAction),
			},
		},
	}
}

func main() {
	// Bootstrap logger for failures before flags are parsed; the root Before
	// hook reconfigures the default once --log-text is known.
	slog.SetDefault(output.NewLogger(os.Stderr, output.LogOptions{Level: os.Getenv("DN_LOG_LEVEL")}))
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		exitWithError(err)
	}
}

// exitWithError centralizes failure exit-code mapping: an error already carrying
// an exit code (a cli.ExitCoder) is honored; any other error maps to
// output.CodeError. It delegates to cli.HandleExitCoder, which prints the
// message to stderr and calls the (overridable) cli.OsExiter — so commands
// return errors and the process exits afterward, never via os.Exit mid-command.
func exitWithError(err error) {
	var ec cli.ExitCoder
	if !errors.As(err, &ec) {
		ec = output.ExitError(err, output.CodeError)
	}
	cli.HandleExitCoder(ec)
}
