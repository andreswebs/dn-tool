---
id: dt-9x3y
status: closed
deps: [dt-mhir, dt-kiqk]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-uzx6
tags: [config, env]
---
# Config precedence: live env over env-file

Define and implement the single, documented precedence order between an `--env-file` and the live environment: **live environment variables override values loaded from the env-file** (design §2.3). Wire `CFG.load` (dt-mhir) and `ParseEnvFile` (dt-kiqk) together into the final resolved `Config`.

## Public interface

```go
// internal/config
func Resolve(fileVars map[string]string, getenv func(string) string) (*Config, error)
// effective lookup: getenv(key) if set, else fileVars[key], else default
```

Build a merged lookup closure (live env wins over file) and feed it to the same defaulting logic from `CFG.load`. Keep the merge layer thin — it only chooses the source per key; defaults/typing live in their own tasks.

## Behaviors (TDD order)

1. **Key only in env-file** — file value used.
2. **Key only in live env** — env value used.
3. **Key in both** — **live env wins** (the documented rule).
4. **Key in neither** — default applies (delegates to CFG.load defaulting).
5. **Empty live value vs set file value** — decide and test the rule: an explicitly empty `DN_X=` in the live env is "set" and wins (document this so it isn't surprising).

## Test strategy

Table-driven over `(fileVars, envVars) -> expected field`. Inject `getenv`. Assert via resolved `*Config`.

## Acceptance

- Precedence is exactly "live env > env-file > default", tested for all four cells.
- The empty-string edge case is decided and documented in the godoc + README.

## References

- Design: [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables) ("live environment variables override values loaded from `--env-file`").

Parent epic: [dt-uzx6](dt-uzx6.md).

## Notes

**2026-06-06T19:00:56Z**

Implemented config.Resolve(fileVars, getenv) + testable resolve(...,hostname) core in internal/config/resolve.go, mirroring the Load/load split (dt-mhir). Merge is a thin per-key source chooser feeding load's existing defaulting/typing: merged(key)=getenv(key) if non-empty else fileVars[key]. Precedence: live env > env-file > default, all 4 cells table-tested (resolve_test.go). EMPTY-LIVE DECISION: the fixed getenv func(string)string signature cannot distinguish DN_X= (set empty) from unset (both ''), so 'empty live wins' from the ticket is not expressible; decided empty live FALLS THROUGH to the env-file (then default) and documented this in the resolve.go godoc + a new README 'Configuration precedence' section. No new deps; make build green.
