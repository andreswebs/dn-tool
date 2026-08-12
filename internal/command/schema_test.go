package command

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
	"github.com/andreswebs/dn-tool/internal/version"
)

// runSchema drives the real CLI's schema command and decodes its stdout, so every
// assertion below is made against what a consumer actually receives.
func runSchema(t *testing.T) (output.SchemaEnvelope, string) {
	t.Helper()
	deps, stdout, stderr := testDeps(nil)
	if got := Run([]string{"schema"}, deps); got != output.CodeOK {
		t.Fatalf("schema exit code = %d, want %d (stderr: %s)", got, output.CodeOK, stderr)
	}
	var env output.SchemaEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode schema stdout %q: %v", stdout.String(), err)
	}
	return env, stdout.String()
}

// ADR 0005: schema emits exactly one newline-terminated JSON object on stdout,
// opening with the same head every other envelope carries.
func TestSchema_EmitsOneEnvelopeOnStdout(t *testing.T) {
	env, raw := runSchema(t)

	const head = `{"schema_version":1,"ok":true,`
	if !strings.HasPrefix(raw, head) {
		t.Errorf("stdout = %q, want it to open with %s", raw, head)
	}
	if strings.Count(raw, "\n") != 1 || !strings.HasSuffix(raw, "\n") {
		t.Errorf("stdout = %q, want exactly one trailing newline", raw)
	}
	if env.Tool != rootName {
		t.Errorf("tool = %q, want %q", env.Tool, rootName)
	}
	if env.Version != version.Current() {
		t.Errorf("version = %q, want %q", env.Version, version.Current())
	}
}

// Drift test: the projected command list is the framework tree, both ways. A
// command added to the tree without appearing here (or a projection that invents
// one) is exactly the drift ADR 0005 forbids.
func TestSchema_ProjectsEveryCommandInTheTree(t *testing.T) {
	env, _ := runSchema(t)

	projected := map[string]output.SchemaCommand{}
	for _, c := range env.Commands {
		if _, dup := projected[c.Name]; dup {
			t.Errorf("command %q projected twice", c.Name)
		}
		projected[c.Name] = c
	}

	inTree := map[string]bool{}
	for _, sub := range newRoot(Deps{}.withDefaults()).Commands {
		if sub.Name == helpCommandName {
			continue
		}
		inTree[sub.Name] = true
		c, ok := projected[sub.Name]
		if !ok {
			t.Errorf("command %q is in the tree but missing from the schema projection", sub.Name)
			continue
		}
		if c.Usage != sub.Usage {
			t.Errorf("command %q usage = %q, want %q", c.Name, c.Usage, sub.Usage)
		}
		if c.Args != sub.ArgsUsage {
			t.Errorf("command %q args = %q, want %q", c.Name, c.Args, sub.ArgsUsage)
		}
	}
	for name := range projected {
		if !inTree[name] {
			t.Errorf("schema projects command %q, which is not in the tree", name)
		}
	}
	if _, ok := projected["schema"]; !ok {
		t.Error("schema must appear in its own projected command list")
	}
}

// Every projected command carries its own declared exit-code set, and the
// tool-level list is the union. A command in the tree with no registry entry is a
// declaration gap this catches.
func TestSchema_ProjectsTheExitCodeRegistry(t *testing.T) {
	env, _ := runSchema(t)

	for _, c := range env.Commands {
		want, ok := output.CommandExitCodes[c.Name]
		if !ok {
			t.Errorf("command %q has no entry in the exit-code registry", c.Name)
			continue
		}
		got := append([]int(nil), c.ExitCodes...)
		wantSorted := append([]int(nil), want...)
		sort.Ints(got)
		sort.Ints(wantSorted)
		if !equalInts(got, wantSorted) {
			t.Errorf("command %q exit codes = %v, want %v", c.Name, got, wantSorted)
		}
	}

	if !equalInts(env.ExitCodes, output.AllExitCodes()) {
		t.Errorf("union exit codes = %v, want %v", env.ExitCodes, output.AllExitCodes())
	}
}

// The one registry exception reaches the consumer as data, not as prose: run may
// exit outside its declared set because it propagates the daemon's status.
func TestSchema_MarksRunAsPassingThroughTheChildExitStatus(t *testing.T) {
	env, _ := runSchema(t)

	for _, c := range env.Commands {
		want := c.Name == "run" && output.RunPassesThroughDaemonExitStatus
		if c.PassesThroughChildExitStatus != want {
			t.Errorf("command %q passthrough = %v, want %v", c.Name, c.PassesThroughChildExitStatus, want)
		}
	}
}

