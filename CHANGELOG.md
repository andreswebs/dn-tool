# Changelog

All notable changes to `dn-tool` are recorded here. The format is loosely based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project is
pre-1.0, so breaking changes may land in any `0.x` release.

## Unreleased

### Changed (breaking)

- **Exit codes now follow the taxonomy in ADR 0001.** Failures are
  classified into the BSD `sysexits` range instead of the previous
  one-code-for-everything scheme. Automation that branched on the old numbers
  must be updated:
  - Success stays `0`.
  - The `--assert-changed` no-op moves from `2` to `1` (it is a recoverable
    result, not a failure).
  - An unknown command moves from `3` (urfave's default) to `64`.
  - Usage errors (unknown command, bad flags or arguments) are `64` (EX_USAGE).
  - Data errors (a rejected API request with HTTP 4xx, a dnclient download
    checksum mismatch, a malformed published checksum) are `65` (EX_DATAERR).
  - An unreachable dependency (API transport error or HTTP 5xx) is `69`
    (EX_UNAVAILABLE).
  - Local input/output failures (config-file writes, the binary download,
    reading local dnclient state, a subprocess that cannot start) are `74`
    (EX_IOERR).
  - Configuration errors (missing/invalid `DN_*` variables, env-file problems,
    an unsupported platform/arch) are `78` (EX_CONFIG).
  - Anything unclassified is `70` (EX_SOFTWARE), treated as a bug.
- **`enroll` no longer leaks `dnclient`'s exit status.** A `dnclient` subprocess
  that ran and exited non-zero now exits `74` (EX_IOERR, code `dnclient_failed`)
  and reports the child's status as `details.exit_status`, instead of exiting with
  the child's status — a code no command declared. `run` is unaffected: it still
  propagates the foreground daemon's status verbatim.
- **`write-config` with no target path moves from `70` to `64`** (EX_USAGE, code
  `missing_argument`): a missing positional argument is a command-line fault.
- **An orphaned enrollment moves from `70` to `78`** (EX_CONFIG, code
  `orphaned_enrollment`): the local-absent/remote-present state is a deployment
  fact the operator resolves with `--force`, not a dn-tool bug.
- **A malformed management API response moves from `70` to `65`** (EX_DATAERR,
  code `api_malformed_response`): a body that cannot be decoded is bad data
  received. `70` stays reserved for dn-tool's own faults, such as encoding a
  request body.
- **Failures now render a JSON error envelope on stderr** instead of plain text
  (ADR 0002/0005): `schema_version`, `ok: false`, and an `error` object carrying
  `code`, `message`, `hint` and `details`,
  where `hint` and `details` are omitted when the error carries none and an
  unclassified error renders as `internal_error`. Nothing is written to stdout on
  failure. Automation that scraped the previous prose lines must read the `code`
  field instead.
- **Results on stdout now open with an envelope head** (ADR 0005): every result
  is one newline-terminated JSON object starting with `schema_version` (an
  integer, bumped on a breaking shape change) and `ok`, followed by the command
  payload. Consumers can detect a contract change instead of discovering it as a
  parse failure.
- **The result field `hostId` is renamed `host_id`.** The whole output contract
  is now `snake_case`, and dn-tool agrees with the `host_id` spelling
  `dnclient.yml` already uses. The defined.net API field names are untouched.
  `jq '.hostId'` pipelines must read `.host_id`.
- **A bare `dn-tool` exits `64`** (EX_USAGE, code `no_command`) with the error
  envelope on stderr, instead of printing the help banner on stdout and exiting
  `0` — ADR 0005 reserves stdout for results. An explicit `dn-tool --help` or
  `dn-tool --version` still writes to stdout and exits `0`.
- **Logging moved to a leveled, destination-aware posture** (ADR 0004).
  `--log-text` is removed; `--log-level error|warn|info|debug` and `--quiet`
  replace it. The default level moves from `info` to `warn`, so a successful
  command writes nothing to stderr. The handler is chosen by destination rather
  than by flag: text when stderr is a terminal, JSON otherwise, so a human
  redirecting stderr to a file now gets JSON with no flag to force text. A bad
  `--log-level` exits `64`; a bad `DN_LOG_LEVEL` exits `78`, where it was
  previously accepted silently as `info`. `DN_LOG_LEVEL` remains the environment
  fallback for the flag, read from the environment only and not from
  `--env-file`; its default in a written env file changes to `warn` to match.
  Every record now carries `command`, `version`, and a per-invocation `run_id`.
- **`run` now exits `130`/`143` after a graceful shutdown on SIGINT/SIGTERM**
  (`128` plus the signal number), overriding the cleanup outcome. It continues to
  pass through the foreground daemon's own exit status when the daemon terminates
  on its own — the one documented exception to the taxonomy.

### Fixed

- **A graceful shutdown and an `--assert-changed` no-op no longer emit an error
  envelope.** Both are statuses rather than failures, so neither reports
  `ok: false` for a command that did not fail. A `run` stopped by SIGINT/SIGTERM
  exits `130`/`143` with log records only on stderr, and an `--assert-changed`
  no-op exits `1` with its `ok: true` result envelope on stdout and nothing on
  stderr — previously one successful invocation wrote both an `ok: true` result
  on stdout and a contradicting `ok: false` error on stderr, and every clean
  container stop was reported to supervisors and agents as a failure. A daemon
  that exits on its own is unaffected: that is a real failure and still carries
  its envelope alongside the daemon's propagated exit status.
- **The `daemon_exit` hint is remediation advice** ("read the dnclient daemon's
  own output on stderr…") instead of a restatement of the exit-code mapping,
  which is already reported as data by `schema`
  (`passes_through_child_exit_status`).

### Added

- **`statuses` in the `schema` envelope.** The two non-failure codes —
  `no_change` (exit `1`) and `signal_shutdown` (exit `130`/`143`) — are reported
  in a list of their own, each with its exit code and meaning, instead of
  appearing in the `errors` inventory they are not part of.
- **An exec-level test suite (`go test ./e2e`).** It builds the binary and
  drives the real process against a fake management API and a `dnclient`
  stand-in, which is the only way to cover process-level behavior: a real
  SIGTERM/SIGINT must exit `143`/`130` after unenrolling, with no error envelope
  on stderr. Skipped on hosts the `install` platform gate rejects.
- **The `schema` command (ADR 0005).** `dn-tool schema` prints the whole
  contract as one JSON envelope on stdout: the tool name and version, every
  command with its usage, argument synopsis, flags and declared exit codes, the
  global flags, the union of every exit code the tool can produce, and the full
  error inventory with each code's exit code and hint. The `run` command carries
  `passes_through_child_exit_status: true`, so the one exception to the
  exit-code registry is discoverable as data. Every field is a projection of the
  declarations that drive behavior (the command tree, the exit-code registry,
  and the error registry), so the reported surface cannot drift from the real
  one.
- **Coded errors (ADR 0002).** Every failure that can reach a consumer now
  carries a stable `snake_case` machine code, an exit code from the ADR 0001
  taxonomy, and a remediation hint, discovered at the boundary through one
  interface (`internal/terr`). Sentinels are immutable and registered, so the
  error surface is enumerable as data; per-invocation context attaches by copy.
  The codes reach consumers through the stderr error envelope.
- A declared, per-command exit-code registry (`internal/output/exitcodes.go`)
  with a conformance test, so every code the tool can produce is data in code
  and is asserted against real exits rather than only documented in prose (ADR
  0001).
