---
id: dt-w86c
status: closed
deps: []
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-69kx
tags: [adr, cli]
---
# Adopt the internal/command Run(args, deps) delegate (ADR 0003)

Part of the ADR conformance adoption epic (dt-69kx). The plan this ticket was
split from is `.local/planning/adr-adoption.md` (decisions recorded there,
2026-08-12); this ticket is self-contained and does not require reading it.

## Context

`docs/adr/0003-cli-structure.md` requires a framework-free contract:
`main` is one line, `internal/command` owns the CLI surface behind
`Run(args []string, deps Deps) int`, and no `urfave/cli` type appears in any
exported identifier of that package. dn-tool's entire CLI currently lives in
package `main` under `cmd/dn-tool/`.

This ticket runs **first** in the epic. Four downstream tickets (dt-uq9d exit
boundary, dt-nljm error envelope, dt-cr62 logging, dt-cjgq golden harness) all
place their code in the delegate; doing this last would mean writing them
twice. In particular ADR 0006 requires golden scenarios that "invoke the
delegate", so dt-cjgq cannot be framework-blind without this.

## Current state (verified 2026-08-12 against commit bc8fa0b)

Files in package `main`: `cmd/dn-tool/main.go`, `install.go`, `enroll.go`,
`unenroll.go`, `run.go`, `write_config.go`, plus nine `*_test.go` files.

Real process environment touched outside `main`:

- `cmd/dn-tool/main.go:22` - `os.Getenv` in `loadConfig`.
- `cmd/dn-tool/main.go:64` - `os.Getenv("DN_LOG_LEVEL")` in `logOptions`.
- `cmd/dn-tool/main.go:96` - `os.Stderr` in the root `Before` hook.
- `cmd/dn-tool/main.go:161` - `os.Stderr` in the `main` bootstrap.
- `cmd/dn-tool/main.go:162` - `os.Args` in `main`.
- `internal/dnclient/client.go:71-72` - `os.Stderr` for the child's stdout and
  stderr. This one is **correct and stays**: redirecting subprocess output away
  from stdout is exactly what ADR 0005 requires. It is not a `Deps` concern.

Framework surface: `newApp() *cli.Command` (`cmd/dn-tool/main.go:88-156`)
returns a framework type, but only within package `main`; no domain package
imports `urfave/cli`, so ADR 0003's no-leak rule already holds for them. The
one remaining leak is `internal/output` (`internal/output/exit.go:6`), which
dt-uq9d removes.

Commands are wired at `cmd/dn-tool/main.go:114-147`; `OnUsageError` is set on
the root at `:94` and on every subcommand at `:152-154`.

## Target shape

Follow the cookiecutter reference,
`go-cookiecutter/{{cookiecutter.project_name}}/`:

- `internal/command/run.go` - the contract, mirroring
  `internal/command/run.go` in the reference. `Run(args []string, deps Deps) int`
  plus `Deps`. dn-tool needs more than the reference's three fields, because of
  the call sites listed above:

  ```go
  type Deps struct {
      In       io.Reader
      Out, Err io.Writer
      Getenv   func(string) string // main.go:22 and :64
  }
  ```

  A terminal-detection field is **not** added here; dt-cr62 adds it when it
  needs it (ADR 0003: "a field is added only when a test must fake it").
- `internal/command/root.go` - the framework interior, mirroring the
  reference's `root.go`: `runRoot(ctx, args, deps)`, `newRoot(deps)` binding
  `Reader`/`Writer`/`ErrWriter` to `deps`, and `neutralize(cmd)` walking the
  tree to disable the framework's own error printing, help-on-error, and exit
  handling so parse errors reach the boundary untouched.
- `internal/command/commands.go` - the command tree (the five existing
  commands, moved verbatim).
- `cmd/dn-tool/main.go` shrinks to exactly the reference's `main`:

  ```go
  func main() {
      os.Exit(command.Run(os.Args[1:], command.Deps{
          In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv,
      }))
  }
  ```

### Keep the structural usage classifier

