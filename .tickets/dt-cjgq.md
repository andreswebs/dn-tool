---
id: dt-cjgq
status: open
deps: [dt-w86c, dt-djf3, dt-nljm, dt-3xxb, dt-cr62]
links: []
created: 2026-08-12T13:21:43Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-69kx
tags: [adr, testing]
---
# Add the golden-triple CLI harness (ADR 0006)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0006-cli-testing.md` requires an in-process golden-triple harness:
scenarios invoke the delegate (`Run(args, deps)`) with buffer streams and compare
stdout, stderr, and the exit code against golden files; an `-update` flag
regenerates them; content is normalized for determinism; and the conformance
obligations of ADRs 0001, 0002, and 0005 are expressed **through** this harness.
Because scenarios touch only the delegate contract, the suite is framework-blind
and survives a framework replacement.

This ticket runs **last** among the code tickets: goldens freeze the output of
dt-djf3, dt-nljm, dt-3xxb, and dt-cr62, so writing them earlier means
regenerating repeatedly.

## Current state (verified 2026-08-12 against commit bc8fa0b)

- No golden harness. Confirmed by grep: **zero** `testdata` directories anywhere
  in the tree, and no test declares an `-update` flag or any `flag.Bool`.
- Existing CLI-level tests, which are **kept, not replaced** (ADR 0006 puts the
  golden suite alongside unit and conformance tests):
  `cmd/dn-tool/exit_conformance_test.go`, `exit_test.go`, `main_test.go`,
  `logging_test.go`, `assert_changed_test.go`, `enroll_test.go`,
  `unenroll_test.go`, `run_test.go`, `write_config_test.go`. All move to
  `internal/command` with dt-w86c.
- `internal/output/exitcodes_test.go` asserts the API/network classes on
  **constructed** errors, not through the CLI - which is exactly the gap the
  golden scenarios close for 65, 69, and 74.
- `internal/run/signal_test.go` covers the 130/143 mapping at the unit level.
- `docs/specs/e2e-testing.md` describes an exec-based container suite.

## Target shape

Follow the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/command/golden_test.go`
and its `testdata/<scenario>/{stdout,stderr,exit}` layout.

- `package command_test`, driving `command.Run(args, command.Deps{…buffers…})`.
- `var update = flag.Bool("update", false, "rewrite golden files")`; regenerate
  with `go test ./internal/command -update`.
- Per scenario, compare three artifacts: `stdout`, `stderr`, `exit`.
- `TestMain` enforces registry coverage on a full, non-updating run (skip the
  check when `-update` or a `-run` filter is in effect, since a filter
  legitimately exercises a subset).
- `runScenario` records every observed exit and asserts registry membership.

### Normalization is load-bearing

The ADR names under-normalization as the harness's main failure mode, and
dn-tool needs considerably more of it than the reference's cwd-and-CRLF pair:

- the working directory and any `t.TempDir()` path (the config and
  `write-config` scenarios);
- **the version string** - `internal/version.Current()` is build-injected and
  appears both in the `schema` envelope (dt-3xxb) and in every log record's
  `version` attribute (dt-cr62);
- **the `run_id`** - dt-cr62 adds it to every record unconditionally;
- log record **timestamps**, for any scenario that emits a record;
- `httptest` **ports** and URLs, for the API-backed scenarios;
- CRLF line endings.

Replace each with a stable token (`$CWD`, `$VERSION`, `$RUN_ID`, `$TIME`,
`$PORT`). Add a test that runs the same scenario twice and asserts byte-identical
goldens, so a missed volatile value fails loudly instead of flaking later.

### Scenarios

At minimum, covering every declared exit code reachable in-process:

| Scenario | Expected |
| -------- | -------- |
| each of the five commands' success path | 0, result envelope with the head |
| `--assert-changed` on a no-op | 1, result envelope still on stdout first |
| unknown command | 64 |
| bad global flag | 64 |
| bad subcommand flag | 64 |
| bare invocation | 64 (per dt-nljm) |
| `write-config` with no path | 64 (per dt-2efx) |
| bad `--log-level` value | 64 (per dt-cr62) |
| malformed `DN_*` value | 78 |
| missing `DN_API_KEY` on unenroll | 78 |
| orphaned enrollment | 78 (per dt-2efx) |
| API 4xx | 65 |
| API 5xx or transport failure | 69 |
| malformed API response body | 65 (per dt-2efx) |
| checksum mismatch | 65 |
| local I/O failure (unwritable path) | 74 |
| `dnclient` exiting non-zero | 74 with `details.exit_status` (per dt-2efx) |
| `schema` | 0 |
| `--help`, `--version` | 0, stdout (the pinned ADR 0005 exemption) |

The API-backed and filesystem-backed scenarios are what finally exercise 65, 69,
and 74 **through the CLI** rather than on constructed errors.

### The out-of-process codes

130, 143, and the daemon passthrough N cannot be reached in-process. ADR 0006's
conditional exec-based clause covers them, and coverage already exists:
`internal/run/signal_test.go` (signal mapping) and `internal/run/run_test.go`
(passthrough), plus the container suite in `docs/specs/e2e-testing.md`.

Exempt them from the in-process registry-coverage check **explicitly and
visibly**, mirroring how `RunPassesThroughDaemonExitStatus`
(`internal/output/exitcodes.go:33-37`) documents the registry's own exception.
A silent exemption would make the coverage check read as "everything is
covered" when it is not. `log()` or comment the exemption where a reader of the
test will see it, and name the tests that do cover those codes.

## Acceptance criteria

- `go test ./internal/command -update` regenerates goldens; a plain run
  immediately afterward is green.
- Goldens are deterministic: the same scenario run twice produces byte-identical
  output, asserted by a test. No absolute path, port, timestamp, version string,
  or run id appears in any golden file.
- Every scenario asserts its exit code is a member of the declared registry for
  that command (`internal/output/exitcodes.go`).
- `TestMain` fails when a declared, in-process-reachable code is not exercised
  by any scenario.
- The 130/143/passthrough exemption is explicit in the test source and names the
  tests that do cover those codes.
- Error scenarios assert the ADR 0005 envelope shape on stderr and **empty
  stdout**.
- The existing exit-conformance, behavior, and scenario tests are **all still
  present and passing**; this suite is added alongside them, not in place of
  them.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. Confirm `make test-race` is also green,
  since the harness runs scenarios in-process and dt-cr62's whole point was
  removing process-global logger state. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

- `docs/specs/e2e-testing.md` - add a short section distinguishing the in-process
  golden suite (this ticket) from the exec-based container suite, and note which
  exit codes each owns. The exit-code table at `:55` should stay accurate.
- `CHANGELOG.md` - no consumer-visible change; skip unless the goldens reveal
  one.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Dependencies

Depends on dt-w86c (drives `Run`), dt-djf3 (result envelope), dt-nljm (error
envelope), dt-3xxb (the `schema` scenario), dt-cr62 (log output lands in golden
stderr). Blocks dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.
