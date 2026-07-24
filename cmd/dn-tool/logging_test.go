package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/urfave/cli/v3"
)

// runCapturingLogOptions runs the real app with the given args, capturing the
// LogOptions that logOptions derives from the parsed command. The install
// subcommand's action is swapped for a recorder so the capture happens after
// global-flag parsing, exercising the real --log-text wiring.
func runCapturingLogOptions(t *testing.T, args []string) output.LogOptions {
	t.Helper()
	var got output.LogOptions
	app := newApp()
	for _, c := range app.Commands {
		if c.Name == "install" {
			c.Action = func(_ context.Context, cmd *cli.Command) error {
				got = logOptions(cmd)
				return nil
			}
		}
	}
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run(%v) returned error: %v", args, err)
	}
	return got
}

// TestLogText_FlagWiredToLogOptions asserts behavior 4: the --log-text global
// flag flows into LogOptions.Text, and defaults to JSON (Text=false) when unset.
func TestLogText_FlagWiredToLogOptions(t *testing.T) {
	if got := runCapturingLogOptions(t, []string{"dn-tool", "--log-text", "install"}); !got.Text {
		t.Errorf("--log-text: LogOptions.Text = false, want true")
	}
	if got := runCapturingLogOptions(t, []string{"dn-tool", "install"}); got.Text {
		t.Errorf("no flag: LogOptions.Text = true, want false")
	}
}
