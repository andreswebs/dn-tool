package output

import (
	"io"
	"log/slog"
	"strings"
)

// LogOptions configures NewLogger.
//
// Level is the DN_LOG_LEVEL string (debug|info|warn|error, case-insensitive);
// an empty or unrecognized value falls back to info, since NewLogger has no
// error return to surface a bad value. Text selects plain-text (key=value)
// output for humans at a terminal; JSON is the default for machines.
type LogOptions struct {
	Level string
	Text  bool
}

// NewLogger builds a slog.Logger writing to w — os.Stderr in production, a
// buffer in tests. stdout is reserved for the single command result
// (WriteResult), so all diagnostics go through this logger to stderr. The
// handler filters to records at or above the level parsed from opts.Level, and
// emits JSON by default or plain text when opts.Text is set (design Req 7,
// §2.8).
func NewLogger(w io.Writer, opts LogOptions) *slog.Logger {
	hopts := &slog.HandlerOptions{Level: parseLevel(opts.Level)}
	var handler slog.Handler
	if opts.Text {
		handler = slog.NewTextHandler(w, hopts)
	} else {
		handler = slog.NewJSONHandler(w, hopts)
	}
	return slog.New(handler)
}

// parseLevel maps a DN_LOG_LEVEL string to a slog.Level, defaulting to info for
// empty or unrecognized input.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
