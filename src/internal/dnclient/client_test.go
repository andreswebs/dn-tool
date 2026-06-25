package dnclient

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestExecClientNeverLogsCode(t *testing.T) {
	const code = "super-secret-enrollment-code"

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	t.Run("success path", func(t *testing.T) {
		buf.Reset()
		bin := writeFakeBinary(t, "exit 0\n")

		if err := NewExecClient(bin).Enroll(context.Background(), "defined", code); err != nil {
			t.Fatalf("Enroll: %v", err)
		}
		if strings.Contains(buf.String(), code) {
			t.Errorf("logs leaked the enrollment code: %s", buf.String())
		}
	})

	t.Run("failure path", func(t *testing.T) {
		buf.Reset()
		bin := writeFakeBinary(t, "exit 1\n")

		err := NewExecClient(bin).Enroll(context.Background(), "defined", code)
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
