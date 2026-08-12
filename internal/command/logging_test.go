package command

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/logctx"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/version"
	"github.com/urfave/cli/v3"
)

// The probe messages, one per level, emitted by the action below. They are
// distinct strings so a test reads the effective threshold off stderr rather
// than off an internal it had to reach for.
const (
	probeDebug = "probe-debug"
	probeInfo  = "probe-info"
	probeWarn  = "probe-warn"
	probeError = "probe-error"
)

// runWithProbe runs the real command tree with args, swapping install's action
// for one that logs at every level through the context logger, and returns
// stderr. Each verbosity rule is therefore asserted end to end: flag parsing,
// level resolution, handler selection, and the logger's travel on the context.
func runWithProbe(t *testing.T, args []string, env map[string]string, isTerminal bool) string {
	t.Helper()
	deps, _, stderr := testDeps(env)
	deps.IsTerminal = func(io.Writer) bool { return isTerminal }
	root := newRoot(deps.withDefaults())
	for _, c := range root.Commands {
		if c.Name == "install" {
			c.Action = func(ctx context.Context, _ *cli.Command) error {
				l := logctx.From(ctx)
				l.Debug(probeDebug)
				l.Info(probeInfo)
				l.Warn(probeWarn)
				l.Error(probeError)
				return nil
			}
		}
	}
	if err := root.Run(context.Background(), append([]string{rootName}, args...)); err != nil {
		t.Fatalf("Run(%v) returned error: %v", args, err)
	}
	return stderr.String()
}

// emitted reports which probe messages survived the level threshold.
func emitted(logs string) map[string]bool {
	out := make(map[string]bool)
	for _, msg := range []string{probeDebug, probeInfo, probeWarn, probeError} {
		out[msg] = strings.Contains(logs, msg)
	}
	return out
}

// firstLine returns the first line of captured stderr.
func firstLine(logs string) string {
	return strings.SplitN(strings.TrimSpace(logs), "\n", 2)[0]
}

// The default verbosity is warn: a healthy run says nothing on stderr, so the
// result on stdout is the only output a caller has to read (ADR 0004).
func TestLogLevel_DefaultIsWarn(t *testing.T) {
	got := emitted(runWithProbe(t, []string{"install"}, nil, false))

	for _, msg := range []string{probeDebug, probeInfo} {
		if got[msg] {
			t.Errorf("%q emitted at the default level, want it suppressed below warn", msg)
		}
	}
	for _, msg := range []string{probeWarn, probeError} {
		if !got[msg] {
			t.Errorf("%q suppressed at the default level, want it emitted", msg)
		}
	}
}

// --log-level raises the verbosity.
func TestLogLevel_FlagSetsTheThreshold(t *testing.T) {
	if got := emitted(runWithProbe(t, []string{"--log-level", "debug", "install"}, nil, false)); !got[probeDebug] {
		t.Errorf("--log-level debug: %q suppressed, want it emitted", probeDebug)
	}
}

// DN_LOG_LEVEL is the environment fallback, read from the injected environment
// rather than the process one.
func TestLogLevel_EnvIsTheFallback(t *testing.T) {
	got := emitted(runWithProbe(t, []string{"install"}, map[string]string{"DN_LOG_LEVEL": "debug"}, false))

	if !got[probeDebug] {
		t.Errorf("DN_LOG_LEVEL=debug: %q suppressed, want it emitted", probeDebug)
	}
}

// Precedence: the flag beats the environment.
func TestLogLevel_FlagBeatsEnv(t *testing.T) {
	got := emitted(runWithProbe(t,
		[]string{"--log-level", "error", "install"},
		map[string]string{"DN_LOG_LEVEL": "debug"}, false))

	if got[probeWarn] {
		t.Errorf("--log-level error with DN_LOG_LEVEL=debug: %q emitted, want the flag to win", probeWarn)
	}
	if !got[probeError] {
		t.Errorf("--log-level error: %q suppressed, want it emitted", probeError)
	}
}

// --quiet raises the floor to error and beats --log-level: it is the operator's
// "say nothing but failures" switch, so an inherited --log-level from a wrapper
// script cannot re-open the tap.
func TestQuiet_BeatsLogLevel(t *testing.T) {
	got := emitted(runWithProbe(t, []string{"--quiet", "--log-level", "debug", "install"}, nil, false))

	for _, msg := range []string{probeDebug, probeInfo, probeWarn} {
		if got[msg] {
			t.Errorf("--quiet: %q emitted, want everything below error suppressed", msg)
		}
	}
	if !got[probeError] {
		t.Errorf("--quiet: %q suppressed, want errors still emitted", probeError)
	}
}

// --quiet also beats DN_LOG_LEVEL, for the same reason.
func TestQuiet_BeatsEnv(t *testing.T) {
	got := emitted(runWithProbe(t, []string{"--quiet", "install"}, map[string]string{"DN_LOG_LEVEL": "debug"}, false))

	if got[probeWarn] {
		t.Errorf("--quiet with DN_LOG_LEVEL=debug: %q emitted, want the flag to win", probeWarn)
	}
}