// Drift test: every failure sentinel registered with terr.New is in the error
// inventory, with its code, exit code, and hint. The command package's import
// graph reaches every package that raises a failure, so this counts the whole real
// surface.
func TestSchema_ProjectsEveryRegisteredErrorSentinel(t *testing.T) {
	env, _ := runSchema(t)

	var sentinels []*terr.E
	for _, s := range terr.All() {
		if !s.NonFailure() {
			sentinels = append(sentinels, s)
		}
	}
	if len(env.Errors) != len(sentinels) {
		t.Fatalf("errors has %d entries, want %d (terr.All() failures)", len(env.Errors), len(sentinels))
	}
	byCode := map[string]output.SchemaError{}
	for _, e := range env.Errors {
		byCode[e.Code] = e
	}
	for _, s := range sentinels {
		got, ok := byCode[s.Code()]
		if !ok {
			t.Errorf("sentinel %q missing from the error inventory", s.Code())
			continue
		}
		if got.ExitCode != s.ExitCode() {
			t.Errorf("sentinel %q exit code = %d, want %d", s.Code(), got.ExitCode, s.ExitCode())
		}
		if got.Hint != s.Hint() {
			t.Errorf("sentinel %q hint = %q, want %q", s.Code(), got.Hint, s.Hint())
		}
	}
	// A spot check that the inventory is the real one and not an empty pass:
	// these three sentinels live in three different packages.
	for _, code := range []string{"usage_error", "config_error", "dnclient_failed"} {
		if _, ok := byCode[code]; !ok {
			t.Errorf("error inventory is missing %q", code)
		}
	}
}

// The global flags are reported, so a consumer discovers the whole invocable
// surface from one call. They are read off the tree, so a new global flag appears
// without touching the projection.
func TestSchema_ReportsGlobalFlags(t *testing.T) {
	env, _ := runSchema(t)

	names := map[string]bool{}
	for _, f := range env.GlobalFlags {
		names[f.Name] = true
		if f.Usage == "" {
			t.Errorf("global flag %q has no usage text", f.Name)
		}
	}
	for _, want := range []string{"--env-file", "--assert-changed", "--log-level", "--quiet"} {
		if !names[want] {
			t.Errorf("global flag %q missing from the schema projection, got %v", want, names)
		}
	}
}

// A command's own flags are scoped to it: --force is enroll's, never a global.
func TestSchema_ReportsCommandScopedFlags(t *testing.T) {
	env, _ := runSchema(t)

	for _, c := range env.Commands {
		if c.Name != "enroll" {
			continue
		}
		names := map[string]bool{}
		for _, f := range c.Flags {
			names[f.Name] = true
		}
		if !names["--force"] {
			t.Errorf("enroll flags = %v, want --force among them", names)
		}
		if names["--env-file"] {
			t.Errorf("enroll flags = %v, want the global --env-file reported only as a global", names)
		}
		return
	}
	t.Error("enroll missing from the projected command list")
}

// schema declares CodeIO, so the one failure it can actually produce resolves to
// it: a stdout that cannot be written is a local I/O fault, not a bug (70).
func TestSchema_UnwritableStdoutIsAnIOError(t *testing.T) {
	deps, _, _ := testDeps(nil)
	deps.Out = failingWriter{}

	if got := Run([]string{"schema"}, deps); got != output.CodeIO {
		t.Errorf("schema with an unwritable stdout = %d, want %d", got, output.CodeIO)
	}
}

// failingWriter is a stdout that always fails, standing in for a closed pipe or a
// full filesystem.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The parallel statuses list (dt-nrq1, option a): the two non-failure codes a
// consumer can see as an exit status are reported as data, with their meaning, and
// never as errors — the schema is where the code-to-meaning mapping lives now that
// no envelope carries it.
func TestSchema_ProjectsNonFailureStatusesSeparately(t *testing.T) {
	env, _ := runSchema(t)

	byCode := map[string]output.SchemaStatus{}
	for _, s := range env.Statuses {
		byCode[s.Code] = s
	}
	for _, want := range []struct {
		code string
		exit int
	}{
		{"no_change", output.CodeResult},
		{"signal_shutdown", output.CodeSIGTERM},
	} {
		got, ok := byCode[want.code]
		if !ok {
			t.Errorf("statuses is missing %q", want.code)
			continue
		}
		if got.ExitCode != want.exit {
			t.Errorf("status %q exit code = %d, want %d", want.code, got.ExitCode, want.exit)
		}
		if got.Meaning == "" {
			t.Errorf("status %q has no meaning", want.code)
		}
	}
	for _, e := range env.Errors {
		if _, isStatus := byCode[e.Code]; isStatus {
			t.Errorf("code %q is reported as both an error and a status", e.Code)
		}
	}
}