The reference carries `internal/command/usage.go`, a string-matching
`classifyUsage`, because urfave's parse errors are untyped. **dn-tool does not
need it**: `onUsageError` (`cmd/dn-tool/main.go:73-75`) already maps parse
failures structurally via the framework's `OnUsageError` hook, which is
strictly better than matching messages, and ADR 0002 permits the string
carve-out but does not require it.

Note the interaction: `neutralize()` in the reference **overwrites**
`OnUsageError` to return the error unchanged. Do not blindly copy that. Either
keep dn-tool's mapping `OnUsageError` (set on the root and every subcommand,
per the v3 non-inheritance quirk documented at `main.go:149-151`) and have
`neutralize()` touch only `ExitErrHandler`, or adopt the reference's
`neutralize` plus `classifyUsage`. **Prefer the first**: it preserves a tested
structural mechanism and adds no string matching. Whichever is chosen, the
usage-class assertions in `cmd/dn-tool/exit_conformance_test.go:37-54` must
still pass.

### Do not change behavior here

This ticket is a **pure move**. Exit codes, output shapes, error text, and the
logging posture are all unchanged; they belong to dt-uq9d, dt-djf3, dt-nljm,
dt-3xxb, dt-cr62. The two `slog.SetDefault` calls move as-is and are removed by
dt-cr62. The bare-invocation change to exit 64 belongs to dt-nljm.

One unavoidable exception: `Run` must return an `int` rather than let the
framework exit. Wire it as the thinnest possible boundary that preserves current
mapping - call `output.ResolveExitCode` on the returned error and emit the error
text to `deps.Err` the way `cli.HandleExitCoder` does today. dt-uq9d then
replaces that body with `output.ExitCodeFor` + `output.EmitError`.

## Test migration

Move all nine `cmd/dn-tool/*_test.go` files to `internal/command`. Two need
rework beyond a move:

- `cmd/dn-tool/exit_conformance_test.go:16-29` - the `runApp` helper builds
  `newApp()` and reads a captured `cli.OsExiter` code via `captureExit`.
  Rewrite it to call `command.Run(args, Deps{…buffers…})` and read the returned
  `int`. This deletes the `captureExit` indirection entirely.
- `cmd/dn-tool/logging_test.go:16-34` - swaps a subcommand's `Action` on a
  `newApp()` tree to capture `logOptions`. It needs an in-package equivalent, or
  it can be left to dt-cr62, which replaces the whole `--log-text` surface this
  test covers. Coordinate: if dt-cr62 has not landed, port it as-is; do not
  delete coverage.

Tests that reference unexported identifiers (`runInstall`, `runEnroll`,
`runUnenroll`, `runRun`, `runWriteConfig`, `logOptions`, `exitWithError`) move
into `package command` alongside the code, so they keep compiling.

## Acceptance criteria

- `internal/command/run.go` declares `Run(args []string, deps Deps) int` and
  `Deps`; `go doc ./internal/command` shows no `urfave` type in any exported
  identifier.
- `cmd/dn-tool/main.go` contains only `main` and is the only place `os.Stdin`,
  `os.Stdout`, `os.Stderr`, `os.Args`, and `os.Getenv` appear
  (`internal/dnclient/client.go:71-72` excepted, per above).
- No command code reads `os.Stdout`/`os.Stderr`/`os.Args` directly; output goes
  through `deps`.
- All nine test files are carried over. **No test is deleted** without its
  coverage being replaced.
- Exit codes are unchanged. These must pass with their expectations intact:
  - `cmd/dn-tool/exit_conformance_test.go` - all five subtests, including
    `bare invocation shows help and exits zero` at `:65-69` (the bare-invocation
    change is dt-nljm's, not this ticket's).
  - `internal/output/exitcodes_test.go`
  - `internal/run/signal_test.go` (130/143)
  - `internal/run/run_test.go` (daemon passthrough)
- stdout still carries exactly one JSON object per successful command.
- Quality gate: `make build` green from the repo root
  (`fmt-check`, `vet`, `lint`, `test`, compile) per
  `docs/agents/build-and-validation.md`. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Docs

