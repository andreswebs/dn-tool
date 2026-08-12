// Package command_test carries the in-process golden-triple harness required by
// ADR 0006: each scenario drives the delegate — command.Run(args, deps) — with
// buffer streams and compares three artifacts against golden files under
// testdata/<scenario>/: stdout, stderr, and the exit code. Regenerate them with
//
//	go test ./internal/command -update
//
// The suite mentions no CLI framework, only the delegate contract, so it is the
// safety net for a framework replacement rather than a casualty of one.
package command_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/andreswebs/dn-tool/internal/command"
	"github.com/andreswebs/dn-tool/internal/dninstall"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/version"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Normalization tokens. Every volatile value a scenario can emit is replaced by
// one of these before comparison; an unnormalized value would make a golden pass
// once and flake forever after, which ADR 0006 names as this harness's main
// failure mode.
const (
	tokenAPIURL     = "$API_URL"
	tokenBinDir     = "$BIN_DIR"
	tokenConfigRoot = "$CONFIG_ROOT"
	tokenCWD        = "$CWD"
	tokenPort       = "$PORT"
	tokenRunID      = "$RUN_ID"
	tokenTime       = "$TIME"
	tokenTmpDir     = "$TMP_DIR"
	tokenVersion    = "$VERSION"
)

// Fixture values that appear in goldens. They are constants rather than
// generated, because a generated host id or network name is one more volatile
// value to normalize.
const (
	fixtureAPIKey    = "test-api-key"
	fixtureHostID    = "host-golden"
	fixtureHostname  = "golden-host"
	fixtureNetwork   = "testnet"
	fixtureNetworkID = "net-golden"
	fixtureRoleID    = "role-golden"
	fixtureVersion   = "1.2.3"
)

// hostGOOS and hostGOARCH are the platform the install-backed scenarios gate on.
// They are variables rather than direct runtime references so a test can pin an
// unsupported host and assert the gate fires: a Linux run cannot otherwise cover
// the skip, which is how a missing gate reached a red macOS gate unnoticed.
var hostGOOS, hostGOARCH = runtime.GOOS, runtime.GOARCH

// masked regexps strip the volatile values no scenario can hand over as a
// literal: the slog record fields the logger stamps on every record (ADR 0004),
// the build-injected version, and any ephemeral port that survived URL masking.
var maskedPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`"time":"[^"]*"`), `"time":"` + tokenTime + `"`},
	{regexp.MustCompile(`"run_id":"[^"]*"`), `"run_id":"` + tokenRunID + `"`},
	{regexp.MustCompile(`\btime=\S+`), "time=" + tokenTime},
	{regexp.MustCompile(`\brun_id=\S+`), "run_id=" + tokenRunID},
	{regexp.MustCompile(`127\.0\.0\.1:\d+`), "127.0.0.1:" + tokenPort},
	{regexp.MustCompile(`\[::1\]:\d+`), "[::1]:" + tokenPort},
}

// observed records every exit code the scenarios produced, plus the scenarios the
// host could not run, so TestMain can hold the suite to the ADR 0006 obligation
// that the declared registry is exercised rather than merely declared, and can
// tell a code nobody wrote a scenario for from one this host cannot reach.
// Scenarios may run in any order, hence the lock.
var observed = struct {
	mu      sync.Mutex
	codes   map[int]bool
	skipped map[string][]string
}{codes: map[int]bool{}, skipped: map[string][]string{}}

func recordObserved(code int) {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	observed.codes[code] = true
}

// recordSkipped registers a scenario the host could not run, under the command
// whose declared codes it would have exercised.
func recordSkipped(sc scenario) {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	observed.skipped[sc.command] = append(observed.skipped[sc.command], sc.name)
}

// exemptFromCoverage are the declared codes no in-process scenario can reach,
// each with the test that does cover it. The exemption is spelled out here, and
// reported by TestMain, because a silent exemption would make the coverage check
// read as "everything is covered" when it is not — the same reason
// output.RunPassesThroughDaemonExitStatus documents the registry's own exception.
var exemptFromCoverage = map[int]string{
	output.CodeInternal: "an unclassified error cannot be provoked through the CLI surface; " +
		"covered by TestExitCode_EmitsErrorEnvelopePerExitClass (error_envelope_test.go)",
	output.CodeSIGINT:  "requires a real signal; covered by TestFinalize_* (internal/run/signal_test.go)",
	output.CodeSIGTERM: "requires a real signal; covered by TestFinalize_* (internal/run/signal_test.go)",
}

