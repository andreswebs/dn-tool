package dnstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, root, network, content string) {
	t.Helper()
	dir := filepath.Join(root, network)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dnclient.yml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// realConfig mirrors the shape dnclient writes (observed from dnclient 0.9.5):
// host_id lives under metadata alongside the org/network identity, and a copy
// lives under host_key with the key material. metaHostID and keyHostID let a test
// give them distinct values to pin which one ReadHostID returns.
func realConfig(metaHostID, keyHostID string) string {
	return "host_key:\n" +
		"  host_ed_key: ed-key-bytes\n" +
		"  host_id: " + keyHostID + "\n" +
		"  counter: 0\n" +
		"metadata:\n" +
		"  org_id: org-1\n" +
		"  network_id: net-1\n" +
		"  host_id: " + metaHostID + "\n" +
		"  host_name: h\n"
}

func TestReadHostIDValidFile(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "defined", realConfig("host-abc123", "host-abc123"))

	got, err := ReadHostID(root, "defined")
	if err != nil {
		t.Fatalf("ReadHostID: %v", err)
	}
	if got != "host-abc123" {
		t.Errorf("host id = %q, want %q", got, "host-abc123")
	}
}

// dnclient writes host_id in two places; the API host identity unenroll deletes
// is metadata.host_id, not the copy under host_key. Distinct values pin that.
func TestReadHostIDReadsMetadataHostID(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "defined", realConfig("host-metadata", "host-keymaterial"))

	got, err := ReadHostID(root, "defined")
	if err != nil {
		t.Fatalf("ReadHostID: %v", err)
	}
	if got != "host-metadata" {
		t.Errorf("host id = %q, want metadata.host_id %q", got, "host-metadata")
	}
}

func TestReadHostIDMissingFileNotEnrolled(t *testing.T) {
	root := t.TempDir()

	_, err := ReadHostID(root, "defined")
	if !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("err = %v, want ErrNotEnrolled", err)
	}
}

func TestReadHostIDMissingFieldErrors(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "defined", "metadata:\n  network_id: net-1\n  host_name: h\n")

	_, err := ReadHostID(root, "defined")
	if err == nil {
		t.Fatal("want error for missing host_id, got nil")
	}
	if errors.Is(err, ErrNotEnrolled) {
		t.Errorf("missing field must not be ErrNotEnrolled: %v", err)
	}
}

func TestReadHostIDMalformedYAMLErrors(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "defined", "host_id: [unterminated\n  : :\n")

	_, err := ReadHostID(root, "defined")
	if err == nil {
		t.Fatal("want error for malformed YAML, got nil")
	}
	if errors.Is(err, ErrNotEnrolled) {
		t.Errorf("malformed YAML must not be ErrNotEnrolled: %v", err)
	}
}
