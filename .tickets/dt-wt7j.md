---
id: dt-wt7j
status: closed
deps: [dt-jbme]
links: []
created: 2026-06-07T20:31:38Z
type: chore
priority: 4
assignee: Andre Silva
parent: dt-uzx6
tags: [config, refactor, architecture]
---
# Collapse three per-command timeout helpers into Config.Timeout(fallback)

Replace the three identical enrollTimeout/unenrollTimeout/installTimeout cmd helpers (differing only in fallback constant) with one Config.Timeout(fallback) method; each command passes its own default. Surfaced by an architecture review (deepening candidate 6, speculative/smallest).

## Problem

Three byte-for-byte identical helpers differ only in their fallback constant:

- `cmd/dn-tool/enroll.go:52` — `enrollTimeout(cfg)` → `defaultEnrollTimeout` (30s)
- `cmd/dn-tool/unenroll.go:59` — `unenrollTimeout(cfg)` → `defaultUnenrollTimeout` (10s)
- `cmd/dn-tool/install.go:58` — `installTimeout(cfg)` → `defaultInstallTimeout` (60s)

Each is `if cfg.APITimeout > 0 { return cfg.APITimeout }; return default`. Deletion test: collapse them and the `>0 ? : fallback` logic concentrates in one place.

## Decision (scoped via architecture-review interview)

**Method on `Config`.** Centralizes the "APITimeout == 0 means unset" convention with the field that owns it (documented at `config.go:27`); each command keeps and passes its own per-command default. Chosen over a cmd-package `resolveTimeout` helper, which would leave the zero-means-unset knowledge in the command layer rather than next to the field.

## Design

```go
// config package — Timeout returns DN_API_TIMEOUT when set, else fallback. A
// zero APITimeout means unset, so the command supplies its own per-command
// default (design §2.3).
func (c *Config) Timeout(fallback time.Duration) time.Duration {
	if c.APITimeout > 0 {
		return c.APITimeout
	}
	return fallback
}
```

Update the four call sites and delete the three helpers:

- `enroll.go:40` → `context.WithTimeout(ctx, cfg.Timeout(defaultEnrollTimeout))`
- `unenroll.go:49` → `cfg.Timeout(defaultUnenrollTimeout)`
- `install.go:43` → `cfg.Timeout(defaultInstallTimeout)`
- `cmd/run.go:53` → `UnenrollTimeout: cfg.Timeout(defaultUnenrollTimeout)`

The three `default*Timeout` constants stay in their command files (genuinely per-command) and become the `fallback` argument.

## Out of scope

`internal/run`'s `unenrollTimeout(deps)` is the same pattern but a different seam — it resolves `run.Deps.UnenrollTimeout` (a `time.Duration` field), not a `Config`. Folding it in would couple `run` to `config` or force a shared duration util; not worth it for a 3-line function. Left untouched.

## Where

- `internal/config/config.go` — add the `Timeout` method.
- `cmd/dn-tool/{enroll,unenroll,install,run}.go` — call `cfg.Timeout(...)`; delete `enrollTimeout`/`unenrollTimeout`/`installTimeout`.

## Tests

- `internal/config/config_test.go` — add a focused `Timeout` test: `APITimeout` set → returns it; unset (0) → returns the fallback.
- The command tests exercise the call sites and survive unchanged (behavior identical).

## Acceptance criteria

- `Config.Timeout(fallback)` exists; the three cmd helpers are deleted.
- All four call sites use `cfg.Timeout(default*Timeout)`; the three default constants are unchanged and still command-local.
- Behavior is identical: a configured `DN_API_TIMEOUT` is honored; otherwise the command's default applies.
- `internal/run`'s `unenrollTimeout(deps)` is untouched.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 6), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.3 (`DN_API_TIMEOUT`, unset → command default).

