package command

import (
	"errors"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrUsage is a misused CLI surface: an unknown flag, a bad flag value, or a bad
// positional argument. It carries EX_USAGE (64) because the fault is in the
// command line handed to dn-tool — nothing about the environment, the network, or
// the local filesystem was reached yet, so no other class applies.
var ErrUsage = terr.New("usage_error", output.CodeUsage,
	"check the invocation against `dn-tool <command> --help`",
	"invalid command line")

// ErrUnknownCommand is a first argument that names no subcommand. It carries
// EX_USAGE (64) for the same reason as any other misuse, and is distinct from
// ErrUsage so a consumer can tell "you asked for something that does not exist"
// from "your flags are wrong" without parsing prose.
var ErrUnknownCommand = terr.New("unknown_command", output.CodeUsage,
	"run `dn-tool --help` for the list of commands",
	"unknown command")

// ErrNoCommand is a bare invocation with no subcommand. It carries EX_USAGE (64)
// as the class for an incomplete command line; today the bare invocation prints
// help and exits 0, and wiring this sentinel to the JSON error envelope is
// dt-nljm's step.
var ErrNoCommand = terr.New("no_command", output.CodeUsage,
	"run `dn-tool --help` for the list of commands",
	"no command given")

// ErrMissingArgument is a required positional argument that was not supplied. It
// carries EX_USAGE (64), like every other command-line fault: an absent argument
// is indistinguishable in kind from a malformed one, and the framework already
// maps its own missing-argument errors to this class.
var ErrMissingArgument = terr.New("missing_argument", output.CodeUsage,
	"supply the argument named in the message; see `dn-tool <command> --help` for the argument list",
	"missing required argument")

// ErrNoChange is the --assert-changed no-op: the command succeeded and made no
// change. It carries 1, a *result* class rather than a failure class (ADR 0001):
// the full result envelope is still written to stdout, and the error exists only
// to carry the distinct status a caller polls for.
//
// It is therefore a status, not a failure (terr.NewStatus): the boundary honors
// the code and writes no error envelope, so one successful invocation never
// reports ok:true on stdout and ok:false on stderr about itself.
var ErrNoChange = terr.NewStatus("no_change", output.CodeResult,
	"the command succeeded and changed nothing; drop --assert-changed if a no-op is an acceptable outcome",
	"no change made")

// errMissingWriteConfigPath is write-config's instance of ErrMissingArgument: the
// target path the configuration is written to. It is built once as a copy, so it
// keeps the sentinel's code, exit code and hint while naming the argument.
var errMissingWriteConfigPath = ErrMissingArgument.Wrap(errors.New("write-config requires a target file path"))
