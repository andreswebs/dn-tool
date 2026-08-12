---
id: dt-3xxb
status: closed
deps: [dt-2efx, dt-djf3, dt-w86c]
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-69kx
tags: [adr, output]
---
# Add the schema command (ADR 0005)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0005-output-contract.md`: "The tool ships a `schema` command. At tool
level it reports the command surface, flags, declared exit codes, and the error
inventory (code, exit code, hint)." Crucially: "Schema output is a projection of
the same declarations that drive behavior: the exit-code registry, the error
registry, and the envelope structs. It is never hand-maintained, so it cannot
drift."

ADR 0002 pairs with this: "Where the tool exposes runtime self-description (a
`schema` command), error codes are enumerable as data, populated at sentinel
construction, so the documented error surface cannot drift from the real one."

## Current state (verified 2026-08-12 against commit bc8fa0b)

- No `schema` command. `newApp` registers only `install`, `enroll`, `unenroll`,
  `run`, `write-config` (`cmd/dn-tool/main.go:114-147`). After dt-w86c this tree
  lives in `internal/command/commands.go`.
- The declarations to project already exist:
  - `internal/output/exitcodes.go:17-31` - `CommandExitCodes`, a per-command
    map.
  - `internal/output/exitcodes.go:33-37` - `RunPassesThroughDaemonExitStatus`,
    the documented registry exception.
  - `terr.All()` - the error inventory, added by dt-2efx.
  - `internal/version.Current()` - the version string.
- `docs/dn-tool-design.md:317-325` (§2.2) documents a five-command surface;
  `README.md:14-20` has the same table.

## Target shape

Follow the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/output/envelopes.go:12-72`
and `internal/command/commands.go:36-82`.

```go
type SchemaEnvelope struct {
    Head
    Tool      string          `json:"tool"`
    Version   string          `json:"version"`
    Commands  []SchemaCommand `json:"commands"`
    ExitCodes []int           `json:"exit_codes"`
    Errors    []SchemaError   `json:"errors"`
}

type SchemaCommand struct {
    Name      string       `json:"name"`
    Usage     string       `json:"usage"`
    Args      string       `json:"args,omitempty"`
    Flags     []SchemaFlag `json:"flags"`
    ExitCodes []int        `json:"exit_codes"`
}

