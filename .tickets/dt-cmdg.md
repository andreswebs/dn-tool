---
id: dt-cmdg
status: closed
deps: [dt-uzx6, dt-cq78, dt-zwgc]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 2
assignee: Andre Silva
tags: [config, write-config]
---

# Configuration file writer (write-config)

Requirement 10 / write-config command. Write the current environment configuration as key-value pairs to a target path. Create the file with owner-only (0600) read/write permissions from the moment of creation; never create broader then restrict.

## Design

Open/create the target with 0600 from creation (no chmod-after). Serialize the resolved config as the same key-value format consumed by --env-file.

## Acceptance Criteria

write-config persists configured values as key-value pairs at the target path with 0600 perms set at creation time; never widened-then-restricted.

## Notes for a fresh agent

- Create with the mode at creation (`os.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0600)`), and account for umask — verify the resulting mode rather than assuming. Never `Create` then `Chmod`: that leaves a world-readable window (the exact SEC2 bug).
- The output format is the same key-value format the `--env-file` loader ([dt-uzx6](dt-uzx6.md)) consumes — round-trip must work. Reuse the config serializer, don't write a second format.
- Decide whether `DN_API_KEY` is written at all: persisting a secret to disk is a sharp edge — at minimum the `0600` guarantee must hold, and the choice should be explicit/documented.

## References

- Design: [Req 10 Configuration file writer](../docs/dn-tool-design.md#10-configuration-file-writer), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.12 step 6](../docs/dn-tool-design.md#212-build--migration-order).
- Research: upstream finding [SEC2](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — `write_config` permission race (created ~0644, chmod 0640 afterward).

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream SEC2 (write_config created the API-key file with umask perms ~0644 then chmod 0640 afterward, leaving a world-readable window) -> create the file with owner-only 0600 at creation time; never widen-then-restrict.

**2026-06-06T22:36:32Z**

Verify-and-close: both children (dt-druh Marshal, dt-meg1 WriteConfigFile+0600) were already closed and the write-config command is fully wired. Re-read all Req-10 acceptance criteria against reality and confirmed each has a green test:
- 0600 at creation under umask 0 (TestWriteConfigFile_CreatesWith0600 / TestRunWriteConfig_WritesFileAndResult): asserts mode==0600 with syscall.Umask(0), so the mode comes from the O_CREATE flag, not an inherited restrictive umask. No chmod-after — SEC2 fixed by omission.
- Round-trip through the REAL loader (TestWriteConfigFile_RoundTrips: WriteConfigFile -> ParseEnvFile -> Resolve, reflect.DeepEqual) so writer and Marshal can't drift.
- API key persisted cleartext, protected only by 0600 (documented decision): TestWriteConfigFile_WritesAPIKeyUnder0600 asserts raw key present AND 'REDACTED' absent AND mode 0600.
- Truncation of an existing larger file keeps 0600 (TestWriteConfigFile_TruncatesExisting).
- Required positional <path> (TestRunWriteConfig_RequiresPath -> errMissingWriteConfigPath) + end-to-end command test.
main.go: write-config = withResult(writeConfigAction); stub-list test (TestNewApp_SubcommandsReturnNotImplemented) already excludes it. Only 'run' remains notImplemented. make build green (vet, golangci-lint 0 issues, all tests, arm64 build). No code change required.
