package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
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
	if !errors.Is(err, ErrMissingArgument) {
		t.Errorf("err = %v, want ErrMissingArgument", err)
	}
}

// dt-2efx: a missing positional argument is a command-line fault, so
// write-config with no path exits 64 (EX_USAGE) rather than 70, which it took
// while the error carried no class at all. CodeUsage is already declared for
// write-config in the registry, so the exit-code contract is unchanged.
func TestWriteConfigCommand_MissingPathIsUsageError(t *testing.T) {
	deps, _, stderr := testDeps(nil)

	if got := Run([]string{"write-config"}, deps); got != output.CodeUsage {
		t.Fatalf("write-config with no path exit code = %d, want %d (stderr: %s)", got, output.CodeUsage, stderr)
	}
}

// TestWriteConfigCommand_EndToEnd drives the wired command with a positional
// path argument and asserts the file is written and the JSON result emitted.
func TestWriteConfigCommand_EndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dn.env")
	deps, stdout, stderr := testDeps(map[string]string{"DN_NETWORK_ID": "net-xyz"})

	if got := Run([]string{"write-config", path}, deps); got != output.CodeOK {
		t.Fatalf("write-config exit code = %d, want %d (stderr: %s)", got, output.CodeOK, stderr)
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
