---
id: dt-brug
status: closed
deps: [dt-j2ab, dt-ccmn, dt-4h21]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, state-machine]
---
# Enroll no-op cell: local config present

Implement the first cell of the §2.4 enrollment state machine: when a **local `dnclient` config for the network already exists**, treat the host as already enrolled and make no changes — no API calls, `Result.Changed=false`, exit 0 (or 2 under `--assert-changed`). This task also introduces the local-config **presence** helper the rest of the state machine builds on.

## Public interface

```go
// internal/enroll
func Enroll(ctx context.Context, cfg *config.Config, deps Deps) (output.Result, error)
//   deps: api client, dnclient.Client, and a localConfig probe

// internal/dnclient
func ConfigExists(networkName string) bool   // os.Stat /etc/defined/<network>/dnclient.yml
```

`ConfigExists` is a plain `os.Stat` on `/etc/defined/<network>/dnclient.yml` (the full YAML parse — host_id — belongs to [UNE.hostid](dt-0nl5.md); enroll only needs presence). Make the config root injectable (a base dir) so tests use a temp dir.

## Behaviors (TDD order)

1. **Local config present → no-op** — `ConfigExists` true → `Enroll` returns `Result{Action:"enroll", Changed:false}`, makes **zero** API calls (assert the mock api client was untouched) and does not invoke `dnclient`.
2. **Exit semantics** — no-op result yields exit 0, or exit 2 with `--assert-changed` (via EXIT.assert).
3. **Presence probe path** — `ConfigExists` checks exactly `/<root>/<network>/dnclient.yml`.

## Test strategy

Inject a config root (temp dir); create the `dnclient.yml` to simulate "enrolled". Use a mock api client that fails the test if called. Assert `Result` and that no API/subprocess calls happened.

## Acceptance

- Present local config → idempotent no-op, no remote calls, correct changed/exit semantics.

## References

- Design: [Req 2](../docs/dn-tool-design.md#2-host-enrollment), [§2.4 state machine](../docs/dn-tool-design.md#24-enrollment-state-machine) (row 1).
- Result/exit: [OUT.result](dt-ccmn.md), [EXIT.assert](dt-4h21.md).

Parent epic: [dt-i0yx](dt-i0yx.md).

## Notes

**2026-06-06T20:22:49Z**

Implemented §2.4 row 1 (local-config-present no-op) plus the state-machine scaffolding the siblings build on.

New code:
- internal/dnclient/config.go: ConfigExists(configRoot, networkName) bool — plain os.Stat presence probe on <root>/<network>/dnclient.yml (no parse; host_id read stays in ReadHostID). Extracted shared configPath() helper and refactored ReadHostID to use it (removes duplicated path join).
- internal/enroll/state.go: Enroll(ctx, cfg, Deps) (output.Result, error) + Deps{API, ConfigRoot} + the API interface seam (ListHosts/CreateHostAndEnrollmentCode/DeleteHost, the subset *api.Client satisfies). Row 1: ConfigExists true -> Result{Action:enroll, Changed:false}, zero API/dnclient calls. Local-absent path returns errLocalAbsentUnimplemented — a deliberate stub for dt-pe29 (create) and dt-xcac (orphan/force).

Scope held by omission: did NOT add the dnclient subprocess Enroller interface to Deps (that's dt-a772) and did NOT wire the enroll command in main.go (still notImplemented — the parent epic/command tickets own that). Exit semantics (behavior 2) are satisfied by composition with the existing withResult wrapper (dt-4h21): Changed=false -> exit 0, or exit 2 under --assert-changed; asserted at unit level via output.ResolveExitCode.

Tests: internal/dnclient/config_test.go (present/absent/other-network/exact-path) reuses the existing writeConfig helper; internal/enroll/state_test.go drives the no-op cell with a failingAPI mock that t.Fatals on any call. make build green (vet, golangci-lint 0 issues, tests, cross-compile).
