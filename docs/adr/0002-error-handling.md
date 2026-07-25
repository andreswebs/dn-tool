# 0002: Error handling

## Context

Tools in this project are agent-first: errors are part of the machine
contract, alongside the exit-code taxonomy (see
`0001-exit-code-taxonomy.md`) and the JSON-on-stdout,
diagnostics-on-stderr stream separation. This ADR fixes one
error-handling model so every failure a consumer sees is classified,
consistent, and machine-actionable.

## Decision

### Core rules

- Errors are values. Functions return `error`; wrap with `%w` to
  preserve the chain; never format an error with `%v` inside
  `fmt.Errorf`. Messages are lowercase, carry no trailing punctuation,
  and include the offending input (quote it with `%q`): a message that
  names its input diagnoses itself.

- Single exit boundary. Commands and libraries return errors; exactly
  one `os.Exit` exists, in `main`, fed by the boundary's error-to-code
  mapping, so deferred cleanup always runs. No `panic` or `log.Fatal`
  in library code; panics are reserved for init-time programmer errors
  (for example duplicate registration), where crashing at startup is
  the correct outcome.

- Handle each error once. Return it, or handle it and degrade
  gracefully; never log and return the same error. Ignored errors are
  permitted only as explicit blank assignments on the sanctioned
  idioms (deferred `Close`/cleanup, best-effort writes to the stderr
  diagnostic sink), consistent with the lint policy that keeps every
  other unchecked error visible.

- Classify by identity, never by message. Callers and the exit
  boundary branch with `errors.Is`/`errors.As` on sentinels and typed
  errors. One sanctioned exception: the CLI framework's own parse
  errors carry no types, so classifying them by message substring is
  permitted at the boundary, isolated in a single function and covered
  by tests that fail when the framework's wording changes.

### Coded errors

- Every failure that can reach the user carries three things: a stable
  machine code (a `snake_case` string), an exit code from ADR 0001,
  and an optional remediation hint written for the consumer, not the
  developer. The boundary discovers them via `errors.As` against a
  single interface (`error` plus `Code()`, `ExitCode()`, `Hint()`),
  which is also how the process exit code is chosen.

- Sentinels are immutable package-level values. Per-invocation context
  attaches by copy: `Wrap(cause)` returns a copy carrying the cause
  (and exposing it via `Unwrap`), and structured payloads attach via a
  copying `WithDetails`-style method. A sentinel is never mutated
  after construction.

- Errors carry render-ready structure, computed at origin: positions,
  suggestions, expected/actual values, valid-value lists. The
  presentation layer renders these fields without re-deriving them. A
  type that needs per-instance structure delegates `Code`, `ExitCode`,
  `Hint`, and `Unwrap` to its class sentinel, so `errors.Is` against
  the sentinel still matches while the instance adds detail.

- Result and status are decoupled. When a command exits with a
  result-class code (1, or a documented sub-code in 2-63, per ADR
  0001), the full result envelope is still written to stdout first;
  the error value that carries the exit code prints nothing itself.

- Every sentinel's declaration documents why it carries its exit code,
  next to the code itself, so the classification survives review and
  refactoring.

### Conditional rules

- Concurrent fan-out: goroutines launched in an errgroup return nil
  and record per-item failures into pre-sized, index-addressed slots;
  the aggregated outcome, not the group error, drives the result and
  exit code. This keeps a discarded `g.Wait()` error correct instead
  of a bug.

- Where the tool exposes runtime self-description (a `schema`
  command), error codes are enumerable as data, populated at sentinel
  construction, so the documented error surface cannot drift from the
  real one.

This ADR fixes the behavioral contract, not a package. The
implementation of the coded-error interface lives in this repository;
conformance is judged against the rules above.

## Consequences

- Agents and scripts branch on code, exit class, and hint without
  parsing prose.
- Every failure path pays a small classification cost at its origin
  (code, exit, hint); unclassified errors surface as internal errors
  (exit 70), which keeps missing classifications visible instead of
  silent.
- The string-matching carve-out concedes that the CLI framework's
  parse errors are untyped; isolating it in one tested function keeps
  the concession from spreading.
- Immutable sentinels plus copy-on-write context make sharing
  sentinels across goroutines safe by construction.
