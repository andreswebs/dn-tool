---
id: dt-i0yx
status: closed
deps: [dt-uzx6, dt-8h9t, dt-koaf, dt-cq78, dt-zwgc]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 0
assignee: Andre Silva
tags: [enroll, api, dnclient, state-machine]
---

# Host enrollment (enroll command + state machine)

Requirement 2 / enroll command. Require API key, network ID, role ID (clear error naming any missing param). Determine hostname (configured else system). Request tun device = network name; optional static IP; optional tags. Modeled as a state decision over local dnclient config x remote host record (§2.4), not a linear sequence. Create remote record -> obtain single-use enrollment code -> run dnclient enroll.

## Design

internal/enroll state machine (§2.4): local present => no-op already-enrolled (exit 0, or 2 with --assert-changed); local absent + remote absent => create+code+enroll; local absent + remote present => orphan, fail with guidance unless --force; with --force => DELETE stale record then enroll fresh. Any API error => fail without running dnclient enroll; dnclient enroll non-zero => fail and surface error. Enrollment code in-memory only.

## Acceptance Criteria

All four §2.4 cells plus --force behave; missing required param fails clearly naming it; hostname/tun/IP/tags requested correctly; API error aborts before dnclient enroll; dnclient enroll failure surfaced.

## Notes for a fresh agent

- **"Remote present?" cannot be a name query** — the API exposes no `name` filter on host-list (API reference §4.2). §2.4 resolves this with **list-and-match**: paginate `GET /v2/hosts?filter.networkID=…` and match `name` client-side, because the `--force` DELETE path needs the existing host's `id`. Create-and-detect (`400 ERR_DUPLICATE_VALUE` on `path: name`) is only a fallback signal.
- Endpoint versions (API reference §3): create host + enrollment code and get/list are **v2** (`POST /v2/host-and-enrollment-code`, `GET /v2/hosts`); host delete is **v1 only** (`DELETE /v1/hosts/{id}`).
- "Local config present" = `/etc/defined/<network>/dnclient.yml` exists (network = `DN_NETWORK_NAME`); this shares the host-id parsing with unenroll ([dt-3gvq](dt-3gvq.md), §2.6).
- Enrollment code is single-use and in-memory only — hand it straight to `dnclient enroll`, never log or persist it.

## References

- Design: [Req 2 Host enrollment](../docs/dn-tool-design.md#2-host-enrollment), [§2.4 Enrollment state machine](../docs/dn-tool-design.md#24-enrollment-state-machine), [§2.5 unenroll/shutdown tension](../docs/dn-tool-design.md#25-the-unenroll--shutdown-tension) (how orphans arise), [§2.12 step 4](../docs/dn-tool-design.md#212-build--migration-order).
- API reference: [§4.1 Create host & enrollment code](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code), [§4.2 List hosts](../docs/research/defined-net-api-reference.md#42-list-hosts) (no name filter), [§4.4 Delete host](../docs/research/defined-net-api-reference.md#44-delete-host), [§5 Host object](../docs/research/defined-net-api-reference.md#5-host-object).
- Research: upstream findings [B4 / D7](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — enroll orphan on `dnclient enroll` failure; two non-atomic sources of truth.

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream B4 (orphan when POST /host-and-enrollment-code succeeds but dnclient enroll then fails: local absent + remote present made the host permanently stuck) and D7 (two non-atomic sources of truth, no reconciliation) -> model enroll as the local x remote state machine (§2.4) with --force recovery for the orphan cell.

**2026-06-06T22:00:04Z**

Verify-and-close that was NOT zero-code (the dt-3gvq pattern): all 5 children built/tested the internal enroll.Enroll state machine, but the 'enroll' command itself was still notImplemented in main.go — the epic owns that command wiring. Added cmd/dn-tool/enroll.go: enrollAction(loadConfig -> runEnroll(cfg, defaultConfigRoot, cmd.Bool("force"))) wrapped by withResult; runEnroll is the testable core (bounds ctx by enrollTimeout, builds api.New + dnclient.NewExecClient at the bin path, calls enroll.Enroll). Wired Action: withResult(enrollAction), dropped enroll from the notImplemented stub-test list. NO API-key pre-gate (unlike unenroll): the §2.4 row-1 no-op needs no key/network, and required-param validation already lives in buildCreateRequest on the create path. defaultEnrollTimeout=30s (§2.3), honoring DN_API_TIMEOUT. Added exported dnclient.BinaryPath(binDir) to single-source the path install writes and enroll execs; refactored install.go to use it. Tests mirror unenroll_test.go: no-op cell (server t.Errors if hit), full create path via httptest + a fake executable dnclient script that logs its args (asserts 'enroll -name testnet -code SECRET-CODE'), timeout helpers.
