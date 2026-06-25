package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

func TestRunWriteConfig_WritesFileAndResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	cfg := &config.Config{
		APIKey:      config.Secret("k"),
		NetworkID:   "net-1",
		NetworkName: "defined",
	}

	res, err := runWriteConfig(cfg, path)
	if err != nil {
		t.Fatalf("runWriteConfig() error = %v", err)
	}
	if res.Action != "write-config" {
		t.Errorf("Action = %q, want write-config", res.Action)
	}
	if !res.Changed {
		t.Error("Changed = false, want true")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}
}

func TestRunWriteConfig_RequiresPath(t *testing.T) {
	_, err := runWriteConfig(&config.Config{}, "")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

// TestWriteConfigCommand_EndToEnd drives the wired command with a positional
// path argument and asserts the file is written and the JSON result emitted.
func TestWriteConfigCommand_EndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	t.Setenv("DN_NETWORK_ID", "net-xyz")

	app := newApp()
	var stdout bytes.Buffer
	app.Writer = &stdout
	app.ErrWriter = &bytes.Buffer{}

	if err := app.Run(context.Background(), []string{"dn-tool", "write-config", path}); err != nil {
		t.Fatalf("Run(write-config) error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(stdout.String(), `"action":"write-config"`) {
		t.Errorf("stdout missing write-config result: %s", stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "DN_NETWORK_ID=net-xyz") {
		t.Errorf("file missing configured value:\n%s", data)
	}
}
