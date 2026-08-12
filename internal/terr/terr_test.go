package terr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewCarriesCodeExitAndHint(t *testing.T) {
	e := New("t_new", 74, "retry the operation", "something failed")

	if got := e.Code(); got != "t_new" {
		t.Errorf("Code() = %q, want %q", got, "t_new")
	}
	if got := e.ExitCode(); got != 74 {
		t.Errorf("ExitCode() = %d, want 74", got)
	}
	if got := e.Hint(); got != "retry the operation" {
		t.Errorf("Hint() = %q, want %q", got, "retry the operation")
	}
	if got := e.Error(); got != "something failed" {
		t.Errorf("Error() = %q, want %q", got, "something failed")
	}
	if e.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil for a bare sentinel", e.Unwrap())
	}
	if e.ErrorDetails() != nil {
		t.Errorf("ErrorDetails() = %v, want nil for a bare sentinel", e.ErrorDetails())
	}
}

// A duplicate code is an init-time programmer error: two sentinels sharing a
// code would make the machine contract ambiguous and the enumerated inventory
// wrong, so registration panics rather than silently accepting it (ADR 0002).
func TestNewPanicsOnDuplicateCode(t *testing.T) {
	if first := New("t_duplicate", 70, "", "first"); first.Code() != "t_duplicate" {
		t.Fatalf("Code() = %q, want %q", first.Code(), "t_duplicate")
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("New with a duplicate code did not panic")
		}
		if msg := fmt.Sprint(r); !strings.Contains(msg, "t_duplicate") {
			t.Fatalf("panic value = %v, want a message naming the duplicate code", r)
		}
	}()
	if second := New("t_duplicate", 70, "", "second"); second != nil {
		t.Fatal("New returned instead of panicking on a duplicate code")
	}
}

// Newf is the one-off, per-invocation form: it carries the same three pieces but
// stays out of the enumerated inventory, so a formatted instance cannot pollute
// the documented error surface.
func TestNewfIsFormattedAndUnregistered(t *testing.T) {
	before := len(All())

	e := Newf("t_newf", 65, "check the payload", "rejected %d of %d items", 2, 5)

	if got := e.Error(); got != "rejected 2 of 5 items" {
		t.Errorf("Error() = %q, want %q", got, "rejected 2 of 5 items")
	}
	if got := len(All()); got != before {
		t.Errorf("registry length = %d, want %d (Newf must not register)", got, before)
	}
}

func TestErrorAppendsCause(t *testing.T) {
	sentinel := New("t_cause", 74, "", "write failed")
	wrapped := sentinel.Wrap(errors.New("disk full"))

	if got := wrapped.Error(); got != "write failed: disk full" {
		t.Errorf("Error() = %q, want %q", got, "write failed: disk full")
	}
	if got := sentinel.Error(); got != "write failed" {
		t.Errorf("sentinel Error() = %q, want %q (receiver must be unmodified)", got, "write failed")
	}
}

func TestWrapCopiesAndPreservesTheChain(t *testing.T) {
	sentinel := New("t_wrap", 74, "hint", "outer")
	cause := errors.New("inner")

	wrapped := sentinel.Wrap(cause)

	if wrapped == sentinel {
		t.Fatal("Wrap returned the receiver, want a copy")
	}
	if sentinel.Unwrap() != nil {
		t.Errorf("receiver gained a cause: %v", sentinel.Unwrap())
	}
	if !errors.Is(wrapped, cause) {
		t.Error("Wrap must preserve the cause for errors.Is")
	}
	if got := wrapped.ExitCode(); got != 74 {
		t.Errorf("copy ExitCode() = %d, want 74", got)
	}
	if got := wrapped.Hint(); got != "hint" {
		t.Errorf("copy Hint() = %q, want %q", got, "hint")
	}
}

func TestWithDetailsCopiesAndLeavesReceiverUnmodified(t *testing.T) {
	sentinel := New("t_details", 74, "", "failed")
	details := map[string]any{"exit_status": 3}

	withDetails := sentinel.WithDetails(details)

	if withDetails == sentinel {
		t.Fatal("WithDetails returned the receiver, want a copy")
	}
	if sentinel.ErrorDetails() != nil {
		t.Errorf("receiver gained details: %v", sentinel.ErrorDetails())
	}
	var d Detailed
	if !errors.As(error(withDetails), &d) {
		t.Fatal("the copy does not satisfy Detailed")
	}
	got, ok := d.ErrorDetails().(map[string]any)
	if !ok {
		t.Fatalf("ErrorDetails() = %T, want map[string]any", d.ErrorDetails())
	}
	if got["exit_status"] != 3 {
		t.Errorf("details[exit_status] = %v, want 3", got["exit_status"])
	}
}