// TestMain enforces registry coverage after the scenarios have run. The check is
// skipped under -update (goldens are being rewritten, not asserted) and under a
// -run filter, where exercising a subset is the point of the filter.
func TestMain(m *testing.M) {
	flag.Parse()
	code := m.Run()
	if code == 0 && !*update && flag.Lookup("test.run").Value.String() == "" {
		failure, note := coverageReport()
		if note != "" {
			fmt.Fprintln(os.Stderr, note)
		}
		if failure != "" {
			fmt.Fprintln(os.Stderr, failure)
			code = 1
		}
	}
	os.Exit(code)
}

// coverageReport audits the declared, in-process-reachable exit codes against
// what the scenarios produced. failure names the codes no scenario exercised and
// fails the suite; note names the codes left unexercised only because the host
// platform skipped their scenario, which is a coverage gap this host cannot close
// and must therefore say out loud rather than pass over in silence. The daemon
// exit-status passthrough (output.RunPassesThroughDaemonExitStatus) is not an
// enumerable code and is covered out of process by
// TestRunAndUnenroll_PropagatesDaemonExitStatus (internal/run/run_test.go).
func coverageReport() (failure, note string) {
	observed.mu.Lock()
	defer observed.mu.Unlock()

	var missing, hostSkipped []int
	for command, codes := range output.CommandExitCodes {
		for _, code := range codes {
			if _, exempt := exemptFromCoverage[code]; exempt || observed.codes[code] {
				continue
			}
			switch {
			case len(observed.skipped[command]) > 0:
				if !slicesContainsInt(hostSkipped, code) {
					hostSkipped = append(hostSkipped, code)
				}
			case !slicesContainsInt(missing, code):
				missing = append(missing, code)
			}
		}
	}
	// A code both a skipped and a run command declares is only truly missing if no
	// command exercised it, and the skip is the likelier cause; drop the overlap.
	filtered := missing[:0]
	for _, code := range missing {
		if !slicesContainsInt(hostSkipped, code) {
			filtered = append(filtered, code)
		}
	}
	missing = filtered

	sort.Ints(missing)
	sort.Ints(hostSkipped)
	if len(hostSkipped) > 0 {
		note = fmt.Sprintf("golden scenarios left declared exit code %v unexercised on %s/%s, "+
			"because the host platform skipped the scenarios that reach them (%s); "+
			"the linux build covers them", hostSkipped, hostGOOS, hostGOARCH, skippedScenarioList())
	}
	if len(missing) > 0 {
		failure = fmt.Sprintf("golden scenarios exercise no declared exit code %v; "+
			"add a scenario or, if the code is unreachable in process, exempt it visibly in exemptFromCoverage", missing)
	}
	return failure, note
}

