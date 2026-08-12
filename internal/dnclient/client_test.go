package dnclient

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/logctx"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// writeFakeBinary writes an executable shell script to a temp dir and returns
// its path, so tests can drive the real execClient against a stand-in for the
// proprietary dnclient binary. body is the script after the `#!/bin/sh` line.
func writeFakeBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dnclient")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}
	return path
}

func TestExecClientEnrollPassesArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeFakeBinary(t, `printf '%s\n' "$@" > `+argsFile+`
`)

	c := NewExecClient(bin)
	if err := c.Enroll(context.Background(), "corpnet", "code-xyz"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading recorded args: %v", err)
	}
	want := "enroll\n-name\ncorpnet\n-code\ncode-xyz\n"
	if string(got) != want {
		t.Errorf("subprocess args = %q, want %q", got, want)
	}
}

func TestExecClientNonZeroExitSurfaced(t *testing.T) {
	bin := writeFakeBinary(t, "exit 7\n")

	c := NewExecClient(bin)
	err := c.Enroll(context.Background(), "defined", "code-xyz")
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "exit status 7") {
		t.Errorf("error %q does not surface the exit status", err.Error())
	}
}

func TestExecClientContextCancellationKillsSubprocess(t *testing.T) {
	bin := writeFakeBinary(t, "exec sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := NewExecClient(bin)
	start := time.Now()
	err := c.Run(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after context cancellation, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Run took %v; subprocess was not killed on cancellation", elapsed)
	}
}

// debugContext returns a context carrying a debug-level JSON logger and the
// buffer it writes to, so a test reads the diagnostics of the code under test
// without touching slog's process-global default (ADR 0004).
func debugContext() (context.Context, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logctx.With(context.Background(), logger), &buf
}

func TestExecClientNeverLogsCode(t *testing.T) {
	const code = "super-secret-enrollment-code"

	t.Run("success path", func(t *testing.T) {
		ctx, buf := debugContext()
		bin := writeFakeBinary(t, "exit 0\n")

		if err := NewExecClient(bin).Enroll(ctx, "defined", code); err != nil {
			t.Fatalf("Enroll: %v", err)
		}
		if strings.Contains(buf.String(), code) {
			t.Errorf("logs leaked the enrollment code: %s", buf.String())
		}
	})

	t.Run("failure path", func(t *testing.T) {
		ctx, buf := debugContext()
		bin := writeFakeBinary(t, "exit 1\n")

		err := NewExecClient(bin).Enroll(ctx, "defined", code)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), code) {
			t.Errorf("error leaked the enrollment code: %v", err)
		}
		if strings.Contains(buf.String(), code) {
			t.Errorf("logs leaked the enrollment code: %s", buf.String())
		}
	})
}

// dt-2efx: a dnclient child that ran and exited non-zero is ErrDNClientFailed
// (exit 74) and its status travels as structured detail. Before this, the
// *exec.ExitError's own ExitCode() satisfied the boundary's ExitCoder interface,
// so enroll silently exited with the child's status — a code no command declares
// — and the "dnclient enroll" context was dropped from the printed message.
func TestExecClientNonZeroExitIsCodedWithExitStatusDetail(t *testing.T) {
	bin := writeFakeBinary(t, "exit 3\n")

	err := NewExecClient(bin).Enroll(context.Background(), "defined", "code-xyz")
	if !errors.Is(err, ErrDNClientFailed) {
		t.Fatalf("err = %v, want ErrDNClientFailed", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeIO {
		t.Errorf("exit code = %d, want %d (the child's status must not leak)", got, output.CodeIO)
	}
	if !strings.Contains(err.Error(), "enroll") {
		t.Errorf("error %q dropped the action context", err.Error())
	}

	var detailed terr.Detailed
	if !errors.As(err, &detailed) {
		t.Fatal("a child that ran must carry its status as structured detail")
	}
	details, ok := detailed.ErrorDetails().(map[string]any)
	if !ok {
		t.Fatalf("ErrorDetails() = %T, want map[string]any", detailed.ErrorDetails())
	}
	if details["exit_status"] != 3 {
		t.Errorf("details[exit_status] = %v, want 3", details["exit_status"])
	}

	// The *exec.ExitError stays in the chain, which is how run maps the daemon's
	// status N onto its own exit code.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Error("the *exec.ExitError must stay reachable for run's passthrough mapping")
	}
}

// A subprocess that never started (binary missing) is the same class, with no
// exit status to report.
func TestExecClientStartFailureIsCodedWithoutDetail(t *testing.T) {
	err := NewExecClient(filepath.Join(t.TempDir(), "absent")).Enroll(context.Background(), "defined", "code")
	if !errors.Is(err, ErrDNClientFailed) {
		t.Fatalf("err = %v, want ErrDNClientFailed", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeIO {
		t.Errorf("exit code = %d, want %d", got, output.CodeIO)
	}
	var detailed terr.Detailed
	if errors.As(err, &detailed) && detailed.ErrorDetails() != nil {
		t.Errorf("details = %v, want none for a child that never ran", detailed.ErrorDetails())
	}
}

// dt-cr62: removing slog.SetDefault must not silence this diagnostic; it is the
// only observability into which dnclient invocation was attempted. It travels on
// the context now, and the argument list (which may carry the enrollment code)
// still stays out of it.
func TestExecClient_LogsTheInvocationThroughTheContextLogger(t *testing.T) {
	ctx, buf := debugContext()
	bin := writeFakeBinary(t, "exit 0\n")

	if err := NewExecClient(bin).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logs := buf.String()
	for _, want := range []string{"running dnclient", `"action":"run"`, bin} {
		if !strings.Contains(logs, want) {
			t.Errorf("diagnostics %q missing %q", logs, want)
		}
	}
}
