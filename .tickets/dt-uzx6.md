---
id: dt-uzx6
status: closed
deps: [dt-vsi6]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 0
assignee: Andre Silva
tags: [config, env]
---

# Configuration interface (env + env-file)

Requirement 6. Read all configuration from environment variables; optionally load key-value pairs from an --env-file path (parsed as plain data, never executed); live env vars override env-file values (single documented precedence order); support DN_API_URL override. Owns the full config variable set and validation.

## Design

Central config struct + loader in internal/config. Documented precedence: live env > --env-file. JSON-array vars (DN_TAGS, DN_STATIC_ADDRESSES) parsed here. Defaults per §2.3 table (e.g. `DN_NETWORK_NAME="defined"`, `DN_API_URL="https://api.defined.net"`).

## Acceptance Criteria

All `DN_*` vars load from env and --env-file; env wins on conflict; env-file is never executed; missing/invalid values surface clear errors; DN_API_URL override honored.

## Notes for a fresh agent

- The full variable table (names, defaults, required-for-which-command) is design §2.3 — implement against it verbatim. Defaults to watch: `DN_NETWORK_NAME=defined`, `DN_API_URL=https://api.defined.net`, `DN_CLIENT_BIN_DIR=/var/lib/defined/bin`, `DN_CLIENT_VERSION=latest`, `DN_LISTEN_PORT` unset→0.
- `DN_TAGS` and `DN_STATIC_ADDRESSES` are JSON arrays — parse and validate them here, not in downstream epics.
- `DN_SKIP_UNENROLL` and `DN_UNENROLL_ON_REBOOT` are consumed by the NixOS module's stop wiring (§2.7), **not** by the binary's own logic — the loader should accept them without branching on them.
- Precedence (live env > `--env-file`) is a documented contract: surface it in `--help`/README.

## References

- Design: [Req 6 Configuration interface](../docs/dn-tool-design.md#6-configuration-interface), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables), [§2.6 Host ID retrieval](../docs/dn-tool-design.md#26-host-id-retrieval).
- Research: upstream findings [SEC3 / S2 / S6](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — config `source`d as shell, defaults that never applied, redundant existence checks.

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream SEC3 (config was source'd as shell, allowing arbitrary code execution) -> parse the env-file as plain key-value data, never execute it. Also supersedes S2 (: ${VAR:-} no-ops that never assigned defaults) and S6 (redundant -v then non-empty checks) with real defaults + explicit required-value validation.

**2026-06-06T20:04:48Z**

Closed the epic's residual integration gap (dt-vsi6 pattern): the 4 children built Load (env-only), ParseEnvFile (reader->map), and Resolve (fileVars+env->Config), but nothing bridged the --env-file flag path to disk. Added config.LoadWithEnvFile(envFilePath, getenv) in internal/config/resolve.go — the single entry point the 5 blocked commands (enroll/unenroll/install/REST-client/write-config) will call: empty path -> Resolve(nil,getenv); set path -> os.Open + ParseEnvFile + Resolve, with clear wrapped errors on open/parse failure (missing file wraps os.ErrNotExist). TDD: 4 tests in loadenvfile_test.go (no-file/live-env, precedence end-to-end with live-env-wins + file-fills + DN_API_URL override, missing-file errors, malformed-file errors). Did NOT wire into commands (still notImplemented) or touch README — those are downstream-ticket scope and the §Precedence README section already exists from dt-9x3y. make build green (vet/lint 0/tests/build).
