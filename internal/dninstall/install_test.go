package dninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
)

// fakeDownloader returns a canned downloads table, standing in for *api.Client.
type fakeDownloader struct {
	downloads *api.Downloads
	err       error
}

func (f fakeDownloader) ListDownloads(_ context.Context) (*api.Downloads, error) {
	return f.downloads, f.err
}

// downloadsFor builds a downloads table that resolves the linux-amd64 binary of
// r.Version to r.URL and designates that version latest, so resolveDownload
// returns r for Platform{linux, amd64} and either an explicit or "latest" version.
func downloadsFor(r resolved) *api.Downloads {
	return &api.Downloads{
		DNClient: map[string]map[string]string{
			r.Version: {"linux-amd64": r.URL},
		},
		VersionInfo: api.VersionInfo{Latest: api.LatestVersions{DNClient: r.Version}},
	}
}

// linuxAMD64 is the resolved host platform the install tests run against,
// independent of the test runner's real architecture.
var linuxAMD64 = Platform{OS: "linux", Arch: "amd64"}

// assertNoTempFiles fails if any leftover dnclient.tmp-* file remains in binDir,
// proving placeBinary cleaned up (no partial-write window).
func assertNoTempFiles(t *testing.T, binDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(binDir, binaryName+".tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("leftover temp files: %v", matches)
	}
}

