---
id: dt-47ab
status: closed
deps: []
links: [dt-577o]
created: 2026-06-07T13:45:14Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-uzx6
tags: [config, env, testability]
---
# Make the dnclient config root overridable via DN_CLIENT_CONFIG_DIR

The dnclient config root /etc/defined is a hardcoded const (main.go), not overridable; binary-level enroll/unenroll/run fs behavior can only be tested as root or in a container. Add a DN_CLIENT_CONFIG_DIR config var (default /etc/defined) like DN_CLIENT_BIN_DIR.

## Design

The dnclient config root is the hardcoded const `defaultConfigRoot = "/etc/defined"` (cmd/dn-tool/main.go), passed by the enroll/unenroll/run actions into the internal cores. The cores ALREADY accept an injected root (enroll.Enroll, unenroll.Unenroll, productionRunDeps all take configRoot) — so the only thing missing is a way for the command layer to source that root from configuration. Today it cannot be overridden, so binary-level enroll/unenroll/run filesystem behavior is only exercisable as root or in a container (it reads/writes /etc/defined).

Fix: make it a normal DN_* config field, exactly like DN_CLIENT_BIN_DIR.

## Decisions (confirmed)
- Mechanism: env var only (consistent with the env-first config model; flags stay reserved for operational toggles). Loadable via --env-file and persisted by write-config like every other setting.
- Env var: DN_CLIENT_CONFIG_DIR, default "/etc/defined" (parallels DN_CLIENT_BIN_DIR = /var/lib/defined/bin).
- Go field: Config.ClientConfigDir (parallels Config.ClientBinDir); fed into the cores' existing `configRoot` parameter.

## Subtlety to record (not a blocker)
The override governs where dn-tool READS/WRITES the local config (<root>/<network>/dnclient.yml). For the REAL dnclient, dn-tool's root must match where dnclient itself writes that file; the /etc/defined default matches dnclient's convention. The override's immediate payoff is the fake-dnclient e2e harness (docs/e2e-testing.md Layer 2) and unprivileged/container testing — not changing the production location.

## Public interface
```go
// internal/config Config
ClientConfigDir string // DN_CLIENT_CONFIG_DIR, default "/etc/defined"
```
No new exported funcs. Commands pass cfg.ClientConfigDir where they currently pass defaultConfigRoot.

## Files
- src/internal/config/config.go    — add ClientConfigDir field + `orDefault(getenv("DN_CLIENT_CONFIG_DIR"), "/etc/defined")` in load(); group near ClientBinDir.
- src/internal/config/marshal.go   — add {"DN_CLIENT_CONFIG_DIR", cfg.ClientConfigDir} to pairs (after DN_CLIENT_BIN_DIR) so write-config round-trips it.
- src/cmd/dn-tool/main.go           — remove the now-unused `const defaultConfigRoot` + its doc comment (default now lives in config.go).
- src/cmd/dn-tool/enroll.go         — enrollAction: runEnroll(ctx, cfg, cfg.ClientConfigDir, cmd.Bool("force")).
- src/cmd/dn-tool/unenroll.go       — unenrollAction: runUnenroll(ctx, cfg, cfg.ClientConfigDir).
- src/cmd/dn-tool/run.go            — runAction: runRun(ctx, cfg, cfg.ClientConfigDir).
- src/internal/config/config_test.go  — DN_CLIENT_CONFIG_DIR in the all-set table + default-when-unset assertion.
- src/internal/config/marshal_test.go — extend round-trip / expected pairs (sample Config gets ClientConfigDir).
- src/cmd/dn-tool/unenroll_test.go (or new run/enroll cmd test) — behavioral wiring test (see Cycle 3).
- docs/dn-tool-design.md §2.3       — add the DN_CLIENT_CONFIG_DIR row (reconcile design with code; it is authoritative). Optional note at §2.6.
- docs/e2e-testing.md               — resolve the "config root hardcoded" caveat + the Layer 2 "Prerequisite: config-root override" section (binary-level fs tests no longer need root); update the contract table note.

