// Package logctx owns dn-tool's diagnostic logger: its construction, its
// verbosity, and the context it travels on (ADR 0004). dn-tool is a daemon
// wrapper that talks to external systems, so it carries the leveled posture:
// a validated level, a destination-selected handler, and records decorated with
// the invocation's identity.
//
// Nothing here calls slog.SetDefault. The logger is built once by the command
// delegate, bound to the injected stderr writer, and put on the context with
// With; every layer below reads it back with From. Process-global logger state
// is hostile to parallel in-process tests, which is the whole reason this
// package exists.
package logctx

import (
	"context"
	"log/slog"
)

// ctxKey is the unexported context key type, so no other package can collide
// with or overwrite the stored logger.
type ctxKey struct{}

// With returns ctx carrying l. A nil logger is ignored rather than stored, so a
// caller that forgot to construct one still gets From's fallback instead of a
// nil-pointer panic at the first record.
func With(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the logger carried by ctx, or slog.Default() when there is none.
// The fallback is what lets a domain unit test call a logging code path with a
// bare context.Background() and no logging setup at all; it is the only
// slog.Default() call site in the tree.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
