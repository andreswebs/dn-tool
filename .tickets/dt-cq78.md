---
id: dt-cq78
status: closed
deps: [dt-vsi6]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 1
assignee: Andre Silva
tags: [observability, logging, output]
---

# Output & observability (JSON result + structured logs)

Requirement 7. Write each command result as a structured JSON object to stdout; write diagnostic/progress logs to stderr as structured JSON by default (log/slog); --log-text emits plain-text logs; honor configured DN_LOG_LEVEL; never log secrets (API key, enrollment codes).

## Design

internal/output: JSON result writer + slog handler setup. Default slog JSON handler to stderr; result object to stdout, e.g. {"action":"enroll","changed":true,"hostId":"host-…","network":"defined"}. Redaction of DN_API_KEY and enrollment codes (fixes upstream SEC5).

## Acceptance Criteria

Results are JSON on stdout; logs are JSON on stderr by default and plain text with --log-text; level filtering works; API key and enrollment codes never appear in logs.

## Notes for a fresh agent

- stdout = exactly one result object per command; stderr = logs and one error object on failure (§2.8). Keep the two streams disjoint so pipelines can `jq` stdout cleanly.
- The result object carries the `changed` flag that the exit-status epic ([dt-zwgc](dt-zwgc.md)) reads for `--assert-changed` — settle the result schema (`action`, `changed`, `hostId`, `network`, …) here so both epics agree.
- Redaction is structural, not a string filter: keep the API key and enrollment codes out of any logged struct/field in the first place (enrollment codes are in-memory only).

## References

- Design: [Req 7 Output and observability](../docs/dn-tool-design.md#7-output-and-observability), [§2.8 Output contract](../docs/dn-tool-design.md#28-output-contract), [§2.10 CLI / libraries](../docs/dn-tool-design.md#210-cli--libraries) (`log/slog`).
- Research: upstream finding [SEC5](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — enrollment code and IDs echoed to stdout and captured by journald.

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream SEC5 (enrollment response, including the single-use code, plus host/network/role IDs were echoed to stdout and captured by journald) -> structured logging with secret fields omitted/redacted; enrollment codes used in-memory only; API key never logged.

**2026-06-06T20:43:50Z**

Verify-and-close (zero code change), the dt-8h9t epic-terminal-action pattern. All 4 children closed (ccmn JSON result writer, zxwf slog JSON-to-stderr+level, 0jfp --log-text, toaj Secret redaction) and re-read of acceptance against reality confirms every bullet is met AND tested:
- Results JSON on stdout: TestWriteResultShape/SingleObject/OmitsEmpty.
- Logs JSON on stderr by default: TestNewLoggerJSONByDefault; wired in main() via bootstrap slog.SetDefault + root Before hook.
- Plain text with --log-text: TestNewLoggerTextMode + TestLogText_FlagWiredToLogOptions (global flag -> logOptions -> handler swap).
- Level filtering (DN_LOG_LEVEL): TestNewLoggerLevelFiltering/LevelParsing, same in text mode.
- Secrets never logged: config.Secret redacts under slog/fmt/json (TestSecretRedacts*), covers DN_API_KEY + enrollment codes (SEC5 closed).
No residual integration seam owned by this epic (contrast dt-vsi6 CI / dt-uzx6 LoadWithEnvFile gaps): the logger IS wired at process level; the only un-wired piece, withResult->commands, is downstream command-ticket work per dt-4h21 scope discipline (commands are still notImplemented). Sibling dt-zwgc (exit-status, both children closed) is the matching verify-and-close; BOTH must close to unblock the P0 command tickets (koaf/i0yx/3gvq/cmdg).
