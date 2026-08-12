// Package terr is dn-tool's coded-error mechanism: every failure that can reach
// a consumer carries a stable snake_case machine code, an exit code from the
// ADR 0001 taxonomy, and an optional remediation hint written for the consumer
// (ADR 0002). The exit boundary discovers all three with a single errors.As
// against Coded, so classification is by identity and never by message.
//
// Sentinels are immutable package-level values built with New, which also
// registers them so the error surface is enumerable as data (All). New declares a
// failure, which renders an error envelope; NewStatus declares the other kind, a
// non-failure status whose exit code is the whole contract (the --assert-changed
// no-op, a graceful signal shutdown) and which the boundary reports with no
// envelope at all. The default is failure, so an unclassified error stays loud.
// Per-invocation
// context attaches by copy — Wrap for a cause, WithDetails for a structured
// payload — so a shared sentinel is never mutated and stays safe across
// goroutines by construction.
//
// The mechanism carries no domain knowledge and no framework dependency: the
// sentinel inventory lives in the packages that raise each failure, next to the
// rationale for its exit code, and nothing here imports the CLI framework
// (ADR 0003's no-leak rule).
package terr

import (
	"errors"
	"fmt"
	"sync"
)

// Coded is the interface the exit boundary discovers with errors.As: an error
// that classifies itself with a machine code, a process exit code, and a hint.
// A type needing per-instance structure may delegate Code/ExitCode/Hint to a
// class sentinel rather than being an *E itself.
type Coded interface {
	error
	Code() string
	ExitCode() int
	Hint() string
}

// Status is the optional interface a coded value implements when its exit code
// is the whole contract and it reports no failure: the boundary honors the code
// but writes no error envelope for it (ADR 0002, "the error value that carries
// the exit code prints nothing itself"). It defaults to failure — an ordinary
// sentinel reports false — so an unclassified error stays loud.
type Status interface {
	NonFailure() bool
}

// Detailed is implemented by an error carrying a render-ready structured payload
// (expected/actual values, a subprocess status, a valid-value list), computed at
// origin so the presentation layer renders it without re-deriving anything.
type Detailed interface {
	ErrorDetails() any
}

// E is the concrete coded error. Its fields are unexported and never mutated
// after construction: Wrap and WithDetails return copies, so the package-level
// sentinels stay immutable however many invocations decorate them.
type E struct {
	code    string
	msg     string
	hint    string
	exit    int
	cause   error
	details any
	// status marks the non-failure kind (NewStatus); the zero value is the
	// failure kind, so a sentinel declared without thinking about this renders
	// an envelope rather than passing silently.
	status bool
}

var (
	_ Coded    = (*E)(nil)
	_ Detailed = (*E)(nil)
	_ Status   = (*E)(nil)
)

// The inventory of registered sentinels, in registration order. Registration
// happens during package initialization, but All may be read at any time (the
// schema command), so the mutex guards both.
var (
	registryMu sync.Mutex
	registry   []*E
	byCode     = map[string]*E{}
)

// New declares a registered sentinel: code is its stable snake_case machine
// code, exit its ADR 0001 process exit code, hint the remediation advice shown
// to the consumer (empty when there is none), and msg its lowercase,
// punctuation-free message.
//
// It panics on a duplicate code. Two sentinels sharing a code would make the
// machine contract ambiguous and the enumerated inventory wrong; that is an
// init-time programmer error, and ADR 0002 reserves panics for exactly this
// case, where crashing at startup is the correct outcome.
func New(code string, exit int, hint, msg string) *E {
	return register(&E{code: code, msg: msg, hint: hint, exit: exit})
}

// NewStatus declares a registered sentinel of the non-failure kind: a coded value
// whose exit code is the whole contract, such as the --assert-changed no-op (1)
// or a graceful signal shutdown (130/143). It carries the same code, exit code
// and hint as any sentinel, and differs only in that the boundary writes no error
// envelope for it, so a consumer never reads ok:false about a command that did
// not fail.
func NewStatus(code string, exit int, hint, msg string) *E {
	return register(&E{code: code, msg: msg, hint: hint, exit: exit, status: true})
}

// register adds e to the inventory. It panics on a duplicate code: two sentinels
// sharing a code would make the machine contract ambiguous and the enumerated
// inventory wrong; that is an init-time programmer error, and ADR 0002 reserves
// panics for exactly this case, where crashing at startup is the correct outcome.
func register(e *E) *E {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := byCode[e.code]; dup {
		panic("terr: duplicate error code " + e.code)
	}
	byCode[e.code] = e
	registry = append(registry, e)
	return e
}

// IsNonFailure reports whether err declares itself a non-failure status anywhere
// in its chain (terr.Status). It answers the exit boundary's one question — does
// this error print an envelope — and defaults to false, so only a value that
// opted out stays quiet.
func IsNonFailure(err error) bool {
	var status Status
	return errors.As(err, &status) && status.NonFailure()
}

// Newf builds an unregistered coded error with a formatted message, for a
// one-off failure that has no package-level sentinel. It stays out of the
// inventory All reports, so a per-invocation instance cannot drift the
// documented error surface; because identity is the code, such an instance still
// matches a sentinel that shares its code under errors.Is.
func Newf(code string, exit int, hint, format string, args ...any) *E {
	return &E{code: code, msg: fmt.Sprintf(format, args...), hint: hint, exit: exit}
}

// All returns the registered sentinels in registration order. The slice is a
// copy, so a consumer (the schema command's error inventory) cannot reshape the
// registry by mutating it.
func All() []*E {
	registryMu.Lock()
	defer registryMu.Unlock()
	out := make([]*E, len(registry))
	copy(out, registry)
	return out
}

// Code is the stable snake_case machine code consumers branch on.
func (e *E) Code() string { return e.code }

// ExitCode is the ADR 0001 process exit code this class of failure exits with.
func (e *E) ExitCode() int { return e.exit }

// Hint is the remediation advice for the consumer, or "" when there is none.
func (e *E) Hint() string { return e.hint }

// NonFailure reports the kind: true for a status declared with NewStatus, false
// for every failure sentinel. Copies made by Wrap and WithDetails keep it, so
// adding per-invocation context cannot turn a status back into a reported failure.
func (e *E) NonFailure() bool { return e.status }

// ErrorDetails is the structured payload attached by WithDetails, or nil.
func (e *E) ErrorDetails() any { return e.details }

// Error is the sentinel's message, with the cause appended as "message: cause"
// once one is attached by Wrap.
func (e *E) Error() string {
	if e.cause == nil {
		return e.msg
	}
	return e.msg + ": " + e.cause.Error()
}

// Unwrap returns the cause attached by Wrap, keeping the error chain walkable so
// callers can still match a lower-level sentinel or typed error through it.
func (e *E) Unwrap() error { return e.cause }

// Is matches by code, so a Wrap/WithDetails copy still matches the sentinel it
// was copied from under errors.Is — classification by identity survives every
// per-invocation decoration.
func (e *E) Is(target error) bool {
	t, ok := target.(*E)
	return ok && t.code == e.code
}

// Wrap returns a copy of the sentinel carrying cause, which Error appends and
// Unwrap exposes. The receiver is left unmodified.
func (e *E) Wrap(cause error) *E {
	c := *e
	c.cause = cause
	return &c
}

// WithDetails returns a copy of the sentinel carrying a structured payload for
// the error envelope. The receiver is left unmodified.
func (e *E) WithDetails(details any) *E {
	c := *e
	c.details = details
	return &c
}
