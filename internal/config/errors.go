package config

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrConfig is a malformed or out-of-range DN_* value. It carries EX_CONFIG (78)
// because the process environment dn-tool was handed is wrong: nothing about the
// command line, the network, or the local filesystem is at fault, and the
// operator fixes it by correcting the variable the message names.
var ErrConfig = terr.New("config_error", output.CodeConfig,
	"correct the DN_* variable named in the message; see docs/dn-tool-design.md §2.3 for accepted values",
	"invalid configuration")

// ErrEnvFile is a --env-file (DN_ENV_FILE) that cannot be opened or parsed. It
// carries EX_CONFIG (78) rather than EX_IOERR: the operator named this file as
// dn-tool's configuration source, so an unreadable or malformed one is a
// configuration fault, not an incidental I/O fault on some file dn-tool chose.
var ErrEnvFile = terr.New("env_file_error", output.CodeConfig,
	"check that the env-file exists, is readable, and holds only KEY=VALUE lines",
	"unusable env-file")

// ErrFileIO is a local filesystem operation that failed — creating, writing, or
// closing a file, making a directory, or inspecting one. It carries EX_IOERR
// (74), the sysexits class for exactly that: the inputs were valid and the
// dependency reachable, but the local filesystem refused the operation.
//
// It is declared here, in the lowest-level package that raises it, and shared by
// every package whose local filesystem steps map to the same class (dninstall's
// binary placement, unenroll's local-config removal, the result writer), so one
// machine code covers one meaning instead of a near-duplicate per package.
var ErrFileIO = terr.New("file_io_error", output.CodeIO,
	"check the path's permissions and that the filesystem is writable and has free space",
	"local file operation failed")