// skippedScenarioList names the skipped scenarios, sorted, for the coverage note.
// The caller holds observed.mu.
func skippedScenarioList() string {
	var names []string
	for _, scenarioNames := range observed.skipped {
		names = append(names, scenarioNames...)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func slicesContainsInt(haystack []int, needle int) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// fixture is a scenario's mutable setup: the environment the delegate resolves
// configuration from, and the literal-to-token substitutions that make its
// output comparable.
type fixture struct {
	env  map[string]string
	subs []substitution

	// installBacked records that the scenario wired the install path, so
	// TestGolden_InstallBackedScenariosSkipOnUnsupportedHost can tell which
	// scenarios owe the host-platform gate without hardcoding a list.
	installBacked bool
}

// substitution replaces one scenario-specific literal (a temp dir, a test
// server's URL) with a stable token.
type substitution struct {
	literal string
	token   string
}

func newFixture() *fixture {
	return &fixture{env: map[string]string{}}
}

func (f *fixture) setenv(key, value string) {
	f.env[key] = value
}

// mask registers literal for replacement by token. An empty literal is ignored,
// so a caller can mask an optional value unconditionally.
func (f *fixture) mask(literal, token string) {
	if literal == "" {
		return
	}
	f.subs = append(f.subs, substitution{literal: literal, token: token})
}

// tempDir returns a scenario-scoped temp dir and masks it, so any path built
// beneath it normalizes without a second call.
func (f *fixture) tempDir(t *testing.T, token string) string {
	t.Helper()
	dir := t.TempDir()
	f.mask(dir, token)
	return dir
}

// normalize replaces every volatile value in s with its token: the scenario's own
// literals first (longest first, so a nested path does not defeat its parent),
// then the pattern-matched ones, then CRLF.
func (f *fixture) normalize(s string) string {
	subs := append([]substitution(nil), f.subs...)
	subs = append(subs, substitution{literal: version.Current(), token: tokenVersion})
	if cwd, err := os.Getwd(); err == nil {
		subs = append(subs, substitution{literal: cwd, token: tokenCWD})
	}
	sort.SliceStable(subs, func(i, j int) bool {
		return len(subs[i].literal) > len(subs[j].literal)
	})
	for _, sub := range subs {
		s = strings.ReplaceAll(s, sub.literal, sub.token)
	}
	for _, m := range maskedPatterns {
		s = m.pattern.ReplaceAllString(s, m.replace)
	}
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// scenario is one golden case: the registry key its exit code must be declared
// under, and a build step that prepares the fixture and returns the arguments.
type scenario struct {
	name    string
	command string
	build   func(t *testing.T, f *fixture) []string
}

// result is a scenario's normalized golden triple.
type result struct {
	stdout, stderr string
	exit           int
}

// run drives the delegate once with a freshly built fixture and returns the
// normalized triple. Every invocation gets its own buffers and environment, so
// nothing leaks between a scenario's two runs (the determinism check) or between
// scenarios.
func (sc scenario) run(t *testing.T) result {
	t.Helper()
	f := newFixture()
	args := sc.build(t, f)
	var stdout, stderr bytes.Buffer
	exit := command.Run(args, command.Deps{
		Out:    &stdout,
		Err:    &stderr,
		Getenv: func(key string) string { return f.env[key] },
	})
	return result{stdout: f.normalize(stdout.String()), stderr: f.normalize(stderr.String()), exit: exit}
}

// runScenario executes sc, asserts the ADR 0001 registry membership and the ADR
// 0005 stream contract, and compares the triple against the goldens.
func runScenario(t *testing.T, sc scenario) {
	t.Helper()
	got := sc.run(t)
	recordObserved(got.exit)

	assertDeclared(t, sc.command, got.exit)
	assertStreamContract(t, got)

	dir := filepath.Join("testdata", sc.name)
	compareGolden(t, filepath.Join(dir, "stdout"), got.stdout)
	compareGolden(t, filepath.Join(dir, "stderr"), got.stderr)
	compareGolden(t, filepath.Join(dir, "exit"), strconv.Itoa(got.exit)+"\n")
}

// assertDeclared holds every observed exit to the per-command registry (ADR
// 0001): a code a command can really produce but does not declare is a contract
// break, whichever side is wrong.
func assertDeclared(t *testing.T, commandName string, exit int) {
	t.Helper()
	declared, ok := output.CommandExitCodes[commandName]
	if !ok {
		t.Fatalf("no exit-code registry entry for %q", commandName)
	}
	if !slicesContainsInt(declared, exit) {
		t.Errorf("exit code %d is not declared for %q (declared: %v)", exit, commandName, declared)
	}
}

// assertStreamContract pins the ADR 0005 split: a failure writes its envelope to
// stderr and nothing at all to stdout, while a non-failure status writes no error
// envelope at all — the --assert-changed no-op (exit 1) still writes its result
// envelope to stdout, and a signal shutdown (130/143) leaves stderr to log records.
//
// The status arms are the guard dt-nrq1 was missing: without them the harness
// pinned an ok:false envelope beside an ok:true result for one successful
// invocation and read it as correct.
func assertStreamContract(t *testing.T, got result) {
	t.Helper()
	switch got.exit {
	case output.CodeOK:
		return
	case output.CodeResult:
		if got.stdout == "" {
			t.Error("stdout is empty on the --assert-changed no-op, want the result envelope")
		}
		assertNoErrorEnvelope(t, got.stderr)
	case output.CodeSIGINT, output.CodeSIGTERM:
		assertNoErrorEnvelope(t, got.stderr)
	default:
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing on failure", got.stdout)
		}
		assertErrorEnvelope(t, got.stderr)
	}
}

// assertNoErrorEnvelope fails if any line of stderr is an error envelope. Log
// records are permitted: the rule is that a non-failure status reports no
// failure, not that it stays silent.
func assertNoErrorEnvelope(t *testing.T, stderr string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(stderr, "\n"), "\n") {
		var env struct {
			Error *struct{} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &env) == nil && env.Error != nil {
			t.Errorf("stderr carries an error envelope for a non-failure status: %q", line)
		}
	}
}

// assertErrorEnvelope decodes the last line of stderr as the ADR 0005 error
// envelope. It is the last line because log records at or above the level
// threshold may precede it; the envelope is always written last, by the exit
// boundary.
func assertErrorEnvelope(t *testing.T, stderr string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	last := lines[len(lines)-1]

	var env struct {
		SchemaVersion int  `json:"schema_version"`
		OK            bool `json:"ok"`
		Error         struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(last), &env); err != nil {
		t.Fatalf("stderr does not end with a JSON error envelope: %v (%q)", err, last)
	}
	if env.SchemaVersion != output.SchemaVersion {
		t.Errorf("envelope schema_version = %d, want %d", env.SchemaVersion, output.SchemaVersion)
	}
	if env.OK {
		t.Error("envelope ok = true on a failure, want false")
	}
	if env.Error.Code == "" || env.Error.Message == "" {
		t.Errorf("envelope error = %+v, want a code and a message", env.Error)
	}
}

// compareGolden compares got against the golden file at path, or rewrites it
// under -update.
func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run `go test ./internal/command -update` to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch\n got: %q\nwant: %q", path, got, string(want))
	}
}

