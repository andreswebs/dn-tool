---
id: dt-toqi
status: closed
deps: [dt-mhir]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-uzx6
tags: [config, validation]
---
# Typed config fields + validation (JSON arrays, bools, port)

Parse and validate the typed config fields: JSON-array vars (`DN_TAGS`, `DN_STATIC_ADDRESSES`), booleans (`DN_IS_LIGHTHOUSE`, `DN_IS_RELAY`), the port (`DN_LISTEN_PORT`), and the timeout (`DN_API_TIMEOUT`). Surface clear errors on malformed values. This supersedes upstream **S2** (`: ${VAR:-}` no-ops that never set defaults) and **S6** (redundant `-v` checks) with real parsing + validation.

## Public interface

Extends the `CFG.load` defaulting: each raw string is parsed into its typed field during `Load`/`Resolve`. Parsing helpers live in `internal/config` (unexported is fine; they're exercised through `Load`). Per-command **required-field** validation (enroll needs API key/network/role) is **not** here — it lives in the command tasks ([ENR.request](dt-j2ab.md)); this task owns only type/format validity.

## Behaviors (TDD order)

1. **JSON array parses** — `DN_TAGS='["a","b"]'` → `[]string{"a","b"}`; empty/unset → `nil`/`[]`.
2. **Invalid JSON array errors clearly** — `DN_TAGS='not json'` → error naming the variable.
3. **Booleans parse** — `true/false` (and document accepted forms); invalid → clear error.
4. **Port parses and bounds** — `DN_LISTEN_PORT=4242` → int; unset → 0; non-numeric / out-of-range → error.
5. **Timeout parses** — `DN_API_TIMEOUT` accepts a Go duration (e.g. `30s`) or documented unit; invalid → error. (Defaults: ~30s enroll / ~10s unenroll per §2.3 — the per-command default is applied by the command, this task validates the override format.)

## Test strategy

Table-driven `(raw value) -> (typed | error)`, asserted through `Load`. Cover the empty/unset case for each.

## Acceptance

- All typed fields parse valid input and reject invalid input with a variable-named error.
- No silent fallback that hides a malformed value (the S2/S6 failure mode).

## References

- Design: [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables).
- Supersedes upstream: [**S2 / S6**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).

Parent epic: [dt-uzx6](dt-uzx6.md).

## Notes

**2026-06-06T18:48:53Z**

Extended config.load with JSON-array parsing (DN_TAGS, DN_STATIC_ADDRESSES via encoding/json -> []string; empty/unset -> nil; invalid -> error naming the var) and port bounds-checking in parsePort (0-65535, 0=auto, out-of-range/negative/non-numeric error). Bools (strconv.ParseBool) and timeout (time.ParseDuration) were already parsed in dt-mhir; added explicit bounds + array coverage here. All behaviors table-driven, asserted through load(). Updated stale struct comments that said 'parsed in dt-toqi'. Unblocks dt-j2ab (enroll request building maps Tags/StaticAddrs/ListenPort into CreateHostRequest).
