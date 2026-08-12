package command

import (
	"errors"
	"regexp"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// snakeCase is the machine-code form ADR 0002 fixes: lowercase words joined by
// underscores, so a consumer can branch on the code as a stable token.
var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// taxonomy is every exit code ADR 0001 defines. A sentinel declaring anything
// else would put a code outside the taxonomy into the machine contract; this is
// the terr-side counterpart of output's registry conformance test.
var taxonomy = map[int]bool{
	output.CodeOK: true, output.CodeResult: true,
	output.CodeUsage: true, output.CodeData: true, output.CodeUnavailable: true,
	output.CodeInternal: true, output.CodeIO: true, output.CodeConfig: true,
	output.CodeSIGINT: true, output.CodeSIGTERM: true,
}

// This package links every package that raises a coded error, so the registry is
// fully populated here — which makes this the place to assert the inventory's
// shape (ADR 0002: the error surface is enumerable as data, and cannot drift from
// the real one).
func TestErrorInventoryIsWellFormed(t *testing.T) {
	all := terr.All()
	if len(all) == 0 {
		t.Fatal("the error inventory is empty; no sentinel registered")
	}

	seen := map[string]bool{}
	for _, e := range all {
		if !snakeCase.MatchString(e.Code()) {
			t.Errorf("code %q is not snake_case", e.Code())
		}
		if seen[e.Code()] {
			t.Errorf("code %q registered twice", e.Code())
		}
		seen[e.Code()] = true

		if !taxonomy[e.ExitCode()] {
			t.Errorf("code %q declares exit code %d, which is outside the ADR 0001 taxonomy", e.Code(), e.ExitCode())
		}
		if e.ExitCode() == output.CodeOK {
			t.Errorf("code %q declares exit code 0; an error is never a clean success", e.Code())
		}
		if e.Error() == "" {
			t.Errorf("code %q has an empty message", e.Code())
		}
		if e.Hint() == "" && !e.NonFailure() {
			t.Errorf("code %q has no remediation hint", e.Code())
		}
	}
}

// The non-failure statuses are pinned as a set (dt-nrq1): a status writes no error
// envelope, so a sentinel that becomes one by accident would silence a real
// failure. Adding a code here is a deliberate, reviewed contract change.
func TestNonFailureStatusesArePinned(t *testing.T) {
	want := map[string]bool{"no_change": true, "signal_shutdown": true}

	for _, e := range terr.All() {
		if e.NonFailure() != want[e.Code()] {
			t.Errorf("code %q NonFailure() = %v, want %v", e.Code(), e.NonFailure(), want[e.Code()])
		}
		delete(want, e.Code())
	}
	for code := range want {
		t.Errorf("status %q is not registered", code)
	}
}

// The CLI-surface sentinels are the ones this package raises; pin their codes so
// a rename is a deliberate, reviewed contract change rather than a silent one.
func TestCommandSentinelCodes(t *testing.T) {
	tests := []struct {
		sentinel *terr.E
		code     string
		exit     int
	}{
		{ErrUsage, "usage_error", output.CodeUsage},
		{ErrUnknownCommand, "unknown_command", output.CodeUsage},
		{ErrNoCommand, "no_command", output.CodeUsage},
		{ErrMissingArgument, "missing_argument", output.CodeUsage},
		{ErrNoChange, "no_change", output.CodeResult},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := tt.sentinel.Code(); got != tt.code {
				t.Errorf("Code() = %q, want %q", got, tt.code)
			}
			if got := tt.sentinel.ExitCode(); got != tt.exit {
				t.Errorf("ExitCode() = %d, want %d", got, tt.exit)
			}
		})
	}
}

// The --assert-changed no-op is a result, not a failure: it keeps exit 1 and its
// own code now that it carries one.
func TestErrNoChangeResolvesToTheResultClass(t *testing.T) {
	if got := output.ExitCodeFor(ErrNoChange); got != output.CodeResult {
		t.Errorf("ExitCodeFor(ErrNoChange) = %d, want %d", got, output.CodeResult)
	}
	if !errors.Is(errMissingWriteConfigPath, ErrMissingArgument) {
		t.Error("write-config's missing-path error must match ErrMissingArgument")
	}
}
