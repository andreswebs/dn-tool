package logctx

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Levels are the accepted --log-level / DN_LOG_LEVEL values, in increasing
// verbosity. They are listed rather than derived from the map below so the
// rejection message reads in a stable, meaningful order.
var Levels = []string{"error", "warn", "info", "debug"}

// levels maps an accepted name to its threshold.
var levels = map[string]slog.Level{
	"error": slog.LevelError,
	"warn":  slog.LevelWarn,
	"info":  slog.LevelInfo,
	"debug": slog.LevelDebug,
}

// DefaultLevel is the verbosity of an invocation that asks for none: warn, so
// stderr stays quiet on a healthy run and the result on stdout is the only
// output a caller has to read (ADR 0004).
const DefaultLevel = "warn"

// ParseLevel maps a level name to its slog.Level, case-insensitively and
// ignoring surrounding space. An empty or unrecognized value is an error, never
// a silent fallback: a misspelled level that quietly logs at some other
// verbosity is indistinguishable from a working one until the records an
// operator needed are missing.
func ParseLevel(s string) (slog.Level, error) {
	if level, ok := levels[strings.ToLower(strings.TrimSpace(s))]; ok {
		return level, nil
	}
	return 0, fmt.Errorf("unknown log level %q, want one of %s", s, strings.Join(Levels, "|"))
}

// NewLogger builds the diagnostic logger writing to w: the injected stderr in
// production, a buffer in tests. stdout is reserved for the single command
// result, so every diagnostic goes here. Records at or above level are emitted;
// text selects the human-readable handler for a terminal sink, JSON otherwise
// (ADR 0004, design Req 7).
func NewLogger(w io.Writer, level slog.Level, text bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if text {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
