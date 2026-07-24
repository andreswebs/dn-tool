package output

import (
	"errors"
	"fmt"
	"testing"
)

// knownCodes is every exit code the taxonomy defines (ADR 0001). The registry
// may only declare codes from this set; a stray literal in CommandExitCodes is a
// drift bug this test catches.
var knownCodes = map[int]string{
	CodeOK:          "CodeOK",
	CodeResult:      "CodeResult",
	CodeUsage:       "CodeUsage",
	CodeData:        "CodeData",
	CodeUnavailable: "CodeUnavailable",
	CodeInternal:    "CodeInternal",
	CodeIO:          "CodeIO",
	CodeConfig:      "CodeConfig",
	CodeSIGINT:      "CodeSIGINT",
	CodeSIGTERM:     "CodeSIGTERM",
}

// TestRegistry_DeclaresOnlyKnownCodes guards the registry-as-data invariant: no
// command may declare a code outside the taxonomy, and no command declares the
// same code twice.
func TestRegistry_DeclaresOnlyKnownCodes(t *testing.T) {
	for cmd, codes := range CommandExitCodes {
		seen := map[int]bool{}
		for _, c := range codes {
			if _, ok := knownCodes[c]; !ok {
				t.Errorf("command %q declares unknown exit code %d", cmd, c)
			}
			if seen[c] {
				t.Errorf("command %q declares exit code %d twice", cmd, c)
			}
			seen[c] = true
		}
	}
}

// TestRegistry_CommandsCovered pins the set of commands with a declared exit-code
// contract, so a new command cannot be added without also declaring its codes.
func TestRegistry_CommandsCovered(t *testing.T) {
	want := []string{"install", "enroll", "unenroll", "write-config", "run", "dn-tool"}
	for _, cmd := range want {
		if _, ok := CommandExitCodes[cmd]; !ok {
			t.Errorf("command %q missing from the exit-code registry", cmd)
		}
	}
	if len(CommandExitCodes) != len(want) {
		t.Errorf("registry has %d commands, want %d", len(CommandExitCodes), len(want))
	}
}

// TestRegistry_RunAddsSignalCodes checks that run — the only long-running command
// — declares the graceful-shutdown signal codes and omits the result code (it
// has no --assert-changed), while the result commands do the opposite.
func TestRegistry_RunAddsSignalCodes(t *testing.T) {
	if !contains(CommandExitCodes["run"], CodeSIGINT) || !contains(CommandExitCodes["run"], CodeSIGTERM) {
		t.Error("run must declare the signal codes 130/143")
	}
	if contains(CommandExitCodes["run"], CodeResult) {
		t.Error("run has no --assert-changed, so it must not declare CodeResult")
	}
	if !contains(CommandExitCodes["enroll"], CodeResult) {
		t.Error("enroll must declare CodeResult for the --assert-changed no-op")
	}
}

// TestConformance_EachClassResolves is the ADR conformance test: for every
// failure/result class, a constructed error resolves through ResolveExitCode to
// the expected code. The cheap surfaces (usage, config, result) and the
// API/network classes are all exercised as constructed errors here; the cmd and
// package suites drive the same classes through real code paths (assert no-op,
// unknown command, config parse failure, API status errors).
func TestConformance_EachClassResolves(t *testing.T) {
	base := errors.New("boom")
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, CodeOK},
		{"result", ExitError(errors.New("no change made"), CodeResult), CodeResult},
		{"usage", Classify(base, CodeUsage), CodeUsage},
		{"data", Classify(base, CodeData), CodeData},
		{"unavailable", Classify(base, CodeUnavailable), CodeUnavailable},
		{"internal", base, CodeInternal},
		{"io", Classify(base, CodeIO), CodeIO},
		{"config", Classify(base, CodeConfig), CodeConfig},
		{"sigint", ExitError(errors.New("terminated"), CodeSIGINT), CodeSIGINT},
		{"sigterm", ExitError(errors.New("terminated"), CodeSIGTERM), CodeSIGTERM},
		// A class assigned at a leaf survives fmt.Errorf %w wrapping up the stack.
		{"wrapped io survives", fmt.Errorf("outer: %w", Classify(base, CodeIO)), CodeIO},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveExitCode(tt.err); got != tt.want {
				t.Errorf("ResolveExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func contains(codes []int, want int) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}