// TestGolden runs every scenario against its goldens. Adding a scenario is a row
// in scenarios() plus one `-update` run. A scenario the host platform skips is
// registered as such, so the coverage check reports the gap instead of failing on
// a code this host was never able to reach.
func TestGolden(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			defer func() {
				if t.Skipped() {
					recordSkipped(sc)
				}
			}()
			runScenario(t, sc)
		})
	}
}

// TestGolden_IsDeterministic runs every scenario twice and compares the two
// normalized triples. A volatile value that escaped normalization fails here,
// loudly and immediately, instead of flaking in someone else's run weeks later.
func TestGolden_IsDeterministic(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			first, second := sc.run(t), sc.run(t)
			if first.stdout != second.stdout {
				t.Errorf("stdout differs between runs:\n first: %q\nsecond: %q", first.stdout, second.stdout)
			}
			if first.stderr != second.stderr {
				t.Errorf("stderr differs between runs:\n first: %q\nsecond: %q", first.stderr, second.stderr)
			}
			if first.exit != second.exit {
				t.Errorf("exit differs between runs: %d then %d", first.exit, second.exit)
			}
		})
	}
}

// withHostPlatform pins the platform the host-platform gate reads, restoring it
// when the test ends. The suite runs no scenario in parallel, so a package-level
// swap is safe.
func withHostPlatform(t *testing.T, goos, goarch string) {
	t.Helper()
	prevOS, prevArch := hostGOOS, hostGOARCH
	t.Cleanup(func() { hostGOOS, hostGOARCH = prevOS, prevArch })
	hostGOOS, hostGOARCH = goos, goarch
}

// TestGolden_HostGateIsPlatformConditional pins the gate itself in both
// directions: it skips on an unsupported host and, more importantly, does not
// skip on a supported one: an unconditional skip would silently drop the
// install-backed scenarios from every CI run.
func TestGolden_HostGateIsPlatformConditional(t *testing.T) {
	for _, tc := range []struct {
		name, goos, goarch string
		wantSkip           bool
	}{
		{name: "linux-amd64", goos: "linux", goarch: "amd64", wantSkip: false},
		{name: "linux-arm64", goos: "linux", goarch: "arm64", wantSkip: false},
		{name: "darwin-arm64", goos: "darwin", goarch: "arm64", wantSkip: true},
		{name: "windows-amd64", goos: "windows", goarch: "amd64", wantSkip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withHostPlatform(t, tc.goos, tc.goarch)
			if got := probeSkip(t, tc.name, requireLinuxHost); got != tc.wantSkip {
				t.Errorf("requireLinuxHost on %s/%s skipped = %v, want %v", tc.goos, tc.goarch, got, tc.wantSkip)
			}
		})
	}
}

