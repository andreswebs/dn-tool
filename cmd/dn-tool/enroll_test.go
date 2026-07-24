package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/output"
)

// writeFakeDNClient writes an executable stand-in for the dnclient binary at
// <binDir>/dnclient that appends its arguments to argLog and exits 0, so the
// create path can run `dnclient enroll` without the proprietary binary.
func writeFakeDNClient(t *testing.T, binDir, argLog string) {
	t.Helper()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", binDir, err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argLog + "\nexit 0\n"
	if err := os.WriteFile(dnclient.BinaryPath(binDir), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake dnclient: %v", err)
	}
}

// Row 1 of the §2.4 state machine: a local dnclient config makes enroll a no-op.
// It contacts neither the management API (the test server fails on any hit) nor
// the dnclient binary, and reports Changed=false without requiring DN_API_KEY.
func TestRunEnroll_AlreadyEnrolled_NoOp(t *testing.T) {
	root := t.TempDir()
	writeDNClientConfig(t, root, "testnet", "host-existing")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("management API contacted on the already-enrolled no-op path")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := &config.Config{NetworkName: "testnet", APIURL: srv.URL}
	res, err := runEnroll(context.Background(), cfg, root, false)
	if err != nil {
		t.Fatalf("runEnroll: %v", err)
	}
	want := output.Result{Action: "enroll", Changed: false}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
}

// The create cell (local absent, remote absent): list returns no match, create
// yields a host + enrollment code, and the wired dnclient binary runs `enroll`.
// Exercises the full command wiring (API client + exec client at the configured
// bin dir) and the changed result.
func TestRunEnroll_CreatePath(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	argLog := filepath.Join(t.TempDir(), "args.log")
	writeFakeDNClient(t, binDir, argLog)

	var listed, created bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/hosts":
			listed = true
			_, _ = w.Write([]byte(`{"data":[],"metadata":{"hasNextPage":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/host-and-enrollment-code":
			created = true
			_, _ = w.Write([]byte(`{"data":{"host":{"id":"host-new","name":"node1"},"enrollmentCode":{"code":"SECRET-CODE","lifetimeSeconds":300}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		APIKey:       "k",
		NetworkID:    "net-1",
		RoleID:       "role-1",
		NetworkName:  "testnet",
		Hostname:     "node1",
		APIURL:       srv.URL,
		ClientBinDir: binDir,
	}
	res, err := runEnroll(context.Background(), cfg, root, false)
	if err != nil {
		t.Fatalf("runEnroll: %v", err)
	}

	if !listed || !created {
		t.Errorf("API calls: listed=%v created=%v, want both true", listed, created)
	}
	want := output.Result{Action: "enroll", Changed: true, HostID: "host-new", Network: "testnet"}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}

	logged, err := os.ReadFile(argLog)
	if err != nil {
		t.Fatalf("dnclient binary was not invoked: %v", err)
	}
	if got := string(logged); got != "enroll -name testnet -code SECRET-CODE\n" {
		t.Errorf("dnclient args = %q, want enroll -name testnet -code SECRET-CODE", got)
	}
}
