package output

import (
	"errors"
	"fmt"
	"testing"

	"github.com/andreswebs/dn-tool/internal/terr"
)

func TestExitCodes_Canonical(t *testing.T) {
	cases := map[string]struct{ got, want int }{
		"CodeOK":          {CodeOK, 0},
		"CodeResult":      {CodeResult, 1},
		"CodeUsage":       {CodeUsage, 64},
		"CodeData":        {CodeData, 65},
		"CodeUnavailable": {CodeUnavailable, 69},
		"CodeInternal":    {CodeInternal, 70},
		"CodeIO":          {CodeIO, 74},
		"CodeConfig":      {CodeConfig, 78},
		"CodeSIGINT":      {CodeSIGINT, 130},
		"CodeSIGTERM":     {CodeSIGTERM, 143},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", name, c.got, c.want)
		}
	}
}

func TestExitCodeFor_UnclassifiedErrorIsInternal(t *testing.T) {
	if got := ExitCodeFor(errors.New("boom")); got != CodeInternal {
		t.Errorf("ExitCodeFor(plain) = %d, want %d", got, CodeInternal)
	}
}

func TestExitCodeFor_CodedErrorCarriesItsOwnCode(t *testing.T) {
	coded := terr.Newf("bad_config", CodeConfig, "", "bad config")
	if got := ExitCodeFor(coded); got != CodeConfig {
		t.Errorf("ExitCodeFor(coded) = %d, want %d", got, CodeConfig)
	}
}

func TestExitCodeFor_FindsCodedErrorThroughWrapping(t *testing.T) {
	coded := terr.Newf("io_failure", CodeIO, "", "local write failed")
	wrapped := fmt.Errorf("writing state: %w", coded)
	if got := ExitCodeFor(wrapped); got != CodeIO {
		t.Errorf("ExitCodeFor(wrapped) = %d, want %d", got, CodeIO)
	}
}

// The load-bearing invariant: errors.As walks the chain outermost-first, so a
// code assigned at an outer layer overrides any inner class code. run's signal
// override and daemon passthrough both depend on this — they wrap an
// already-coded error to force the process exit status.
func TestExitCodeFor_OutermostCodedErrorWins(t *testing.T) {
	inner := terr.Newf("dnclient_failed", CodeIO, "", "dnclient invocation failed")
	outer := terr.Newf("signal_shutdown", CodeSIGTERM, "", "terminated by signal").Wrap(inner)
	if got := ExitCodeFor(outer); got != CodeSIGTERM {
		t.Errorf("ExitCodeFor(outer) = %d, want %d (the outermost code must win)", got, CodeSIGTERM)
	}
}