## TDD plan (vertical slices)
- Cycle 1 (RED->GREEN): config_test — ClientConfigDir loads from DN_CLIENT_CONFIG_DIR and defaults to /etc/defined. Add the field + load line.
- Cycle 2 (RED->GREEN): marshal_test — DN_CLIENT_CONFIG_DIR round-trips through Marshal/ParseEnvFile. Add to pairs.
- Cycle 3 (RED->GREEN, the testability win): a cmd test that drives the real `unenroll` action with t.Setenv("DN_CLIENT_CONFIG_DIR", tmp), t.Setenv("DN_API_KEY",...), DN_API_URL=httptest server, and a <net>/dnclient.yml seeded under tmp — asserts the DELETE fires and the local dir under tmp is removed, with NO access to /etc/defined and no root. RED today (action uses the /etc/defined const -> ErrNotEnrolled); GREEN after switching the actions to cfg.ClientConfigDir and removing the const.
- Cleanup @ GREEN: confirm no `defaultConfigRoot` refs remain; gofmt/lint; update design §2.3 and docs/e2e-testing.md.

## Out of scope
No CLI flag (env-only, confirmed). No change to the internal cores (already injectable). No change to the real dnclient's own config location.

## Acceptance Criteria

- DN_CLIENT_CONFIG_DIR sets the dnclient config root; default /etc/defined when unset; honored by enroll, unenroll, and run.
- Loadable via --env-file and persisted by write-config (Marshal/ParseEnvFile round-trip includes it).
- The hardcoded defaultConfigRoot const is removed; the internal cores are unchanged (still take an injected root).
- A behavioral cmd test proves `unenroll` reads/writes the overridden root with no root privileges and no /etc/defined access.
- docs/dn-tool-design.md §2.3 lists the new variable; docs/e2e-testing.md's config-root caveats are updated to reflect it.
- make build is green (gofmt, vet, golangci-lint 0 issues, all tests, cross-compile).


## Notes

**2026-06-07T15:43:31Z**

DONE. Made the dnclient config root overridable via DN_CLIENT_CONFIG_DIR (default /etc/defined), env-only as decided.

Changes:
- internal/config/config.go: added Config.ClientConfigDir, loaded via orDefault(getenv("DN_CLIENT_CONFIG_DIR"), "/etc/defined") (parallels ClientBinDir).
- internal/config/marshal.go: emit DN_CLIENT_CONFIG_DIR so write-config round-trips it.
- cmd/dn-tool/{enroll,unenroll,run}.go: actions now pass cfg.ClientConfigDir to the cores instead of the hardcoded const.
- cmd/dn-tool/main.go: removed the now-unused `const defaultConfigRoot = "/etc/defined"` (default now sourced from config).
- docs/dn-tool-design.md §2.3: added the DN_CLIENT_CONFIG_DIR row.
- docs/e2e-testing.md: config-root caveat resolved — Layer 2 binary tests now run unprivileged via DN_CLIENT_CONFIG_DIR; fake dnclient inherits it from dn-tool's env; removed the open "DN_CONFIG_ROOT override" prerequisite.

TDD (RED->GREEN per cycle):
- C1: config_test asserts load-from-env + default /etc/defined -> add field + load line.
- C2: marshal_test (SelfContained key + round-trip fixtures) -> emit the pair; fixtures set ClientConfigDir to their resolved value (custom for fully-populated, /etc/defined for defaults), exactly as they already do for ClientBinDir/ClientVersion. Also fixed the writefile_test round-trip fixture.
- C3 (testability win): TestUnenrollCommand_HonorsConfigRootOverride drives the real `unenroll` app with DN_CLIENT_CONFIG_DIR=t.TempDir() + httptest API + seeded dnclient.yml; RED proved the action read /etc/defined; GREEN after switching actions to cfg.ClientConfigDir. No root, no container.

Internal cores unchanged (already took an injected configRoot). No CLI flag (env-only, confirmed). Real-dnclient note recorded: the override governs where dn-tool reads/writes; for the real daemon the root must match dnclient's own write location (default matches).

Verification: make build green (gofmt, vet, golangci-lint 0 issues, all tests, cross-compile). Binary smoke: `DN_CLIENT_CONFIG_DIR=$tmp dn-tool unenroll` reads $tmp/<net>/dnclient.yml (not /etc/defined), exit 1 not-enrolled.
