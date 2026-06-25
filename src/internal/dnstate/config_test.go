package dnstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigExists(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "defined", "host_id: host-1\n")

		if !ConfigExists(root, "defined") {
			t.Errorf("ConfigExists = false, want true for present config")
		}
	})

	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()

		if ConfigExists(root, "defined") {
			t.Errorf("ConfigExists = true, want false for absent config")
		}
	})

	t.Run("present for a different network only", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "other", "host_id: host-1\n")

		if ConfigExists(root, "defined") {
			t.Errorf("ConfigExists = true, want false: config exists only for a different network")
		}
	})

	// Behavior 3: the probe checks exactly <root>/<network>/dnclient.yml — not a
	// bare <root>/dnclient.yml and not some other file under the network dir.
	t.Run("checks exactly the network dnclient.yml path", func(t *testing.T) {
		root := t.TempDir()

		if err := os.WriteFile(filepath.Join(root, "dnclient.yml"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write stray root config: %v", err)
		}
		netDir := filepath.Join(root, "defined")
		if err := os.MkdirAll(netDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(netDir, "other.yml"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write stray network file: %v", err)
		}

		if ConfigExists(root, "defined") {
			t.Errorf("ConfigExists = true, want false: only stray paths exist, not the exact dnclient.yml")
		}
	})
}
