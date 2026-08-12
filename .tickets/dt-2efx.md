---
id: dt-2efx
status: closed
deps: []
links: [dt-cjgq, dt-nljm, dt-nrq1]
created: 2026-08-12T13:21:42Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-69kx
tags: [adr, errors]
---
# Introduce internal/terr and dn-tool's coded-error inventory (ADR 0002)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

`docs/adr/0002-error-handling.md` requires that every failure reaching a user
carries three things: a stable `snake_case` machine code, an exit code from ADR
0001, and an optional remediation hint written for the consumer. The boundary
discovers them with `errors.As` against one interface. Sentinels are immutable
package-level values; per-invocation context attaches by copy. Where the tool
has a `schema` command, the error codes are enumerable as data.

dn-tool today carries exit codes on a bare `cli.ExitCoder` wrapper with no code
and no hint. This ticket adds the type and classifies every failure path; the
boundary swap that consumes it is dt-uq9d.

## Current state (verified 2026-08-12 against commit bc8fa0b)

- No `internal/terr` directory.
- `internal/output/exit.go:31-38` - `exitError{err error; code int}`, the only
  carrier. `ExitError` (`:57-62`) forces a code; `Classify` (`:70-79`) defers to
  any code already present (innermost wins).
- 30 `output.Classify` call sites, listed in the mapping table below.
- Exit-code constants: `internal/output/exit.go:14-25`.
- Registry: `internal/output/exitcodes.go:17-31`.

## Target shape

