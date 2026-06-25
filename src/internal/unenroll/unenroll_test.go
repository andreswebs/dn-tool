package unenroll

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/dnstate"
	"github.com/andreswebs/dn-tool/internal/output"
)

// fakeDeleter scripts api.DeleteHost: it records the host ID it was called with
// and returns the configured error (nil models both a 2xx and the api layer's
// idempotent 404→nil mapping).
type fakeDeleter struct {
	err    error
	called bool
	gotID  string
}

func (f *fakeDeleter) DeleteHost(ctx context.Context, hostID string) error {
	f.called = true
	f.gotID = hostID
	if err := ctx.Err(); err != nil {
		return err // a real ctx-bounded client surfaces the deadline/cancel error
	}
	return f.err
}

// enrolledRoot writes a fixture dnclient.yml carrying metadata.host_id under
// <root>/<network>/ (the shape dnclient writes) and returns the temp root.
func enrolledRoot(t *testing.T, network, hostID string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, network)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yml := "metadata:\n  network_id: net-1\n  host_id: " + hostID + "\n"
	if err := os.WriteFile(filepath.Join(dir, "dnclient.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return root
}

func TestUnenrollDeleteSuccessRemovesLocal(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-abc")
	del := &fakeDeleter{err: nil}

	res, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !del.called || del.gotID != "host-abc" {
		t.Errorf("DeleteHost called=%v id=%q, want called with host-abc", del.called, del.gotID)
	}
	if res.Action != "unenroll" {
		t.Errorf("Action = %q, want unenroll", res.Action)
	}
	if !res.Changed {
		t.Errorf("Changed = false, want true")
	}
	if _, err := os.Stat(filepath.Join(root, "defined")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("local config dir still present after unenroll: %v", err)
	}
}

// A 404 already-absent remote is mapped to nil by api.DeleteHost, so from this
// layer it is indistinguishable from a 2xx: local config is still removed and no
// error surfaces.
func TestUnenrollDelete404TreatedAsSuccess(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-gone")
	del := &fakeDeleter{err: nil}

	res, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Changed {
		t.Errorf("Changed = false, want true")
	}
	if _, err := os.Stat(filepath.Join(root, "defined")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("local config dir still present: %v", err)
	}
}

// The module owns the API-key precondition: a missing key fails with
// ErrMissingAPIKey before any local read or remote call (design Req 4 ordering).
func TestUnenroll_MissingAPIKey(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-abc") // enrolled, so only the key gates
	del := &fakeDeleter{}

	_, err := Unenroll(context.Background(), Input{NetworkName: "defined"}, Deps{API: del, ConfigRoot: root})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey", err)
	}
	if del.called {
		t.Error("DeleteHost called despite missing API key; the key must gate before any remote call")
	}
}

func TestUnenrollNotEnrolledFailsClearly(t *testing.T) {
	root := t.TempDir() // no <root>/defined/dnclient.yml
	del := &fakeDeleter{err: nil}

	_, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, dnstate.ErrNotEnrolled) {
		t.Errorf("error %v, want errors.Is ErrNotEnrolled", err)
	}
	if del.called {
		t.Errorf("DeleteHost should not be called when not enrolled")
	}
}

// On a delete failure the local config is retained: removal is gated behind a
// successful (or idempotent-404) delete, so the local/remote invariant holds
// (the failure messaging itself is dt-f7nx).
func TestUnenrollDeleteFailureRetainsLocal(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-x")
	del := &fakeDeleter{err: errors.New("boom")}

	_, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(root, "defined")); statErr != nil {
		t.Errorf("local config dir removed despite delete failure: %v", statErr)
	}
}

// Behavior 1: a server-side (5xx-class) delete failure must surface as a
// non-zero process exit. The package returns a plain error, which the exit-code
// layer resolves to CodeError (1) — never CodeOK or the assert-changed code 2.
func TestUnenrollDeleteFailureExitsNonZero(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-5xx")
	del := &fakeDeleter{err: errors.New("500 server error")}

	_, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if code := output.ResolveExitCode(err); code == output.CodeOK {
		t.Errorf("ResolveExitCode = %d, want non-zero", code)
	}
	if code := output.ResolveExitCode(err); code == output.CodeNoOpAssert {
		t.Errorf("ResolveExitCode = %d (assert-changed), want a failure code", code)
	}
}

// Behavior 2: when the context deadline trips during the delete, the local
// config is retained and the deadline error is surfaced (recoverable via
// errors.Is) — the invariant holds exactly as for any other delete failure.
func TestUnenrollDeadlineExceededRetainsLocal(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-slow")
	del := &fakeDeleter{} // err nil; the cancelled ctx drives the failure

	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()

	_, err := Unenroll(ctx, Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v, want errors.Is context.DeadlineExceeded", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "defined")); statErr != nil {
		t.Errorf("local config dir removed despite deadline failure: %v", statErr)
	}
}

// Behavior 3: the delete-failure error states the §2.5 consequence plainly so
// the operator is not surprised — the remote record may persist, the local
// config is retained, and the host remains enrolled and will resume on boot.
func TestUnenrollDeleteFailureMessageIsHonest(t *testing.T) {
	root := enrolledRoot(t, "defined", "host-x")
	del := &fakeDeleter{err: errors.New("boom")}

	_, err := Unenroll(context.Background(), Input{NetworkName: "defined", APIKey: "k"}, Deps{API: del, ConfigRoot: root})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"may persist",
		"local config retained",
		"remains enrolled",
		"next boot",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("failure message %q missing honest phrase %q", msg, want)
		}
	}
}
