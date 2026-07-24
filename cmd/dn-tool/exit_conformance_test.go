package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
)

// runApp drives the real app end to end with args, returning the resolved
// process exit code. It mirrors production: cli auto-exits via cli.OsExiter
// inside Run when an action returns an ExitCoder, and main's exitWithError maps
// any remaining plain error afterward. captureExit records the OsExiter code
// (sentinel -1 = never called = success).
func runApp(t *testing.T, args ...string) int {
	t.Helper()
	captured, _ := captureExit(t)
	app := newApp()
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}
	if err := app.Run(context.Background(), append([]string{"dn-tool"}, args...)); err != nil {
		exitWithError(err)
	}
	if *captured == -1 {
		return output.CodeOK
	}
	return *captured
}

// The declared usage and configuration classes (ADR 0001) are reachable through
// the real CLI surface, not only as constructed errors: an unknown command, a
// bad global flag, a bad subcommand flag, and a malformed DN_* value each resolve
// to the code the registry declares. This is the cmd-side of the conformance
// requirement; the output package asserts the API/network classes on constructed
// errors, and assert_changed_test.go drives the result class (1).
func TestConformance_UsageAndConfigClasses(t *testing.T) {
	t.Run("unknown command -> usage", func(t *testing.T) {
		if got := runApp(t, "bogus"); got != output.CodeUsage {
			t.Errorf("unknown command exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("bad global flag -> usage", func(t *testing.T) {
		if got := runApp(t, "--nonexistent"); got != output.CodeUsage {
			t.Errorf("bad global flag exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("bad subcommand flag -> usage", func(t *testing.T) {
		if got := runApp(t, "enroll", "--nonexistent"); got != output.CodeUsage {
			t.Errorf("bad subcommand flag exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("malformed DN_* value -> config", func(t *testing.T) {
		// A malformed DN_LISTEN_PORT fails config resolution before write-config
		// touches the filesystem or the API, so this stays in-process.
		t.Setenv("DN_LISTEN_PORT", "not-a-number")
		if got := runApp(t, "write-config", t.TempDir()+"/out.env"); got != output.CodeConfig {
			t.Errorf("malformed DN_* exit code = %d, want %d", got, output.CodeConfig)
		}
	})

	t.Run("bare invocation shows help and exits zero", func(t *testing.T) {
		if got := runApp(t); got != output.CodeOK {
			t.Errorf("bare invocation exit code = %d, want %d", got, output.CodeOK)
		}
	})
}
