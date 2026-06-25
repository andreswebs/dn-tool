package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnstate"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/unenroll"
)

// writeDNClientConfig creates <root>/<network>/dnclient.yml carrying
// metadata.host_id and returns the network directory. It mirrors the on-disk
// layout unenroll reads (dnclient nests host_id under metadata).
func writeDNClientConfig(t *testing.T, root, network, hostID string) string {
	t.Helper()
	dir := filepath.Join(root, network)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "dnclient.yml")
	if err := os.WriteFile(path, []byte("metadata:\n  host_id: "+hostID+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return dir
}

// The wired command propagates the unenroll module's API-key precondition: a
// missing key fails clearly (with unenroll.ErrMissingAPIKey) without touching the
// filesystem or network (design Req 4 / §2.3). The check itself is the module's
// responsibility now (tested in internal/unenroll); this pins the wiring.
func TestRunUnenroll_MissingAPIKey(t *testing.T) {
	cfg := &config.Config{NetworkName: "defined"}
	if _, err := runUnenroll(context.Background(), cfg, t.TempDir()); !errors.Is(err, unenroll.ErrMissingAPIKey) {
		t.Fatalf("err = %v, want unenroll.ErrMissingAPIKey", err)
	}
}

// No local config => fail clearly as not-enrolled; the remote API is never
// contacted because the host_id read fails first.
func TestRunUnenroll_NotEnrolled(t *testing.T) {
	cfg := &config.Config{APIKey: "k", NetworkName: "defined", APIURL: "http://127.0.0.1:0"}
	if _, err := runUnenroll(context.Background(), cfg, t.TempDir()); !errors.Is(err, dnstate.ErrNotEnrolled) {
		t.Fatalf("err = %v, want ErrNotEnrolled", err)
	}
}

// Happy path: DELETE the remote record by its host_id, then remove the local
// network config dir (but not the config root), and report a changed result.
func TestRunUnenroll_DeletesThenRemovesLocal(t *testing.T) {
	const hostID = "host-abc"
	root := t.TempDir()
	netDir := writeDNClientConfig(t, root, "testnet", hostID)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := &config.Config{APIKey: "k", NetworkName: "testnet", APIURL: srv.URL}
	res, err := runUnenroll(context.Background(), cfg, root)
	if err != nil {
		t.Fatalf("runUnenroll: %v", err)
	}

	if gotMethod != http.MethodDelete || gotPath != "/v1/hosts/"+hostID {
		t.Errorf("request = %s %s, want DELETE /v1/hosts/%s", gotMethod, gotPath, hostID)
	}
	want := output.Result{Action: "unenroll", Changed: true, HostID: hostID, Network: "testnet"}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	if _, statErr := os.Stat(netDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("network dir still present after unenroll: stat err = %v", statErr)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Errorf("config root wrongly removed: %v", statErr)
	}
}

// Invariant §2.5: a delete failure (non-2xx, non-404) retains the local config —
// never an orphan — and exits non-zero (code 1, never the assert-changed code 2).
// A 403 is used so retryablehttp does not retry (unlike 5xx/429), keeping the
// test fast while still exercising the failure branch.
func TestRunUnenroll_DeleteFailureRetainsLocal(t *testing.T) {
	const hostID = "host-xyz"
	root := t.TempDir()
	netDir := writeDNClientConfig(t, root, "testnet", hostID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cfg := &config.Config{APIKey: "k", NetworkName: "testnet", APIURL: srv.URL}
	_, err := runUnenroll(context.Background(), cfg, root)
	if err == nil {
		t.Fatal("expected error on delete failure, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(netDir, "dnclient.yml")); statErr != nil {
		t.Errorf("local config wrongly removed on delete failure: %v", statErr)
	}
	if got := output.ResolveExitCode(err); got != output.CodeError {
		t.Errorf("ResolveExitCode = %d, want %d (never code 2 for a failure)", got, output.CodeError)
	}
}

// The unenroll command honors DN_CLIENT_CONFIG_DIR: it reads the host_id and
// removes the local config under the overridden root, not the hardcoded
// /etc/defined. This is what makes binary-level unenroll testable without root
// or a container. Driving the real app exercises loadConfig ->
// cfg.ClientConfigDir -> runUnenroll end to end.
func TestUnenrollCommand_HonorsConfigRootOverride(t *testing.T) {
	const hostID = "host-override"
	root := t.TempDir()
	netDir := writeDNClientConfig(t, root, "testnet", hostID)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("DN_API_KEY", "k")
	t.Setenv("DN_NETWORK_NAME", "testnet")
	t.Setenv("DN_API_URL", srv.URL)
	t.Setenv("DN_CLIENT_CONFIG_DIR", root)

	app := newApp()
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}
	if err := app.Run(context.Background(), []string{"dn-tool", "unenroll"}); err != nil {
		t.Fatalf("unenroll: %v", err)
	}

	if gotPath != "/v1/hosts/"+hostID {
		t.Errorf("DELETE path = %q, want /v1/hosts/%s", gotPath, hostID)
	}
	if _, statErr := os.Stat(netDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("network dir under override root still present: stat err = %v", statErr)
	}
}