None. The design doc's §2.9 structure tree is stale in ways this ticket makes
staler; that is dt-o9hi's residual sweep (contradiction row 18).

## Dependencies

None. Blocks dt-uq9d, dt-3xxb, dt-cr62, dt-cjgq, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner. Staging is permitted only
where a workflow explicitly requires it.

## Notes

**2026-08-12T13:57:25Z**

Moved the whole CLI surface from package main into internal/command behind Run(args []string, deps Deps) int (ADR 0003). cmd/dn-tool/main.go is now just os.Exit(command.Run(os.Args[1:], Deps{In,Out,Err,Getenv})) and is the only place os.Std*/os.Args/os.Getenv appear (internal/dnclient/client.go:71-72 excepted, per the ticket).

Layout: internal/command/run.go = the framework-free contract (Deps, Deps.withDefaults, Run, exitCode); root.go = the interior (rootName, runRoot, newRoot, neutralize, logOptions, onUsageError, rootAction); commands.go = the tree + the actions struct that carries Deps + withResult/resultAction/errNoChange/loadConfig. The five action files moved verbatim except that each wired action became a method on actions (a.install/a.enroll/a.unenroll/a.run/a.writeConfig) so loadConfig resolves from deps.Getenv instead of os.Getenv. cmd/dn-tool/run.go became internal/command/run_command.go (run.go was taken by the contract); main_test.go became root_test.go (TestNewApp_* -> TestNewRoot_*).

Decisions worth knowing:
- neutralize() sets ExitErrHandler on the ROOT only, and touches nothing else. Command.handleExitCoder recurses to the parent before consulting the field, so the root's handler covers the whole tree; this is the opposite of OnUsageError, which is looked up on the command being parsed with no parent walk and is therefore still set on the root AND every subcommand. Per the ticket's preference, dn-tool keeps its structural OnUsageError mapping and adds no classifyUsage string matching.
- exitCode(err, w) in run.go replaces exitWithError. It reproduces cli.HandleExitCoder's selection deliberately: the innermost error carrying a code supplies both the printed message and the code (via output.ResolveExitCode), unclassified -> CodeInternal. It stays framework-free by declaring a local structural interface { error; ExitCode() int } rather than importing cli.ExitCoder. dt-uq9d replaces this body with output.ExitCodeFor + EmitError, and will also fix the discarded-wrapper-context defect this faithfully preserves.
- Deps.withDefaults() maps unset fields to io.Discard / empty reader / empty env. Needed: a nil Writer makes urfave fall back to the real os.Stdout, which would silently reintroduce process coupling.

Tests: all nine files carried over, none deleted. captureExit and the two TestExitWithError_* tests are gone, replaced by direct TestExitCode_* tests on the new boundary (nil/plain/coded/wrapped-coded). runApp() now calls Run(args, deps) and reads the returned int; assert_changed_test drives its stub tree through neutralize()+exitCode. The end-to-end unenroll and write-config tests dropped t.Setenv in favour of the injected Getenv, so the CLI suite no longer mutates the real process environment. New: TestRun_* contract tests (usage code, success + JSON on deps.Out, config from deps.Getenv, zero-Deps safety), TestLogOptions_LevelComesFromDepsGetenv, and conformance_test.go's TestPackageNeverTouchesTheProcessEnvironment, a source-level guard that fails if os.Getenv/os.Args/os.Std* reappears in the package's non-test files (verified it fails on a planted violation).

Behavior unchanged, verified on the built binary: bare=0, unknown command=64, bad global flag=64, bad subcommand flag=64, malformed DN_LISTEN_PORT=78, write-config emits one JSON object and exits 0, --version prints. go doc ./internal/command shows no urfave type in any exported identifier. make build green. Left for other tickets: bare invocation -> 64 (dt-nljm), the two slog.SetDefault calls (dt-cr62), design doc section 2.9's stale structure tree (dt-o9hi).
