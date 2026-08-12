---
id: dt-djf3
status: open
deps: []
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-69kx
tags: [adr, output]
---
# Add the result envelope head and rename hostId to host_id (ADR 0005)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0005-output-contract.md`: "Every result envelope opens with the same
head: `schema_version` (an integer, bumped on breaking shape changes) and `ok`
(a boolean), followed by the command-specific payload." Collections never
serialize as `null`, enforced by the envelope's own `MarshalJSON` rather than by
call-site discipline. One newline-terminated JSON object per invocation.

dn-tool emits a flat object with no head.

## Current state (verified 2026-08-12 against commit bc8fa0b)

- `internal/output/result.go:14-19` - the `Result` struct:

  ```go
  type Result struct {
      Action  string `json:"action"`
      Changed bool   `json:"changed"`
      HostID  string `json:"hostId,omitempty"`
      Network string `json:"network,omitempty"`
  }
  ```

- `internal/output/result.go:24-27` - `WriteResult`, a bare
  `json.NewEncoder(w).Encode(r)`.
- Written to stdout at `cmd/dn-tool/main.go:48` inside `withResult` (this call
  site moves to `internal/command` with dt-w86c).
- `Result` is constructed at: `internal/dninstall/install.go:87`, `:93`;
  `internal/enroll/state.go:61`, `:110-113`;
  `internal/unenroll/unenroll.go:87-90`; `cmd/dn-tool/write_config.go:39`. It is
  also returned through `internal/run.Deps`' closures
  (`internal/run/run.go:62-66`).

## Target shape

Follow the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/output/output.go:15-43`.

In `internal/output`:

```go
// SchemaVersion is the version of the output contract, bumped on breaking
// shape changes to any envelope.
const SchemaVersion = 1

// Head opens every result envelope: embed it as the first field of each
// command's envelope struct.
type Head struct {
    SchemaVersion int  `json:"schema_version"`
    OK            bool `json:"ok"`
}

func OKHead() Head { return Head{SchemaVersion: SchemaVersion, OK: true} }

// EmitJSON marshals v and writes it to w followed by a newline.
func EmitJSON(w io.Writer, v any) error
```

### One shared envelope, not five

Decision recorded 2026-08-12. All five commands emit the same `Result` payload,
so five per-command envelope structs would be five identical types:

```go
type ResultEnvelope struct {
    Head
    Result
}
```

`encoding/json` inlines embedded structs at their declaration position, so the
emitted object is
`{"schema_version":1,"ok":true,"action":…,"changed":…,"host_id":…,"network":…}`
with the head first. **Assert the key order in a test** rather than trusting
it. Split into per-command envelopes when payloads actually diverge.

**No `MarshalJSON` is added for `ResultEnvelope`.** It carries no collection, and
a no-op coalescer would be precisely the unused infrastructure ADR 0004 calls a
defect. The null-coalescing mechanism lands with the `schema` envelope
(dt-3xxb), which does carry collections.

### The tag rename

`internal/output/result.go:17` `hostId` → `host_id`.

Ownership was verified 2026-08-12; the rename touches **no external contract**.
Three distinct spellings are in play and `hostId` is dn-tool's own:

| Where | Spelling | Owner |
| ----- | -------- | ----- |
| defined.net API `Host` object | `id` | defined.net (`internal/api/endpoints.go:34`) |
| `dnclient.yml` local state | `host_id` | the proprietary dnclient (`internal/dnstate/hostid.go:42`) |
| dn-tool's stdout result | `hostId` | **dn-tool alone** (`internal/output/result.go:17`) |

defined.net never uses `hostId`: on the `Host` object the field is plain `id`,
and its camelCase fields are `networkID`, `roleID`, `ipAddresses`, and so on.
**Do not touch** `internal/api/endpoints.go:19-67` or
`internal/api/pagination.go:21-22` - those tags are the defined.net wire format.
The *value* `Result.HostID` carries is a defined.net identifier; the *field
name* is not. Renaming also makes dn-tool agree with `dnclient.yml`'s own
spelling, which it currently contradicts.

Live consumers of the old spelling, all in scope here:
`internal/output/result.go:17`, `internal/output/result_test.go:11`, `:27`,
`:79-80`, and `docs/dn-tool-design.md:542`. `docs/specs/e2e-testing.md`'s `jq`
assertions do not reference it. Closed tickets under `.tickets/` and
`docs/specs/learnings.md:1106` mention it as historical record - **leave them alone**.

## Acceptance criteria

- Every command's stdout is exactly one newline-terminated JSON object opening
  with `schema_version` then `ok`, asserted by a key-order test.
- `WriteResult` either routes through `EmitJSON` with the envelope or is
  replaced by it; no caller can emit a headless result.
- `host_id` replaces `hostId`; `omitempty` behavior is unchanged (absent when
  empty).
- Nothing else appears on stdout on success, and nothing at all on failure.
- `internal/output/result_test.go` updated: the documented field/tag shape
  (`:11`), the single-object-plus-one-newline assertion, and the `omitempty`
  assertions (`:79-80`).
- `--assert-changed` still writes the full result envelope to stdout **before**
  returning the result-class exit code, per ADR 0002 ("the full result envelope
  is still written to stdout first"). `cmd/dn-tool/assert_changed_test.go` must
  still pass.
- Exit codes unchanged: `internal/output/exitcodes_test.go` and
  `cmd/dn-tool/exit_conformance_test.go` pass.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

Both land with this ticket, per the epic's rule that a doc edit rides with the
code change that makes it true:

- `docs/dn-tool-design.md:541-542` (§2.8) - the flat example
  `{"action":"enroll","changed":true,"hostId":"host-…","network":"defined"}`
  becomes the envelope shape with the head and `host_id` (contradiction rows 9
  and, for the tag, the rename above).
- `docs/specs/e2e-testing.md` - the `jq` assertions and result-shape references at
  `:108`, `:119`, `:206-216`, `:229` (contradiction row 15).
- `CHANGELOG.md`, Unreleased / Changed (breaking): stdout results now open with
  `schema_version` and `ok`; `hostId` renamed to `host_id`.
- Run `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Dependencies

None; can run in parallel with dt-w86c and dt-2efx. If dt-w86c has not landed,
the stdout write site is `cmd/dn-tool/main.go:48`; if it has, it is the moved
`withResult` in `internal/command`. Blocks dt-3xxb, dt-cjgq, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.
