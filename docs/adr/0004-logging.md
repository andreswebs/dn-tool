# 0004: Logging

## Context

Tools in this project are agent-first: stdout carries exactly one
machine-readable result, and stderr carries everything else. Within
stderr, two kinds of content must not blur: the error envelope, which
is contract (see the error-handling ADR), and diagnostic log records,
which are commentary. This ADR fixes one logging standard with two
conformant postures, so the tool carries exactly as much logging
machinery as its work requires.

## Decision

### Invariants (both postures)

- Logging uses stdlib `log/slog` and writes only to the injected
  stderr stream, never stdout.
- Domain packages do not log. Problems surface as returned coded
  errors; diagnostics belong to the orchestration layer.
- Log records never duplicate error-envelope content. The envelope is
  the contract; logs add surrounding diagnostics or they add nothing.

### Choosing a posture

- A tool that talks to external systems or runs long (network I/O,
  daemons, subprocess lifecycles) carries the leveled logger described
  below.
- A single-shot computational tool may instead stay silent: coded
  errors are the failure channel, and advisories, when needed, flow
  through a machine-readable warning channel on stderr (injected as a
  callback, so libraries stay stream-blind). Such a tool does not
  scaffold logger construction, level flags, or handler selection.

Unused logging infrastructure is a defect under this ADR, not a
default: adopt the leveled posture when the need exists, not before.

### The leveled posture

- Verbosity: a validated `--log-level error|warn|info|debug` flag,
  default `warn`; a `--quiet` flag raising the floor to `error` and
  winning over `--log-level`; and the environment variable
  `<TOOL>_LOG_LEVEL` as fallback, with precedence: flags over
  environment over default.
- Handler: chosen by the destination. When stderr is not a terminal,
  records are JSON (agents and pipelines get parseable diagnostics);
  when it is, records are text (humans get readable ones). A tool may
  add `--log-format json|text` as an explicit override; the default
  requires no flag.
- Construction and travel: the delegate (`Run`, see the CLI-structure
  ADR) constructs the logger once, after flags are parsed, bound to
  the injected stderr writer, and carries it on `context.Context` via
  a small `logctx` package (`With`/`From`); `From` falls back to
  `slog.Default()` so plain unit tests need no setup. No library calls
  `slog.SetDefault`; process-global logger state is hostile to
  parallel in-process tests.
- Decoration: records carry `command` and `version` attributes; a
  run-correlation id is added when invocations are long-running or
  concurrent enough for interleaving to matter.

### Secrets

Any value that is a credential lives in a `Secret`-style type that
redacts on all four leak surfaces (`slog.LogValuer`, `fmt.Stringer`,
`fmt.GoStringer`, `json.Marshaler` all yield `REDACTED`), with the raw
value reachable only through an explicit, greppable `Reveal()` method.
This is mandatory wherever the tool handles credentials and
inapplicable elsewhere. Logging is where credentials leak first, which
is why the rule lives in this ADR.

## Consequences

- An agent piping the tool gets JSON diagnostics and a clean result
  stream; a human at a terminal gets readable text; neither configures
  anything.
- A silent-posture tool stays lean and carries no dead logging
  infrastructure; the cost is a posture decision, made once and
  recorded by what the code scaffolds.
- Context injection adds one tiny package and a retrieval call at use
  sites, in exchange for parallel-safe in-process tests and no global
  mutation.
- Secret-by-construction redaction cannot be forgotten at a call site,
  because the type, not the discipline, enforces it.
