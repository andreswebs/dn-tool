---
id: dt-7lys
status: closed
deps: [dt-370b]
links: [dt-370b]
created: 2026-07-22T19:45:02Z
type: task
priority: 2
assignee: Andre Silva
---
# Migrate exit codes to the fleet taxonomy (ADR 0001)

## Context

`docs/adr/0001-exit-code-taxonomy.md` (already in this repo) adopts a
family-wide taxonomy: 0 success; 1 recoverable result; 2-63 optional
result sub-codes; 64 EX_USAGE; 65 EX_DATAERR; 70 EX_SOFTWARE;
74 EX_IOERR; 78 EX_CONFIG; other sysexits on exact fit (69
EX_UNAVAILABLE is used below); 130/143 after graceful shutdown on
SIGINT/SIGTERM. dn-tool currently uses 0 OK, 1 for every failure,
2 for the `--assert-changed` no-op, and (via urfave defaults) 3 for an
unknown command. This ticket is larger than a renumbering: dn-tool
must start classifying failures, and its `run` command must adopt the
signal exit rule. Breaking change (v0.x); include release notes.

## Old to new mapping (decided; implement exactly this)

| Old | Condition | New |
| --- | --------- | --- |
| 0 | success | 0 (unchanged) |
| 2 | `--assert-changed` no-op (`errNoChange`) | 1 (it is a result, not a failure) |
| 3 | unknown command (urfave default) | 64 |
| 1 | usage errors (flag parse failures, bad arguments) | 64 |
| 1 | data errors: dnclient download checksum mismatch | 65 |
| 1 | defined.net API: transport/network errors and HTTP 5xx | 69 |
| 1 | defined.net API: HTTP 4xx rejections | 65 |
| 1 | local I/O: file reads/writes, download I/O, subprocess start failures | 74 |
| 1 | configuration: missing/invalid `DN_*` env vars, env-file problems, unsupported platform/arch | 78 |
| 1 | anything unclassified | 70 |
| any | `run`: foreground daemon exited with status N | N (unchanged; documented registry exception) |
| 0 | `run`: graceful shutdown after SIGINT/SIGTERM | 130/143 (new; see below) |

## Mechanism (use what exists; verified 2026-07-22)

`internal/output/exit.go` already has the right primitive:
`output.ExitError(err, code)` wraps any error as a `cli.ExitCoder`
while preserving the chain, and `exitWithError` in
`cmd/dn-tool/main.go:141-147` honors it. The migration is: replace the
three consts with the taxonomy set, then wrap errors with the right
class code at the production boundaries listed below.

1. `internal/output/exit.go:12-16`: replace
   `CodeOK/CodeError/CodeNoOpAssert` with the taxonomy set, for
   example: `CodeOK = 0`, `CodeResult = 1` (assert no-op),
   `CodeUsage = 64`, `CodeData = 65`, `CodeUnavailable = 69`,
   `CodeInternal = 70`, `CodeIO = 74`, `CodeConfig = 78`. Update the
   doc comment (it cites design Req 8 and the "code 2 reserved"
   rule, both superseded by the ADR). `ResolveExitCode`'s fallback
   (`exit.go:37-43`) returns `CodeInternal` instead of `CodeError`.
2. `cmd/dn-tool/main.go:48-50` (`withResult`): the assert-changed
   no-op returns `output.ExitError(errNoChange, output.CodeResult)`.
3. Usage errors: urfave parse failures currently fall through to the
   generic fallback (1), and unknown commands take urfave's default 3.
   Set `OnUsageError` on the root command (and verify subcommands
   inherit or set it) plus `CommandNotFound`, each returning
   `output.ExitError(err, output.CodeUsage)`.
4. Config class: `internal/config` errors are plain `fmt.Errorf`
   (`config.go:82-105`, `resolve.go` env-file paths, and the
   `required environment variable` message). Wrap at the package
   boundary: the exported load/resolve functions return
   `output.ExitError(err, output.CodeConfig)` (or a package-local
   equivalent to avoid the import if `output` importing is awkward;
   `output` has no dependencies, so importing it is fine).
   `internal/dninstall`'s unsupported platform/arch error also maps to
   `CodeConfig`.
