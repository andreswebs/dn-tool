---
id: dt-xyp4
status: closed
deps: []
links: [dt-cjgq]
created: 2026-08-12T17:39:20Z
type: bug
priority: 1
assignee: Andre Silva
tags: [testing, golden]
---
# Guard the run golden scenario on a Linux host (make test is red on macOS)

Found while verifying dt-nrq1 on 2026-08-12. Unrelated to that defect; filed
separately because it blocks the quality gate on the owner's own machine.
Related: dt-cjgq, which built the harness.

## The defect

`make test` fails on macOS:

```txt
--- FAIL: TestGolden/run (0.00s)
    golden_test.go:362: testdata/run/stderr mismatch
         got: "{\"schema_version\":1,\"ok\":false,\"error\":{\"code\":\"unsupported_platform\",\"message\":\"install: unsupported platform: operating system \\\"darwin\\\": dn-tool requires linux\",\"hint\":\"run dn-tool on linux/amd64 or linux/arm64\"}}\n"
        want: ""
    golden_test.go:362: testdata/run/exit mismatch
         got: "78\n"
        want: "0\n"
```

Every other package passes; `internal/command` is the only failure.

## Cause

`run` installs before it enrolls, so it inherits `install`'s platform gate. The
harness already has a helper for exactly this, and its doc comment even names
`run`:

`internal/command/golden_test.go:452-462`:

```go
// requireLinuxHost skips a scenario whose command gates on the host platform.
// install (and run, which installs first) rejects a non-Linux host with
// ErrUnsupportedPlatform, so on a macOS dev machine these scenarios would pin
// the platform rejection rather than the behavior they exist for. The Linux
// build - every CI run and every release - exercises them.
func requireLinuxHost(t *testing.T) {
```

The three install-backed scenarios call it (`golden_scenarios_test.go:99`,
`:115`, `:131`), but `runSuccess()` at `golden_scenarios_test.go:373-411` does
not, even though its `build` calls `installFixture` at `:405`. So on darwin the
scenario runs the real install step, hits `DetectPlatform`, and produces exit 78
with an `unsupported_platform` envelope instead of the golden's empty stderr and
exit 0.

The golden itself is correct; it was generated on a supported host.

## Fix

Add `requireLinuxHost(t)` as the first statement of `runSuccess()`'s `build`
func, matching the three install-backed scenarios.

Then audit the rest for the same omission: any scenario whose `build` calls
`installFixture` or otherwise reaches `dninstall`, or that runs a command
gating on `runtime.GOOS`, needs the guard. Check `run`, `install-*`, and any
`enroll` scenario that installs first.

## Consider: make the guard structural

A one-line omission silently produced a red gate on a whole platform. Rather
than relying on each scenario author remembering the call, consider making it
harder to forget: a `needsLinuxHost bool` field on the `scenario` struct that
the runner honors, or having `installFixture` itself call `requireLinuxHost`,
since every caller needs the gate by definition. The second is the smaller
change and removes the choice entirely. Decide in the ticket; the one-line fix
is acceptable if the structural change is filed separately.

## Acceptance criteria

- `make test` and `make build` are green on macOS (darwin/arm64) with no golden
  regenerated, since the goldens were never wrong.
- `make test` stays green on Linux, and the `run` scenario still **executes**
  there rather than skipping, so coverage is unchanged where it matters. Verify
  the skip is platform-conditional and not unconditional.
- Every scenario reaching the install path carries the guard.
- The skip message names the reason, so a macOS run reports a skip rather than
  looking like silent coverage.
- No golden file content changes.

## Do not commit

Leave git commits and branching to the repo owner.

## Notes

**2026-08-12T17:51:30Z**

Fixed structurally, and the fix had a second half the ticket did not anticipate.

1) The gate now lives in installFixture (internal/command/golden_scenarios_test.go), not in each
   scenario: it sets fixture.installBacked and calls requireLinuxHost as its first statements. The
   three explicit requireLinuxHost calls in installSuccess/installNoOpAssertChanged/
   installChecksumMismatch were removed as redundant, so there is exactly one gate for the four
   install-backed scenarios (install-success, install-noop-assert-changed, install-checksum-mismatch,
   run). No other scenario reaches dninstall or gates on runtime.GOOS: only internal/command/install.go
   calls DetectPlatform in non-test code, and only installFixture reaches it from the harness.

2) Adding the skip alone would have left 'make test' red on macOS, for a different reason. CodeResult
   (1) is produced in process by exactly one scenario (install-noop-assert-changed), so once it skips,
   TestMain's registry-coverage check fails the package for an unexercised declared code. That check
   had been silently suppressed on macOS all along, because it only runs when m.Run() returned 0 and
   TestGolden/run was failing. coverageFailure was therefore replaced by coverageReport, returning
   (failure, note): a code no scenario covers still fails, while a code left unexercised only because
   the host skipped its scenario is printed to stderr as a note naming the codes and the scenarios.
   TestGolden records the skips (recordSkipped, keyed by command). Verified against ADR 0006's
   'every code is exercised by at least one scenario': the scenario still exists and every Linux run
   and release exercises it; this mirrors the existing exemptFromCoverage precedent of a visible,
   never-silent exemption.

3) The skip is now testable from Linux. requireLinuxHost and downloadsJSON read package-level
   hostGOOS/hostGOARCH instead of runtime.GOOS/GOARCH directly. New tests:
   - TestGolden_InstallBackedScenariosSkipOnUnsupportedHost pins darwin/arm64 and asserts every
     scenario whose build sets fixture.installBacked skips. This is the red test: before the fix it
     failed on 'run' alone, exactly as reported. It derives the scenario list from installFixture
     rather than a hardcoded list, so a new install-backed scenario is covered on arrival.
   - TestGolden_HostGateIsPlatformConditional asserts the gate does NOT skip on linux/amd64 and
     linux/arm64 and DOES on darwin/arm64 and windows/amd64, per the 'verify the skip is
     platform-conditional and not unconditional' criterion.
   - TestCoverageReport_SeparatesHostSkipsFromMissingScenarios keeps the new tolerance from becoming
     a blanket excuse: the same unexercised code with nothing skipped is still a failure.

Verification: 'make build' green on linux/arm64. GOOS=darwin GOARCH=arm64 go vet ./... passes. A
macOS run was simulated with a throwaway test that pinned hostGOOS=darwin and ran the whole scenario
list plus coverageReport: all 24 scenarios pass, the 4 install-backed ones SKIP with the reason in the
message ('install-backed scenario needs a supported host platform: unsupported platform: operating
system "darwin": dn-tool requires linux'), and coverageReport returns no failure and the note naming
exit code 1 and the four scenarios. On Linux nothing skips and TestGolden/run still executes (0.17s).
No golden file content changed; testdata/ was not touched.

Next: dt-nrq1 is now unblocked from the 'fix or land dt-xyp4 first' note. It still carries an open
design decision for the owner (whether schema gains a parallel 'statuses' list), which decides whether
testdata/schema/stdout changes.
