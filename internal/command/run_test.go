package command

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/run"
)

// testDeps builds a Deps whose every field is a fake, so a test never touches
// the real process environment, and returns the captured streams.
func testDeps(env map[string]string) (deps Deps, stdout, stderr *bytes.Buffer) {
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	return Deps{
		In:     strings.NewReader(""),
		Out:    stdout,
		Err:    stderr,
		Getenv: func(key string) string { return env[key] },
	}, stdout, stderr
}

// The contract: Run takes the arguments without the program name and returns the
// process exit code, so main is one line and never inspects errors.
func TestRun_UnknownCommandReturnsUsageCode(t *testing.T) {
	deps, _, stderr := testDeps(nil)
	if got := Run([]string{"bogus"}, deps); got != output.CodeUsage {
		t.Errorf("Run(bogus) = %d, want %d", got, output.CodeUsage)
	}
	if !strings.Contains(stderr.String(), "bogus") {
		t.Errorf("stderr = %q, want it to name the rejected command", stderr.String())
	}
}

// Success is reported as CodeOK with the single JSON result object on
// deps.Out — nothing is written to the real stdout.
func TestRun_SuccessReturnsCodeOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	deps, stdout, stderr := testDeps(nil)

	if got := Run([]string{"write-config", path}, deps); got != output.CodeOK {
		t.Fatalf("Run(write-config) = %d, want %d (stderr: %s)", got, output.CodeOK, stderr)
	}
	if !strings.Contains(stdout.String(), `"action":"write-config"`) {
		t.Errorf("stdout = %q, want the write-config result object", stdout.String())
	}
}

// Configuration comes from Deps.Getenv, not os.Getenv: a value that exists only
// in the injected environment must reach the resolved config.
func TestRun_ConfigurationComesFromDepsGetenv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	deps, _, stderr := testDeps(map[string]string{"DN_NETWORK_ID": "net-injected"})

	if got := Run([]string{"write-config", path}, deps); got != output.CodeOK {
		t.Fatalf("Run(write-config) = %d, want %d (stderr: %s)", got, output.CodeOK, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "DN_NETWORK_ID=net-injected") {
		t.Errorf("written config does not carry the injected value:\n%s", data)
	}
}

// A Deps with unset fields is inert rather than fatal: a caller that only cares
// about the exit code must not nil-panic inside a command.
func TestRun_ZeroDepsDoesNotPanic(t *testing.T) {
	if got := Run([]string{"bogus"}, Deps{}); got != output.CodeUsage {
		t.Errorf("Run with a zero Deps = %d, want %d", got, output.CodeUsage)
	}
}

func TestExitCode_NilErrorIsOK(t *testing.T) {
	var stderr bytes.Buffer
	if got := exitCode(nil, &stderr); got != output.CodeOK {
		t.Errorf("exitCode(nil) = %d, want %d", got, output.CodeOK)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing written on success", stderr.String())
	}
}

func TestExitCode_PlainErrorMapsToCodeInternal(t *testing.T) {
	var stderr bytes.Buffer
	if got := exitCode(errors.New("boom"), &stderr); got != output.CodeInternal {
		t.Errorf("exitCode(plain) = %d, want %d", got, output.CodeInternal)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), "boom")
	}
}

func TestExitCode_CodedErrorHonored(t *testing.T) {
	var stderr bytes.Buffer
	if got := exitCode(ErrNoChange, &stderr); got != output.CodeResult {
		t.Errorf("exitCode(coded) = %d, want %d", got, output.CodeResult)
	}
}

// The guard the dt-nrq1 regression escaped through: a non-failure status carries
// an exit code and nothing else. Writing an ok:false envelope for one would
// contradict the ok:true result envelope on stdout for the very same invocation,
// and would report a clean container stop as a failure.
func TestExitCode_NonFailureStatusWritesNoEnvelope(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"no_change", ErrNoChange, output.CodeResult},
		{"no_change wrapped", ErrNoChange.Wrap(errors.New("install")), output.CodeResult},
		{"signal_shutdown", run.ErrSignalShutdown, output.CodeSIGTERM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if got := exitCode(tt.err, &stderr); got != tt.want {
				t.Errorf("exitCode() = %d, want %d", got, tt.want)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing for a non-failure status", stderr.String())
			}
		})
	}
}

func TestExitCode_WrappedCodedErrorHonored(t *testing.T) {
	var stderr bytes.Buffer
	wrapped := fmt.Errorf("resolving configuration: %w", ErrUsage)
	if got := exitCode(wrapped, &stderr); got != output.CodeUsage {
		t.Errorf("exitCode(wrapped coded) = %d, want %d", got, output.CodeUsage)
	}
}
