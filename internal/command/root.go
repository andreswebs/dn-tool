package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/logctx"
	"github.com/andreswebs/dn-tool/internal/version"
	"github.com/urfave/cli/v3"
)

// rootName is the program name the framework reports in help and usage text. It
// is supplied here rather than taken from the arguments, because Run's contract
// is the arguments without the program name.
const rootName = "dn-tool"

// runRoot builds the framework tree and drives it. It is the seam between the
// framework-free contract in run.go and the replaceable interior: everything
// urfave-specific is below this line.
func runRoot(ctx context.Context, args []string, deps Deps) error {
	return newRoot(deps).Run(ctx, append([]string{rootName}, args...))
}

// newRoot assembles the command tree with every stream bound to deps, so no
// command reaches for a real process stream, and neutralizes the framework's own
// error handling before returning.
func newRoot(deps Deps) *cli.Command {
	acts := actions{deps: deps}
	root := &cli.Command{
		Name:         rootName,
		Usage:        "enroll and unenroll Linux hosts in a defined.net Managed Nebula network",
		Version:      version.Current(),
		Reader:       deps.In,
		Writer:       deps.Out,
		ErrWriter:    deps.Err,
		Action:       rootAction,
		OnUsageError: onUsageError,
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			logger, err := buildLogger(deps, cmd)
			if err != nil {
				return ctx, err
			}
			return logctx.With(ctx, logger), nil
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
			&cli.StringFlag{
				Name:      "log-level",
				Usage:     "diagnostic verbosity on stderr: `" + strings.Join(logctx.Levels, "|") + "`",
				Value:     logctx.DefaultLevel,
				Validator: validateLogLevel,
			},
			&cli.BoolFlag{
				Name:  "quiet",
				Usage: "log only errors; overrides --log-level",
			},
		},
		Commands: commands(acts),
	}
	// urfave/cli resolves OnUsageError on the command being parsed, without
	// walking to the parent, so a subcommand's own flag/argument errors would
	// otherwise miss the CodeUsage mapping. Set it on each one.
	for _, sub := range root.Commands {
		sub.OnUsageError = onUsageError
	}
	neutralize(root)
	return root
}

// neutralize disables the framework's own error printing and process exit, so a
// failure travels back to Run untouched and the exit boundary stays in one place
// (ADR 0003). Only ExitErrHandler needs setting: the framework resolves it by
// walking to the root command, and dn-tool maps usage errors structurally
// through OnUsageError rather than through the framework's default printer.
func neutralize(root *cli.Command) {
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}
}

// buildLogger constructs the one diagnostic logger for this invocation, after
// flags are parsed and bound to the injected stderr (ADR 0004). Nothing here
// touches slog's process-global default: the logger travels on the context, so
// parallel in-process tests each keep their own.
//
// Every record is decorated with the invocation's identity — the command, the
// build version, and a run id — so records from concurrent hosts writing to one
// collector stay separable.
func buildLogger(deps Deps, cmd *cli.Command) (*slog.Logger, error) {
	level, err := resolveLogLevel(deps, cmd)
	if err != nil {
		return nil, err
	}
	logger := logctx.NewLogger(deps.Err, level, deps.IsTerminal(deps.Err))
	return logger.With(
		"command", invokedCommand(cmd),
		"version", version.Current(),
		"run_id", logctx.NewRunID(),
	), nil
}

// resolveLogLevel applies the ADR 0004 precedence: --quiet, then --log-level,
// then DN_LOG_LEVEL from the injected environment, then the default. The flag
// value is already validated by the framework (validateLogLevel), so only the
// environment value can still fail here — and it fails as ErrConfig (78) rather
// than as a usage error, because the fault is in the environment and not in the
// command line.
//
// DN_LOG_LEVEL is read from the environment only, never from --env-file: the
// logger is built before configuration loads, so that it can report a
// configuration failure.
func resolveLogLevel(deps Deps, cmd *cli.Command) (slog.Level, error) {
	if cmd.Bool("quiet") {
		return slog.LevelError, nil
	}
	if cmd.IsSet("log-level") {
		return logctx.ParseLevel(cmd.String("log-level"))
	}
	if env := deps.Getenv("DN_LOG_LEVEL"); env != "" {
		level, err := logctx.ParseLevel(env)
		if err != nil {
			return 0, config.ErrConfig.Wrap(fmt.Errorf("DN_LOG_LEVEL: %w", err))
		}
		return level, nil
	}
	return logctx.ParseLevel(logctx.DefaultLevel)
}

// validateLogLevel rejects an unknown --log-level at parse time, so a misspelled
// level is a usage error (64) through OnUsageError rather than a silent fallback
// to some other verbosity.
func validateLogLevel(s string) error {
	_, err := logctx.ParseLevel(s)
	return err
}

// invokedCommand names the subcommand the invocation dispatched to, for the
// `command` log attribute. The root Before hook runs before dispatch, so the
// name is read off the remaining arguments; a bare invocation reports the tool
// itself, matching the "dn-tool" key in the exit-code registry.
func invokedCommand(cmd *cli.Command) string {
	if name := cmd.Args().First(); name != "" {
		return name
	}
	return rootName
}

// onUsageError maps a CLI-surface misuse — an unknown flag, a bad flag value, a
// bad positional argument — onto ErrUsage (ADR 0001, exit 64).
// It is set on the root and every subcommand because urfave/cli looks up
// OnUsageError on the command being parsed and does not inherit it from a parent.
func onUsageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	return ErrUsage.Wrap(err)
}

// rootAction runs only when no subcommand matched: both an unrecognized first
// argument and a bare `dn-tool` are usage errors (exit 64, not urfave's default
// 3), so stdout stays reserved for results and the usage banner never lands there
// (ADR 0005). A matched subcommand is dispatched before this runs, and the
// explicit --help/--version requests are handled earlier still, keeping their
// stdout-and-exit-0 exemption.
func rootAction(_ context.Context, cmd *cli.Command) error {
	if args := cmd.Args(); args.Present() {
		return ErrUnknownCommand.Wrap(fmt.Errorf("%q", args.First()))
	}
	return ErrNoCommand
}
