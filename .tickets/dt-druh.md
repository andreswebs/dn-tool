---
id: dt-druh
status: closed
deps: [dt-mhir, dt-kiqk]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-cmdg
tags: [config, write-config]
---
# Serialize config to env-file format

Serialize the resolved configuration to the same `KEY=VALUE` env-file format the `--env-file` loader consumes, so a written file round-trips back through [CFG.envfile](dt-kiqk.md). The actual file creation with `0600` permissions is [WC.perms](dt-meg1.md); this task owns the byte format.

## Public interface

```go
// internal/config
func Marshal(cfg *config.Config) ([]byte, error)  // env-file KV form; inverse of ParseEnvFile
```

Emit `DN_*` lines matching the §2.3 variable names. JSON-array fields (`DN_TAGS`, `DN_STATIC_ADDRESSES`) are written back as the JSON array string they were parsed from. Quote values that need it so the parser reads them back identically.

## Behaviors (TDD order)

1. **Round-trips** — `ParseEnvFile(Marshal(cfg))` yields the same effective config (the key property — test it directly).
2. **JSON arrays preserved** — tags/static-addresses serialize to valid JSON arrays that re-parse.
3. **Values needing quotes are quoted** — a value with spaces survives the round trip.
4. **Defaults vs set** — decide and test whether unset/default fields are emitted (recommended: emit the resolved values so the file is self-contained; document).

## Test strategy

Property-style round-trip test (`Marshal` → `ParseEnvFile` → compare) across representative configs. No filesystem here.

## Acceptance

- Output is valid env-file format that round-trips through the loader; arrays and quoted values preserved.

## References

- Design: [Req 10](../docs/dn-tool-design.md#10-configuration-file-writer), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables).
- Inverse of [CFG.envfile](dt-kiqk.md).

Parent epic: [dt-cmdg](dt-cmdg.md).

## Notes

**2026-06-06T22:30:36Z**

Implemented config.Marshal (internal/config/marshal.go) — inverse of ParseEnvFile/Resolve. Emits all §2.3 DN_* vars in fixed table order so the file is self-contained. Key decisions: (1) empty/nil JSON-array fields serialize to EMPTY value (DN_TAGS=), NOT '[]' — parseJSONArray('[]') yields a non-nil empty slice which breaks reflect.DeepEqual round-trip against a Load-produced nil; '' maps back to nil. (2) quote() wraps values in double quotes when ParseEnvFile would alter them (whitespace trimmed, or leading/trailing matching quote stripped); safe even with interior quotes since unquote strips only the single outermost layer (so JSON arrays containing spaces, e.g. tags with spaces, round-trip). (3) DN_API_KEY written in cleartext via Secret.Reveal() per dt-meg1 decision — this is a sanctioned Reveal site (secret.go names 'the env-file writer'); 0600 protection is dt-meg1's job, not here. (4) APITimeout via Duration.String() ('0s' round-trips to 0). Round-trip property tested directly (Marshal->ParseEnvFile->Resolve->DeepEqual). dt-meg1 now unblocked.
