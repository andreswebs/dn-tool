package command

import (
	"context"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// actions binds the wired command actions to the injected dependencies. The
// actions themselves receive only the parsed command from the framework, so the
// environment they resolve configuration from has to be carried here rather than
// read from the process.
type actions struct {
	deps Deps
}

// runCommandName names the one command that propagates a child process's exit
// status. It is a constant because two places have to agree on it: the tree below
// and the schema projection that reports the passthrough as data.
const runCommandName = "run"

// commands is the subcommand tree. Adding a command is a data change here plus
// its action; the flags a command owns stay scoped to it (--force is enroll's,
// never a global).
func commands(acts actions) []*cli.Command {
	return []*cli.Command{
		{
			Name:   "install",
			Usage:  "download and verify dnclient into the binary directory",
			Action: withResult(acts.install),
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
			Action: withResult(acts.enroll),
		},
		{
			Name:   "unenroll",
			Usage:  "delete the remote host record and remove local config",
			Action: withResult(acts.unenroll),
		},
		{
			Name:   runCommandName,
			Usage:  "install, enroll, then run dnclient; unenroll on termination",
			Action: acts.run,
		},
		{
			Name:      "write-config",
			Usage:     "persist the current environment configuration to a 0600 file",
			ArgsUsage: "<path>",
			Action:    withResult(acts.writeConfig),
		},
		{
			Name:   "schema",
			Usage:  "print the machine-readable command, exit-code and error surface",
			Action: schema,
		},
	}
}

// loadConfig resolves the dn-tool configuration for a command: the injected
// environment overlaid on an optional --env-file (design §2.3 precedence). It is
// the shared bootstrap each command uses before contacting the API.
func (a actions) loadConfig(cmd *cli.Command) (*config.Config, error) {
	return config.LoadWithEnvFile(cmd.String("env-file"), a.deps.Getenv)
}

// resultAction is a command action that yields a machine-readable output.Result
// alongside its error, so the wrapper can both emit the result and read its
// Changed flag for --assert-changed.
type resultAction func(context.Context, *cli.Command) (output.Result, error)

// withResult adapts a result-producing action into a cli action: it writes the
// Result as one JSON object to stdout, then — when --assert-changed is set and
// the command made no change — returns the distinct, non-error result status
// (ErrNoChange, exit 1). A command error propagates unchanged (its class code, or
// the CodeInternal fallback) regardless of --assert-changed; the result class is
// never produced for failures.
func withResult(action resultAction) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		res, err := action(ctx, cmd)
		if err != nil {
			return err
		}
		if werr := output.WriteResult(cmd.Root().Writer, res); werr != nil {
			return config.ErrFileIO.Wrap(werr)
		}
		if cmd.Bool("assert-changed") && !res.Changed {
			return ErrNoChange
		}
		return nil
	}
}
