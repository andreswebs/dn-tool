package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// sampleConfig is a fully-populated resolved Config used to exercise the writer's
// round-trip and secret-handling behaviors.
func sampleConfig() *Config {
	return &Config{
		APIKey:           Secret("super-secret-key"),
		NetworkID:        "net-123",
		RoleID:           "role-456",
		NetworkName:      "defined",
		Hostname:         "host-a",
		IPAddress:        "10.0.0.5",
		Tags:             []string{"env:prod", "team:core"},
		IsLighthouse:     true,
		IsRelay:          false,
		StaticAddrs:      []string{"203.0.113.1:4242"},
		ListenPort:       4242,
		APIURL:           "https://api.defined.net",
		APITimeout:       30 * time.Second,
		ClientBinDir:     "/var/lib/defined/bin",
		ClientConfigDir:  "/etc/defined",
		ClientSocket:     "/var/run/defined/dnclient.defined.sock",
		ClientVersion:    "1.2.3",
		LogLevel:         "info",
		SkipUnenroll:     true,
		UnenrollOnReboot: true,
	}
}

// TestWriteConfigFile_CreatesWith0600 asserts the file is created with exactly
// owner-only permissions even under a permissive umask — proving the mode was
// applied at creation (from the open flags), not inherited from the umask. A
// permissive umask (0) would yield 0666 for a default create; 0600 here can only
// come from the explicit creation mode (closes SEC2).
func TestWriteConfigFile_CreatesWith0600(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	path := filepath.Join(t.TempDir(), "dn.env")
	if err := WriteConfigFile(path, sampleConfig()); err != nil {
		t.Fatalf("WriteConfigFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}
}

// TestWriteConfigFile_RoundTrips asserts the written file parses back through the
// env-file loader to the same resolved config.
func TestWriteConfigFile_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	cfg := sampleConfig()
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatalf("WriteConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	vars, err := ParseEnvFile(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParseEnvFile() error = %v\nfile:\n%s", err, data)
	}
	got, err := Resolve(vars, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("round-trip mismatch:\n got = %+v\nwant = %+v", got, cfg)
	}
}

// TestWriteConfigFile_WritesAPIKeyUnder0600 asserts the secret is persisted in
// cleartext (the documented dt-meg1 decision) and that its protection is the
// file's 0600 mode — the key is present, REDACTED is absent, mode is 0600.
func TestWriteConfigFile_WritesAPIKeyUnder0600(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	path := filepath.Join(t.TempDir(), "dn.env")
	if err := WriteConfigFile(path, sampleConfig()); err != nil {
		t.Fatalf("WriteConfigFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "super-secret-key") {
		t.Errorf("file does not contain the API key:\n%s", data)
	}
	if strings.Contains(string(data), "REDACTED") {
		t.Errorf("file contains REDACTED — secret was scrubbed, breaking the round trip:\n%s", data)
	}
}

// TestWriteConfigFile_TruncatesExisting asserts writing over an existing file
// replaces its contents (no stale trailing bytes) and keeps 0600.
func TestWriteConfigFile_TruncatesExisting(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	path := filepath.Join(t.TempDir(), "dn.env")
	if err := os.WriteFile(path, []byte("STALE=leftover-data-that-is-very-long\n"), 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}
	if err := WriteConfigFile(path, sampleConfig()); err != nil {
		t.Fatalf("WriteConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "STALE") {
		t.Errorf("stale content survived truncation:\n%s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}
}
