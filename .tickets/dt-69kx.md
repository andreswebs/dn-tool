---
id: dt-69kx
status: closed
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

## Notes

**2026-08-12T15:56:31Z**

Verify-and-close (zero code change). All nine children were already closed; this epic's terminal action was to confirm every acceptance bullet against reality rather than to write new code.

Verified empirically on the freshly built binary (bin/dn-tool-linux-arm64):
- Bare 'dn-tool' -> exit 64, JSON error envelope on stderr, code no_command.
- 'write-config' with no path -> exit 64, code missing_argument (was 70).
- Unknown command -> exit 64, code unknown_command.
- 'dn-tool schema' -> exit 0, one JSON object opening with schema_version + ok, listing all six commands with their flags, declared exit codes and passes_through_child_exit_status, the global flag set, and the full 26-entry coded-error inventory with exit_code + hint per code.
- Global flags: --log-level error|warn|info|debug (default warn) and --quiet present; --log-text absent.

Verified by grep across the tree (excluding .tickets/, .local/ and docs/research/, whose historical references are expected):
- No slog.SetDefault call site remains in Go source; only comments in logctx/dninstall/api/run/dnclient tests documenting that its removal must not silence those diagnostics.
- No hostId identifier or literal in non-test Go source; the rename to host_id is complete.
- No output.Classify call site; the ExitCoder boundary is gone, the sole surviving ExitCoder mention is the regression comment at internal/dnclient/client_test.go:131 recording defect 1.
- Both epic defects are fixed and pinned: enroll's dnclient failure classifies as 74/dnclient_failed with details.exit_status, and the three unclassified-70 paths now map to 64/78/65.

ADR 0006 harness in place: internal/command/testdata holds the golden-triple scenarios (bare-invocation, bad-global-flag, bad-log-level, bad-subcommand-flag, enroll-{success,orphaned,dnclient-failure,api-rejected,api-unavailable,api-malformed-response}, and more), with 87 RUN/PASS lines across the golden, conformance, schema, envelope and logging suites.

ADR 0001 non-regression holds: the guard tests (internal/output/exitcodes_test.go, internal/command/exit_conformance_test.go, internal/run/signal_test.go, internal/run/run_test.go) all pass; note exit_conformance_test.go now lives under internal/command/, moved there by dt-w86c when the Run(args, deps) delegate absorbed the root command.

CHANGELOG Unreleased already carries every breaking entry this epic promised, written by the owning child tickets; no CHANGELOG edit was needed here.

make build green from the repo root (fmt-check, vet, lint 0 issues, test, compile). No files changed, nothing committed.
