package logctx

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A buffer is not a terminal: an in-process test sink must select the JSON
// handler, matching what a pipeline gets.
func TestIsTerminalWriter_BufferIsNotATerminal(t *testing.T) {
	if IsTerminalWriter(&bytes.Buffer{}) {
		t.Error("IsTerminalWriter(*bytes.Buffer) = true, want false")
	}
}

// A regular file is not a terminal: redirected stderr gets JSON.
func TestIsTerminalWriter_RegularFileIsNotATerminal(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "stderr.log"))
	if err != nil {
		t.Fatalf("creating the file: %v", err)
	}
	defer func() { _ = f.Close() }()

	if IsTerminalWriter(f) {
		t.Error("IsTerminalWriter(regular file) = true, want false")
	}
}

// A pipe is not a terminal: this is the shape stderr has under a supervisor or
// a container runtime, which is exactly where JSON is wanted.
func TestIsTerminalWriter_PipeIsNotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating the pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	if IsTerminalWriter(w) {
		t.Error("IsTerminalWriter(pipe) = true, want false")
	}
}

// A nil writer is not a terminal, so a partially populated Deps cannot panic
// handler selection.
func TestIsTerminalWriter_NilIsNotATerminal(t *testing.T) {
	if IsTerminalWriter(nil) {
		t.Error("IsTerminalWriter(nil) = true, want false")
	}
}