// digestOf returns the lowercase-hex SHA-256 of content — the published-digest
// value a caller would hand to needsInstall.
func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// writeBinaryFixture writes content to path (creating parents) as a 0o755 file,
// standing in for an installed dnclient binary.
func writeBinaryFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// Behavior 1: no file at the target path → install needed.
func TestNeedsInstall_MissingFileNeedsInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnclient")
	need, _, err := needsInstall(path, digestOf("anything"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !need {
		t.Error("missing binary: need = false, want true")
	}
}

// Behavior 2: a present binary whose digest matches → skip (no install, changed=false).
func TestNeedsInstall_MatchingDigestSkips(t *testing.T) {
	const content = "dnclient-v1-binary-bytes"
	path := filepath.Join(t.TempDir(), "dnclient")
	writeBinaryFixture(t, path, content)

	need, reason, err := needsInstall(path, digestOf(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if need {
		t.Errorf("matching binary: need = true (%s), want false", reason)
	}
}

// Behavior 3: a present binary whose digest differs → re-download.
func TestNeedsInstall_MismatchNeedsReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnclient")
	writeBinaryFixture(t, path, "stale-binary")

	need, _, err := needsInstall(path, digestOf("the-wanted-binary"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !need {
		t.Error("mismatched binary: need = false, want true")
	}
}

// Behavior 4: a target that cannot be read as a file surfaces a clear error
// rather than silently skipping. A directory at the path is a deterministic,
// uid-independent stand-in for an unreadable/locked target (reading it yields a
// non-ENOENT I/O error); need must be false so the error cannot read as
// "reinstall".
func TestNeedsInstall_UnreadableTargetErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnclient")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	need, _, err := needsInstall(path, digestOf("x"))
	if err == nil {
		t.Fatal("unreadable target: err = nil, want a clear error")
	}
	if need {
		t.Error("unreadable target: need = true, want false")
	}
}

// Digest comparison is case- and whitespace-insensitive on the expected value,
// so a published checksum with surrounding newline or upper-case hex still
// matches the streamed lowercase digest.
func TestNeedsInstall_NormalizesExpectedDigest(t *testing.T) {
	const content = "dnclient-binary"
	path := filepath.Join(t.TempDir(), "dnclient")
	writeBinaryFixture(t, path, content)

	noisy := "  " + strings.ToUpper(digestOf(content)) + "\n"
	need, _, err := needsInstall(path, noisy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if need {
		t.Error("normalized matching digest: need = true, want false")
	}
}

// installFixture wires a verifyServer + fakeDownloader for an Install run and
// returns the deps, the target bin dir, and the served binary contents.
func installFixture(t *testing.T, srv verifyServer) (InstallDeps, string) {
	t.Helper()
	r, stop := srv.start(t)
	t.Cleanup(stop)
	return InstallDeps{
		API:        fakeDownloader{downloads: downloadsFor(r)},
		HTTPClient: http.DefaultClient,
		Platform:   linuxAMD64,
	}, t.TempDir()
}

// Behavior 1: a fresh install places an executable binary and reports changed.
func TestInstall_FreshInstallPlacesBinary(t *testing.T) {
	binary := []byte("fresh dnclient binary")
	deps, binDir := installFixture(t, verifyServer{binary: binary})

	res, err := Install(context.Background(), deps, InstallOptions{BinaryPath: filepath.Join(binDir, binaryName)})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Changed || res.Action != "install" {
		t.Errorf("result = %+v, want {install true}", res)
	}

	binPath := filepath.Join(binDir, binaryName)
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("reading placed binary: %v", err)
	}
	if string(got) != string(binary) {
		t.Errorf("placed contents = %q, want %q", got, binary)
	}
	fi, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat placed binary: %v", err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Errorf("placed binary mode = %v, want executable", fi.Mode())
	}
	assertNoTempFiles(t, binDir)
}

// Behavior 2: a matching binary already present → skip, changed=false, and the
// binary endpoint is never hit (it is forced to 500, which a wrongful download
// would surface as an error).
func TestInstall_IdempotentSkip(t *testing.T) {
	binary := []byte("already-installed dnclient")
	deps, binDir := installFixture(t, verifyServer{binary: binary, binaryStatus: http.StatusInternalServerError})
	binPath := filepath.Join(binDir, binaryName)
	writeBinaryFixture(t, binPath, string(binary))

	res, err := Install(context.Background(), deps, InstallOptions{BinaryPath: binPath})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res.Changed {
		t.Errorf("result = %+v, want Changed=false (skip)", res)
	}
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(got) != string(binary) {
		t.Errorf("binary changed on skip: got %q, want %q", got, binary)
	}
}

// Behavior 3: a stale binary is replaced atomically — final contents are the new
// binary and no temp file lingers.
func TestInstall_MismatchReplacesAtomically(t *testing.T) {
	fresh := []byte("the new dnclient binary")
	deps, binDir := installFixture(t, verifyServer{binary: fresh})
	binPath := filepath.Join(binDir, binaryName)
	writeBinaryFixture(t, binPath, "the stale dnclient binary")

	res, err := Install(context.Background(), deps, InstallOptions{BinaryPath: binPath})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Changed {
		t.Errorf("result = %+v, want Changed=true (replace)", res)
	}
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != string(fresh) {
		t.Errorf("replaced contents = %q, want %q", got, fresh)
	}
	assertNoTempFiles(t, binDir)
}

// Behavior 4: a missing bin dir is created.
func TestInstall_CreatesBinDir(t *testing.T) {
	binary := []byte("dnclient into nested dir")
	deps, parent := installFixture(t, verifyServer{binary: binary})
	binDir := filepath.Join(parent, "nested", "bin")

	if _, err := Install(context.Background(), deps, InstallOptions{BinaryPath: filepath.Join(binDir, binaryName)}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, binaryName)); err != nil {
		t.Errorf("binary not placed in created dir: %v", err)
	}
}

// Behavior 5: a verification failure propagates, places no binary, and leaves no
// partial/temp file (fail-closed + atomic).
func TestInstall_VerifyFailureLeavesNoBinary(t *testing.T) {
	wrong := hex.EncodeToString(make([]byte, 32)) // valid hex, wrong digest
	deps, binDir := installFixture(t, verifyServer{binary: []byte("real binary"), checksumBody: wrong})

	res, err := Install(context.Background(), deps, InstallOptions{BinaryPath: filepath.Join(binDir, binaryName)})
	if err == nil {
		t.Fatal("Install with wrong checksum: want error, got nil")
	}
	if res.Changed {
		t.Errorf("result = %+v, want zero value on failure", res)
	}
	if _, statErr := os.Stat(filepath.Join(binDir, binaryName)); !os.IsNotExist(statErr) {
		t.Errorf("binary present after verify failure: stat err = %v", statErr)
	}
	assertNoTempFiles(t, binDir)
}