5. Data class: `internal/dninstall`'s checksum-verification failure in
   `downloadAndVerify` (see `install.go:53-62`) wraps as `CodeData`.
   The API client (`internal/api/client.go`): in the error path,
   `APIError` with `StatusCode >= 500` and transport errors wrap as
   `CodeUnavailable`; `APIError` with 4xx wraps as `CodeData`. Note
   `enroll` already branches on `APIError.Has("ERR_DUPLICATE_VALUE",
   "name")` in-flow; that recovery path is unaffected.
6. I/O class: download/network file operations, config-file writes
   (`internal/config/writefile.go`), dnclient subprocess start
   failures (`internal/dnclient`) wrap as `CodeIO`.
7. Wrap at the outermost package boundary of each internal package, not
   at every call site: the `exitError` chain survives `%w` wrapping,
   and `errors.As` at the boundary finds the innermost assigned code.
   Beware double-wrapping: if an inner error already carries a code,
   do not wrap again at a higher layer (check with
   `errors.As(&cli.ExitCoder)` first, mirroring `exitWithError`).

## The run command: signals and passthrough

- `internal/run/run.go:149` uses `signal.NotifyContext(ctx, SIGINT,
  SIGTERM)` and unenrolls on shutdown; today a signal-triggered clean
  shutdown exits 0 (`run.go:196-207` returns nil), and a shutdown
  unenroll failure returns the unenroll error (1). Per the ADR's
  signal rule, after cleanup the process must exit 128+signum. Record
  which signal fired (capture it instead of, or alongside,
  `NotifyContext`), and after cleanup return
  `output.ExitError(err, 130 or 143)`, overriding both the clean-exit
  0 and any unenroll-failure code. Follow the ordering reasoning
  already documented in the surrounding comments (`run.go:166-171`).
- `daemonExitCode` (`run.go:216-224`): passthrough of the daemon's own
  exit status N is unchanged and stays a documented exception; its
  fallback for signal-death/non-exec errors changes from `CodeError`
  to `CodeInternal` (70) unless the shutdown was signal-initiated, in
  which case the signal code wins.

## Registry as data (new requirement from the ADR)

Add a declared table, for example `internal/output/exitcodes.go`, with
per-command sets (commands: enroll, unenroll, install, run,
write-config, and the root):

- most commands: `{0, 1, 64, 65, 69, 70, 74, 78}` (1 only where
  `--assert-changed` applies)
- `run`: the same set plus 130, 143, and a documented note that it
  also passes through the daemon's own exit status.

Add a conformance test that exercises at least one scenario per
declared code where feasible in-process (usage error, config error,
assert no-op are all cheap; API/network classes can assert
`ResolveExitCode` on constructed errors). The e2e container suite
(`docs/e2e-testing.md`) covers the rest; update its exit-code table at
line 55.

## Tests to update

- `cmd/dn-tool/exit_test.go`: `TestExitWithError_PlainErrorMapsToCodeError`
  (line 32; expected value becomes 70) and the two ExitCoder tests
  (lines 43, 51; update const names/values).
- `cmd/dn-tool/main_test.go`: any assertions on `CodeNoOpAssert`/2.
- `internal/run` tests covering `daemonExitCode` and shutdown paths:
  new expectations for 130/143 and the 70 fallback.
- Grep `src -rn "CodeError\|CodeNoOpAssert\|CodeOK"` and update every
  reference.

## Docs to update

- `docs/dn-tool-design.md`: Req 8 (meaningful exit codes, user story
  around line 224) and the daemon exit-status mention around line 181;
  restate against the ADR table.
- `docs/e2e-testing.md`: exit-code table at line 55 (`0/1/2/3`), the
  "missing key, exit 1" example at line 123, and the exit-code
  mapping/propagation mentions (lines 66-67, 220, 226).