`internal/terr/terr.go`, matching the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/terr/terr.go` in shape:

- `Coded` interface: `error` plus `Code() string`, `ExitCode() int`,
  `Hint() string`.
- `Detailed` interface: `ErrorDetails() any`.
- Concrete `E` with unexported `code, msg, hint string`, `exit int`,
  `cause error`, `details any`.
- `New(code string, exit int, hint, msg string) *E` - registers for enumeration
  and **panics on a duplicate code** (init-time programmer error; crashing at
  startup is the correct outcome per ADR 0002).
- `Newf(code string, exit int, hint, format string, args ...any) *E` -
  unregistered, for one-off per-invocation errors.
- `All() []*E` - a copy of the registry in registration order; backs the
  `schema` command (dt-3xxb).
- `Error()` appends the cause as `"message: cause"`; `Unwrap()` returns it.
- `Is(target error) bool` matches by code, so `Wrap`/`WithDetails` copies still
  match their sentinel under `errors.Is`.
- `Wrap(cause error) *E` and `WithDetails(details any) *E` return **copies**;
  the receiver is never mutated.

`terr` must not import `urfave/cli` (ADR 0003 no-leak rule).

### Sentinel placement

One `errors.go` per raising package: `internal/config/errors.go`,
`internal/dninstall/errors.go`, `internal/dnclient/errors.go`,
`internal/enroll/errors.go`, `internal/unenroll/errors.go`,
`internal/api/errors.go`, `internal/dnstate/errors.go`, and the CLI-surface ones
in `internal/command/errors.go`. `terr.New` registers into one global inventory
regardless, and ADR 0002 requires the "why this exit code" rationale to sit next
to the code, which means next to the code that raises it.

**Every sentinel declaration documents why it carries its exit code**, in a doc
comment, per ADR 0002. Follow the cookiecutter's
`internal/command/errors.go` for the comment style.

## The mapping (decided; implement exactly this)

Exit codes are fixed by what the conformance tests already assert, except the
four rows marked **CHANGE**.

| Sentinel | Code | Exit | Replaces |
| -------- | ---- | ---- | -------- |
| `ErrUsage` | `usage_error` | 64 | `onUsageError`, `cmd/dn-tool/main.go:74` |
| `ErrUnknownCommand` | `unknown_command` | 64 | `rootAction`, `cmd/dn-tool/main.go:83` |
| `ErrNoCommand` | `no_command` | 64 | new; the bare invocation. Wiring is dt-nljm |
| `ErrMissingArgument` | `missing_argument` | 64 | `cmd/dn-tool/write_config.go:14`, returned at `:34` - **CHANGE, was 70** |
| `ErrNoChange` | `no_change` | 1 | `errNoChange`, `cmd/dn-tool/main.go:29`, used at `:52` |
| `ErrConfig` | `config_error` | 78 | `internal/config/config.go:60`, `internal/config/resolve.go:54` |
| `ErrEnvFile` | `env_file_error` | 78 | `internal/config/resolve.go:28`, `:32` |
| `ErrMissingRequiredConfig` | `missing_required_config` | 78 | `internal/enroll/enroll.go:82`, reaching `state.go:66` |
| `ErrRoleConflict` | `role_conflict` | 78 | `internal/enroll/validate.go:19`, `:23`, `:27`, reaching `state.go:57` |
| `ErrUnsupportedPlatform` | `unsupported_platform` | 78 | `internal/dninstall/platform.go:35`, `:39` |
| `ErrNoDownload` | `no_download_for_version` | 78 | `internal/dninstall/download.go:51`, `:57` |
| `ErrAPIRejected` | `api_rejected` | 65 | `internal/api/client.go:199-203`, the 4xx branch |
| `ErrAPIUnavailable` | `api_unavailable` | 69 | `internal/api/client.go:181` (transport), `:201` (5xx) |
| `ErrAPIMalformedResponse` | `api_malformed_response` | 65 | `internal/api/pagination.go:58`, `internal/api/client.go:208`, `:229`, `:232` - **CHANGE, was 70** |
| `ErrChecksumMismatch` | `checksum_mismatch` | 65 | `internal/dninstall/verify.go:50` |
| `ErrChecksumMalformed` | `checksum_malformed` | 65 | `internal/dninstall/verify.go:78` |
| `ErrDownloadIO` | `download_failed` | 74 | `internal/dninstall/verify.go:36`, `:40`, `:45`, `:62`, `:66`, `:71` |
| `ErrFileIO` | `file_io_error` | 74 | `internal/config/writefile.go:32`, `:37`, `:40`; `internal/dninstall/install.go:107`, `:112`, `:125`, `:128`, `:131`, `:134`, `:165` |
| `ErrLocalState` | `local_state_error` | 74 | `internal/unenroll/unenroll.go:74` |
| `ErrNotEnrolled` | `not_enrolled` | 74 | `internal/dnstate/hostid.go:20`. Stays 74 (no behavior change); see the note below |
| `ErrDNClientFailed` | `dnclient_failed` | 74 | `internal/dnclient/client.go:79` - **CHANGE, see the defect section** |
| `ErrOrphanedEnrollment` | `orphaned_enrollment` | 78 | `internal/enroll/state.go:82`, `:99` - **CHANGE, was 70** |
| `ErrMissingAPIKey` | `missing_api_key` | 78 | `internal/unenroll/unenroll.go:31`, used at `:67` |
| `ErrDaemonExit` | `daemon_exit` | 70 (class) | `internal/run/run.go:254`; per-invocation code, see below |
| `ErrSignalShutdown` | `signal_shutdown` | 143 (class) | `internal/run/run.go:29`, `:190-195`; per-invocation code, see below |

Left at 70 deliberately, because they are genuinely dn-tool's own bugs, which is
what EX_SOFTWARE means: `internal/api/client.go:162` (encoding the **request**
body) and `:169` (building the **request**).

`ErrNotEnrolled` note to carry forward, not to act on: "unenroll a host that was
never enrolled" surfacing as an I/O error (74) is arguably misleading, but
changing it is out of this epic's approved scope. Record it with `tk add-note`
rather than changing it.

## The dnclient defect (CHANGE, verified empirically 2026-08-12)

`output.Classify(..., CodeIO)` at `internal/dnclient/client.go:79` is a
**no-op** whenever the child ran and exited non-zero. Driving the real chain:

```txt
raw error message: "exit status 3", ExitCode=3
*exec.ExitError satisfies cli.ExitCoder: true
wrapped message: "dnclient enroll failed: exit status 3"
ResolveExitCode(wrapped) = 3       (CodeIO = 74)
cli.HandleExitCoder would print: "exit status 3" and exit 3
```

`*exec.ExitError` satisfies `cli.ExitCoder` because it promotes `ExitCode()`
from the embedded `*os.ProcessState`, so `Classify` defers to it and `CodeIO` is
never attached. Two consequences: `enroll` exits with the child's status - a
code **not declared** in `CommandExitCodes["enroll"]`
(`internal/output/exitcodes.go:18`) - and the boundary discards the
`"dnclient enroll failed: "` context, because `cli.HandleExitCoder` prints the
innermost `ExitCoder` that `errors.As` found, not the outer wrapper.

Fix: in `execClient.exec` (`internal/dnclient/client.go:68-82`), branch on the
failure mode explicitly instead of relying on the deferral. Both branches wrap
in `ErrDNClientFailed` (74); when the child ran, attach its status as structured
detail so the number survives where it belongs:

```go
if err := cmd.Run(); err != nil {
    e := ErrDNClientFailed.Wrap(fmt.Errorf("dnclient %s: %w", action, err))
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        e = e.WithDetails(map[string]any{"exit_status": exitErr.ExitCode()})
    }
    return e
}
```

`run` is unaffected: `terr.E.Unwrap()` returns the cause, so `daemonExitCode`'s
`errors.As(err, &*exec.ExitError)` (`internal/run/run.go:272-280`) still finds
status N through the terr chain, and `run` re-forces the code at the outermost
layer (`:254`). Replace the now-wrong comment at `internal/dnclient/client.go:73-78`,
which asserts the deferral is how `run` propagates N.

## The two per-invocation exit codes

`internal/run/run.go` needs codes computed per invocation (130/143 from the
signal; the daemon's N). ADR 0002 sanctions the pattern: "a type that needs
per-instance structure delegates `Code`, `ExitCode`, `Hint`, and `Unwrap` to its
class sentinel, so `errors.Is` against the sentinel still matches while the
instance adds detail." Add to `internal/run`:

```go
type exitErr struct {
    class *terr.E // ErrSignalShutdown or ErrDaemonExit
    code  int     // the per-invocation code: 130/143, or the daemon's N
    cause error
}

