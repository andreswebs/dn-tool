---
id: dt-cr62
status: open
deps: [dt-w86c]
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-69kx
tags: [adr, logging]
---
# Rework logging to the ADR 0004 leveled posture

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0004-logging.md` defines two conformant postures. dn-tool is a
long-running daemon wrapper that talks to external systems, so its posture is
deliberately the **leveled** one, not the silent one. The ADR's leveled posture
requires:

- `--log-level error|warn|info|debug`, validated, default `warn`; `--quiet`
  raising the floor to `error` and beating `--log-level`; `<TOOL>_LOG_LEVEL` as
  fallback; precedence flags over environment over default.
- Handler chosen by destination: JSON when stderr is not a terminal, text when it
  is. `--log-format` is optional ("a tool **may** add") and the default requires
  no flag.
- The delegate constructs the logger once, after flags are parsed, bound to the
  injected stderr writer, and carries it on `context.Context` via a small
  `logctx` package (`With`/`From`, with `From` falling back to `slog.Default()`
  so plain unit tests need no setup).
- **"No library calls `slog.SetDefault`; process-global logger state is hostile
  to parallel in-process tests."**
- Records carry `command` and `version`; a run-correlation id when interleaving
  matters.
- Domain packages do not log; diagnostics belong to the orchestration layer.

Decision recorded 2026-08-12: **follow the ADR strictly.** Default `warn`,
`--log-text` dropped, and **no `--log-format`** is added.

## Current state (verified 2026-08-12 against commit bc8fa0b)

Two `slog.SetDefault` call sites, both to remove:

- `cmd/dn-tool/main.go:161` - the bootstrap in `main`.
- `cmd/dn-tool/main.go:96` - the reconfigure in the root `Before` hook.

Flag surface and level resolution:

- `cmd/dn-tool/main.go:62-67` - `logOptions`, reading
  `os.Getenv("DN_LOG_LEVEL")` from the **live environment only** (bypassing
  `--env-file`) and `cmd.Bool("log-text")`.
- `cmd/dn-tool/main.go:110-112` - the `--log-text` global flag declaration.
- No `--log-level`, no `--quiet`, no terminal detection anywhere in the tree.
- `internal/output/log.go:15-18` - `LogOptions{Level string; Text bool}`.
- `internal/output/log.go:26-35` - `NewLogger`, JSON unless `Text`.
- `internal/output/log.go:39-50` - `parseLevel`, defaulting to `info` and
  **silently accepting garbage**.

The level is also a config value, which the flag surface must reconcile with:

- `internal/config/config.go:35` - `LogLevel string // DN_LOG_LEVEL, default "info"`.
- `internal/config/config.go:76` - the `"info"` default.
- `internal/config/marshal.go:53` - round-tripped by the env-file writer.

Call sites on the process-global default logger, which removing `SetDefault`
would otherwise **silence**:

- `internal/dninstall/install.go:85` - `slog.Default().Debug("install decision",
  "path", …, "version", …, "needsInstall", …, "reason", …)`. Inside
  `Install(ctx, …)`, so `ctx` is in scope.
- `internal/dnclient/client.go:69` - `slog.Default().Debug("running dnclient",
  "action", …, "bin", …)`. Inside `exec(ctx, …)`, so `ctx` is in scope.
- `internal/api/client.go:137` - `slog.Default()` handed to
  `newRetryableClient` (`:110-125`) as the retryablehttp leveled logger via
  `slogLeveledLogger` (`:94-103`). **`New(cfg)` has no context**; see below.
- `internal/run/run.go:252` - `slog.Error("unenroll failed during shutdown; host
  may still be enrolled", …)`, on the path where the error is **deliberately
  dropped** in favor of the daemon's exit code. Inside `runAndUnenroll(ctx, …)`.
- `internal/run/run.go:257` - the same message, on the path where the error is
  **also returned**.

## Target shape

### logctx

New package `internal/logctx`, mirroring the sibling fleet repo `terminology`'s
`internal/logctx/logctx.go` and `runid.go`:

```go
type ctxKey struct{}

func With(ctx context.Context, l *slog.Logger) context.Context
func From(ctx context.Context) *slog.Logger // falls back to slog.Default()
func NewRunID() string                      // 16 hex chars from crypto/rand
```

`From`'s `slog.Default()` fallback is what lets domain unit tests keep running
with no logging setup at all.

### Flag surface

