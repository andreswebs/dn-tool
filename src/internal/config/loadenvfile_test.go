package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadWithEnvFile_NoFileUsesLiveEnv: an empty path reads nothing from disk
// and builds the Config from the live environment and §2.3 defaults.
func TestLoadWithEnvFile_NoFileUsesLiveEnv(t *testing.T) {
	getenv := mapEnv(map[string]string{
		"DN_HOSTNAME":   "h1",
		"DN_NETWORK_ID": "env-net",
	})
	cfg, err := LoadWithEnvFile("", getenv)
	if err != nil {
		t.Fatalf("LoadWithEnvFile() error = %v", err)
	}
	if cfg.NetworkID != "env-net" {
		t.Errorf("NetworkID = %q, want %q", cfg.NetworkID, "env-net")
	}
	if cfg.NetworkName != "defined" {
		t.Errorf("NetworkName = %q, want default %q", cfg.NetworkName, "defined")
	}
}

// TestLoadWithEnvFile_PrecedenceEndToEnd: a configured file is read from disk,
// its values fill keys absent from the live env, and a live var overrides a
// file value with the same key (design §2.3 precedence, end-to-end).
func TestLoadWithEnvFile_PrecedenceEndToEnd(t *testing.T) {
	path := writeEnvFile(t, "DN_NETWORK_ID=file-net\nDN_ROLE_ID=file-role\nDN_API_URL=https://file.example\n")
	getenv := mapEnv(map[string]string{
		"DN_HOSTNAME":   "h1",
		"DN_NETWORK_ID": "env-net", // overrides the file value
	})
	cfg, err := LoadWithEnvFile(path, getenv)
	if err != nil {
		t.Fatalf("LoadWithEnvFile() error = %v", err)
	}
	if cfg.NetworkID != "env-net" {
		t.Errorf("NetworkID = %q, want live-env %q", cfg.NetworkID, "env-net")
	}
	if cfg.RoleID != "file-role" {
		t.Errorf("RoleID = %q, want file %q", cfg.RoleID, "file-role")
	}
	if cfg.APIURL != "https://file.example" {
		t.Errorf("APIURL = %q, want file override %q", cfg.APIURL, "https://file.example")
	}
}

// TestLoadWithEnvFile_MissingFileErrors: a configured path that does not exist
// is an operator error and must surface clearly, not be silently ignored.
func TestLoadWithEnvFile_MissingFileErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.env")
	_, err := LoadWithEnvFile(missing, emptyEnv)
	if err == nil {
		t.Fatal("LoadWithEnvFile() error = nil, want error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}

// TestLoadWithEnvFile_MalformedFileErrors: a parse failure from the env-file
// propagates as a clear error rather than yielding a partial config.
func TestLoadWithEnvFile_MalformedFileErrors(t *testing.T) {
	path := writeEnvFile(t, "DN_NETWORK_ID=ok\nthis-line-has-no-equals\n")
	_, err := LoadWithEnvFile(path, emptyEnv)
	if err == nil {
		t.Fatal("LoadWithEnvFile() error = nil, want parse error")
	}
}

// writeEnvFile writes content to a temp file and returns its path.
func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dn.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing env-file: %v", err)
	}
	return path
}
