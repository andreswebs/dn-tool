package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/output"
)

// decodeErrorEnvelope decodes the single error envelope written to stderr,
// failing the test unless it is exactly one newline-terminated JSON object.
func decodeErrorEnvelope(t *testing.T, stderr string) (env, errObj map[string]any) {
	t.Helper()
	if !strings.HasSuffix(stderr, "\n") || strings.Count(stderr, "\n") != 1 {
		t.Fatalf("stderr must be exactly one newline-terminated line, got %q", stderr)
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	obj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope has no error object: %v", env)
	}
	return env, obj
}

// The exit boundary renders one envelope per failure class (ADR 0002/0005): the
// declared code travels to the consumer next to the exit code it maps to, for
// every class the taxonomy defines plus the unclassified fallback.
func TestExitCode_EmitsErrorEnvelopePerExitClass(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
		exit int
	}{
		{"usage", ErrUsage.Wrap(errors.New("bad flag")), "usage_error", output.CodeUsage},
		{"data", api.ErrAPIRejected.Wrap(errors.New("422")), "api_rejected", output.CodeData},
		{"unavailable", api.ErrAPIUnavailable.Wrap(errors.New("dial")), "api_unavailable", output.CodeUnavailable},
		{"io", dnclient.ErrDNClientFailed.Wrap(errors.New("exec")), "dnclient_failed", output.CodeIO},
		{"config", config.ErrConfig.Wrap(errors.New("DN_NETWORK_ID")), "config_error", output.CodeConfig},
		{"unclassified", errors.New("boom"), "internal_error", output.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if got := exitCode(tt.err, &stderr); got != tt.exit {
				t.Errorf("exit code = %d, want %d", got, tt.exit)
			}

			env, obj := decodeErrorEnvelope(t, stderr.String())
			if got := env["ok"]; got != false {
				t.Errorf("ok = %v, want false", got)
			}
			if got := obj["code"]; got != tt.code {
				t.Errorf("error.code = %v, want %q", got, tt.code)
			}
			if got := obj["message"]; got != tt.err.Error() {
				t.Errorf("error.message = %v, want %q", got, tt.err.Error())
			}
		})
	}
}

// The message-loss half of the dt-2efx defect: a dnclient child that exited
// non-zero must render its full context and its status, not a bare
// "exit status N" with the action discarded.
func TestExitCode_DnclientFailureRendersFullMessageAndStatus(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "dnclient")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("writing the fake dnclient: %v", err)
	}
	err := dnclient.NewExecClient(bin).Enroll(context.Background(), "defined", "code-xyz")
	if err == nil {
		t.Fatal("the fake dnclient exited 3; want an error")
	}

	var stderr bytes.Buffer
	if got := exitCode(err, &stderr); got != output.CodeIO {
		t.Errorf("exit code = %d, want %d", got, output.CodeIO)
	}

	_, obj := decodeErrorEnvelope(t, stderr.String())
	if got := obj["code"]; got != "dnclient_failed" {
		t.Errorf("error.code = %v, want %q", got, "dnclient_failed")
	}
	msg, _ := obj["message"].(string)
	if !strings.Contains(msg, "dnclient enroll") {
		t.Errorf("error.message = %q, want the action context, not a bare exit status", msg)
	}
	details, ok := obj["details"].(map[string]any)
	if !ok {
		t.Fatalf("error.details = %v, want the child's status", obj["details"])
	}
	if got := details["exit_status"]; got != float64(3) {
		t.Errorf("error.details.exit_status = %v, want 3", got)
	}
}

// ADR 0005: nothing appears on stdout on failure. Driven through the real CLI,
// so the framework's own help-on-error path is covered too.
func TestRun_FailureWritesNothingToStdout(t *testing.T) {
	for _, args := range [][]string{
		{"bogus"},
		{"--nonexistent"},
		{"enroll", "--nonexistent"},
		{"write-config"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			deps, stdout, stderr := testDeps(nil)
			if got := Run(args, deps); got == output.CodeOK {
				t.Fatalf("Run(%v) = %d, want a failure code", args, got)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing on failure", stdout.String())
			}
			if _, obj := decodeErrorEnvelope(t, stderr.String()); obj["code"] == "" {
				t.Errorf("stderr envelope carries no code: %q", stderr.String())
			}
		})
	}
}

// A bare `dn-tool` is an incomplete command line, not a request for help: it is a
// usage error with an envelope on stderr and an empty stdout, because ADR 0005
// bans usage banners on stdout.
func TestRun_BareInvocationIsUsageError(t *testing.T) {
	deps, stdout, stderr := testDeps(nil)

	if got := Run(nil, deps); got != output.CodeUsage {
		t.Fatalf("Run() = %d, want %d", got, output.CodeUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing on a bare invocation", stdout.String())
	}

	_, obj := decodeErrorEnvelope(t, stderr.String())
	if got := obj["code"]; got != "no_command" {
		t.Errorf("error.code = %v, want %q", got, "no_command")
	}
	if hint, _ := obj["hint"].(string); !strings.Contains(hint, "--help") {
		t.Errorf("error.hint = %q, want it to point at --help", hint)
	}
}

// The documented exemption: a consumer asking for help or the version is not
// asking for a result, so both still write to stdout and exit 0. Pinned here so
// it stays a decision rather than an accident of framework wiring.
func TestRun_HelpAndVersionStillWriteToStdout(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			deps, stdout, stderr := testDeps(nil)
			if got := Run([]string{arg}, deps); got != output.CodeOK {
				t.Fatalf("Run(%s) = %d, want %d (stderr: %s)", arg, got, output.CodeOK, stderr)
			}
			if stdout.Len() == 0 {
				t.Errorf("Run(%s) wrote nothing to stdout", arg)
			}
			if stderr.Len() != 0 {
				t.Errorf("Run(%s) stderr = %q, want nothing", arg, stderr.String())
			}
		})
	}
}

// The same exemption for a subcommand's help.
func TestRun_SubcommandHelpWritesToStdout(t *testing.T) {
	deps, stdout, stderr := testDeps(nil)
	if got := Run([]string{"enroll", "--help"}, deps); got != output.CodeOK {
		t.Fatalf("Run(enroll --help) = %d, want %d (stderr: %s)", got, output.CodeOK, stderr)
	}
	if !strings.Contains(stdout.String(), "enroll") {
		t.Errorf("stdout = %q, want the enroll help", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}
