---
id: dt-o9hi
status: open
deps: [dt-w86c, dt-2efx, dt-uq9d, dt-djf3, dt-nljm, dt-3xxb, dt-cr62, dt-cjgq]
links: []
created: 2026-08-12T13:21:43Z
type: task
priority: 3
assignee: Andre Silva
parent: dt-69kx
tags: [adr, docs]
---
# Reconcile residual design-doc and code-comment contradictions

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/dn-tool-design.md` is the authoritative spec
(`docs/agents/code-conventions.md`: "if code and the design disagree, reconcile
the design first"), and it still describes pre-ADR behavior in 19 identified
places. The epic's rule is that **each doc edit rides with the ticket that
changes the corresponding code**, so the doc and code land conformant together.

This ticket is the **residual**: the rows no code ticket touches, plus a final
sweep proving nothing was missed. It runs last, after every other ticket in the
epic.

## Rows owned by this ticket

Verified 2026-08-12 against commit bc8fa0b; re-read each before editing, since
the earlier tickets will have shifted line numbers.

| # | Location | Reads today | Fix |
| - | -------- | ----------- | --- |
| 1 | `docs/dn-tool-design.md:231` | "exit codes follow the family-wide taxonomy in ADR 0001" | Drop "family-wide"; ADR 0001 uses no "family" wording anywhere |
| 2 | `CHANGELOG.md:11` | "Exit codes now follow the family-wide taxonomy in ADR 0001." | Same. Edit the historical entry's wording only; do not rewrite its substance |
| 3 | `internal/output/exit.go:9` | doc comment: "for the whole CLI family" | Same. **Check first**: dt-uq9d rewrites this file and may already have fixed it |
| 16 | `docs/dn-tool-design.md:369` (§2.4 table) | "No-op, 'already enrolled'. Exit 0 (or 2 w/ `--assert-changed`)" | Exit 1, not 2 - stale since the dt-7lys migration |
| 17 | `internal/enroll/state.go:41-42` | doc comment: "Changed=false (exit 0, or exit 2 under `--assert-changed`)" | Same. **Check first**: dt-2efx touches this file and may already have fixed it |
| 18 | `docs/dn-tool-design.md:547-570` (§2.9 tree) | marked "tentative"; omits `internal/{run,unenroll,dnstate,dninstall,version,output}`, and now also `internal/{command,terr,logctx}`; shows download/verify/install under `dnclient/` where the code has `dninstall/` plus `dnclient/` | Redraw against the real tree. Either drop the "tentative" marker and keep it current, or state plainly that it is illustrative - do not leave it silently wrong |

Rows 4-15 and 19 belong to dt-cr62 (4-8, 12-14), dt-djf3 (9, 15), dt-nljm (10),
and dt-3xxb (11, 12, 19). **Verify each landed** rather than re-fixing it.

## The final sweep

The scan behind the 19-row inventory covered `docs/`, `README.md`,
`CHANGELOG.md`, and Go doc comments. `docs/research/` was deliberately **out of
scope**: it is a historical record of the defined.net API and the upstream
analysis, not a statement about dn-tool's own contract. `docs/specs/learnings.md` was
checked and carried no contradiction, though `:1106` mentions the old `hostId`
spelling as historical record and should be left alone. Closed tickets under
`.tickets/` are likewise historical record - do not rewrite them.

Grep sweep, which must come back clean (excluding `.tickets/`, `docs/research/`,
`docs/specs/learnings.md`, and `.local/`):

- `family` - no "family-wide taxonomy" anywhere, in docs or code comments.
- `log-text` - gone entirely.
- `JSON handler default`, `slog setup` - gone.
- `hostId` - only in historical records.
- `exit 2`, `exit \`2\`` - no reference to the old assert-no-op code.
- the flat result example `{"action":` without a `schema_version` sibling.
- a five-command surface with no `schema` in any command table.

Anything the sweep turns up that is not already in the 19 rows is in scope here:
record it with `tk add-note` on this ticket, then fix it.

## One extra item: a link broken by the docs/specs move

`docs/e2e-testing.md` and `docs/learnings.md` moved to `docs/specs/` in the
working tree on 2026-08-12 (uncommitted at the time this ticket was written).
The move left one broken relative link behind:

- `docs/specs/e2e-testing.md:55` links to `adr/0001-exit-code-taxonomy.md`,
  which resolved from `docs/` but not from `docs/specs/`. It needs
  `../adr/0001-exit-code-taxonomy.md`.

Check the rest of both moved files for the same class of breakage (any
`](adr/`, `](agents/`, `](research/`, or `](dn-tool-design.md)` link) and fix
each. No other tracked file in the repo referenced the old paths, so nothing
else needs repointing; the references in closed tickets are historical record
and stay as they are.

## Acceptance criteria

- Every row above is fixed, and rows 4-15 and 19 are confirmed landed with their
  owning tickets (spot-check each, do not assume).
- The grep sweep above returns clean.
- No in-repo document or code comment still describes: the old exit-2 assert
  no-op, plain-text errors, `--log-text`, JSON-everywhere logging, ambient slog
  setup in `output/`, a headless flat result, `hostId`, or a command surface
  without `schema`.
- `docs/dn-tool-design.md` §2.9's structure tree matches the real tree, or says
  clearly that it is illustrative.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <file>` clean on every
  markdown file touched. If the repo has no local markdownlint config, use
  `~/.markdownlint.yaml`.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md` - code comments are touched here, so the
  gate still applies.

## Documentation conventions

Per the repo's authoring rules: no em-dashes, no emojis, and no references to
local filesystem paths outside this project. Documentation must read as
standalone and publishable.

## Dependencies

Depends on every other ticket in the epic: dt-w86c, dt-2efx, dt-uq9d, dt-djf3,
dt-nljm, dt-3xxb, dt-cr62, dt-cjgq. Blocks nothing.

## Do not commit

Leave git commits and branching to the repo owner.