// TestGolden_InstallBackedScenariosSkipOnUnsupportedHost is the guard the missing
// gate needed: with an unsupported host pinned, every scenario that wires the
// install path must skip rather than run install and pin its platform rejection.
// It derives the scenario list from installFixture's own calls, so a new
// install-backed scenario is covered the moment it is added.
func TestGolden_InstallBackedScenariosSkipOnUnsupportedHost(t *testing.T) {
	withHostPlatform(t, "darwin", "arm64")

	installBacked := 0
	for _, sc := range scenarios() {
		f := newFixture()
		skipped := probeSkip(t, sc.name, func(t *testing.T) { sc.build(t, f) })
		if !f.installBacked {
			continue
		}
		installBacked++
		if !skipped {
			t.Errorf("scenario %q wires the install path but its build did not skip on darwin/arm64: "+
				"it would pin install's platform rejection instead of its own behavior", sc.name)
		}
	}
	if installBacked == 0 {
		t.Fatal("no scenario reported wiring the install path; the probe is no longer measuring anything")
	}
}

// probeSkip runs fn as a subtest and reports whether it skipped. A skip unwinds
// the goroutine, so the verdict is read in a deferred call; a failing fn reports
// itself through the subtest and is not counted as a skip.
func probeSkip(t *testing.T, name string, fn func(t *testing.T)) bool {
	t.Helper()
	var skipped bool
	t.Run("probe/"+name, func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		fn(t)
	})
	return skipped
}

// TestCoverageReport_SeparatesHostSkipsFromMissingScenarios pins the coverage
// audit's two verdicts. The tolerance exists so a host that cannot run the
// install-backed scenarios is not failed for their codes; it must not become a
// blanket excuse, so the same unexercised code with nothing skipped still fails.
func TestCoverageReport_SeparatesHostSkipsFromMissingScenarios(t *testing.T) {
	// Every declared code except CodeResult (1), which only install-backed
	// scenarios produce in process.
	allButResult := map[int]bool{}
	for _, codes := range output.CommandExitCodes {
		for _, code := range codes {
			if code != output.CodeResult {
				allButResult[code] = true
			}
		}
	}

	t.Run("host skip is reported, not failed", func(t *testing.T) {
		withObserved(t, allButResult, map[string][]string{"install": {"install-noop-assert-changed"}})
		failure, note := coverageReport()
		if failure != "" {
			t.Errorf("failure = %q, want none when the host skipped the covering scenario", failure)
		}
		if !strings.Contains(note, "install-noop-assert-changed") || !strings.Contains(note, "[1]") {
			t.Errorf("note = %q, want it to name exit code 1 and the skipped scenario", note)
		}
	})

	t.Run("nothing skipped is a failure", func(t *testing.T) {
		withObserved(t, allButResult, map[string][]string{})
		failure, note := coverageReport()
		if !strings.Contains(failure, "[1]") {
			t.Errorf("failure = %q, want it to name the unexercised exit code 1", failure)
		}
		if note != "" {
			t.Errorf("note = %q, want none when nothing was skipped", note)
		}
	})
}

// withObserved pins the coverage audit's inputs, restoring them when the test
// ends, so a case can describe a run this host did not have.
func withObserved(t *testing.T, codes map[int]bool, skipped map[string][]string) {
	t.Helper()
	observed.mu.Lock()
	defer observed.mu.Unlock()
	prevCodes, prevSkipped := observed.codes, observed.skipped
	t.Cleanup(func() {
		observed.mu.Lock()
		defer observed.mu.Unlock()
		observed.codes, observed.skipped = prevCodes, prevSkipped
	})
	observed.codes, observed.skipped = codes, skipped
}

// TestGolden_ExemptionsAreDocumented pins the exemption list itself: every code
// left out of the coverage check must name the test that does cover it, so the
// list cannot quietly grow into an excuse.
func TestGolden_ExemptionsAreDocumented(t *testing.T) {
	for code, reason := range exemptFromCoverage {
		if !strings.Contains(reason, "_test.go") {
			t.Errorf("exemption for exit code %d does not name a covering test file: %q", code, reason)
		}
	}
}