// A bad --log-level is a misused command line, exit 64, not a silent fallback to
// some other verbosity.
func TestLogLevel_InvalidFlagValueIsUsageError(t *testing.T) {
	deps, _, stderr := testDeps(nil)

	if got := Run([]string{"--log-level", "bogus", "install"}, deps); got != output.CodeUsage {
		t.Errorf("Run(--log-level bogus) = %d, want %d", got, output.CodeUsage)
	}
	if !strings.Contains(stderr.String(), "bogus") {
		t.Errorf("stderr = %q, want it to name the rejected value", stderr.String())
	}
}

// A bad DN_LOG_LEVEL is a configuration error, exit 78: the fault is in the
// environment, not in the command line.
func TestLogLevel_InvalidEnvValueIsConfigError(t *testing.T) {
	deps, _, stderr := testDeps(map[string]string{"DN_LOG_LEVEL": "bogus"})

	if got := Run([]string{"install"}, deps); got != output.CodeConfig {
		t.Errorf("Run with DN_LOG_LEVEL=bogus = %d, want %d", got, output.CodeConfig)
	}
	if !strings.Contains(stderr.String(), "DN_LOG_LEVEL") {
		t.Errorf("stderr = %q, want it to name the offending variable", stderr.String())
	}
}

// Handler selection is by destination: a pipeline gets JSON…
func TestHandlerSelection_JSONWhenNotATerminal(t *testing.T) {
	line := firstLine(runWithProbe(t, []string{"--log-level", "debug", "install"}, nil, false))

	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("non-terminal stderr is not JSON: %v (%q)", err, line)
	}
}

// …and a human at a terminal gets text, with no flag on either side.
func TestHandlerSelection_TextWhenATerminal(t *testing.T) {
	line := firstLine(runWithProbe(t, []string{"--log-level", "debug", "install"}, nil, true))

	var rec map[string]any
	if json.Unmarshal([]byte(line), &rec) == nil {
		t.Fatalf("terminal stderr produced JSON, want text: %q", line)
	}
	if !strings.Contains(line, "msg="+probeDebug) {
		t.Errorf("text record %q does not carry msg=%s", line, probeDebug)
	}
}

// Every record carries the invocation's identity, so records interleaved from
// concurrent invocations stay separable.
func TestRecordsCarryCommandVersionAndRunID(t *testing.T) {
	line := firstLine(runWithProbe(t, []string{"--log-level", "debug", "install"}, nil, false))

	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("stderr is not JSON: %v (%q)", err, line)
	}
	if rec["command"] != "install" {
		t.Errorf("command = %v, want %q", rec["command"], "install")
	}
	if rec["version"] != version.Current() {
		t.Errorf("version = %v, want %q", rec["version"], version.Current())
	}
	if id, _ := rec["run_id"].(string); id == "" {
		t.Error("run_id missing from the record")
	}
}

// One invocation, one run id: the correlation only works if the id is stable
// across every record of a single run.
func TestRunID_IsStableWithinAnInvocation(t *testing.T) {
	logs := runWithProbe(t, []string{"--log-level", "debug", "install"}, nil, false)

	ids := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("stderr line is not JSON: %v (%q)", err, line)
		}
		id, _ := rec["run_id"].(string)
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Errorf("run ids across one invocation = %d distinct, want 1 (%q)", len(ids), logs)
	}
}

// The API key must not reach stderr even at the most verbose level; redaction
// is the Secret type's job and this asserts it holds through a whole command
// (docs/specs/e2e-testing.md).
func TestDebugLevelNeverRevealsASecret(t *testing.T) {
	const key = "super-secret-api-key"
	path := filepath.Join(t.TempDir(), "dn.env")
	deps, _, stderr := testDeps(map[string]string{"DN_API_KEY": key, "DN_HOSTNAME": "host.example"})

	if got := Run([]string{"--log-level", "debug", "write-config", path}, deps); got != output.CodeOK {
		t.Fatalf("Run(write-config) = %d, want %d (stderr: %q)", got, output.CodeOK, stderr.String())
	}
	if strings.Contains(stderr.String(), key) {
		t.Errorf("stderr leaked the API key: %q", stderr.String())
	}
}

// A failure is reported once: the error envelope is the contract, so the boundary
// must not also log the same error. Nothing on stderr may be a log record for a
// failing invocation at the loudest level (ADR 0002/0004).
func TestFailure_IsNotBothLoggedAndEnveloped(t *testing.T) {
	deps, _, stderr := testDeps(map[string]string{"DN_HOSTNAME": "host.example"})

	if got := Run([]string{"--log-level", "debug", "write-config"}, deps); got != output.CodeUsage {
		t.Fatalf("Run(write-config with no path) = %d, want %d", got, output.CodeUsage)
	}

	var envelopes int
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("stderr line is not JSON: %v (%q)", err, line)
		}
		if _, isRecord := rec["level"]; isRecord {
			t.Errorf("the failure was logged as well as enveloped: %q", line)
			continue
		}
		envelopes++
	}
	if envelopes != 1 {
		t.Errorf("error envelopes on stderr = %d, want exactly 1 (%q)", envelopes, stderr.String())
	}
}