- `AGENTS.md` and `README.md`: grep for exit-code mentions and update
  any literals.

## Acceptance

- The consts, boundary wrapping, usage/command-not-found handlers,
  and assert-no-op change are in place; `ResolveExitCode` falls back
  to 70.
- `run` exits 130/143 after graceful signal shutdown (unit-tested via
  the existing Deps-injected fakes), and still passes through the
  daemon's real exit status otherwise.
- The declared registry exists and its conformance test passes.
- All tests updated; `make build` passes from the repo root.
- Docs updated; no in-repo document still describes 1-for-everything
  or 2-for-no-op.
- Release notes drafted (breaking change: failures classified into
  64/65/69/70/74/78; assert no-op moves from 2 to 1; unknown command
  moves from 3 to 64; run exits 130/143 on signal shutdown).
- Leave committing to the repo owner; do not create git commits.

## Notes

**2026-07-24T20:37:23Z**

Blocked-by / re-check note (see migration ticket dt-370b): the Go module is being moved from src/ to the repo root. After that migration lands, this ticket's file paths must be re-read against the NEW layout. Specifically: every path in this ticket is now repo-root-relative with NO 'src/' prefix (internal/output/exit.go, cmd/dn-tool/main.go, internal/config/*, internal/api/client.go, internal/dninstall/*, internal/run/run.go, cmd/dn-tool/exit_test.go, etc. are already written without src/ and stay correct). The one instruction to fix: 'Grep src -rn "CodeError|CodeNoOpAssert|CodeOK"' becomes 'grep -rn' from the repo root (drop the 'src' argument). Also note: make build now runs from the repo root and the module toolchain is read from ./go.mod (not src/go.mod). No Dockerfile or Nix paths appear in this ticket, but if any are added later, use the post-migration layout (module at repo root; Dockerfile copies cmd/ and internal/ directly; nix/package.nix src filtered via lib.fileset).

**2026-07-24T23:07:22Z**

Migrated exit codes to the ADR 0001 taxonomy. output/exit.go: replaced CodeOK/CodeError/CodeNoOpAssert with CodeOK(0)/CodeResult(1)/CodeUsage(64)/CodeData(65)/CodeUnavailable(69)/CodeInternal(70)/CodeIO(74)/CodeConfig(78)/CodeSIGINT(130)/CodeSIGTERM(143); ResolveExitCode fallback now CodeInternal; added Classify() (defer-to-inner-code boundary wrapper, complement to ExitError's force). Classification wired at package boundaries: config->Config, config writefile->IO, dninstall platform/version->Config, checksum/malformed->Data, download+file ops->IO, api 4xx->Data & 5xx/transport->Unavailable, dnclient exec start-failure->IO (defers to *exec.ExitError which already satisfies cli.ExitCoder, preserving run's daemon passthrough), enroll validation->Config, unenroll missing-key->Config & local I/O->IO. cmd/main.go: withResult no-op->CodeResult; exitWithError fallback->CodeInternal; OnUsageError on root+every subcommand (v3 does not inherit it); custom root Action maps unknown command->64 (urfave default was 3, and CommandNotFound cannot set a code) and bare invocation->help/0. run: Lifecycle captures the signal via a second signal.Notify channel (NotifyContext discards which fired) and finalize()/withSignalExitCode()/signalExitCode() apply 128+signum, overriding the cleanup code; daemonExitCode fallback now CodeInternal. Registry: internal/output/exitcodes.go (CommandExitCodes) + conformance tests (exitcodes_test.go asserts only-known-codes & each class resolves; exit_conformance_test.go drives real CLI for usage/config/help; signal_test.go for 130/143). Docs: design Req 5 & Req 8, e2e-testing.md table+examples, new CHANGELOG.md release notes, learnings.md. Verified real binary: unknown cmd/bad flag=64, config=78, unreachable API=69, bare=0, version=0. make build green.
