package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/version"
	"github.com/urfave/cli/v3"
)

func commandNames(t *testing.T) map[string]bool {
	t.Helper()
	app := newApp()
	names := make(map[string]bool)
	for _, c := range app.Commands {
		names[c.Name] = true
	}
	return names
}

func TestNewApp_HasFiveSubcommands(t *testing.T) {
	names := commandNames(t)
	for _, want := range []string{"install", "enroll", "unenroll", "run", "write-config"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
	// "help" is auto-added by urfave/cli; ignore it. Assert no unexpected extras.
	for got := range names {
		switch got {
		case "install", "enroll", "unenroll", "run", "write-config", "help":
		default:
			t.Errorf("unexpected subcommand %q", got)
		}
	}
}

func TestNewApp_GlobalFlagsRegistered(t *testing.T) {
	app := newApp()
	flags := make(map[string]bool)
	for _, f := range app.Flags {
		for _, n := range f.Names() {
			flags[n] = true
		}
	}
	for _, want := range []string{"env-file", "assert-changed", "log-text"} {
		if !flags[want] {
			t.Errorf("missing global flag --%s", want)
		}
	}
	if flags["force"] {
		t.Error("--force must not be a global flag")
	}
}

func TestNewApp_ForceIsEnrollScoped(t *testing.T) {
	app := newApp()

	hasForce := func(c *cli.Command) bool {
		for _, f := range c.Flags {
			for _, n := range f.Names() {
				if n == "force" {
					return true
				}
			}
		}
		return false
	}

	for _, c := range app.Commands {
		switch c.Name {
		case "enroll":
			if !hasForce(c) {
				t.Error("enroll must have --force flag")
			}
		default:
			if hasForce(c) {
				t.Errorf("%s must not have --force flag", c.Name)
			}
		}
	}
}

func TestNewApp_VersionReportsCurrent(t *testing.T) {
	old := version.Override
	t.Cleanup(func() { version.Override = old })
	version.Override = "v1.2.3-test"

	var stdout bytes.Buffer
	app := newApp()
	app.Writer = &stdout

	if err := app.Run(context.Background(), []string{"dn-tool", "--version"}); err != nil {
		t.Fatalf("Run(--version) returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "v1.2.3-test") {
		t.Errorf("--version output %q does not contain %q", stdout.String(), "v1.2.3-test")
	}
}

// Every subcommand must be wired to an action. There are no notImplemented stubs
// left: a nil Action would mean a command silently does nothing, which is the
// class of gap that left `run` unwired despite its internal package being
// complete.
func TestNewApp_AllSubcommandsWired(t *testing.T) {
	app := newApp()
	for _, c := range app.Commands {
		if c.Action == nil {
			t.Errorf("subcommand %q has a nil Action (unwired)", c.Name)
		}
	}
}
