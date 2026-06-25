---
id: dt-zxwf
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cq78
tags: [output, logging]
---
# slog setup: JSON to stderr + level filtering

Set up structured logging with `log/slog`: a JSON handler writing to **stderr** by default, honoring the configured `DN_LOG_LEVEL`. stdout is reserved for the command result ([OUT.result](dt-ccmn.md)); logs/diagnostics go to stderr only.

## Public interface

```go
// internal/output
type LogOptions struct {
    Level string // DN_LOG_LEVEL: debug|info|warn|error
    Text  bool   // --log-text -> plain text (OUT.text / dt-0jfp)
}
func NewLogger(w io.Writer, opts LogOptions) *slog.Logger  // w is stderr in production, a buffer in tests
```

Inject the writer so tests capture output. Default handler = `slog.NewJSONHandler`. Map `DN_LOG_LEVEL` string → `slog.Level`.

## Behaviors (TDD order)

1. **JSON to the writer by default** — a logged record is valid JSON with `msg`/`level`/attrs.
2. **Level filtering** — at `level=warn`, `Info` is suppressed, `Warn`/`Error` emitted.
3. **Invalid level** — unknown `DN_LOG_LEVEL` → clear error or documented fallback to `info` (decide and test).
4. **Writer is stderr in production** — wiring uses `os.Stderr`; nothing logged to stdout.

## Test strategy

`NewLogger(&buf, opts)`; log records; parse `buf` as JSON lines and assert fields/levels. No global logger mutation in tests.

## Acceptance

- JSON logs to stderr by default; `DN_LOG_LEVEL` filters correctly.
- stdout untouched by logging.

## References

- Design: [Req 7](../docs/dn-tool-design.md#7-output-and-observability), [§2.8 Output contract](../docs/dn-tool-design.md#28-output-contract), [§2.10 libraries](../docs/dn-tool-design.md#210-cli--libraries).

Parent epic: [dt-cq78](dt-cq78.md).

## Notes

**2026-06-06T19:21:13Z**

slog setup landed in internal/output/log.go: NewLogger(w io.Writer, LogOptions{Level,Text}) *slog.Logger — JSON handler to the injected writer (os.Stderr in prod), level-filtered. parseLevel maps debug|info|warn|error (case-insensitive, trimmed); empty/unknown falls back to info (behavior 3) since the signature has no error return. Behaviors 1-3 fully tested in log_test.go via buffer + NDJSON decode, no global-logger mutation. Behavior 4: main() now slog.SetDefault(NewLogger(os.Stderr, {Level: os.Getenv(DN_LOG_LEVEL)})) so the api retry logger (slogLeveledLogger over slog.Default) emits JSON-to-stderr honoring DN_LOG_LEVEL — closes the dt-egz4 handoff. Scope boundary held: LogOptions.Text is declared but NOT branched on (always JSON); dt-0jfp owns the TextHandler branch + --log-text flag wiring, so it gets a real red test. Live env only at process start; env-file precedence for log level is later command-wiring's job.
