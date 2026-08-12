// Package e2e carries the exec-level suite of docs/specs/e2e-testing.md: it
// builds the real binary and drives it as a process, which is the only way to
// reach behavior the in-process golden harness structurally cannot — process-level
// behavior such as a real termination signal (ADR 0006 exempts 130/143 from the
// in-process suite for exactly this reason, and dt-nrq1 is the regression that
// exemption let through).
//
// It stays hermetic: a fake management API on loopback, and a shell stand-in for
// the proprietary dnclient daemon served as the download install verifies and
// places. Nothing here touches the real defined.net service or /etc/defined.
package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/dninstall"
)

// The fixture identifiers the fake API answers with. They are constants because a
// generated value would only add noise to a failure message.
const (
	fixtureAPIKey    = "test-api-key"
	fixtureHostID    = "host-e2e"
	fixtureHostname  = "e2e-host"
	fixtureNetwork   = "testnet"
	fixtureNetworkID = "net-e2e"
	fixtureRoleID    = "role-e2e"
	fixtureVersion   = "1.2.3"
	binaryPath       = "/dnclient"
)

// toolBinary is the built binary every test execs, built once by TestMain.
var toolBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dn-tool-e2e")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: creating the build dir: %v\n", err)
		os.Exit(1)
	}
	// Sanctioned blank assignment: a failed cleanup of a temp build dir cannot
	// change the suite's result, and there is nothing to do about it here.
	defer func() { _ = os.RemoveAll(dir) }()

	toolBinary = filepath.Join(dir, "dn-tool")
	build := exec.Command("go", "build", "-o", toolBinary, "./cmd/dn-tool")
	build.Dir = ".."
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: building the binary: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// requireSupportedHost skips a test whose command gates on the host platform:
// install — and run, which installs first — rejects anything but linux/amd64 and
// linux/arm64, so on another host the test would pin that rejection instead of the
// behavior it exists for.
func requireSupportedHost(t *testing.T) {
	t.Helper()
	if _, err := dninstall.DetectPlatform(runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skipf("exec-level lifecycle test needs a supported host platform: %v", err)
	}
}

// lifecycle wires the fake API and the dnclient stand-in for a run invocation, and
// returns the environment the binary runs with plus the paths a test polls to know
// the lifecycle reached the daemon.
type lifecycle struct {
	env     []string
	socket  string
	yml     string
	deleted chan struct{}
}

// newLifecycle serves everything `dn-tool run` reaches: the downloads table, the
// stand-in binary and its published checksum, the enrollment create call, and the
// shutdown delete. The stand-in plays both dnclient roles — `run` creates the
// control socket and blocks until signalled, `enroll` writes the dnclient.yml the
// shutdown unenroll reads the host id from.
func newLifecycle(t *testing.T) *lifecycle {
	t.Helper()
	work := t.TempDir()
	configRoot := filepath.Join(work, "config")
	binDir := filepath.Join(work, "bin")
	socket := filepath.Join(work, "dnclient.sock")
	netDir := filepath.Join(configRoot, fixtureNetwork)

	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
run)
	trap 'exit 0' TERM INT
	: > %[1]q
	while :; do sleep 0.05; done
	;;
enroll)
	mkdir -p %[2]q
	printf 'metadata:\n  host_id: %%s\n' %[3]q > %[2]q/dnclient.yml
	;;
esac
exit 0
`, socket, netDir, fixtureHostID)

	lc := &lifecycle{
		socket:  socket,
		yml:     filepath.Join(netDir, "dnclient.yml"),
		deleted: make(chan struct{}, 1),
	}

	// Unstarted, so the download URL the downloads table advertises is known
	// before the handler that serves it can run: assigning it after Start would be
	// a data race with the first request.
	srv := httptest.NewUnstartedServer(nil)
	binURL := "http://" + srv.Listener.Addr().String() + binaryPath
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/downloads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(downloadsJSON(t, binURL)))
		case r.URL.Path == binaryPath:
			_, _ = w.Write([]byte(script))
		case r.URL.Path == binaryPath+".sha256":
			_, _ = w.Write([]byte(digestOf(script)))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[],"metadata":{"hasNextPage":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/host-and-enrollment-code":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w,
				`{"data":{"host":{"id":%q,"name":%q},"enrollmentCode":{"code":"ENROLL-CODE","lifetimeSeconds":300}}}`,
				fixtureHostID, fixtureHostname)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/hosts/"+fixtureHostID:
			select {
			case lc.deleted <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv.Start()
	t.Cleanup(srv.Close)

	lc.env = []string{
		"PATH=" + os.Getenv("PATH"),
		"DN_API_URL=" + srv.URL,
		"DN_API_KEY=" + fixtureAPIKey,
		"DN_NETWORK_ID=" + fixtureNetworkID,
		"DN_ROLE_ID=" + fixtureRoleID,
		"DN_NETWORK_NAME=" + fixtureNetwork,
		"DN_HOSTNAME=" + fixtureHostname,
		"DN_CLIENT_CONFIG_DIR=" + configRoot,
		"DN_CLIENT_BIN_DIR=" + binDir,
		"DN_CLIENT_VERSION=" + fixtureVersion,
		"DN_CLIENT_SOCKET=" + socket,
	}
	return lc
}

// downloadsJSON resolves the host's own os-arch to binURL at fixtureVersion, which
// it also designates latest.
func downloadsJSON(t *testing.T, binURL string) string {
	t.Helper()
	platform, err := dninstall.DetectPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("resolving the host platform: %v", err)
	}
	key := platform.OS + "-" + platform.Arch
	return fmt.Sprintf(`{"data":{"dnclient":{%q:{%q:%q}},"versionInfo":{"latest":{"dnclient":%q}}}}`,
		fixtureVersion, key, binURL, fixtureVersion)
}

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// waitForFile polls until path exists, so a broken fixture fails the test instead
// of hanging it.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never appeared; the lifecycle did not get that far", path)
}
