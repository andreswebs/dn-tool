---
id: dt-nljm
status: open
deps: [dt-uq9d]
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-69kx
tags: [adr, output, errors]
---
# Emit the JSON error envelope on stderr (ADR 0002/0005)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0005-output-contract.md`: "Failures render on stderr as an error
envelope whose contract shape is JSON: `schema_version`, `ok: false`, and an
`error` object with `code`, `message`, and optional `hint` and `details`,
populated from the typed errors of the error-handling ADR." Nothing appears on
stdout on failure.

dn-tool prints plain text via `cli.HandleExitCoder`.

## Current state (verified 2026-08-12 against commit bc8fa0b)

- `cmd/dn-tool/main.go:173-179` - `exitWithError`, delegating to
  `cli.HandleExitCoder` at `:178`, which prints the message to stderr and calls
  the overridable `cli.OsExiter`. After dt-w86c this lives in the delegate;
  after dt-uq9d the `HandleExitCoder` call is already gone and stderr rendering
  is whatever dt-uq9d left in place.
- No error envelope of any kind exists.
- Bare invocation: `cmd/dn-tool/main.go:81-86` (`rootAction`) prints the root
  help to **stdout** via `cli.ShowRootCommandHelp` and exits 0. Asserted at
  `cmd/dn-tool/exit_conformance_test.go:65-69`.

Verified side effect of the current design, worth knowing: `cli.HandleExitCoder`
prints the **innermost** `ExitCoder` that `errors.As` finds, not the outer
wrapper, so a `dnclient` failure today renders as bare `exit status 3` with the
`"dnclient enroll failed: "` context discarded. dt-2efx fixes the classification;
this ticket is what finally puts the full message on stderr.

## Target shape

Follow the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/output/output.go:45-88`.

```go
type errorEnvelope struct {
    SchemaVersion int         `json:"schema_version"`
    OK            bool        `json:"ok"`
    Error         errorDetail `json:"error"`
}

type errorDetail struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Hint    string `json:"hint,omitempty"`
    Details any    `json:"details,omitempty"`
}

func EmitError(w io.Writer, err error)
```

Behavior, matching the reference exactly:

- `code` and `hint` come from `errors.As` against `terr.Coded`; `details` from
  `terr.Detailed`.
- An unclassified error renders as `internal_error` so a missing classification
  stays visible rather than being silently mislabeled.
- `ok` is always `false`.
- The write is **best-effort**: a failure to report an error on stderr is
  unrecoverable, so it is not escalated over the error it describes. This is one
  of the sanctioned blank-assignment sites under ADR 0002 and
  `docs/agents/build-and-validation.md`'s no-`_ =`-to-silence-lint rule - the
  distinction is that here the ignored error is the sanctioned idiom, not a
  silenced lint.
- If marshalling fails (unmarshalable `details`), degrade to the envelope
  without `details`, which cannot fail to marshal.

Called from the delegate's boundary (`internal/command/run.go`), replacing
whatever dt-uq9d left there.

### Nothing on stdout on failure

Verify the framework's help-on-error path does not write to `deps.Out`.
`neutralize()` (added by dt-w86c, per the reference's
`internal/command/root.go:55-63`) is what disables help-on-error; confirm it
covers the subcommands too, since urfave v3 resolves these hooks per-command
without walking to the parent - the same quirk documented at
`cmd/dn-tool/main.go:149-151`.

### Bare invocation and help (decided 2026-08-12)

ADR 0005 bans usage banners on stdout. Resolution:

- **A bare `dn-tool` becomes a usage error**: return `ErrNoCommand`
  (`no_command`, 64, declared by dt-2efx), so stderr carries the error envelope
  with a hint pointing at `--help`, and stdout carries nothing. This replaces
  today's help-on-stdout-exit-0 at `cmd/dn-tool/main.go:81-86`. Mirrors the
  reference's `internal/command/root.go:67-72`.
- **Explicit `--help` and `--version` keep writing to stdout and exiting 0**, as
  a documented and tested exemption: a consumer asking for help is not asking
  for a result. Add a test that pins this exemption so it is a decision, not an
  accident.

`CommandExitCodes["dn-tool"]` (`internal/output/exitcodes.go:30`) is
**unchanged**: it already declares `{CodeOK, CodeUsage, CodeInternal}`, and 0
stays reachable through `--help`/`--version`.

## Acceptance criteria

- Every failure writes exactly one newline-terminated JSON object to stderr and
  **nothing** to stdout.
- A test asserts the envelope shape for at least one sentinel per exit class
  (64, 65, 69, 74, 78) plus the unclassified case.
- An unclassified error renders `"code":"internal_error"` and exits 70.
- A `dnclient` failure renders the full message (not bare `exit status N`) with
  `details.exit_status` present, closing the message-loss half of the dt-2efx
  defect.
- `hint` and `details` are omitted when empty, not emitted as `null`.
- Unmarshalable `details` degrades to a valid envelope instead of writing
  nothing.
- Bare `dn-tool` exits 64 with an error envelope on stderr and empty stdout.
- `dn-tool --help` and `dn-tool --version` still exit 0 with output on stdout,
  pinned by a test.
- `cli.HandleExitCoder` and `cli.OsExiter` appear nowhere in the tree.
- Exit codes otherwise unchanged: `internal/output/exitcodes_test.go`,
  `internal/run/signal_test.go`, `internal/run/run_test.go` pass.
- `cmd/dn-tool/exit_conformance_test.go:65-69` is updated from
  `bare invocation shows help and exits zero` to the new 64 behavior. This is
  the **only** assertion in that file whose expectation changes; the other four
  subtests keep theirs.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

- `docs/dn-tool-design.md:543` (§2.8) - "stderr: one JSON object per error"
  leaves the shape unfixed; replace with the concrete
  `{schema_version, ok: false, error: {code, message, hint, details}}`
  (contradiction row 10).
- `CHANGELOG.md`, Unreleased / Changed (breaking): failures now render a JSON
  error envelope on stderr instead of plain text; a bare `dn-tool` exits 64 with
  an envelope instead of printing help and exiting 0.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Dependencies

Depends on dt-uq9d (which owns the boundary this writes into), and transitively
on dt-w86c and dt-2efx. Blocks dt-cjgq, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.
