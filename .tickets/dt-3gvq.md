---
id: dt-3gvq
status: closed
deps: [dt-uzx6, dt-8h9t, dt-cq78, dt-zwgc]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 0
assignee: Andre Silva
tags: [unenroll, api, dnclient, yaml]
---

# Host unenrollment (unenroll command)

Requirement 4 / unenroll command. Require API key. Read the remote host_id from local `/etc/defined/<network>/dnclient.yml` (YAML). No local config => fail clearly (not enrolled). DELETE the remote record; on 2xx or already-absent (404) remove the local config dir. Bounded configurable deadline shorter than the service-stop timeout. On delete failure within deadline: retain local config, report, exit non-zero. Invariant: never remove local config while the remote record may still exist.

## Design

Parse host_id via gopkg.in/yaml.v3 (§2.6), replacing upstream grep|awk; absent/malformed => fail clearly. Invariant per §2.5: local+remote removed together; 404 treated as idempotent success. Bounded by DN_API_TIMEOUT (~10s) < surrounding TimeoutStopSec.

## Acceptance Criteria

host_id read from dnclient.yml; missing local config fails clearly; remote DELETE then local removal on 2xx/404; delete failure retains local config and exits non-zero; invariant never broken.

## Notes for a fresh agent

- Delete is **v1 only** — `DELETE /v1/hosts/{hostID}` (API reference §4.4); there is no v2 successor. A `404` means already-absent → treat as idempotent success and proceed to remove local config (distinguish it from other 4xx via the error envelope, §2.3).
- `host_id` is parsed from `/etc/defined/<network>/dnclient.yml` with `gopkg.in/yaml.v3` (§2.6) — not `grep | awk`. Absent/malformed file → fail clearly.
- The binary always unenrolls when called; **reboot-vs-poweroff and skip logic live in the NixOS module (§2.7), not here.** Don't add `DN_SKIP_UNENROLL`/`DN_UNENROLL_ON_REBOOT` branching to this command.
- The bounded deadline (`DN_API_TIMEOUT`, ~10s) is the binary-side counterpart to the module's `TimeoutStopSec` (finding D5) — it must be strictly shorter so the binary always wins the stop race and exits cleanly.

## References

- Design: [Req 4 Host unenrollment](../docs/dn-tool-design.md#4-host-unenrollment), [§2.5 unenroll/shutdown tension](../docs/dn-tool-design.md#25-the-unenroll--shutdown-tension), [§2.6 Host ID retrieval](../docs/dn-tool-design.md#26-host-id-retrieval), [§2.7 NixOS module shape](../docs/dn-tool-design.md#27-nixos-module-shape-servicesdnclient), [§2.12 step 5](../docs/dn-tool-design.md#212-build--migration-order).
- API reference: [§4.4 Delete host](../docs/research/defined-net-api-reference.md#44-delete-host), [§2.3 Error envelope](../docs/research/defined-net-api-reference.md#23-error-envelope).
- Research: upstream findings [B3 / D5](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — unenroll deleted local config on API failure (orphan); no `TimeoutStopSec` on the stop path.

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream B3 (unenroll deleted local config even when the API DELETE failed, creating an unrecoverable orphan). Resolution per §2.5 invariant: remove local config only after DELETE returns 2xx or 404; on any other failure within the bounded deadline, retain local config and exit non-zero. Bounded deadline is the binary-side counterpart to the module's TimeoutStopSec (D5).

**2026-06-06T21:25:53Z**

Wired the unenroll command end-to-end in main.go (the missing seam — all three children built internal logic but none owned command wiring, per dt-brug's deferral). Added cmd/dn-tool/unenroll.go: runUnenroll(ctx, *config.Config, configRoot) testable core (no cli dep) + unenrollAction thin wrapper; shared loadConfig(cmd)=LoadWithEnvFile(env-file, os.Getenv) and defaultConfigRoot=/etc/defined in main.go; command now Action: withResult(unenrollAction). API key required up front (errMissingAPIKey, no Reveal); context bounded by DN_API_TIMEOUT or defaultUnenrollTimeout=10s (binary-side counterpart to module TimeoutStopSec). Acceptance, all covered by green tests + CLI smoke test: host_id read from dnclient.yml (TestRunUnenroll_DeletesThenRemovesLocal asserts DELETE /v1/hosts/<id>); missing local config fails clearly (TestRunUnenroll_NotEnrolled -> ErrNotEnrolled, exit 1); remote DELETE then local removal on 2xx/404 (DeletesThenRemovesLocal; 404->nil in api layer); delete failure retains local config + exits non-zero (DeleteFailureRetainsLocal, 403, ResolveExitCode==1, never code 2); invariant never broken (local removed strictly after delete success inside unenroll.Unenroll). Smoke: 'unenroll' with no key -> 'DN_API_KEY is required to unenroll' exit 1; with key, no config -> '/etc/defined/<net>/dnclient.yml: host does not appear to be enrolled' exit 1. make build green (vet, golangci-lint 0 issues, all tests). First command wired; sets the testable-core command pattern for enroll/install/run/write-config.