func (e *exitErr) Code() string    { return e.class.Code() }
func (e *exitErr) Hint() string    { return e.class.Hint() }
func (e *exitErr) ExitCode() int   { return e.code } // overrides the class
func (e *exitErr) Unwrap() error   { return e.cause }
func (e *exitErr) Is(t error) bool { return errors.Is(e.class, t) }
```

The class sentinels keep the variance honest in their **hints**, not in a
misleading fixed code:

- `signal_shutdown`, class exit 143 - hint: "the process was terminated by a
  signal after a graceful shutdown; 130 for SIGINT, 143 for SIGTERM".
- `daemon_exit`, class exit 70 - hint: "the dnclient daemon exited with its own
  status, which dn-tool propagates verbatim; 70 when it had no clean status".

This replaces `output.ExitError` at `internal/run/run.go:194` and `:254`.
`errSignalShutdown` (`:29`) becomes `ErrSignalShutdown`; the typed-nil rationale
in its comment still applies and should be preserved.

## Scope boundary

This ticket **adds** `terr` and converts the call sites. It does **not** delete
`exitError`/`ExitError`/`Classify`/`ResolveExitCode` or change the boundary -
that is dt-uq9d, which depends on this. To keep the tree green in between, make
`terr.E` satisfy `cli.ExitCoder` transitionally (it already has
`ExitCode() int`, so this is free) so the existing boundary keeps resolving
codes while the call sites migrate. dt-uq9d removes that coupling.

## Acceptance criteria

- `internal/terr` exists with the exact interface and method set above.
- `terr` unit tests cover: duplicate-code panic; `Wrap` and `WithDetails` leave
  the receiver unmodified; `Is` matches a wrapped copy against its sentinel;
  `All` returns a copy (mutating the result does not affect the registry);
  `Error()` with and without a cause.
- Every `output.Classify` call site in the mapping table returns a coded
  sentinel instead, and each of the four **CHANGE** rows behaves as specified.
- Every sentinel's declaration documents why it carries its exit code.
- `grep -rn 'urfave' internal/terr/` is empty.
- Exit codes: these pass with expectations **unchanged**, since none of them
  covers a CHANGE row -
  - `internal/output/exitcodes_test.go`
  - `cmd/dn-tool/exit_conformance_test.go` (all five subtests)
  - `internal/run/signal_test.go` (130/143)
  - `internal/run/run_test.go` (daemon passthrough N, and the 70 fallback for a
    daemon with no clean status)
- New tests assert each CHANGE row: `write-config` with no path → 64; orphaned
  enrollment → 78; a malformed API response → 65; `dnclient enroll` exiting 3 →
  **74** with `details.exit_status == 3`, and `run` still propagating 3.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

- `CHANGELOG.md`, under Unreleased / Changed (breaking), four entries:
  `enroll` no longer leaks `dnclient`'s exit status (now 74 with
  `details.exit_status`); `write-config` with no path moves 70 → 64; orphaned
  enrollment moves 70 → 78; a malformed API response moves 70 → 65.
- `internal/enroll/state.go:41-42` - the doc comment still says
  "exit 0, or exit 2 under `--assert-changed`", stale since the dt-7lys
  migration made it exit 1. Fix it here since this ticket touches the file
  (contradiction row 17).

## Dependencies

None; can run in parallel with dt-w86c and dt-djf3. Blocks dt-uq9d, dt-3xxb,
dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.

## Notes

**2026-08-12T14:46:22Z**

Implemented internal/terr (Coded/Detailed/E, New with duplicate-code panic, Newf, All, Wrap/WithDetails copies, Is-by-code) plus the full 25-sentinel inventory. All 33 output.Classify call sites are gone from non-test code — including the two the ticket's table omitted (internal/command/commands.go result-write failure and internal/unenroll/unenroll.go local-config removal), both mapped to config.ErrFileIO (74, no behavior change). output.Classify/ExitError/ResolveExitCode are untouched and still back the boundary; dt-uq9d deletes them.

Deviations and judgment calls worth knowing:
- ErrFileIO is raised by four packages (config, dninstall, unenroll, command), so it is declared once in internal/config/errors.go (lowest package in the import graph) and referenced as config.ErrFileIO elsewhere. One machine code, one meaning; the per-package errors.go rule is the default, not an invariant. dninstall now imports config.
- ErrInternal (internal_error, 70) from the planning doc's inventory is NOT declared: it is the ExitCodeFor fallback, which is dt-uq9d's. ErrNoCommand IS declared but unwired (dt-nljm).
- validateRoles and missingParam now raise their coded errors at origin (validate.go / enroll.go) rather than being classified at state.go:57/:66, which just return err.
- Sentinel messages are class labels and the specific detail rides in the wrapped cause, so Error() reads 'class: detail'. This changes some message text (e.g. 'no published dnclient download: version "1.2.3" is not in the downloads table'). All existing substring assertions still pass.
- run.exitErr.Error() returns the cause's message, not the class label, preserving the old output.ExitError behavior at the boundary.

All pre-existing exit-code expectations pass unmodified (output/exitcodes_test.go, command/exit_conformance_test.go, run/signal_test.go, run/run_test.go). New tests: write-config with no path -> 64; orphan (both the pre-check and the create-time duplicate backstop) -> 78 + ErrOrphanedEnrollment; malformed API response (single + list envelope) -> 65; dnclient exit 3 -> 74 with details.exit_status=3, action context retained and *exec.ExitError still reachable; run still propagating 3 through a coded daemon error; an inventory conformance test in internal/command (snake_case codes, unique, taxonomy exit codes, non-empty message and hint) — it lives there because that package links every raising package, so terr.All() is fully populated.

Carried forward, not acted on (per the ticket): not_enrolled stays 74, so 'unenroll a host that was never enrolled' still surfaces as an I/O error. Reconsider when the class list is next revisited.

Also fixed the stale 'exit 2 under --assert-changed' doc comment in internal/enroll/state.go (contradiction row 17), and corrected the dt-7lys learning in docs/specs/learnings.md that reasoned Classify's ExitCoder deferral was safe — it was the source of the enroll exit-status leak.
