// Command dn-tool enrolls and unenrolls Linux hosts in a defined.net Managed
// Nebula network, wrapping the defined.net REST API and the dnclient daemon.
package main

import (
	"context"
	"errors"
	"fmt"
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
// guard), so signalling the distinct result status (CodeResult) needs a real
// error to attach.
var errNoChange = errors.New("no change made")

// resultAction is a command action that yields a machine-readable output.Result
// alongside its error, so the wrapper can both emit the result and read its
// Changed flag for --assert-changed.
type resultAction func(context.Context, *cli.Command) (output.Result, error)

// withResult adapts a result-producing action into a cli action: it writes the
// Result as one JSON object to stdout, then — when --assert-changed is set and
// the command made no change — returns the distinct, non-error result status
// (CodeResult, exit 1). A command error propagates unchanged (its class code, or
// the CodeInternal fallback) regardless of --assert-changed; CodeResult is a
// result class and is never produced for failures.
func withResult(action resultAction) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		res, err := action(ctx, cmd)
		if err != nil {
			return err
		}
		if werr := output.WriteResult(cmd.Root().Writer, res); werr != nil {
			return output.Classify(werr, output.CodeIO)
		}
		if cmd.Bool("assert-changed") && !res.Changed {
			return output.ExitError(errNoChange, output.CodeResult)
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

// onUsageError maps a CLI-surface misuse — an unknown flag, a bad flag value, a
// bad positional argument — onto the taxonomy's usage class (ADR 0001, exit 64).
// It is set on the root and every subcommand because urfave/cli looks up
// OnUsageError on the command being parsed and does not inherit it from a parent.
func onUsageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	return output.ExitError(err, output.CodeUsage)
}

// rootAction runs only when no subcommand matched: a bare `dn-tool` prints the
// root help and exits 0, while an unrecognized first argument is a usage error
// (exit 64, not urfave's default 3). A matched subcommand is dispatched before
// this runs, and --help/--version are handled earlier still.
func rootAction(_ context.Context, cmd *cli.Command) error {
	if args := cmd.Args(); args.Present() {
		return output.ExitError(fmt.Errorf("unknown command %q", args.First()), output.CodeUsage)
	}
	return cli.ShowRootCommandHelp(cmd)
}

func newApp() *cli.Command {
	app := &cli.Command{
		Name:         "dn-tool",
		Usage:        "enroll and unenroll Linux hosts in a defined.net Managed Nebula network",
		Version:      version.Current(),
		Action:       rootAction,
		OnUsageError: onUsageError,
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
	// urfave/cli resolves OnUsageError on the command being parsed, without
	// walking to the parent, so a subcommand's own flag/argument errors would
	// otherwise miss the CodeUsage mapping. Set it on each one.
	for _, sub := range app.Commands {
		sub.OnUsageError = onUsageError
	}
	return app
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
// an exit code (a cli.ExitCoder) is honored; any unclassified error maps to
// output.CodeInternal (an unclassified failure is treated as a bug, per ADR
// 0001). It delegates to cli.HandleExitCoder, which prints the message to stderr
// and calls the (overridable) cli.OsExiter — so commands return errors and the
// process exits afterward, never via os.Exit mid-command.
func exitWithError(err error) {
	var ec cli.ExitCoder
	if !errors.As(err, &ec) {
		ec = output.ExitError(err, output.CodeInternal)
	}
	cli.HandleExitCoder(ec)
}
