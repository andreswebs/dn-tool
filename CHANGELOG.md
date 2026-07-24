# Changelog

All notable changes to `dn-tool` are recorded here. The format is loosely based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project is
pre-1.0, so breaking changes may land in any `0.x` release.

## Unreleased

### Changed (breaking)

- **Exit codes now follow the family-wide taxonomy in ADR 0001.** Failures are
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
- **`run` now exits `130`/`143` after a graceful shutdown on SIGINT/SIGTERM**
  (`128` plus the signal number), overriding the cleanup outcome. It continues to
  pass through the foreground daemon's own exit status when the daemon terminates
  on its own — the one documented exception to the taxonomy.

### Added

- A declared, per-command exit-code registry (`internal/output/exitcodes.go`)
  with a conformance test, so every code the tool can produce is data in code and
  is asserted against real exits rather than only documented in prose (ADR 0001).
