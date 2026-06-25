package output

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodes_Canonical(t *testing.T) {
	if CodeOK != 0 {
		t.Errorf("CodeOK = %d, want 0", CodeOK)
	}
	if CodeError != 1 {
		t.Errorf("CodeError = %d, want 1", CodeError)
	}
	if CodeNoOpAssert != 2 {
		t.Errorf("CodeNoOpAssert = %d, want 2", CodeNoOpAssert)
	}
}

func TestExitError_WrapsCodeAndMessage(t *testing.T) {
	ec := ExitError(errors.New("boom"), CodeError)
	if ec == nil {
		t.Fatal("ExitError returned nil for a non-nil error")
	}
	if got := ec.ExitCode(); got != CodeError {
		t.Errorf("ExitCode() = %d, want %d", got, CodeError)
	}
	if got := ec.Error(); got != "boom" {
		t.Errorf("Error() = %q, want %q", got, "boom")
	}
}

func TestExitError_PreservesWrappedChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	ec := ExitError(fmt.Errorf("context: %w", sentinel), CodeNoOpAssert)
	if !errors.Is(ec, sentinel) {
		t.Error("ExitError must preserve the wrapped error chain for errors.Is")
	}
	if got := ec.ExitCode(); got != CodeNoOpAssert {
		t.Errorf("ExitCode() = %d, want %d", got, CodeNoOpAssert)
	}
}

func TestExitError_NilErrorReturnsNilInterface(t *testing.T) {
	ec := ExitError(nil, CodeError)
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
		{"plain error is failure", errors.New("boom"), CodeError},
		{"ExitCoder honored", ExitError(errors.New("x"), CodeNoOpAssert), CodeNoOpAssert},
		{"wrapped ExitCoder honored", fmt.Errorf("ctx: %w", ExitError(errors.New("x"), CodeNoOpAssert)), CodeNoOpAssert},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveExitCode(tt.err); got != tt.want {
				t.Errorf("ResolveExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}
