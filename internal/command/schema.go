package command

import (
	"context"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/version"
	"github.com/urfave/cli/v3"
)

// helpCommandName is the subcommand urfave/cli injects into the tree at run time.
// The projection skips it: it is framework scaffolding rather than a declared
// dn-tool command, so it has no entry in the exit-code registry, and the --help
// flag it duplicates is reported on every command anyway.
const helpCommandName = "help"

// schema emits dn-tool's runtime self-description (ADR 0005). It is not a
// resultAction: it carries its own envelope rather than an output.Result, and it
// has nothing to assert --assert-changed against.
func schema(_ context.Context, cmd *cli.Command) error {
	root := cmd.Root()
	if err := output.EmitJSON(root.Writer, schemaEnvelope(root)); err != nil {
		return config.ErrFileIO.Wrap(err)
	}
	return nil
}

// schemaEnvelope projects the live command tree and the two registries into the
// envelope. Reading the tree off the running root — rather than restating the
// surface here — is what makes the projection undriftable: a command, a flag, or
// an argument synopsis appears because it exists, not because someone remembered
// to list it.
func schemaEnvelope(root *cli.Command) output.SchemaEnvelope {
	commands := make([]output.SchemaCommand, 0, len(root.Commands))
	for _, sub := range root.Commands {
		if sub.Name == helpCommandName {
			continue
		}
		commands = append(commands, output.SchemaCommand{
			Name:      sub.Name,
			Usage:     sub.Usage,
			Args:      sub.ArgsUsage,
			Flags:     schemaFlags(sub.Flags),
			ExitCodes: output.CommandExitCodes[sub.Name],
			PassesThroughChildExitStatus: sub.Name == runCommandName &&
				output.RunPassesThroughDaemonExitStatus,
		})
	}
	return output.SchemaEnvelope{
		Head:        output.OKHead(),
		Tool:        rootName,
		Version:     version.Current(),
		Commands:    commands,
		GlobalFlags: schemaFlags(root.Flags),
		ExitCodes:   output.AllExitCodes(),
		Errors:      output.ErrorInventory(),
		Statuses:    output.StatusInventory(),
	}
}

// schemaFlags projects a flag set as the consumer invokes it, primary name first
// with its dashes. A flag that carries no documentable usage (nothing in the tree
// does today) still appears by name, because an undocumented flag is still part of
// the surface.
func schemaFlags(flags []cli.Flag) []output.SchemaFlag {
	projected := make([]output.SchemaFlag, 0, len(flags))
	for _, f := range flags {
		names := f.Names()
		if len(names) == 0 {
			continue
		}
		flag := output.SchemaFlag{Name: "--" + names[0]}
		if doc, ok := f.(cli.DocGenerationFlag); ok {
			flag.Usage = doc.GetUsage()
		}
		projected = append(projected, flag)
	}
	return projected
}