// TestGolden_NoVolatileValuesInGoldens is the belt to the determinism check's
// braces: it reads the committed goldens back and rejects any absolute path,
// version string, port, timestamp or run id that a future scenario might bake in.
func TestGolden_NoVolatileValuesInGoldens(t *testing.T) {
	forbidden := []struct {
		pattern *regexp.Regexp
		what    string
	}{
		{regexp.MustCompile(`/tmp/|/var/folders/|` + regexp.QuoteMeta(os.TempDir())), "an absolute temp path"},
		{regexp.MustCompile(`127\.0\.0\.1:\d+`), "an ephemeral port"},
		{regexp.MustCompile(`"time":"\d`), "a timestamp"},
		{regexp.MustCompile(`"run_id":"[0-9a-zA-Z]`), "a run id"},
		{regexp.MustCompile(regexp.QuoteMeta(version.Current())), "the build version"},
	}
	err := filepath.WalkDir("testdata", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, f := range forbidden {
			if f.pattern.Match(data) {
				t.Errorf("%s contains %s: %s", path, f.what, f.pattern)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking testdata: %v", err)
	}
}

// digestOf is the lowercase-hex SHA-256 of content, the form the downloads API
// publishes a binary's checksum in (§6.3).
func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// downloadsJSON is the GET /v1/downloads payload resolving the host's own
// os-arch to binURL at fixtureVersion, which it also designates latest.
func downloadsJSON(t *testing.T, binURL string) string {
	t.Helper()
	platform, err := dninstall.DetectPlatform(hostGOOS, hostGOARCH)
	if err != nil {
		t.Fatalf("resolving the host platform: %v", err)
	}
	key := platform.OS + "-" + platform.Arch
	return fmt.Sprintf(`{"data":{"dnclient":{%q:{%q:%q}},"versionInfo":{"latest":{"dnclient":%q}}}}`,
		fixtureVersion, key, binURL, fixtureVersion)
}

// requireLinuxHost skips a scenario whose command gates on the host platform.
// install (and run, which installs first) rejects a non-Linux host with
// ErrUnsupportedPlatform, so on a macOS dev machine these scenarios would pin
// the platform rejection rather than the behavior they exist for. The Linux
// build — every CI run and every release — exercises them.
//
// installFixture calls this, so every install-backed scenario carries the gate by
// construction rather than by its author remembering to ask for it.
func requireLinuxHost(t *testing.T) {
	t.Helper()
	if _, err := dninstall.DetectPlatform(hostGOOS, hostGOARCH); err != nil {
		t.Skipf("install-backed scenario needs a supported host platform: %v", err)
	}
}

// apiServer starts a test server, masks its URL, and points DN_API_URL at it.
func apiServer(t *testing.T, f *fixture, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	f.mask(srv.URL, tokenAPIURL)
	f.setenv("DN_API_URL", srv.URL)
	return srv
}

// enrollEnv sets the DN_* variables the enroll and unenroll paths need, with the
// hostname pinned: an unset DN_HOSTNAME falls back to the real system hostname,
// which would land in every golden that names the enrollment.
func enrollEnv(f *fixture, configRoot, binDir string) {
	f.setenv("DN_API_KEY", fixtureAPIKey)
	f.setenv("DN_NETWORK_ID", fixtureNetworkID)
	f.setenv("DN_ROLE_ID", fixtureRoleID)
	f.setenv("DN_NETWORK_NAME", fixtureNetwork)
	f.setenv("DN_HOSTNAME", fixtureHostname)
	f.setenv("DN_CLIENT_CONFIG_DIR", configRoot)
	f.setenv("DN_CLIENT_BIN_DIR", binDir)
}

// writeFakeDNClient places an executable shell stand-in for the dnclient binary
// at <binDir>/dnclient and returns its content, so a caller can publish the
// matching checksum and let install treat it as already present.
func writeFakeDNClient(t *testing.T, binDir, script string) string {
	t.Helper()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", binDir, err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "dnclient"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake dnclient: %v", err)
	}
	return script
}

// writeDNClientConfig creates <root>/<network>/dnclient.yml carrying hostID, the
// local "already enrolled" signal unenroll reads the host id from.
func writeDNClientConfig(t *testing.T, root, network, hostID string) {
	t.Helper()
	dir := filepath.Join(root, network)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "metadata:\n  host_id: " + hostID + "\n"
	if err := os.WriteFile(filepath.Join(dir, "dnclient.yml"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing dnclient.yml: %v", err)
	}
}
