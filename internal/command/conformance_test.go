package command

import (
	"os"
	"strings"
	"testing"
)

// ADR 0003: command code reads the process environment only through Deps. This
// is the cheap drift test the ADR calls for — one direct os.Getenv or os.Stdout
// in this package silently re-welds the CLI to the real process, and no
// behavioral test would catch it.
func TestPackageNeverTouchesTheProcessEnvironment(t *testing.T) {
	forbidden := []string{"os.Getenv", "os.Args", "os.Stdin", "os.Stdout", "os.Stderr"}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, bad := range forbidden {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s references %s; the process environment reaches commands only through Deps", name, bad)
			}
		}
	}
}
