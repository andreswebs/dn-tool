# 0005: Output contract

## Context

Tools in this project are agent-first, and the preceding ADRs (exit
codes, error handling, CLI structure, logging) all lean on a stream
contract they reference but do not define. This ADR defines it: what
stdout and stderr carry, the shape of results, errors, and warnings,
and how the tool describes its own contract at runtime.

## Decision

### Streams

- stdout carries exactly the machine-readable result and nothing
  else: no logs, no warnings, no usage banners, no subprocess noise
  (redirect child stdout to stderr when wrapping other programs).
- stderr carries everything else: the error envelope, warning lines,
  and diagnostic logs (per the logging ADR, which also forbids logs
  from duplicating envelope content).

### Result envelope

- Results are JSON by default; human-readable output is opt-in via a
  format flag. JSON is never colorized.
- Every result envelope opens with the same head: `schema_version`
  (an integer, bumped on breaking shape changes) and `ok` (a
  boolean), followed by the command-specific payload.
- Collections never serialize as `null`: absent lists are `[]`,
  absent maps are `{}`, enforced by the envelope's own `MarshalJSON`,
  not by call-site discipline.
- One newline-terminated JSON object per invocation is the default.
  Streaming and batch outputs may emit NDJSON, one self-contained
  object per line.

### Error envelope

- Failures render on stderr as an error envelope whose contract shape
  is JSON: `schema_version`, `ok: false`, and an `error` object with
  `code`, `message`, and optional `hint` and `details`, populated
  from the typed errors of the error-handling ADR.
- When the human format is selected the same content may render
  readably; the JSON shape remains the contract, and nothing appears
  on stdout on failure.

### Warnings

- Advisories that do not change the exit code render on stderr as
  NDJSON objects marked as warnings, one per line, distinct from log
  records. Libraries raise them through an injected callback and
  never write to the stream themselves.

### Self-description

- The tool ships a `schema` command. At tool level it reports the
  command surface, flags, declared exit codes, and the error
  inventory (code, exit code, hint); where envelopes vary per
  command, `schema --command CMD` adds that command's envelope shape.
- Schema output is a projection of the same declarations that drive
  behavior: the exit-code registry, the error registry, and the
  envelope structs. It is never hand-maintained, so it cannot drift.
- A flat single-command tool satisfies this with the minimal
  tool-level form.

## Consequences

- A consumer can discover the entire contract at runtime from one
  entry point: `schema` for the surface, one envelope head for
  results, one error shape for failures.
- `jq`-based pipelines never guard against `null` collections or
  scrape prose.
- The envelope head costs two fields on every result; in exchange,
  output shape changes are versioned and detectable instead of
  silent.
- The `schema` command costs little because the registries the other
  ADRs already mandate contain everything it reports.
