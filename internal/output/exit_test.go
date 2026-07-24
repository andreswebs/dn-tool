package output

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodes_Canonical(t *testing.T) {
	cases := map[string]struct{ got, want int }{
		"CodeOK":          {CodeOK, 0},
		"CodeResult":      {CodeResult, 1},
		"CodeUsage":       {CodeUsage, 64},
		"CodeData":        {CodeData, 65},
		"CodeUnavailable": {CodeUnavailable, 69},
		"CodeInternal":    {CodeInternal, 70},
		"CodeIO":          {CodeIO, 74},
		"CodeConfig":      {CodeConfig, 78},
		"CodeSIGINT":      {CodeSIGINT, 130},
		"CodeSIGTERM":     {CodeSIGTERM, 143},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", name, c.got, c.want)
		}
	}
}

func TestExitError_WrapsCodeAndMessage(t *testing.T) {
	ec := ExitError(errors.New("boom"), CodeInternal)
	if ec == nil {
		t.Fatal("ExitError returned nil for a non-nil error")
	}
	if got := ec.ExitCode(); got != CodeInternal {
		t.Errorf("ExitCode() = %d, want %d", got, CodeInternal)
	}
	if got := ec.Error(); got != "boom" {
		t.Errorf("Error() = %q, want %q", got, "boom")
	}
}

func TestExitError_PreservesWrappedChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	ec := ExitError(fmt.Errorf("context: %w", sentinel), CodeResult)
	if !errors.Is(ec, sentinel) {
		t.Error("ExitError must preserve the wrapped error chain for errors.Is")
	}
	if got := ec.ExitCode(); got != CodeResult {
		t.Errorf("ExitCode() = %d, want %d", got, CodeResult)
	}
}

func TestExitError_NilErrorReturnsNilInterface(t *testing.T) {
	ec := ExitError(nil, CodeInternal)
	if ec != nil {
		t.Errorf("ExitError(nil, …) = %v, want a true nil interface", ec)
	}
}

func TestResolveExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, CodeOK},
		{"unclassified error is internal", errors.New("boom"), CodeInternal},
		{"ExitCoder honored", ExitError(errors.New("x"), CodeResult), CodeResult},
		{"wrapped ExitCoder honored", fmt.Errorf("ctx: %w", ExitError(errors.New("x"), CodeResult)), CodeResult},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveExitCode(tt.err); got != tt.want {
				t.Errorf("ResolveExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassify_AssignsCodeToUnclassifiedError(t *testing.T) {
	sentinel := errors.New("bad config")
	classified := Classify(sentinel, CodeConfig)
	if got := ResolveExitCode(classified); got != CodeConfig {
		t.Errorf("ResolveExitCode = %d, want %d", got, CodeConfig)
	}
	if !errors.Is(classified, sentinel) {
		t.Error("Classify must preserve the wrapped chain for errors.Is")
	}
}

func TestClassify_DefersToInnerCode(t *testing.T) {
	// An inner Data classification survives an outer Config classification: the
	// innermost assigned class wins, so boundary wrapping never overrides a more
	// specific inner class.
	inner := Classify(errors.New("api rejected payload"), CodeData)
	outer := Classify(fmt.Errorf("resolving config: %w", inner), CodeConfig)
	if got := ResolveExitCode(outer); got != CodeData {
		t.Errorf("ResolveExitCode = %d, want %d (inner class must win)", got, CodeData)
	}
}

func TestClassify_NilErrorReturnsNil(t *testing.T) {
	if got := Classify(nil, CodeIO); got != nil {
		t.Errorf("Classify(nil, …) = %v, want nil", got)
	}
}
