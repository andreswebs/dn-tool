package logctx

import (
	"regexp"
	"testing"
)

var runIDPattern = regexp.MustCompile(`^[a-z2-7]{16}$`)

// The run id correlates the records of one invocation, so it must be a fixed
// width of lowercase characters, normalizable by a golden test with one
// substitution.
func TestNewRunID_Shape(t *testing.T) {
	id := NewRunID()
	if !runIDPattern.MatchString(id) {
		t.Errorf("NewRunID() = %q, want 16 lowercase base32 characters", id)
	}
}

// Two invocations must not share an id, or interleaved records cannot be
// separated.
func TestNewRunID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id := NewRunID()
		if seen[id] {
			t.Fatalf("NewRunID() repeated %q", id)
		}
		seen[id] = true
	}
}
