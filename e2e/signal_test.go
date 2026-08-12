package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// dt-nrq1, the case only a real process can make: a container stop is a graceful
// shutdown, not a failure. `dn-tool run` must exit 128+signum and report no error
// envelope on stderr, so a supervisor and an agent both read a clean stop as a
// clean stop.
//
// This is the layer that owns the assertion: the in-process golden harness cannot
// deliver a signal, which is why the ok:false envelope for a graceful stop reached
// real output unnoticed.
func TestRun_TerminationSignalExitsSignalCodeWithoutAnErrorEnvelope(t *testing.T) {
	tests := []struct {
		name string
		sig  syscall.Signal
		want int
	}{
		{"SIGTERM", syscall.SIGTERM, 143},
		{"SIGINT", syscall.SIGINT, 130},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireSupportedHost(t)
			lc := newLifecycle(t)

			var stdout, stderr bytes.Buffer
			cmd := exec.Command(toolBinary, "run")
			cmd.Env = lc.env
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatalf("starting %s run: %v", toolBinary, err)
			}

			// The signal arrives only once the host is actually running: the
			// daemon created its control socket and enroll wrote the local config.
			waitForFile(t, lc.socket)
			waitForFile(t, lc.yml)
			if err := cmd.Process.Signal(tt.sig); err != nil {
				t.Fatalf("signalling %v: %v", tt.sig, err)
			}

			if got := waitExitCode(t, cmd); got != tt.want {
				t.Errorf("exit code = %d, want %d (128 + signum after cleanup)\nstderr: %s", got, tt.want, stderr.String())
			}
			assertNoErrorEnvelope(t, stderr.String())
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing (run emits no result envelope)", stdout.String())
			}

			// The shutdown unenroll really ran: the graceful stop is what the
			// signal code reports, and it is only graceful if the host was removed.
			select {
			case <-lc.deleted:
			case <-time.After(time.Second):
				t.Error("the shutdown unenroll never deleted the remote host record")
			}
		})
	}
}

// waitExitCode waits for the process and reports its exit status, failing the test
// on any other kind of wait error.
func waitExitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("waiting for the process: %v", err)
	}
	return exitErr.ExitCode()
}

// assertNoErrorEnvelope fails if any line of stderr is an error envelope. Log
// records and the daemon stand-in's own output are permitted: the contract is that
// a graceful shutdown reports no failure, not that it stays silent.
func assertNoErrorEnvelope(t *testing.T, stderr string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(stderr, "\n"), "\n") {
		var env struct {
			OK    *bool     `json:"ok"`
			Error *struct{} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &env) != nil {
			continue
		}
		if env.Error != nil {
			t.Errorf("stderr carries an error envelope for a graceful shutdown: %q", line)
		}
		if env.OK != nil && !*env.OK {
			t.Errorf("stderr reports ok:false for a graceful shutdown: %q", line)
		}
	}
}
