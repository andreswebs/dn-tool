package version

import "testing"

func TestCurrent_Override(t *testing.T) {
	old := Override
	t.Cleanup(func() { Override = old })

	Override = "v9.9.9"
	got := Current()
	if got != "v9.9.9" {
		t.Errorf("Current() = %q, want %q", got, "v9.9.9")
	}
}

func TestCurrent_DevFallback(t *testing.T) {
	old := Override
	t.Cleanup(func() { Override = old })

	Override = ""
	got := Current()
	if got != "dev" {
		t.Errorf("Current() = %q, want %q", got, "dev")
	}
}

func TestCurrent_OverrideEmptyStringFallsThrough(t *testing.T) {
	old := Override
	t.Cleanup(func() { Override = old })

	Override = ""
	got := Current()
	if got == "" {
		t.Error("Current() returned empty string; expected non-empty fallback")
	}
	if got != "dev" {
		t.Errorf("Current() = %q, want %q", got, "dev")
	}
}