// Identity is the code, so per-invocation copies still match their sentinel:
// classification by errors.Is survives Wrap, WithDetails, and %w wrapping.
func TestIsMatchesCopiesAgainstTheirSentinel(t *testing.T) {
	sentinel := New("t_is", 78, "", "bad config")
	other := New("t_is_other", 78, "", "other")

	copyOf := sentinel.Wrap(errors.New("detail")).WithDetails("x")

	if !errors.Is(copyOf, sentinel) {
		t.Error("a Wrap/WithDetails copy must match its sentinel under errors.Is")
	}
	if !errors.Is(fmt.Errorf("context: %w", copyOf), sentinel) {
		t.Error("a %w-wrapped copy must still match its sentinel")
	}
	if errors.Is(copyOf, other) {
		t.Error("a copy must not match a different sentinel that shares an exit code")
	}
	if errors.Is(sentinel, errors.New("bad config")) {
		t.Error("identity must be the code, never the message")
	}
}

func TestAllReturnsACopyInRegistrationOrder(t *testing.T) {
	first := New("t_all_first", 70, "", "first")
	second := New("t_all_second", 70, "", "second")

	all := All()
	if len(all) < 2 {
		t.Fatalf("All() returned %d entries, want at least 2", len(all))
	}
	iFirst, iSecond := indexOf(all, first), indexOf(all, second)
	if iFirst < 0 || iSecond < 0 {
		t.Fatalf("All() is missing a registered sentinel: %d, %d", iFirst, iSecond)
	}
	if iFirst > iSecond {
		t.Errorf("All() order = %d before %d, want registration order", iSecond, iFirst)
	}

	all[iFirst] = nil
	if again := All(); again[iFirst] == nil {
		t.Error("mutating All()'s result changed the registry; All must return a copy")
	}
}

func indexOf(all []*E, want *E) int {
	for i, e := range all {
		if e == want {
			return i
		}
	}
	return -1
}

// The exit boundary must default to loud: an ordinary sentinel is a failure and
// therefore renders an error envelope, so a classification nobody thought about
// cannot silently become a silent status.
func TestNewIsAFailureByDefault(t *testing.T) {
	e := New("t_failure_default", 74, "", "failed")

	if e.NonFailure() {
		t.Error("New() sentinel reports NonFailure() = true, want false")
	}
	if IsNonFailure(e) {
		t.Error("IsNonFailure() = true for a New() sentinel, want false")
	}
	if IsNonFailure(errors.New("boom")) {
		t.Error("IsNonFailure() = true for an unclassified error, want false")
	}
	if IsNonFailure(nil) {
		t.Error("IsNonFailure(nil) = true, want false")
	}
}

// NewStatus declares the other kind of coded value: one whose exit code is the
// whole contract and which reports no failure, so the boundary writes no error
// envelope for it (ADR 0002, "the error value ... prints nothing itself").
func TestNewStatusIsARegisteredNonFailure(t *testing.T) {
	before := len(All())

	s := NewStatus("t_status", 1, "drop the flag", "nothing changed")

	if !s.NonFailure() {
		t.Error("NewStatus() sentinel reports NonFailure() = false, want true")
	}
	if !IsNonFailure(s) {
		t.Error("IsNonFailure() = false for a NewStatus() sentinel, want true")
	}
	if got := len(All()); got != before+1 {
		t.Errorf("registry length = %d, want %d (a status is part of the inventory)", got, before+1)
	}
	if got := s.Code(); got != "t_status" {
		t.Errorf("Code() = %q, want %q", got, "t_status")
	}
	if got := s.ExitCode(); got != 1 {
		t.Errorf("ExitCode() = %d, want 1", got)
	}
}

// The marker travels with the per-invocation copies, or the very decoration that
// adds context would turn a status back into a reported failure.
func TestNonFailureSurvivesCopiesAndWrapping(t *testing.T) {
	s := NewStatus("t_status_copy", 143, "", "terminated by signal")

	if !s.Wrap(errors.New("cause")).NonFailure() {
		t.Error("a Wrap copy lost the non-failure marker")
	}
	if !s.WithDetails("x").NonFailure() {
		t.Error("a WithDetails copy lost the non-failure marker")
	}
	if !IsNonFailure(fmt.Errorf("context: %w", s)) {
		t.Error("IsNonFailure() must find the marker through a %w wrapping")
	}
}
