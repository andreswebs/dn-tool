---
id: dt-69kx
status: open
deps: []
links: []
created: 2026-08-12T13:21:42Z
type: epic
priority: 1
assignee: Andre Silva
tags: [adr, conformance]
---
# ADR conformance adoption (ADRs 0002-0006)

Close dn-tool's remaining gaps against the six standardization ADRs in
`docs/adr/`. ADR 0001 (the exit-code taxonomy) is already conformant after
dt-7lys and must not regress; this epic covers 0002-0006.

The full plan, with verified `file:line` references and the reasoning behind
every decision, is at `.local/planning/adr-adoption.md`. Each child ticket is
self-contained, so an implementer does not need to read the plan.

## Children, in dependency order

| Ticket | Task |
| ------ | ---- |
| dt-w86c | Adopt the `internal/command` `Run(args, deps) int` delegate (ADR 0003). **First** |
| dt-2efx | Introduce `internal/terr` and the coded-error inventory (ADR 0002) |
| dt-djf3 | Result envelope head, `hostId` → `host_id` (ADR 0005) |
| dt-uq9d | Replace the `cli.ExitCoder` boundary with `ExitCodeFor` (ADR 0001/0002) |
| dt-nljm | JSON error envelope on stderr (ADR 0002/0005) |
| dt-3xxb | The `schema` command (ADR 0005) |
| dt-cr62 | Leveled logging posture, no `slog.SetDefault` (ADR 0004) |
| dt-cjgq | Golden-triple CLI harness (ADR 0006). **Last among code tickets** |
| dt-o9hi | Residual doc and code-comment contradictions. **Last** |

```txt
dt-w86c ──┬─> dt-uq9d ─> dt-nljm ─┐
dt-2efx ──┴─> dt-3xxb <───────────┼─> dt-cjgq ─> dt-o9hi
dt-djf3 ──────┘                   │
dt-cr62 ──────────────────────────┘
```

dt-w86c, dt-2efx, and dt-djf3 have no dependencies and can run in parallel;
dt-cr62 depends only on dt-w86c, so it parallels the whole terr/envelope chain.
Use `tk ready` rather than this diagram.

## Two defects fixed along the way

Both found while verifying the plan, both confirmed empirically 2026-08-12:

1. **`enroll` leaks the `dnclient` subprocess exit status.**
   `output.Classify(..., CodeIO)` at `internal/dnclient/client.go:79` is a no-op
   whenever the child ran and exited non-zero, because `*exec.ExitError`
   satisfies `cli.ExitCoder` and `Classify` defers to it. So `dnclient enroll`
   exiting 3 makes dn-tool exit 3, a code undeclared in
   `CommandExitCodes["enroll"]`, and `cli.HandleExitCoder` prints bare
   `exit status 3` with the context discarded. Fixed by dt-2efx (classification)
   and dt-nljm (message). `run`'s deliberate passthrough is unaffected.
2. **Unclassified paths silently take exit 70.** `write-config` with no path
   argument, the orphaned-enrollment guidance, and malformed API responses all
   surface as `internal_error`. Fixed by dt-2efx: 64, 78, and 65 respectively.

## Net consumer-visible changes

Breaking, pre-1.0, all requiring CHANGELOG entries in their owning ticket:

- `enroll` on a `dnclient` failure: the child's status N → 74 `dnclient_failed`
  with `details.exit_status`.
- `write-config` with no path: 70 → 64.
- Orphaned enrollment: 70 → 78.
- Malformed API response: 70 → 65.
- Bare `dn-tool`: help on stdout exit 0 → error envelope on stderr exit 64.
- `--log-text` removed; `--log-level` and `--quiet` added; default level
  `info` → `warn`; stderr logs text at a terminal, JSON otherwise.
- stdout results open with `schema_version` and `ok`; `hostId` → `host_id`.
- stderr failures render a JSON error envelope instead of plain text.
- New `schema` command.

## Must not regress

ADR 0001 conformance is already in place and is load-bearing for every ticket:
the constants at `internal/output/exit.go:14-25`, the per-command registry at
`internal/output/exitcodes.go:17-31`, usage and unknown-command mapping to 64,
graceful-shutdown 130/143, and daemon exit-status passthrough. The guard tests
are `internal/output/exitcodes_test.go`,
`cmd/dn-tool/exit_conformance_test.go`, `internal/run/signal_test.go`, and
`internal/run/run_test.go`.

## Quality gate (every ticket)

`make build` green from the repo root - `fmt-check`, `vet`, `lint`, `test`, then
compile - per `docs/agents/build-and-validation.md`. Do not silence lint with
`_ =`. TDD per `docs/agents/code-conventions.md`.

## Do not commit

Leave git commits and branching to the repo owner throughout.