- `--log-level error|warn|info|debug`, default `warn`, **validated**: a bad value
  is a usage error (exit 64) through the framework's flag validation, replacing
  `parseLevel`'s silent fallback. Note the framework wraps validator errors as
  `invalid value %q for flag -%s: …`, which the existing `OnUsageError` mapping
  already turns into 64.
- `--quiet` raises the floor to `error` and **beats** `--log-level`.
- `DN_LOG_LEVEL` as the env fallback. **No rename**: dn-tool's prefix is `DN_`
  and its whole configuration surface is `DN_*`, so `DN_LOG_LEVEL` is already
  ADR 0004's `<TOOL>_LOG_LEVEL` instantiated. This is a confirmation, not a
  change.
- Precedence: flags over environment over default.
- **Remove `--log-text`** (`cmd/dn-tool/main.go:110-112`) and **do not add
  `--log-format`**. The ADR makes the format flag optional and the destination
  default flag-free; strict adoption adds nothing.

Consequence to state in the CHANGELOG: a human redirecting stderr to a file now
gets JSON with no flag to force text. That is the ADR's intent.

### Handler selection

Text when the stderr sink is a terminal, JSON otherwise. This needs a fakeable
seam, which ADR 0003 explicitly sanctions ("a field is added only when a test
must fake it (`Getenv`, a clock, **terminal detection**)"). Add to
`internal/command.Deps`:

```go
IsTerminal func(io.Writer) bool // nil means "not a terminal"
```

`main` supplies the real implementation. Prefer detecting via
`os.Stderr.Stat()` and `os.ModeCharDevice` over adding a dependency; if a
dependency is warranted, `golang.org/x/term` is the conventional choice - decide
in the ticket and note which, since it affects `go.mod` and the Nix vendor hash.

### Construction and travel

Construct **once** in the delegate after flag parse, bound to `deps.Err`, then
`logctx.With(ctx, logger)` so it travels. Records carry:

- `command` - the invoked command name.
- `version` - `internal/version.Current()`.
- `run_id` - `logctx.NewRunID()`, on **every** command unconditionally
  (decision 2026-08-12: the ADR's "when interleaving matters" is a necessity
  threshold, not a prohibition elsewhere; a conditional id is a branch to test
  and makes golden normalization inconsistent between commands).

`internal/output/log.go` moves into `internal/logctx` (or is rewritten there and
deleted). ADR 0004 rejects ambient logger construction in the output package,
which `docs/dn-tool-design.md:559` currently documents as `output/`'s job.

### Call-site migration (decided per site)

| Site | Disposition |
| ---- | ----------- |
| `internal/dninstall/install.go:85` | **Keep** as a diagnostic, via `logctx.From(ctx)`. It is not a problem, so there is no error to return, and it is the only observability into the idempotency decision. |
| `internal/dnclient/client.go:69` | **Keep** as a diagnostic, via `logctx.From(ctx)`. Preserve the deliberate omission of the argument list, which may carry the enrollment code. |
| `internal/api/client.go:137` | **Keep**, with an injected logger; see the signature decision below. Retry attempts are exactly what an operator needs on an unstable boot network (design Req 9). |
| `internal/run/run.go:252` | **Keep** as a diagnostic, via `logctx.From(ctx)`. This is ADR 0002's sanctioned "handle it and degrade gracefully" case - the error is dropped in favor of the daemon's code, so the log is its only trace. |
| `internal/run/run.go:257` | **Drop the log; return only.** Logging and returning the same error violates ADR 0002's handle-once rule, and once dt-nljm lands the envelope carries the message, so the log would duplicate envelope content - which ADR 0004 forbids outright. |

### The api.New signature (decided 2026-08-12)

`New(cfg)` (`internal/api/client.go:137`) has no context, but the logger it binds
is used later inside retryablehttp's retry loop. Plain positional injection
(`New(cfg, logger)`) would touch **20+ existing call sites** across
`internal/api/client_test.go`, `pagination_test.go`, and `endpoints_test.go`,
which all call `New(&config.Config{APIURL: srv.URL})`. Use a functional option
instead - still explicit injection from the orchestration layer, zero test churn:

```go
func New(cfg *config.Config, opts ...Option) *Client
func WithLogger(l *slog.Logger) Option
```

Production call sites pass `api.WithLogger(logctx.From(ctx))`:
`cmd/dn-tool/install.go:47`, `cmd/dn-tool/enroll.go:45`,
`cmd/dn-tool/unenroll.go:44` (paths as of today; after dt-w86c they live in
`internal/command`). Tests keep calling `New(cfg)` and get the silent transport
that `newRetryableClient:120-124` already supports for a nil logger.

### config.Config.LogLevel

**Keep the field.** It is round-tripped by the env-file writer and asserted by
`internal/config/marshal_test.go`, so deleting it would break `write-config`.
But it is **not** what configures the logger: the logger is built before config
loads, so it can report a config-loading failure. Change its default `info` →
`warn` (`internal/config/config.go:76`) so there is one default rather than two.
A written env-file stays useful because sourcing it puts `DN_LOG_LEVEL` in the
environment, where it is the documented fallback.

## Acceptance criteria

- `grep -rn 'slog.SetDefault' .` returns nothing.
- `grep -rn 'slog.Default()' --include='*.go' .` returns hits **only** inside
  `internal/logctx` (the `From` fallback).
- Every diagnostic in the call-site table still emits (except `run.go:257`,
  deliberately dropped), each verified by a test driving that path with an
  injected logger and asserting the record. **The `SetDefault` removal must not
  silence a single existing diagnostic.**
- One test per verbosity rule: `--log-level` validated (bad value → 64);
  `--quiet` beats `--log-level`; `DN_LOG_LEVEL` is used when no flag is given;
  a flag beats the env var; the default is `warn`.
- Handler selection tested both ways with a faked `IsTerminal`: text for a
  terminal sink, JSON otherwise.
- Records carry `command`, `version`, and `run_id`.
- No log record duplicates error-envelope content: the boundary must not both
  `EmitError` and log the same error.
- `--log-text` is gone from the flag surface and from all docs.
- Secrets: `internal/config/secret_test.go` redaction assertions still pass, and
  a test asserts that at `debug` level no stderr line contains a revealed
  secret. This mirrors `docs/specs/e2e-testing.md:114` and `:135`.
- Exit codes unchanged: `internal/output/exitcodes_test.go`,
  `cmd/dn-tool/exit_conformance_test.go`, `internal/run/signal_test.go`,
  `internal/run/run_test.go` pass.
- `internal/api/*_test.go` compile and pass **unchanged** (the functional-option
  shape is chosen precisely for this).
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. If a dependency is added for terminal
  detection, `go.mod`/`go.sum` and the Nix vendor hash are updated in the same
  change. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Tests that churn

- `cmd/dn-tool/logging_test.go` - asserts the `--log-text` wiring end to end
  (`:38-45`) through a captured `logOptions` (`:16-34`). Replaced wholesale by
  the verbosity, precedence, and handler-selection tests above. Do not simply
  delete it; its coverage must be replaced.
- `internal/config/config_test.go:142` - asserts the `LogLevel` default is
  `"info"`; becomes `"warn"`.
- `internal/output/log_test.go` - moves with the package.
- `internal/config/marshal_test.go` and `writefile_test.go` use `"info"` as
  fixture values; those are fixtures, not defaults, so they may stay, but check
  any that assert the **default**.

## Docs

- `docs/dn-tool-design.md` Req 7 (`:209-224`) - "structured JSON by default" and
  "WHERE human-readable logging is requested" both contradict destination-based
  handler selection (contradiction row 4).
- `docs/dn-tool-design.md:332` (§2.2 global flags) - replace `--log-text` with
  `--log-level` and `--quiet` (row 5).
- `docs/dn-tool-design.md:355` (§2.3) - `DN_LOG_LEVEL` default `info` → `warn`,
  and note it is the env **fallback** for the flag, not settable via
  `--env-file` (row 6).
- `docs/dn-tool-design.md:574` (§2.10) - "JSON handler default (JSON everywhere)"
  → TTY-adaptive (row 7).
- `docs/dn-tool-design.md:559` (§2.9) - `output/  # JSON result writer + slog
  setup` → the logger is constructed in the delegate via `logctx` (row 8).
- `docs/specs/e2e-testing.md:54` and `:108` - the `--log-text` references (rows 13,
  14).
- `README.md:22-23` - the global-flags line naming `--log-text` (row 12,
  logging half).
- `CHANGELOG.md`, Unreleased / Changed (breaking): `--log-text` removed;
  `--log-level` and `--quiet` added; default level `info` → `warn`; stderr logs
  are now text at a terminal and JSON otherwise.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Dependencies

Depends on dt-w86c only (the delegate is where the logger is constructed and
where `Deps.IsTerminal` lives). Independent of dt-2efx, dt-uq9d, dt-djf3,
dt-3xxb, so this is the best candidate to run in parallel with the terr and
envelope chain. Blocks dt-cjgq, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.