type SchemaFlag  struct { Name, Usage string }
type SchemaError struct { Code string; ExitCode int; Hint string }
```

### Null-coalescing MarshalJSON

This is where ADR 0005's rule lands (dt-djf3 deliberately added no coalescer,
having no collections). Per the reference's `envelopes.go:47-72`, give
`SchemaEnvelope` and `SchemaCommand` a `MarshalJSON` using the
`type alias …` trick so `Commands`, `ExitCodes`, `Errors`, and `Flags` serialize
as `[]` rather than `null` when absent. Enforced by the envelope, not by
call-site discipline.

### dn-tool specifics beyond the reference

The reference has a flat `ExitCodes []int`; dn-tool's registry is **per-command**.
So:

- `SchemaEnvelope.ExitCodes` carries the **union** of every command's declared
  codes, sorted ascending and de-duplicated.
- Each `SchemaCommand.ExitCodes` carries that command's own declared set from
  `CommandExitCodes`.
- `run` additionally carries the passthrough exception as **data**, not prose -
  a `passes_through_child_exit_status bool` field on `SchemaCommand`, fed from
  `RunPassesThroughDaemonExitStatus` (`internal/output/exitcodes.go:37`). A
  consumer must be able to discover that `run` may exit outside its declared set
  without reading prose.

`schema --command CMD` (the per-command envelope form) is **out of scope**: ADR
0005 makes it conditional on envelopes varying per command, and after dt-djf3 all
five commands share one `ResultEnvelope`. Add it when payloads diverge.

### Register schema's own exit codes

Add `"schema"` to `CommandExitCodes` (`internal/output/exitcodes.go:17-31`):
`{CodeOK, CodeUsage, CodeInternal, CodeIO}` - it can be misused (64), can fail
writing stdout (74), and carries the unclassified fallback (70). The `schema`
command itself must appear in its own projected command list.

## Acceptance criteria

- `dn-tool schema` emits exactly one newline-terminated JSON object on stdout,
  opening with `schema_version` and `ok` (it uses `Head` from dt-djf3).
- It reports: the tool name and version; all six commands (the five existing plus
  `schema`) with usage, args synopsis, and flags; the union exit-code list; each
  command's own exit-code list; the `run` passthrough flag; and every
  `terr.All()` entry with `code`, `exit_code`, and `hint`.
- Absent collections serialize as `[]`, never `null` - asserted by a test that
  marshals an envelope with nil slices.
- **Drift tests**, the point of the whole ADR requirement:
  - every command in the framework tree appears in the projection, and vice
    versa;
  - every code in `CommandExitCodes` appears in the union list, and the union
    contains nothing undeclared;
  - every sentinel registered via `terr.New` appears in the error inventory
    (count and codes match `terr.All()`).
- Global flags are reported (`--env-file`, `--assert-changed`, plus dt-cr62's
  logging flags if that has landed; if not, whatever the tree declares - the
  drift test is what keeps this honest either way).
- Exit codes unchanged for existing commands:
  `internal/output/exitcodes_test.go` and the conformance tests pass, with the
  registry extended by the `schema` entry.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

- `docs/dn-tool-design.md:317-325` (§2.2) - add `schema` to the command-surface
  table (contradiction row 11).
- `docs/dn-tool-design.md:231-233` (Req 8) - it cites
  `internal/output/exitcodes.go` as the registry, which stays true; add the
  cross-reference that `schema` is now the runtime projection of it
  (contradiction row 19).
- `README.md:14-20` - add `schema` to the command table (contradiction row 12,
  command-table half; the `--log-text` half is dt-cr62's).
- `CHANGELOG.md`, Unreleased / Added: the `schema` command.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Dependencies

Depends on dt-2efx (needs `terr.All()`), dt-djf3 (shares `Head` and `EmitJSON`),
and dt-w86c (the command tree lives in the delegate). Blocks dt-cjgq, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.

## Notes

**2026-08-12T15:37:04Z**

Implemented the schema command (ADR 0005) as a projection of live declarations.

Code:
- internal/output/schema.go (new): SchemaEnvelope / SchemaCommand / SchemaFlag / SchemaError, both container types with a null-coalescing MarshalJSON (type-alias trick, so the embedded Head still marshals first); AllExitCodes() (sorted, de-duplicated union of CommandExitCodes) and ErrorInventory() (terr.All() projection, registration order).
- internal/command/schema.go (new): the action. Reads cmd.Root() and projects name/usage/ArgsUsage/flags per command, so nothing about the surface is restated. Skips urfave's injected 'help' subcommand (framework scaffolding, no exit-code declaration); keeps --help/--version as flags since they are genuinely invocable. Emits via output.EmitJSON to root.Writer; a write failure maps to config.ErrFileIO (74).
- internal/output/exitcodes.go: added "schema": {0, 64, 70, 74}.
- internal/command/commands.go: registered the command; extracted runCommandName so the tree and the passthrough marker cannot disagree.

Contract: {schema_version, ok, tool, version, commands[], global_flags[], exit_codes[], errors[]}. Each command carries its own exit_codes plus passes_through_child_exit_status (always emitted, true only for run, fed from RunPassesThroughDaemonExitStatus). exit_codes at tool level is the union [0,1,64,65,69,70,74,78,130,143]. All 25 registered sentinels appear with code/exit_code/hint.

schema --command CMD stayed out of scope per the ticket: all commands share one ResultEnvelope.

Tests (TDD, red first): internal/output/schema_test.go covers nil-collection coalescing at both levels, head-first order, args omission, the union's sortedness/completeness, and the inventory projection. internal/command/schema_test.go drives the real CLI and holds the drift tests: tree-vs-projection both ways, per-command registry match, union match, passthrough only on run, every terr.All() sentinel present (with a cross-package spot check), global and command-scoped flags, and the unwritable-stdout -> 74 path.

Two existing tests moved with the new command: output/exitcodes_test.go's covered-commands list gained "schema", and command/root_test.go's TestNewRoot_HasFiveSubcommands became HasSixSubcommands.

Docs: design §2.2 table, design Req 8 (schema is the registry's runtime projection), README command table, CHANGELOG Unreleased/Added, docs/specs/learnings.md. Note markdownlint-cli2 could not use ~/.markdownlint.yaml (absent in this environment); the new prose wraps at 80 and reports no MD013 errors, while the pre-existing 48 in learnings.md were left untouched.

make build green. Not committed, per repo policy. Next in the epic: dt-cjgq (the golden-triple harness should pin a schema triple), then dt-o9hi.
