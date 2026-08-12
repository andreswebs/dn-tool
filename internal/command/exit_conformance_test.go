package command

import (
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
)

// runApp drives the real CLI end to end with args, returning the resolved process
// exit code. It is the whole contract main relies on, so these assertions are
// framework-blind: buffers in, an int out.
func runApp(t *testing.T, env map[string]string, args ...string) int {
	t.Helper()
	deps, _, _ := testDeps(env)
	return Run(args, deps)
}

// The declared usage and configuration classes (ADR 0001) are reachable through
// the real CLI surface, not only as constructed errors: an unknown command, a
// bad global flag, a bad subcommand flag, and a malformed DN_* value each resolve
// to the code the registry declares. This is the cmd-side of the conformance
// requirement; the output package asserts the API/network classes on constructed
// errors, and assert_changed_test.go drives the result class (1).
func TestConformance_UsageAndConfigClasses(t *testing.T) {
	t.Run("unknown command -> usage", func(t *testing.T) {
		if got := runApp(t, nil, "bogus"); got != output.CodeUsage {
			t.Errorf("unknown command exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("bad global flag -> usage", func(t *testing.T) {
		if got := runApp(t, nil, "--nonexistent"); got != output.CodeUsage {
			t.Errorf("bad global flag exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("bad subcommand flag -> usage", func(t *testing.T) {
		if got := runApp(t, nil, "enroll", "--nonexistent"); got != output.CodeUsage {
			t.Errorf("bad subcommand flag exit code = %d, want %d", got, output.CodeUsage)
		}
	})

	t.Run("malformed DN_* value -> config", func(t *testing.T) {
		// A malformed DN_LISTEN_PORT fails config resolution before write-config
		// touches the filesystem or the API, so this stays in-process.
		env := map[string]string{"DN_LISTEN_PORT": "not-a-number"}
		if got := runApp(t, env, "write-config", t.TempDir()+"/out.env"); got != output.CodeConfig {
			t.Errorf("malformed DN_* exit code = %d, want %d", got, output.CodeConfig)
		}
	})

	// A bare invocation is an incomplete command line, not a help request: ADR
	// 0005 keeps the usage banner off stdout, so it is exit 64 with an error
	// envelope on stderr. The stdout-and-exit-0 exemption belongs to an explicit
	// --help/--version, pinned in error_envelope_test.go.
	t.Run("bare invocation -> usage", func(t *testing.T) {
		if got := runApp(t, nil); got != output.CodeUsage {
			t.Errorf("bare invocation exit code = %d, want %d", got, output.CodeUsage)
		}
	})
}
