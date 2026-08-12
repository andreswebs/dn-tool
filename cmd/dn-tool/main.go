// Command dn-tool enrolls and unenrolls Linux hosts in a defined.net Managed
// Nebula network, wrapping the defined.net REST API and the dnclient daemon.
//
// The CLI surface lives in internal/command behind Run(args, deps) int (ADR
// 0003); this file is the only place the real process environment is touched.
package main

import (
	"os"

	"github.com/andreswebs/dn-tool/internal/command"
	"github.com/andreswebs/dn-tool/internal/logctx"
)

func main() {
	os.Exit(command.Run(os.Args[1:], command.Deps{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv,
		IsTerminal: logctx.IsTerminalWriter,
	}))
}
