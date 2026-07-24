package run

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/andreswebs/dn-tool/internal/output"
)

// loadedSigCh returns a channel already carrying sig, modelling a signal that
// NotifyContext caught and Lifecycle's capture recorded before finalize runs.
func loadedSigCh(sig os.Signal) chan os.Signal {
	ch := make(chan os.Signal, 1)
	ch <- sig
	return ch
}

// ADR 0001 signal rule: a graceful shutdown on SIGINT/SIGTERM exits 128+signum
// after cleanup. A clean cleanup (nil) is given the signal code via the sentinel.
func TestFinalize_CleanSignalShutdownExitsSignalCode(t *testing.T) {
	cases := []struct {
		name string
		sig  os.Signal
		want int
	}{
		{"SIGINT", syscall.SIGINT, output.CodeSIGINT},
		{"SIGTERM", syscall.SIGTERM, output.CodeSIGTERM},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := finalize(nil, loadedSigCh(tt.sig))
			if got := output.ResolveExitCode(err); got != tt.want {
				t.Fatalf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

// The signal code overrides an unenroll failure that occurred during the same
// shutdown: ADR 0001 says the signal code wins over the interrupted work's code.
func TestFinalize_SignalCodeOverridesUnenrollFailure(t *testing.T) {
	unenrollErr := output.Classify(errors.New("unenroll boom"), output.CodeIO)
	err := finalize(unenrollErr, loadedSigCh(syscall.SIGTERM))
	if got := output.ResolveExitCode(err); got != output.CodeSIGTERM {
		t.Fatalf("exit code = %d, want %d (signal code must override the cleanup code)", got, output.CodeSIGTERM)
	}
	// The underlying failure stays in the chain for diagnostics.
	if !errors.Is(err, unenrollErr) {
		t.Error("finalize dropped the underlying unenroll error from the chain")
	}
}

// No signal fired (the daemon terminated on its own): finalize returns the
// cleanup result verbatim, so the daemon's passthrough or class code stands.
func TestFinalize_NoSignalReturnsCleanupResultUnchanged(t *testing.T) {
	empty := make(chan os.Signal, 1)

	if err := finalize(nil, empty); err != nil {
		t.Fatalf("clean daemon exit with no signal: err = %v, want nil", err)
	}

	daemonErr := output.ExitError(errors.New("daemon exited 7"), 7)
	if got := output.ResolveExitCode(finalize(daemonErr, empty)); got != 7 {
		t.Fatalf("exit code = %d, want 7 (daemon passthrough must survive)", got)
	}
}

func TestSignalExitCode(t *testing.T) {
	if got := signalExitCode(syscall.SIGINT); got != output.CodeSIGINT {
		t.Errorf("SIGINT -> %d, want %d", got, output.CodeSIGINT)
	}
	if got := signalExitCode(syscall.SIGTERM); got != output.CodeSIGTERM {
		t.Errorf("SIGTERM -> %d, want %d", got, output.CodeSIGTERM)
	}
}
