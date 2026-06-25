package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
)

// The run command reuses the unenroll core verbatim: the Unenroll lifecycle step
// must perform the real remote DELETE and local-config removal. Driving the
// closure productionRunDeps builds (against an httptest server + temp config
// root) proves the wiring without the real dnclient binary or network.
func TestProductionRunDeps_UnenrollClosureUnenrolls(t *testing.T) {
	const hostID = "host-run-1"
	root := t.TempDir()
	netDir := writeDNClientConfig(t, root, "testnet", hostID)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := &config.Config{APIKey: "k", NetworkName: "testnet", APIURL: srv.URL}
	deps := productionRunDeps(cfg, root)

	res, err := deps.Unenroll(context.Background())
	if err != nil {
		t.Fatalf("Unenroll closure: %v", err)
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
}

// The run command reuses the enroll core: with a local dnclient config already
// present, the Enroll lifecycle step is the §2.4 row-1 no-op — Changed=false and
// no remote call. Pointing APIURL at an unroutable address proves the closure
// makes no network call on the no-op path.
func TestProductionRunDeps_EnrollClosureNoOpWhenLocallyEnrolled(t *testing.T) {
	root := t.TempDir()
	writeDNClientConfig(t, root, "testnet", "host-x")

	cfg := &config.Config{APIKey: "k", NetworkName: "testnet", APIURL: "http://127.0.0.1:0"}
	deps := productionRunDeps(cfg, root)

	res, err := deps.Enroll(context.Background())
	if err != nil {
		t.Fatalf("Enroll closure: %v", err)
	}
	if res.Action != "enroll" || res.Changed {
		t.Errorf("result = %+v, want a no-op enroll (Changed=false)", res)
	}
}

// Every lifecycle step must be populated: a nil step closure or daemon would
// nil-panic inside run.Lifecycle. UnenrollTimeout must mirror the unenroll
// command's bound so a configured DN_API_TIMEOUT is honored on shutdown.
func TestProductionRunDeps_AllStepsWired(t *testing.T) {
	cfg := &config.Config{ClientBinDir: "/opt/dn/bin", APITimeout: 7 * time.Second}
	deps := productionRunDeps(cfg, "/etc/defined")

	if deps.Install == nil || deps.Enroll == nil || deps.Unenroll == nil {
		t.Fatal("a lifecycle step closure is nil; run.Lifecycle would nil-panic")
	}
	if deps.Daemon == nil {
		t.Fatal("Daemon is nil; run.Lifecycle would nil-panic running the foreground daemon")
	}
	if deps.WaitReady == nil {
		t.Fatal("WaitReady is nil; run.Lifecycle would nil-panic waiting for the daemon socket")
	}
	if deps.UnenrollTimeout != cfg.Timeout(defaultUnenrollTimeout) {
		t.Errorf("UnenrollTimeout = %v, want %v (DN_API_TIMEOUT must not be capped)", deps.UnenrollTimeout, cfg.Timeout(defaultUnenrollTimeout))
	}
}
