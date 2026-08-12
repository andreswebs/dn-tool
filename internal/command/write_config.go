package command

import (
	"context"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// writeConfig is the wired write-config command: it resolves the configuration
// (env-file beneath the live environment) and persists it to the positional
// target path as a 0600 key-value file. It is a resultAction, so withResult
// emits its Result and applies the exit-code / --assert-changed semantics.
func (a actions) writeConfig(_ context.Context, cmd *cli.Command) (output.Result, error) {
	cfg, err := a.loadConfig(cmd)
	if err != nil {
		return output.Result{}, err
	}
	return runWriteConfig(cfg, cmd.Args().First())
}

// runWriteConfig is the testable core: it writes the resolved Config to path as
// a 0600 env-file (config.WriteConfigFile enforces the at-creation mode; SEC2)
// and reports the write as a change. The path is required.
func runWriteConfig(cfg *config.Config, path string) (output.Result, error) {
	if path == "" {
		return output.Result{}, errMissingWriteConfigPath
	}
	if err := config.WriteConfigFile(path, cfg); err != nil {
		return output.Result{}, err
	}
	return output.Result{Action: "write-config", Changed: true}, nil
}
