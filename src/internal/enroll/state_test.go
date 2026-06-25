package enroll

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/output"
)

// failingAPI satisfies the API seam but fails the test if any method is called.
// The local-config-present no-op (§2.4 row 1) must make zero API calls, so this
// mock proves the absence of remote traffic.
type failingAPI struct{ t *testing.T }

func (f failingAPI) ListHosts(_ context.Context, _ string) ([]api.Host, error) {
	f.t.Fatalf("ListHosts called: no-op cell must make zero API calls")
	return nil, nil
}

func (f failingAPI) CreateHostAndEnrollmentCode(_ context.Context, _ api.CreateHostRequest) (*api.HostAndCode, error) {
	f.t.Fatalf("CreateHostAndEnrollmentCode called: no-op cell must make zero API calls")
	return nil, nil
}

func (f failingAPI) DeleteHost(_ context.Context, _ string) error {
	f.t.Fatalf("DeleteHost called: no-op cell must make zero API calls")
	return nil
}

func writeLocalConfig(t *testing.T, root, network string) {
	t.Helper()
	dir := filepath.Join(root, network)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dnclient.yml"), []byte("host_id: host-1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// Behavior 1: local config present → idempotent no-op. Result reports the enroll
// action with Changed=false, no error, and not a single API call is made.
func TestEnrollLocalConfigPresentNoOp(t *testing.T) {
	root := t.TempDir()
	writeLocalConfig(t, root, "defined")

	cfg := validConfig()
	cfg.NetworkName = "defined"

	deps := Deps{API: failingAPI{t: t}, ConfigRoot: root}

	got, err := Enroll(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("Enroll returned error for already-enrolled host: %v", err)
	}

	want := output.Result{Action: "enroll", Changed: false}
	if got != want {
		t.Errorf("Enroll result = %+v, want %+v", got, want)
	}
}

// Behavior 2: the no-op result flows to the documented exit codes — 0 normally,
// and the distinct non-error code 2 under --assert-changed (EXIT.assert). The
// command wrapper maps Changed=false to those codes; assert the contract holds
// for the value Enroll produces.
func TestEnrollNoOpExitSemantics(t *testing.T) {
	root := t.TempDir()
	writeLocalConfig(t, root, "defined")

	cfg := validConfig()
	cfg.NetworkName = "defined"

	res, err := Enroll(context.Background(), cfg, Deps{API: failingAPI{t: t}, ConfigRoot: root})
	if err != nil {
		t.Fatalf("Enroll returned error: %v", err)
	}
	if res.Changed {
		t.Fatalf("no-op result must report Changed=false")
	}

	// Without --assert-changed: success, exit 0.
	if code := output.ResolveExitCode(nil); code != output.CodeOK {
		t.Errorf("no-op exit code = %d, want %d", code, output.CodeOK)
	}

	// With --assert-changed: a no-op yields the distinct, non-error code 2.
	assertErr := output.ExitError(errors.New("no change made"), output.CodeNoOpAssert)
	if code := output.ResolveExitCode(assertErr); code != output.CodeNoOpAssert {
		t.Errorf("no-op --assert-changed exit code = %d, want %d", code, output.CodeNoOpAssert)
	}
}
