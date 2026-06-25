# Learnings

Running notes of non-obvious problems solved and decisions worth carrying
forward. Newest first.

## write-config — 0600 is umask-independent by construction; the writer lives in config returning error, not output.Result (dt-meg1, 2026-06-06)

- **The SEC2 fix is structural and simpler than it looks: a single
  `os.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0o600)`, no chmod-after, is
  *already* umask-independent — because 0600 has no group/other bits and the
  umask can only *clear* bits, never set them.** So for any normal umask the
  created file is exactly 0600; there is no widen-then-restrict window to close
  because the file is never wider. The upstream SEC2 bug was the *opposite*
  pattern (create at umask ~0644, then `chmod 0640`), so the fix is to NOT chmod
  after — the ticket's "never widen-then-restrict" is satisfied by omission, not
  by an extra narrowing step. A chmod-to-0600-after-0600-create would be harmless
  but is exactly the anti-pattern to avoid, so it's left out.
- **The umask-0 test is the load-bearing assertion, not ceremony.** Setting
  `syscall.Umask(0)` makes a *default-mode* create yield 0666, so asserting the
  file is 0600 under umask 0 proves the mode came from the open flag, not from an
  inherited restrictive umask masking a bug. Restore the old umask in
  `t.Cleanup` (umask is process-global; don't leak it to sibling tests — and
  don't `t.Parallel()` these).
- **`WriteConfigFile(path, cfg) error` lives in `internal/config` and returns a
  bare error, NOT the ticket's literal `(output.Result, error)`.** Returning
  `output.Result` would add a `config -> output` import edge; today no such edge
  exists (`output` is stdlib + urfave only, and `api`/`config` don't import it).
  Keeping config free of output preserves that, and the trivial
  `Result{Action:"write-config", Changed:true}` is built one layer up in
  `cmd/dn-tool/write_config.go`. Same "trust the better contract over the toy
  signature" call as [[dt-hts2]]/[[dt-5o0x]]; the ticket explicitly allowed
  "internal/config *or* cmd", so the split (FS write in config, Result in cmd) is
  in-scope.
- **The target path is the positional arg (`cmd.Args().First()`,
  `ArgsUsage: "<path>"`), required up front** (`errMissingWriteConfigPath`),
  mirroring the [[dt-3gvq]] thin-action/testable-core split: `writeConfigAction`
  loads config + reads the arg, `runWriteConfig(cfg, path)` is the pure core the
  unit tests drive without booting the CLI. Round-trip is tested through the
  *real* loader (`WriteConfigFile` → `ParseEnvFile` → `Resolve`, `reflect.DeepEqual`)
  so the writer and [[dt-druh]] `Marshal` can't drift. API key persisted in
  cleartext, protected only by 0600 (the documented decision); the round-trip test
  asserts the raw key is present AND `REDACTED` is absent. Wiring `write-config`
  leaves only `run` as `notImplemented` (updated the stub-list test). Closing
  dt-meg1 makes parent epic [[dt-cmdg]] a verify-and-close.

## config.Marshal round-trip — empty JSON-array fields must serialize to EMPTY, not "[]" (dt-druh, 2026-06-06)

- **The round-trip property (`ParseEnvFile(Marshal(cfg))` == cfg) breaks on
  `nil` vs `[]string{}` for unset array fields.** A `Load`/`Resolve`-produced
  Config has `Tags == nil` when `DN_TAGS` is unset, because `parseJSONArray("")`
  returns `nil`. But `parseJSONArray("[]")` runs `json.Unmarshal([]byte("[]"),
  &out)` which yields a **non-nil empty slice** — so emitting `DN_TAGS=[]`
  round-trips to `[]string{}`, and `reflect.DeepEqual` fails (the structs even
  *print* identically with `%+v`, so the diff is invisible by eye — compare with
  `reflect.DeepEqual` and trust it). Fix: `marshalJSONArray` returns `""` for an
  empty/nil slice → emits `DN_TAGS=` → parses back to `nil`. Non-empty slices
  serialize as real JSON arrays.
- **`quote()` is the structural inverse of `envfile.go`'s `unquote()`, not a
  general shell-quoter.** `ParseEnvFile` does `TrimSpace(value)` *then* strips one
  outermost matching quote layer. So Marshal must quote a value only when the
  parser would otherwise alter it: (a) it contains whitespace (would be trimmed),
  or (b) it begins and ends with a matching quote char (would be unwrapped).
  Wrapping in `"..."` is safe even when the value has interior quotes — `unquote`
  removes only the single outermost layer — which is why a JSON tag array
  containing spaces, e.g. `["owner:Jane Doe"]`, round-trips when wrapped as
  `"[\"owner:Jane Doe\"]"`. No escaping scheme is needed because the parser has
  none; the two functions are co-designed.
- **`DN_API_KEY` is written in cleartext via `Secret.Reveal()` — this is the
  sanctioned site `secret.go` calls "the env-file writer".** Persisting the key is
  the documented [[dt-meg1]] decision; its protection is the file's `0600` mode,
  enforced by the *writer* (dt-meg1), not by `Marshal`. A test asserts the raw key
  is present AND that `REDACTED` is absent (a redacted secret would silently break
  the round trip). `APITimeout` round-trips via `Duration.String()` — `0` → `"0s"`
  → `parseTimeout` → `0`.
- **Scope: `Marshal([]byte, error)` is pure bytes, no filesystem.** Closing
  dt-druh unblocks dt-meg1, which owns `WriteConfigFile` + the `0600`-at-creation
  guarantee (SEC2). The round-trip is tested through the *real* loader
  (`Marshal`→`ParseEnvFile`→`Resolve`), not a hand-rolled parser, so the two
  halves can't drift.

## Lighthouse/relay field mapping — ListenPort passed through unconditionally, static addresses gated on lighthouse only (dt-wleh, 2026-06-06)

- **`req.ListenPort = cfg.ListenPort` is set for *every* host, not just
  lighthouse/relay — because 0 already means "auto-select" to the API.** Behaviors
  3 (configured port used) and 4 (plain host → system-selected) collapse to a
  single unconditional assignment: a plain host has `cfg.ListenPort==0` → sends 0 →
  auto, a lighthouse/relay (validated non-zero by [[dt-854m]]) sends its port, and
  a plain host that *happens* to set `DN_LISTEN_PORT` gets it passed through (the
  API allows a port on any host; validation only *requires* one for
  lighthouse/relay, never forbids it elsewhere). The `CreateHostRequest.ListenPort`
  field has no `omitempty` (correct — `listenPort:0` must serialize so the API sees
  the explicit auto request), so this doesn't change the plain-host body that the
  [[dt-j2ab]] tests already locked.
- **Static addresses are mapped ONLY under `IsLighthouse`, not relay or plain.**
  Req 3 / the API table: `staticAddresses` is "≥1 required if isLighthouse"; relay
  needs only role + port. So `if cfg.IsLighthouse { req.StaticAddresses =
  cfg.StaticAddrs }` — a relay or plain host never carries static addresses even if
  `DN_STATIC_ADDRESSES` is set in config. The `PlainHostHasNoRoleFields` regression
  guard sets `StaticAddrs` in the config with neither role and asserts
  `req.StaticAddresses == nil` — proving the role mapping can't bleed into the
  common path.
- **"Lighthouse role" == the `isLighthouse` boolean, not a separate roleID.** The
  ticket's "lighthouse role + static addresses" phrasing maps to setting the
  `isLighthouse` flag true; `roleID` stays `DN_ROLE_ID` (the API has no distinct
  lighthouse role field). Confirmed against API reference §4.1 / §5 before mapping —
  don't read "role" as a second identifier.
- **The `CreateHostRequest` struct already had all four fields** (StaticAddresses/
  ListenPort/IsLighthouse/IsRelay) from [[dt-j2ab]]'s endpoints.go — dt-wleh only
  populates them in `buildCreateRequest`. No API-layer change. Field-mapping tests
  assume valid input (validation is [[dt-854m]]'s contract), so they're pure
  `(Config)->CreateHostRequest` table tests, no mocks. Closes the last open child of
  epic [[dt-hxvr]] → it's now a verify-and-close.

## Lighthouse/relay validation gates — placed ABOVE the ConfigExists no-op, not just before remote calls (dt-854m, 2026-06-06)

- **`validateRoles(cfg)` runs at the very top of `Enroll`, before the
  `dnclient.ConfigExists` no-op check — not merely "before remote calls".** The
  ticket said "called at the top of Enroll, before remote calls"; `ConfigExists`
  is a local FS probe, so either placement satisfies the literal "before remote
  calls". I chose the strongest reading: gate first, so an **already-enrolled**
  host whose config later goes invalid (e.g. both `DN_IS_LIGHTHOUSE` and
  `DN_IS_RELAY` set) fails fast and surfaces the misconfig, rather than silently
  no-op'ing on the row-1 cell. The existing [[dt-brug]] no-op test stays green
  because `validConfig()` sets neither role. This is the fail-fast-on-misconfig
  intent of Req 3 ("creates no record") taken to its logical end.
- **Three gates, fixed check order: mutual-exclusion → lighthouse-needs-addr →
  port.** `IsLighthouse && IsRelay` first (the conflicting-intent error), then
  `IsLighthouse && len(StaticAddrs)==0`, then `(IsLighthouse||IsRelay) &&
  ListenPort==0`. A plain host (neither flag) short-circuits to nil — every gate
  is guarded by a role flag, so the common case is zero cost. Config fields were
  already parsed by [[dt-uzx6]]/[[dt-toqi]] (`parseJSONArray`, `parsePort`); this
  task is pure validation, no new parsing.
- **Behavior 6 ("gates before remote calls") reuses the [[dt-brug]] `failingAPI`
  mock with an EMPTY temp config root.** The no-op test points `failingAPI` (every
  method `t.Fatal`s) at a root *with* local config so row 1 never reaches the API;
  this test points it at an *empty* root + an invalid role config, so without the
  gate the state machine would fall through to `ListHosts` and trip the Fatal.
  Same mock, opposite setup — proves the gate structurally (the recurring
  call-the-Fatal-mock-to-prove-zero-traffic discipline), no call counter needed.
- **Held scope: did NOT touch `buildCreateRequest`.** Mapping the lighthouse/relay/
  static-address/listen-port fields onto `api.CreateHostRequest` is the sibling
  child dt-wleh (now unblocked); `buildCreateRequest` still builds the plain-host
  body (its doc comment already flags the deferral). Validation and field-mapping
  are deliberately two tickets — gate fails fast, mapping shapes the body — and
  conflating them here would pre-empt dt-wleh's tests (the [[dt-j2ab]]/[[dt-xcac]]
  scope-by-omission discipline). Closing dt-854m leaves only dt-wleh under epic
  [[dt-hxvr]].

## run daemon exit-status — `*exec.ExitError` already satisfies `cli.ExitCoder`, but map it explicitly to guard the signal-kill (-1) case (dt-r2ks, 2026-06-06)

- **The "daemon code N → dn-tool exit N" behavior passed before any code change —
  because `*exec.ExitError` *structurally* implements `cli.ExitCoder`.** `cli.ExitCoder`
  is just `error + ExitCode() int`, and `*exec.ExitError` has both (`ExitCode()`
  via the embedded `*os.ProcessState`). So [[dt-icq8]]'s `output.ResolveExitCode`
  (`errors.As(err, &cli.ExitCoder)`) was *already* extracting the daemon's code
  through the exec layer's `%w` wrap. Every mock-based behavior test (exit N,
  wrapped exit N, clean→0, unenroll-fail→1) was green on the existing
  `runAndUnenroll`. The TDD driver that actually went red was the **signal-kill**
  case (below) — without it, dt-r2ks would have been a no-op-code close.
- **The real bug the incidental path hides: a signal-terminated daemon maps to
  -1, i.e. a garbage process exit (`os.Exit(-1)`→255).** `*exec.ExitError.ExitCode()`
  returns `-1` when the process was killed by a signal (`Exited()==false`) — and
  on shutdown `exec.CommandContext` SIGKILLs the daemon when the run ctx cancels,
  so this is the *normal* signal-driven-shutdown shape, not an edge case. Relying
  on `ResolveExitCode` to pass the raw `-1` through is wrong. Fix: an explicit
  `daemonExitCode(err)` in `run` — `errors.As(&*exec.ExitError)` **and**
  `ExitCode() >= 0` → N, else `output.CodeError`. Then wrap the daemon error in
  `output.ExitError(daemonErr, daemonExitCode(daemonErr))` so the mapping is
  intentional and robust to the `cli.ExitCoder` coincidence, not dependent on it.
- **Test fixtures for genuine exit codes: run a real subprocess, don't fabricate
  an `*exec.ExitError`.** Its `ProcessState` can't be hand-built. `sh -c "exit 7"`
  yields a real `*exec.ExitError` with `ExitCode()==7`; `sh -c "kill -TERM $$"`
  yields the `-1` signal-terminated shape. Same exec-fixture posture as [[dt-a772]].
- **Precedence rule pinned: daemon code dominates, unenroll failure never masked
  silently.** daemon-errored → return its code, `slog.Error`-log a coincident
  unenroll failure (don't drop it, don't let it override N). daemon-clean +
  unenroll-failed → return the unenroll error (→ `CodeError`): a clean daemon exit
  must NOT mask a failed unenroll, because the host may still be enrolled. This
  supersedes the [[dt-n5p5]] interim "daemon error takes precedence" with the full
  rule. Both clean → nil → 0.
- **HELD SCOPE again: no `main.go` wiring; `run` stays `notImplemented`.** dt-r2ks
  is the internal exit-mapping only; the [[dt-cmn7]] epic wires `run.Lifecycle`
  (its sole remaining open work now that all three children — dt-flal/dt-n5p5/dt-r2ks
  — are closed). Same scope-by-omission discipline as the rest of the run epic.
  Note for dt-cmn7: graceful SIGTERM *forwarding* to the daemon (so it can exit 0
  itself rather than being SIGKILLed to -1) is a `dnclient` exec-client concern,
  not done here — without it a signal shutdown maps to `CodeError`, which is
  defensible but not a clean 0. Flag if the epic wants true graceful daemon stop.

## run signal handling — `Lifecycle` owns `signal.NotifyContext`, unenroll gets a Background-rooted fresh deadline, snapshot the ctx at call time (dt-n5p5, 2026-06-06)

- **`signal.NotifyContext` lives in `run.Lifecycle`, the testable core takes an
  injected context.** Split into two: `Lifecycle(ctx,cfg,deps)` wraps
  `signal.NotifyContext(ctx, SIGINT, SIGTERM)` + `defer stop()` and delegates to
  the unexported `runAndUnenroll(ctx,cfg,deps)`. Tests drive `runAndUnenroll` with
  a plain `context.WithCancel` (cancel == "signal arrived") so there are **no real
  OS signals** in unit tests — exactly the [[dt-a772]] "avoid OS-signal flakiness"
  posture. `Lifecycle` itself is still covered without signals: a *pre-cancelled
  parent* ctx propagates through `NotifyContext` to the daemon, exercising the
  wrapper. This is where the [[dt-flal]] note said the signal ctx would land
  (dt-flal held command wiring for "the signal/exit children" — this is that child).
- **Unenroll MUST run under `context.WithTimeout(context.Background(), …)`, never a
  child of the cancelled run ctx.** The whole point of the signal path is that the
  run ctx is *already cancelled* when unenroll fires; deriving the unenroll ctx
  from it would abort the remote DELETE before it starts. Rooting at
  `context.Background()` gives a fresh, non-cancelled deadline — the binary-side
  mirror of the module `TimeoutStopSec`/D5. `Deps.UnenrollTimeout` (zero →
  `defaultUnenrollTimeout` 10s, matching the unenroll command's own default).
- **Test subtlety that bit immediately: snapshot the unenroll ctx state AT CALL
  TIME, not after.** `runAndUnenroll` does `defer cancel()` on the unenroll ctx, so
  a mock that stashes the raw `context.Context` and lets the test inspect it *after
  `runAndUnenroll` returns* always sees `context.Canceled` — the deferred cancel
  already fired. First draft did exactly this and the behavior-2 test failed with
  "context already cancelled" against a correct implementation. Fix: the mock
  records `unenrollCapture{errAtCall: ctx.Err(), hasDeadline: ctx.Deadline()!=nil}`
  *inside* the call. General rule: when asserting a handed context's liveness,
  capture `Err()`/`Deadline()` synchronously in the double, never the ctx value.
- **Always unenroll once the foreground daemon returns — there is no container
  skip knob.** `DN_SKIP_UNENROLL` and reboot-vs-poweroff discrimination are
  systemd-module-only (§2.7, Q3→A); `run` is the non-systemd path so it
  unconditionally unenrolls on daemon exit (signal *or* clean self-exit). Tested
  both: cancellation-driven and a plain clean daemon return both reach `unenroll`.
- **Interim precedence, explicitly leaving the exit-code mapping to [[dt-r2ks]].**
  `runAndUnenroll` returns the daemon error when present (the foreground outcome
  dt-r2ks maps via `exec.ExitError`), returns the unenroll error on an
  otherwise-clean shutdown (host may still be enrolled — can't be silent), and
  `slog.Error`-logs a unenroll failure that *coincides* with a daemon error rather
  than dropping it. dt-r2ks owns the final daemon-vs-unenroll precedence + the
  `errors.As(&exec.ExitError{})` → exit-N mapping. HELD SCOPE: no `main.go` wiring;
  `run` stays `notImplemented` — the [[dt-cmn7]] epic wires `run.Lifecycle`
  (same scope-by-omission discipline as [[dt-flal]]/[[dt-i0yx]]).
- **`cap` is a Go builtin — revive `redefines-builtin-id` rejects it.** Named a
  test capture var/param `cap` and the gate failed lint (not vet/compile). Renamed
  to `capture`. Add to the avoid-shadowing list alongside the usual `len`/`new`.

## run compose core — function-typed Install/Enroll deps, daemon error returned bare, command wiring held for the signal/exit children (dt-flal, 2026-06-06)

- **`internal/run.Run` composes, it does not fork.** `Deps.Install`/`Deps.Enroll`
  are `func(ctx) (output.Result, error)` closures, not new interfaces — the
  command layer will wrap the existing [[dt-svmu]] install and [[dt-i0yx]] enroll
  cores as closures so `run` reuses them verbatim (the dt-cmn7 "reuse, don't fork"
  note). Function fields are the honest seam here: install/enroll are package-level
  functions with their own already-built deps, so an interface would be ceremony —
  closures let tests record call order and inject failures, and let the command
  layer bind the production cores without `run` knowing their collaborators.
- **The daemon error returns unwrapped; install/enroll errors wrap with `%w`.**
  The daemon (`dnclient.Client.Run`) is the foreground process whose termination
  *is* run's outcome, so its error flows out bare for [[dt-r2ks]] to map (it
  extracts `*exec.ExitError`'s code via `errors.As` — the [[dt-a772]] exec shim
  already keeps `%w`, so `errors.As` still crosses it even if a caller wraps). The
  pre-daemon steps wrap (`fmt.Errorf("install: %w", …)`) to name which step
  aborted. Same two-contract error discipline as [[dt-pe29]].
- **The abort-before-daemon ordering is the Req-5 invariant, proven structurally.**
  `Run` short-circuits on the first install/enroll error, so the daemon never
  starts unless both succeeded — the host is never left running a daemon it never
  enrolled. Tests assert this via a shared `recorder` of step names
  (`["install","enroll","daemon.run"]`) and check the events slice truncates on
  failure (`["install"]` / `["install","enroll"]`), not via comments — the
  [[dt-pe29]] structural-guard style.
- **HELD SCOPE: no `main.go` wiring; `run` stays `notImplemented`.** dt-flal is the
  compose *core* only. The command-layer context is built by the siblings:
  [[dt-n5p5]] wraps `Run` with `signal.NotifyContext(SIGTERM,SIGINT)` and the
  signal→unenroll path, and [[dt-r2ks]] does the daemon-exit→exit-code mapping.
  Wiring the command now would write command code those two will immediately
  rewrite (signal ctx + exit mapping) and pre-empt their tests — the recurring
  scope-by-omission discipline ([[dt-j2ab]]/[[dt-xcac]]). Closing dt-flal unblocks
  both. Note this is the inverse of the [[dt-i0yx]] call where the epic *did* own
  the wiring: there no child claimed `main.go`; here two explicitly do.

## Enroll command wired — the epic owns the command, no API-key pre-gate, BinaryPath single-sources the exec path (dt-i0yx, 2026-06-06)

- **Second instance of the [[dt-3gvq]] "all-children-closed epic that is NOT a
  verify-and-close" pattern.** The 5 children (dt-brug/pe29/xcac/a772/j2ab) built
  and exhaustively tested the internal `enroll.Enroll` state machine, but no child
  owned `main.go` wiring — `enroll` was still `Action: notImplemented`. dt-i0yx
  _is_ the enroll command, so the integration is its remaining work. Same
  re-read-acceptance-against-reality check that confirms some epics are done
  ([[dt-8h9t]]/[[dt-cq78]]) exposes the missing seam here. `cmd/dn-tool/enroll.go`
  mirrors `unenroll.go` exactly: thin `enrollAction` (loadConfig → `runEnroll`)
  wrapped by `withResult`; `runEnroll(ctx, cfg, configRoot, force)` is the
  testable core (no `*cli.Command`), bounding ctx by `enrollTimeout` and building
  `api.New(cfg)` + `dnclient.NewExecClient(dnclient.BinaryPath(cfg.ClientBinDir))`.
- **No API-key pre-gate, unlike [[dt-3gvq]] unenroll.** unenroll checks
  `cfg.APIKey == ""` up front because it always hits the API. enroll must NOT:
  §2.4 row 1 (local config present) is a no-op that needs no key, network, or
  dnclient call. Required-param validation (DN_API_KEY/DN_NETWORK_ID/DN_ROLE_ID)
  already lives in `buildCreateRequest` ([[dt-j2ab]]) on the create path only, so
  pre-gating in the command would wrongly fail the legitimate already-enrolled
  no-op. The command layer adds only the timeout clock; the state machine owns the
  decision. `defaultEnrollTimeout = 30s` (§2.3), honoring `DN_API_TIMEOUT`.
- **Added exported `dnclient.BinaryPath(binDir)` to single-source the exec path.**
  `binaryName`/the `filepath.Join(binDir, "dnclient")` was unexported in install;
  enroll needs the same path to exec what install wrote. Rather than re-hardcode
  "dnclient" at the command layer, exposed `BinaryPath` and refactored
  `install.go` to call it too — install-writes and enroll-execs can never drift.
- **Testing the create path needs a real executable, not a mock.** The command
  builds the _production_ exec client, so the create-cell command test writes a
  fake `dnclient` shell script to a temp bin dir (`#!/bin/sh … >> argLog; exit 0`)
  and asserts the logged args are `enroll -name testnet -code SECRET-CODE` —
  proving the bin-path→exec→`-name`/`-code` wiring end-to-end. The internal
  package mocks the `Client` interface; the command layer can't (it constructs the
  concrete client), so a fixture binary is the honest seam. The no-op test points
  `APIURL` at a server that `t.Error`s on any hit, proving row 1 touches nothing.
- Closing dt-i0yx unblocks dt-hxvr (lighthouse/relay enrollment) and dt-cmn7 (run,
  which composes install+enroll+daemon+unenroll).

## Enroll orphan/force cell — `--force` is a Deps option, the match loop folds delete into the create path (dt-xcac, 2026-06-06)

- **`--force` went on `enroll.Deps` (`Force bool`), not `config.Config`.** It's an
  enroll-scoped CLI flag (`main_test.go`'s `TestNewApp_ForceIsEnrollScoped` already
  pins it to the enroll subcommand only), never a `DN_*` env var — so it doesn't
  belong in `Config`. It's also not a collaborator, but threading it through `Deps`
  keeps `Enroll(ctx, cfg, deps)`'s signature stable and leaves every existing
  [[dt-pe29]]/[[dt-brug]] test compiling with `Force` defaulting false (zero value =
  the safe default-fail). The command layer (parent epic dt-i0yx) will set
  `Force: cmd.Bool("force")`.
- **The orphan cell is the same list-and-match loop, not a new branch.** Replaced
  the `errOrphanUnimplemented` early-return with: on `h.Name == cfg.Hostname`, if
  `!Force` return a guidance error naming the host+id and instructing `--force`
  (no delete/create/enroll, Changed=false); if `Force`, `DeleteHost(ctx, h.ID)`
  then `break` and fall through to the **existing** create→code→enroll path. So
  the force path reuses [[dt-pe29]]'s create leg verbatim — DELETE is the only new
  side effect, slotted before the create that was already there.
- **Force-on-absent/absent needs no special case — the loop just doesn't fire.**
  Behavior 4 (no spurious delete when nothing matches) falls out for free: no name
  match → no delete → the create path runs exactly as the plain absent/absent cell.
  Tested it explicitly anyway (the [[dt-pe29]] structural-guard discipline) since
  "harmless" is a claim worth a regression lock.
- **Two error-wrap contracts again ([[dt-pe29]]'s pattern):** the delete failure
  wraps with `%w` (`fmt.Errorf("deleting stale host record %s: %w", …)`) so behavior
  3's `errors.As(&*api.APIError)` crosses it; the orphan guidance is a plain
  `fmt.Errorf` (no wrap — it's an operator instruction, not an API error to inspect)
  and the test asserts the literal `--force` substring.
- **Extended the shared `scriptedAPI` mock with a *guarded* delete rather than a
  second mock.** `DeleteHost` Fatal's unless `allowDelete` is set, so the create-cell
  tests keep their "must not delete" guard untouched while orphan tests opt in and
  record `deletedID` + `createSawDelete` (set as `deleteCalls > 0` inside the create
  mock) to assert delete-before-create ordering structurally. One mock, two
  contracts, no redeclaration clash. Closes the last open task under epic dt-i0yx
  (only the epic verify-and-close remains; dt-i0yx unblocks dt-hxvr lighthouse/relay
  and dt-cmn7 run).

## Enroll create cell — list-then-match before create, abort-before-subprocess is the B4 fix (dt-pe29, 2026-06-06)

- **The B4 orphan fix is an ordering invariant, not a cleanup step: every
  management-API error returns _before_ `deps.DNClient.Enroll` is reached.** The
  create cell runs ListHosts → (name match) → CreateHostAndEnrollmentCode →
  `dnclient enroll`, and a failure at either API step short-circuits with the
  subprocess untouched. So the only way a remote record exists without a started
  enrollment is the §2.5 enroll-path orphan (create succeeded, then `dnclient
  enroll` itself failed) — the narrow case `--force` recovers. Tests prove the
  guard structurally: the `recordingEnroller`/`scriptedAPI` mocks count calls and
  assert `enroller.calls == 0` after a create/list error, rather than trusting a
  comment.
- **A ListHosts failure must abort, not fall through to create.** Remote presence
  is unknown on a list error, so proceeding to create risks a duplicate (or, with
  the API's own duplicate-name 400, a confusing failure). Treat unknown as "stop"
  — `fmt.Errorf("listing hosts …: %w", err)`. The happy path's "no match → create"
  only fires on a _successful_ empty/non-matching list.
- **Wrap API errors with `%w`, return the subprocess error bare.** Behavior 3
  asserts `errors.As(err, &*api.APIError)` so the create/list wraps use `%w` to
  keep the chain crossable; behavior 4 asserts `errors.Is(err, enrollErr)` on the
  dnclient failure, so that one is returned verbatim (identity preserved, no
  added context that would obscure dnclient's own message). Two different
  error-inspection contracts, two different wrapping choices.
- **Added `DNClient dnclient.Client` to `enroll.Deps` here — the field
  [[dt-brug]]/[[dt-a772]] deliberately deferred to "the cell that runs enroll."**
  This is that cell. The no-op (row 1) and orphan-placeholder paths still never
  touch it, so the [[dt-brug]] `failingAPI` no-op test needs no `DNClient` and
  stays green. Held scope: the remote-present branch returns a new
  `errOrphanUnimplemented` placeholder — the orphan/force cell is dt-xcac, and
  implementing DELETE-then-recreate here would pre-empt its tests (the
  [[dt-j2ab]]/[[dt-toqi]] scope-by-omission discipline). Renamed the old
  `errLocalAbsentUnimplemented` accordingly.
- **The code is `Reveal()`d at exactly one new site** — the `dnclient enroll`
  hand-off — matching the [[dt-toaj]] single-greppable-accessor rule. Behavior 5's
  test asserts the revealed code reaches the mock's `gotCode` _and_ never appears
  in the `output.Result` fields; the never-logged guarantee itself lives in the
  [[dt-a772]] exec shim (logs only action+bin, never args).

## dnclient Enroller interface — the -name arg is load-bearing; `exec sleep` in the kill test (dt-a772, 2026-06-06)

- **Deviated from the ticket's `Enroll(ctx, code string)` to `Enroll(ctx,
networkName, code string)` — the `-name` is not optional flavor, it is the
  per-network contract the rest of the tool already assumes.** Upstream ran
  `dnclient enroll -name "$DN_NETWORK_NAME" -code "$CODE"`, and that `-name` is
  what makes dnclient write `/etc/defined/<network>/dnclient.yml` — the exact path
  `ConfigExists`/`ReadHostID` ([[dt-brug]], §2.6) and unenroll read. Drop it and
  the whole per-network config-path scheme breaks. So the "toy signature" here was
  genuinely incomplete, not just minimal; same "trust the real contract" call as
  [[dt-hts2]]/[[dt-4h21]]. `NewExecClient(binPath)` stays stateless per the ticket
  (network name is a per-call arg, not constructor state) — the client is a thin
  exec shim, not a config holder.
- **`Run(ctx, args...)` prepends the `run` subcommand**; the caller (dt-flal)
  passes only `-server <url> -name <network>`. The method name _is_ the subcommand,
  so prepending it (vs. making the caller repeat "run") keeps the seam honest.
- **Behavior 4 (never log the code) is satisfied by structure, not scrubbing:**
  the exec shim logs only `action`+`bin` at debug — never the arg list — so the
  code (an arg) can't reach our logs, and the surfaced error is `exec.ExitError`'s
  bare `"exit status N"` (no args). Child stdout AND stderr both go to `os.Stderr`
  (stdout is reserved for the JSON result, §2.8); if dnclient itself echoed the
  code that'd be its leak, but our wiring adds none. Test pins both the success and
  failure paths against a slog buffer + the error string.
- **The ctx-cancellation test must `exec sleep 30`, not bare `sleep 30`.** A bare
  `sleep` is a _grandchild_ of the test process (sh → sleep); `CommandContext`
  SIGKILLs only the direct `sh`, orphaning `sleep`, which keeps the inherited
  stderr fd (the go-test output pipe) open — so `go test` blocks on EOF for the
  full 30s _even though the test asserts <5s and passes_ (the package run jumps
  30s, the test itself 0.05s). `exec sleep` makes sh _replace_ itself, so there's
  one direct child to kill — and that also models the real case (dnclient is the
  single direct child). If a kill-on-cancel test passes but the package wall-clock
  balloons, look for an orphaned grandchild holding a std fd.
- **Held scope: did NOT add the `Client` to `enroll.Deps`.** [[dt-brug]] explicitly
  deferred the dnclient field to "the cell that runs enroll" — that's dt-pe29
  (create cell), which this unblocks along with dt-flal (run). Wiring it here would
  pre-empt dt-pe29's state-machine tests (the [[dt-j2ab]]/[[dt-toqi]]
  scope-by-omission discipline). Mockability is asserted with a compile-time
  `var _ Client = (*execClient)(nil)`, not a mock-tests-the-mock unit test.

## Install orchestration + command wiring — atomic placement, OS gate at the boundary (dt-svmu, 2026-06-06)

- **Second command wired, reusing the [[dt-3gvq]] command-layer pattern.**
  `cmd/dn-tool/install.go` mirrors `unenroll.go`: thin `installAction(ctx, cmd)`
  (loads config via the shared `loadConfig`, bounds ctx, builds `api.New(cfg)`,
  calls the internal orchestrator) wrapped by `withResult`. `internal/dnclient.
Install(ctx, InstallDeps, InstallOptions)` is the testable core taking injected
  deps (no cli, no live API) — `Downloader` interface + `*http.Client` + resolved
  `Platform`. Same "tiny cli shell, plain-value core" seam.
- **OS/arch gate lives in `installAction`, not `Install` or `main`.** Per the
  dt-koaf note ("gate the OS check at the install boundary so non-install commands
  stay testable on macOS"), the command calls `DetectPlatform(runtime.GOOS,
runtime.GOARCH)` and passes the resolved `Platform` into `Install` via deps.
  So `install` fails clearly on darwin dev ("unsupported operating system") while
  unenroll/etc. still run, and `Install`'s tests inject `Platform{linux, amd64}`
  regardless of the test runner's real arch.
- **Exposed `api.Client.HTTPClient()` — the design already called for it.**
  `verify.go`'s `DownloadAndVerify` doc says "pass api.Client's StandardClient"
  so binary fetches share the Req-9 retry/backoff. The resilient client was built
  in `api.New` but unexported; added a one-line accessor rather than rebuilding a
  client at the command layer (the retryable constructor is unexported in `api`).
- **Atomic placement = temp-in-target-dir → verify → fsync → chmod → rename, with
  a deferred `os.Remove(tmpPath)`.** `placeBinary` writes the download to
  `os.CreateTemp(binDir, "dnclient.tmp-*")`, and only a _fully verified, fsynced,
  closed, +x_ temp is `os.Rename`d onto the final path — so the final path only
  ever changes via an atomic rename of a good binary, and any download/verify
  failure removes the temp (the deferred Remove is a no-op ENOENT after a
  successful rename). Tests prove it with `assertNoTempFiles` (glob
  `dnclient.tmp-*`) + content assertions on success/failure.
- **Checksum is fetched twice on the install path, once on skip — deliberate.**
  `Install` does `fetchChecksum` for `NeedsInstall`'s digest; when an install is
  needed, `DownloadAndVerify` re-fetches it internally. The skip path (the common
  boot no-op) does a single small `.sha256` GET + local hash, no binary download.
  Re-fetching a tiny checksum keeps fail-closed verification single-sourced in
  `DownloadAndVerify` (see [[dt-5o0x]]) — worth it.
- **No API-key pre-gate on install (unlike [[dt-3gvq]]).** Design §2.3 scopes
  `DN_API_KEY` "Required for enroll/unenroll" — _not_ install. `GET /v1/downloads`
  is authenticated by the client; a missing key surfaces as the API's own clear
  error, not a bespoke pre-flight check. Held scope: didn't reuse
  `errMissingAPIKey` here.
- **`defaultInstallTimeout = 60s` (vs unenroll's 10s) — a documented judgment
  call.** §2.3 bounds enroll ~30s / unenroll ~10s but is silent on install, whose
  cost is the binary download, not a JSON call. Honor `DN_API_TIMEOUT` when set,
  else 60s. Closing dt-svmu (the last open child) makes epic [[dt-koaf]] a
  verify-and-close; it unblocks dt-flal (run) and dt-a772 (Enroller).

## Install idempotency — NeedsInstall is a pure digest check, not a Resolved+ctx fetch (dt-5o0x, 2026-06-06)

- **The ticket's "Public interface" draft (`NeedsInstall(ctx, path, want
Resolved)`) lost to its own "Test strategy" (`No network — the expected digest
is passed in`).** Same conflict-resolution as [[dt-4h21]]/[[dt-brug]]: the two
  halves of the ticket disagreed, so I trusted the concrete behavioral half.
  `Resolved` (from [[dt-hts2]]) carries only the checksum _URL_, never the digest
  — the digest comes from the sibling `.sha256` fetch ([[dt-4fgk]]
  `fetchChecksum`/`DownloadAndVerify`) — so a `want Resolved` signature _forces_ a
  network fetch, contradicting "no network." Final shape: `NeedsInstall(path,
expectedDigest string) (need bool, reason string, err error)` — pure, no ctx.
- **Keeping it network-free single-sources fail-closed verification.** The
  download path already fetches + verifies the published digest fail-closed
  ([[dt-4fgk]]); having `NeedsInstall` re-fetch would split that integrity logic
  across two functions. The composition dt-svmu will write: caller fetches the
  digest once, passes it to `NeedsInstall`; on `need` it calls `DownloadAndVerify`
  (which re-verifies). The tiny `.sha256` double-fetch is negligible and worth the
  single-sourced verification.
- **Digest identity _is_ the version+integrity check.** A given dnclient version
  has a fixed binary digest, so "matches version AND checksum" collapses to one
  comparison: on-disk SHA-256 == expectedDigest → skip; else (missing or differs)
  → (re)install. No separate version probe (the binary exposes none cheaply).
- **On any inspection error, return `need=false` (not true).** An unreadable/
  locked target surfaces a clear error _and_ withholds the install signal, so a
  caller that fails to check `err` can't be tricked into clobbering an
  unreadable target. `os.ErrNotExist` is the _only_ error that means "install"
  (need=true); every other read error propagates.
- **Test the unreadable branch with a directory at the path, not chmod 000.**
  uid is 2000 here (non-root, so 000 would work) but a directory is
  uid-independent and deterministic everywhere: `os.Open` succeeds, the `io.Copy`
  read returns a non-ENOENT EISDIR error — exactly the "surfaces an I/O error,
  doesn't silently skip" behavior, with no root-skip guard.
- **Reused-helper check ([[dt-brug]] pattern) before adding `writeBinaryFixture`/
  `digestOf`.** `hostid_test.go` already had `writeConfig(t,root,network,content)`
  (YAML configs) — different name + purpose, so no redeclaration clash in the
  shared package test scope. Closes one of dt-koaf's two open children; dt-svmu
  (placement + result/exit wiring) is the remaining one and consumes this
  `need`/`changed` signal.

## First command wired end-to-end — unenroll sets the command-layer pattern (dt-3gvq, 2026-06-06)

- **An "all children closed" epic that is NOT a verify-and-close.** Unlike
  [[dt-8h9t]]/[[dt-cq78]]/[[dt-zwgc]] (which re-read acceptance and closed with
  zero code), dt-3gvq's three children built the _internal_ unenroll logic but no
  child owned `main.go` wiring — the command was still `notImplemented`. The
  [[dt-brug]] note had explicitly deferred command wiring "to the parent
  epic/command tickets." The same "re-read acceptance against reality" discipline
  that confirms some epics are done here exposed the missing seam: dt-3gvq _is_
  the unenroll command, so the integration is its remaining work. Re-reading
  beats trusting the `tk ready` "all children closed" signal either way.
- **Testable-core / thin-wrapper split, applied at the command layer.** Mirrored
  the package-wide `load(getenv,hostname)`/`Load` and `resolve`/`Resolve` shape:
  `unenrollAction(ctx, *cli.Command)` (loads config from the cmd, passes
  `defaultConfigRoot`) is the thin wrapper; `runUnenroll(ctx, *config.Config,
configRoot string)` is the testable core taking plain values. The core has **no
  `*cli.Command` dependency**, so the four command tests inject a hand-built
  `config.Config` + `t.TempDir()` + `httptest` URL — no `t.Setenv`, no booting the
  CLI. This is the cleanest seam for the remaining command epics (enroll/install/
  run/write-config) to copy: keep the cli-coupled shell tiny, test the core.
- **`config.LoadWithEnvFile(cmd.String("env-file"), os.Getenv)` is the shared
  command bootstrap.** Extracted as `loadConfig(cmd)` in `main.go` (cross-command
  infra, lives with `logOptions`/`exitWithError`), plus `defaultConfigRoot =
"/etc/defined"` — the design's "config root default lives in the caller" made
  concrete ([[dt-0nl5]]). The internal layers stay root-injectable; only the
  command layer names `/etc/defined`.
- **API key required at the command layer, not the API client.** unenroll only
  needs `DN_API_KEY` (enroll's network/role validation lives in `internal/api`
  per [[dt-j2ab]]), so `runUnenroll` checks `cfg.APIKey == ""` up front and
  returns `errMissingAPIKey` before constructing the client or reading the FS.
  `cfg.APIKey == ""` compares the `config.Secret` without `Reveal()` — emptiness
  doesn't need the value, so no secret leak ([[secret.go]] discipline).
- **Bounded deadline = `context.WithTimeout` in the core; default 10s.**
  `DN_API_TIMEOUT` (`cfg.APITimeout`) when >0, else `defaultUnenrollTimeout =
10*time.Second` — the binary-side counterpart to the module's `TimeoutStopSec`
  (§2.5/D5). The §2.5 local/remote invariant itself stays _inside_
  `unenroll.Unenroll`; the command only sets the clock.
- **Use a 403 (not a 5xx) to test the delete-failure branch fast.** The api
  transport retries 5xx/429 4× with ≥1s backoff ([[dt-egz4]]) — a 500 test would
  take seconds. A 403 is non-2xx, non-404, and _not_ retried, so it exercises the
  "delete failed → local retained, exit 1" invariant immediately.
- **Updated `TestNewApp_SubcommandsReturnNotImplemented` to drop `unenroll`.** A
  wired command breaks the blanket stub test — pruning its name (leaving the
  other four) is part of wiring, not an afterthought. Closing dt-3gvq unblocks
  the `run` command (dt-cmn7), which composes enroll+unenroll.

## Output epic — verify-and-close, the logger seam was already wired (dt-cq78, 2026-06-06)

- **Third instance of the [[dt-8h9t]]/[[dt-vsi6]] "all children closed → re-read
  acceptance, then close" pattern — this one needed zero code.** dt-cq78's four
  children ([[dt-ccmn]] JSON result writer, [[dt-zxwf]] slog JSON-to-stderr +
  level, [[dt-0jfp]] `--log-text`, [[dt-toaj]] `Secret` redaction) each shipped a
  piece; re-reading the epic's acceptance bullets against reality confirmed every
  one is met _and_ test-covered: `TestWriteResultShape/SingleObject/OmitsEmpty`
  (stdout JSON), `TestNewLoggerJSONByDefault` (stderr JSON default),
  `TestNewLoggerTextMode` + `TestLogText_FlagWiredToLogOptions` (`--log-text`),
  `TestNewLoggerLevelFiltering/LevelParsing` (level), `TestSecretRedacts*`
  (DN_API_KEY + enrollment codes, SEC5). Closed with no edit.
- **The dt-vsi6/dt-uzx6 "residual integration seam" check came back negative
  here — and proving the negative is the work.** Those epics each still owned an
  un-built bridge (CI; `LoadWithEnvFile`). dt-cq78's only process-level wiring is
  the default logger, and `main()` already does it twice: a bootstrap
  `slog.SetDefault(NewLogger(os.Stderr, …))` for pre-flag-parse failures, then the
  root `Before` hook re-runs it once `--log-text` is parsed. The one thing that
  _looks_ un-wired — `withResult`→commands (commands are still `notImplemented`) —
  is **downstream command-ticket work by explicit prior decision** ([[dt-4h21]]:
  "did NOT wire `withResult` into the still-`notImplemented` commands; that's each
  command ticket's job"). So it's not an epic gap; it's a correctly-deferred seam.
- **Test for "is the epic actually done": (a) every acceptance bullet has a green
  test, (b) every process-level seam the epic _owns_ is wired in `main`, (c) any
  un-wired seam is one a _closed_ ticket already assigned downstream.** All three
  held → close. Sibling [[dt-zwgc]] (exit-status, both children closed) is the
  matching verify-and-close; **both** must close before the P0 command tickets
  ([[dt-koaf]]/[[dt-i0yx]]/[[dt-3gvq]]/dt-cmdg) unblock — closing one alone moves
  no `tk ready` needle, so dt-zwgc is the natural next pick.

## Plain-text logging — flag wired via a root Before hook, not main() (dt-0jfp, 2026-06-06)

- **`NewLogger` only needed a two-branch handler swap; [[dt-zxwf]] had already
  shipped the `LogOptions.Text` field as a no-op contract.** Implemented exactly
  as dt-zxwf predicted: `if opts.Text { slog.NewTextHandler } else {
slog.NewJSONHandler }`, same `*slog.HandlerOptions{Level:…}` either way so
  level filtering is identical across modes (behavior 3 is free — the handler,
  not the format, owns the threshold). The text-mode test asserts the output is
  _not_ JSON-parseable AND contains `msg=hello`/`level=INFO`/`host=abc`; a
  JSON-only regression can't satisfy both.
- **The flag can't be read in `main()` — flags aren't parsed until inside
  `Run`.** `main()` calls `slog.SetDefault` _before_ `Run`, so it can't see
  `--log-text`. The fix is a root `Before` hook
  (`func(ctx, *cli.Command) (context.Context, error)`) that re-runs
  `slog.SetDefault(NewLogger(os.Stderr, logOptions(cmd)))` once global flags are
  parsed. `main()` keeps a **bootstrap** JSON logger so failures _before_ the
  hook (e.g. a flag-parse error) still log; the hook reconfigures it. Two
  `SetDefault` calls is the standard bootstrap-then-refine pattern, not a smell.
- **Behavior 4 ("flag wired") is tested by driving the real app and swapping one
  action — the only way to observe a post-parse value.** Extracted a pure
  `logOptions(cmd) output.LogOptions` (reads `cmd.Bool("log-text")` +
  `DN_LOG_LEVEL`); the test runs `newApp()` with `install`'s `Action` replaced
  by a recorder that captures `logOptions(cmd)`, then asserts `.Text` tracks
  `--log-text`. This exercises the genuine flag→`cmd.Bool`→`LogOptions` path
  (urfave v3 inherits root flags into subcommands, the [[dt-ecf9]] property)
  without needing to intercept `os.Stderr`. The hook itself is one line of
  composition over two already-tested pieces (`NewLogger` text branch +
  `logOptions`), so it needs no separate writer-injection seam — same
  "test the pieces, the composition is trivial" call as [[dt-zxwf]].
- **Closes the last open child of epic [[dt-cq78]]** (output & observability).
  With dt-0jfp done, both dt-cq78 and [[dt-zwgc]] (all children already closed)
  are verify-and-close candidates that unblock the P0 install/enroll/unenroll
  commands (the [[dt-8h9t]]/[[dt-vsi6]] epic-terminal-action pattern).

## Download + SHA-256 verify — fetch checksum first, fail closed (dt-4fgk, 2026-06-06)

- **Fetch the `.sha256` BEFORE the binary, not after.** The ticket's interface
  ordered it that way and it's the cheapest fail-closed path: a missing/5xx
  checksum aborts before a single binary byte is downloaded, so `dest` stays
  empty (asserted by `TestDownloadAndVerifyChecksumFetchFailureFailsClosed`).
  Closes upstream SEC1 (no integrity verification). Both GETs check
  `resp.StatusCode` before reading the body — same D4 status-before-body
  discipline as [[dt-z99h]]'s `api.do`.
- **Stream the hash with `io.Copy(dest, io.TeeReader(resp.Body, sha256.New()))`
  — never buffer the whole binary.** The digest is computed over the _actual_
  bytes written, so a truncated/corrupt body mismatches
  (`TestDownloadAndVerifyDigestOverActualStream` serves the full binary's
  checksum but a truncated body). On mismatch we return an error; `dest` may
  already hold (partial) bytes — that's by design. The contract is "on error,
  don't trust dest"; the _installer_ (dt-5o0x) writes to a temp + rename, so
  this function is only the sink. Did **not** add temp-file/atomic-place logic
  here — that's dt-5o0x's job (scope-by-omission, the [[dt-j2ab]] pattern).
- **Signature takes a plain `*http.Client`, not `*api.Client`.** The install
  command injects `api`'s `StandardClient()` (resilient/retrying transport) so a
  transient binary 5xx is retried ("after retry" behavior 4); unit tests pass
  `http.DefaultClient`. Keeping it a stdlib `*http.Client` means `internal/dnclient`
  needs no `internal/api` import for _this_ file (it already imports api for
  `Resolved`/`ResolveDownload` from [[dt-hts2]], but the verify path adds no new
  coupling and is trivially unit-testable with httptest).
- **Tolerate whitespace in the checksum body even though §6.3 says "no trailing
  data."** Real CDN/file artifacts routinely carry a trailing newline, so
  `strings.ToLower(strings.TrimSpace(body))` then validate exactly
  `sha256.Size*2` (64) hex chars via `hex.DecodeString`. A strict byte-equal
  check would produce brittle false mismatches that look like tampering. The
  `TestDownloadAndVerifyToleratesChecksumWhitespace` case (`"  "+digest+"\n"`)
  locks this in. Malformed/short/non-hex digest → clear error (fail closed).
- Unblocks dt-5o0x (install idempotency: skip/replace), which composes
  [[dt-hts2]]'s `ResolveDownload` + this. Parent epic dt-koaf (install) is still
  blocked on the output/exit epics ([[dt-cq78]]/[[dt-zwgc]]).

## Download URL resolution — pure function, drop the ticket's ctx (dt-hts2, 2026-06-06)

- **The ticket's `ResolveDownload(ctx, dl, p, version)` ctx param was dropped —
  the function does zero I/O.** It operates entirely on the `*api.Downloads` that
  `api.ListDownloads` already fetched, so a context argument is misleading dead
  weight (and `context-as-argument` aside, a pure function taking ctx is a smell).
  Same "trust the real contract over a toy signature" call as [[dt-4h21]]/[[dt-brug]].
  The caller dt-4fgk resolves first (pure), then does the download/verify I/O with
  its own ctx. revive's default set has no unused-parameter rule, so keeping it
  wouldn't have been _flagged_ — dropping it is a design call, not a lint fix.
- **"latest" resolves to the CONCRETE version string, not the map's literal
  `"latest"` alias.** The §6.1 downloads object has both `dnclient["latest"][key]`
  and `dnclient["0.9.5"][key]` (mirrors), plus `versionInfo.latest.dnclient ==
"0.9.5"`. When version is unset/`latest`, resolve via `VersionInfo.Latest.DNClient`
  to the concrete `"0.9.5"`, then look the URL up under _that_ key — so
  `Resolved.Version` is always concrete. dt-4fgk's "skip download if installed
  version matches" (design Req 1) can't compare against the string `"latest"`; it
  needs the concrete version. Empty `versionInfo.latest.dnclient` → clear error
  (the no-latest-reported guard test).
- **The `linux-amd64` os-arch key is built here, finishing what [[dt-ewgz]]
  deferred.** dt-ewgz's note explicitly held the `DownloadKey()`/URL-path helper
  out of `Platform` ("that's dt-hts2's job"). Implemented as an unexported method
  `func (p Platform) downloadKey() string { return p.OS + "-" + p.Arch }` in the
  download file, keeping `Platform` a pure data holder. Errors name the offending
  version (behavior 3) and the platform key (behavior 4) so an operator sees what
  to fix. ChecksumURL is the sibling `URL + ".sha256"` (§6.3), derived — never a
  JSON field. `internal/dnclient` now imports `internal/api` (no cycle: api→config
  only). Unblocks dt-4fgk (download + SHA-256 verify, fail-closed).

## Secret redaction — structural type, asymmetric marshal/unmarshal (dt-toaj, 2026-06-06)

- **One redaction has to cover four leak paths, not just slog.** `config.Secret`
  (a `type Secret string` in `internal/config/secret.go`) implements
  `slog.LogValuer` _and_ `fmt.Stringer` _and_ `fmt.GoStringer` _and_
  `json.Marshaler` — all returning `"REDACTED"`. slog alone is insufficient: when
  you log a _struct_ that embeds the secret (`slog.Any("config", cfg)`), the JSON
  handler doesn't recurse into `LogValue` for nested fields — it `json.Marshal`s
  the struct, so the field leaks unless `MarshalJSON` redacts. And a bare
  `type X string` leaks under `%v/%s/%q/%+v` (needs `String`) and under `%#v`
  (needs `GoString` — the one verb `String` doesn't cover for a defined string
  type). The behavior-1 test sweeps all five verbs + json.Marshal so a future
  "convenience" path can't silently reopen a hole. "Treat any appearance of the
  raw secret as a hard failure" (ticket) means redact every common serializer,
  not just the logger.
- **The killer subtlety: redact on the way OUT, keep the real value on the way
  IN — by implementing `MarshalJSON` but NOT `UnmarshalJSON`.** `api.EnrollmentCode.Code`
  is now `config.Secret`, and the code arrives _as JSON_ from the create
  response. Because `Secret` has no custom `UnmarshalJSON`, encoding/json falls
  back to default string assignment, so `{"code":"SECRET"}` unmarshals to the
  real `Secret("SECRET")` — the already-green `TestCreateHostAndEnrollmentCode`
  (asserts `Code != "SECRET"` is false) is the regression guard that receive
  still works. Only marshal/log is redacted. If you'd added `UnmarshalJSON`
  returning REDACTED you'd have silently broken enrollment. Asymmetry is the
  whole point.
- **`Config.APIKey` and `api.EnrollmentCode.Code` both become `Secret`; `Reveal()`
  is the single greppable raw accessor, used at exactly one prod site today** —
  `api/client.go`'s bearer header (`"Bearer "+c.apiKey.Reveal()`), proven by the
  pre-existing `TestDoSetsBearerAuth` asserting the literal `Bearer secret-token`
  still transmits (redaction must not break real auth). The Client's internal
  `apiKey` field is also `config.Secret` (not `string`) so even a stray `%+v` of
  the Client can't leak. Untyped string constants convert into `Secret` fields
  for free, so existing test literals (`APIKey: "secret-token"`,
  `c.APIKey = ""`, `cfg.APIKey == ""`) compile unchanged; only the one table test
  reading the value back needed `cfg.APIKey.Reveal()`.
- **`Secret` lives in `internal/config`, not `internal/output`.** It's stdlib-only
  (`log/slog`); `output` imports `urfave/cli/v3`, and `api` already imports
  `config` — so putting it in `config` adds zero new dependency edges and lets
  `api.EnrollmentCode.Code` reference it without `api`→`output`. Same
  define-the-safe-primitive-before-the-consumer pattern as [[dt-4b0e]]'s
  `retryPolicy`: the create cell ([[dt-pe29]]) and dnclient enroller (dt-a772)
  will call `Code.Reveal()` at the `dnclient enroll` hand-off; the write-config
  serializer (dt-cmdg/[[dt-druh]]) will call `APIKey.Reveal()` for the env-file.
  Held scope — did not wire those (they don't exist yet). Closes upstream SEC5;
  parent epic [[dt-cq78]] now has only [[dt-0jfp]] (--log-text) open.

## Enroll state machine row 1 — the no-op cell + the scaffolding siblings extend (dt-brug, 2026-06-06)

- **This ticket owns the `Enroll`/`Deps`/`API` foundation; the other three §2.4
  cells are separate sibling tickets, so the local-absent path is a deliberate
  stub.** dt-i0yx's children split the state machine four ways: dt-brug (row 1
  no-op), dt-pe29 (create cell), dt-xcac (orphan/force cell), dt-a772 (the
  dnclient subprocess `Enroller` interface). So `Enroll` returns
  `errLocalAbsentUnimplemented` for `ConfigExists==false` — a documented
  placeholder dt-pe29/dt-xcac replace, not an oversight. Resisted defining the
  subprocess seam in `Deps` (that's dt-a772) — same scope-by-omission discipline
  as [[dt-j2ab]]/[[dt-toqi]]. `Deps{API, ConfigRoot}` only; the dnclient client
  field is added when a cell that actually runs `dnclient enroll` lands.
- **The `enroll.API` interface is the natural subset of `*api.Client`
  (ListHosts/CreateHostAndEnrollmentCode/DeleteHost), grounded — not invented.**
  Defining all three even though row 1 calls none is fine: interface methods are
  _not_ flagged by `unused` (unlike funcs/vars, the [[dt-z99h]] pattern), and the
  method names must match `*api.Client` verbatim to satisfy it, so siblings can
  only _use_ them, never need to rename. The no-op test injects a `failingAPI`
  whose every method `t.Fatal`s — proving "zero API calls" structurally rather
  than by call-count.
- **The ticket's literal `ConfigExists(networkName string) bool` signature lost
  to the prose's "make the config root injectable."** Mirrored
  `ReadHostID(configRoot, networkName)` → `ConfigExists(configRoot, networkName)
bool` for consistency and testability (temp-dir root). Same "trust the real
  contract over a ticket's toy signature" call as [[dt-4h21]]. Extracted a shared
  unexported `configPath(root, network)` and pointed both `ConfigExists` and
  `ReadHostID` at it — kills the duplicated `filepath.Join(...,"dnclient.yml")`.
  `ConfigExists` is a _presence_ probe only (`os.Stat`, err==nil): a non-not-exist
  stat error reads as not-present so enroll proceeds to the create path rather
  than masking the condition (the full host_id parse stays in `ReadHostID`).
- **Behavior 2 (exit semantics) needed no command wiring — it's composition with
  the already-shipped `withResult`.** `Enroll` only has to return `Changed=false`;
  [[dt-4h21]]'s wrapper turns that into exit 0 (or 2 under `--assert-changed`).
  Asserted at the unit level via `output.ResolveExitCode` rather than booting the
  CLI or pre-empting the enroll-command wiring (still `notImplemented`; the parent
  epic owns it). `errNoChange` lives in `package main`, so the in-`enroll` test
  uses a throwaway `errors.New` to exercise the `ExitError(non-nil, 2)` path.
- **Reuse the existing same-package test helper.** `internal/dnclient`'s
  `hostid_test.go` already had `writeConfig(t, root, network, content)`; my
  first draft redeclared a 3-arg `writeConfig` and hit "redeclared in this block."
  Go test files in a package share scope — grep the package's `_test.go` for a
  helper before adding one.

## --assert-changed wrapper — the ticket pseudocode was wrong, the typed-nil guard wins (dt-4h21, 2026-06-06)

- **The ticket's literal `return ExitError(nil, CodeNoOpAssert)` does NOT
  produce exit 2 — it produces exit 0.** [[dt-icq8]] deliberately made
  `output.ExitError(nil, …)` return a _true nil interface_ (the typed-nil trap
  guard, asserted by `TestExitError_NilErrorReturnsNilInterface`), so a nil err
  collapses to nil → `ResolveExitCode(nil)` → `CodeOK`. The fix is a non-nil
  package sentinel `errNoChange = errors.New("no change made")`; the no-op path
  returns `ExitError(errNoChange, CodeNoOpAssert)`. Trust the building block's
  contract over a ticket's pseudocode when they conflict.
- **The wrapper lives in `cmd/dn-tool`, not `internal/output`.** It needs the
  `*cli.Command` to read the global `--assert-changed` flag and the stdout
  writer, so it can't sit in `output`. Shape: `withResult(action resultAction)
cli.ActionFunc` where `resultAction = func(ctx, *cli.Command) (output.Result,
error)`. It writes the Result to `cmd.Root().Writer` _then_ applies the no-op
  check — so the machine-readable result is emitted even on the exit-2 path (a
  pipeline still gets the outcome). The **error path is checked before** the
  `!Changed` branch, so code 2 can never collide with a failure (behavior 4).
- **`cmd.Bool("assert-changed")` reads the root flag from the subcommand for
  free** — urfave/cli v3 inherits root `Flags` into subcommand lookups
  ([[dt-ecf9]]), no `Persistent:true` needed.
- **Tests MUST capture `cli.OsExiter`, not the returned error.** urfave/cli v3
  _auto-exits inside `Run`_ when an action returns a `cli.ExitCoder` ([[dt-icq8]]):
  it calls `cli.HandleExitCoder` → `cli.OsExiter` (real `os.Exit`) before `Run`
  returns, so a stub returning `ExitError` kills the test binary unless OsExiter
  is overridden. Reused the existing `captureExit(t)` helper (sentinel `-1` =
  never called = success 0) and mirrored `main()`'s `if err != nil {
exitWithError(err) }` so the helper reports the true _production_ exit code for
  all four cells (ExitCoder via Run's internal call; plain-err/nil via the
  mimic). Driving stub commands through a real `*cli.Command` app is the
  integration the ticket asked for.
- **`withResult`/`errNoChange` are unexported with only `_test.go` callers** —
  not flagged by `unused` (the [[dt-z99h]]/[[dt-j2ab]] pattern). Held scope: did
  NOT wire `withResult` into the still-`notImplemented` commands; that's each
  command ticket's job ([[dt-brug]] enroll-no-op cell, dt-i0yx, etc.). This was
  the **last open child of epic [[dt-zwgc]]** (exit-status semantics) — closing
  it lets that epic close and unblocks the P0 enroll/unenroll commands.

## API resilience epic — sometimes "all children closed" really does mean done (dt-8h9t, 2026-06-06)

- **The dt-uzx6/dt-vsi6 "re-read acceptance against reality" discipline cuts
  both ways: it can also confirm an epic is _complete_, not just expose a seam.**
  This P0 epic appeared in `tk ready` with all 5 children closed (egz4 retry,
  z99h core+D4 status-before-body, 255n endpoints, ihil pagination, 4b0e error
  typing). Applying the same re-read, every acceptance bullet was met _and_
  tested: the httptest matrix the epic names verbatim
  (success / 4xx-no-retry / 5xx+429-retry / timeout / malformed-body) all pass,
  the public `New(cfg)` entry point is exercised by ~24 tests, `code`/`path` is
  surfaced via `APIError.Has`, pagination is exposed via `ListHosts`, and full
  paths (no version-prefix assumption) handle the v1-delete / v2-create split.
  **Closed with zero code change** — fabricating work would have been worse than
  doing none. `tk` does not auto-close an epic when its children close, so the
  verify-and-close _is_ the epic's terminal action; it unblocked the 3 command
  tickets (dt-3gvq, dt-i0yx, dt-koaf).
- **The one acceptance phrase that looks open — "all calls bounded by
  DN_API_TIMEOUT" — is a per-command seam the epic does NOT own.** The client
  honors any context deadline (proven by `TestOverallDeadlineBoundsRetries`,
  which bounds the _retry chain_, not just one request); binding
  `config.APITimeout` → call ctx is downstream consumption. The evidence it's
  command-owned is concrete and pre-existing: `config.go` deliberately leaves
  `APITimeout` 0 ("command picks default"), §2.3 gives _different_ per-command
  defaults (~30s enroll / ~10s unenroll), and the already-closed
  `unenroll.go` comments "already bounded by DN*API_TIMEOUT at the command
  layer." So there is no command-agnostic bridge to build (the differing
  defaults preclude one) — unlike [[dt-uzx6]]'s `LoadWithEnvFile`, whose
  flag-path→reader seam had no per-command variation and was genuinely unowned.
  Building a timeout bridge in `api` would pre-empt the command tickets — the
  exact [[dt-toqi]]/[[dt-j2ab]] scope-by-omission anti-pattern. \*\*Test for "is
  this a real seam": does any \_closed* code already assume someone else fills
  it, and does per-command variation block a single shared helper? If yes to
  both, it's downstream, and the epic is done.\*\*

## Config epic — the residual gap was the integration entry point (dt-uzx6, 2026-06-06)

- **Same dt-vsi6 lesson, second instance: an epic with all children closed
  still owns its integration scaffolding.** The 4 children delivered the
  _pieces_ — `Load(getenv)` (env-only, [[dt-mhir]]), `ParseEnvFile(io.Reader)`
  (reader→map, [[dt-kiqk]]), `Resolve(fileVars, getenv)` (precedence,
  [[dt-9x3y]]) — but **nothing bridged the `--env-file` flag _path_ to disk**.
  `ParseEnvFile` takes a reader; no one called `os.Open`. Re-reading the epic's
  acceptance ("all DN\_* load from env *and\* --env-file") against reality exposed
  the seam. The fix is one function, `config.LoadWithEnvFile(envFilePath,
getenv)` in `resolve.go`: empty path → `Resolve(nil, getenv)`; set path →
  `os.Open` + `ParseEnvFile` + `Resolve`, wrapping open/parse failures with
  `%w` (missing file wraps `os.ErrNotExist`, asserted via `errors.Is`).
- **This is the single entry point the 5 blocked commands call** (enroll,
  unenroll, install, REST client, write-config) — they were blocked on the epic
  precisely because they need _one_ call to turn the flag + live env into a
  validated `*Config`. Held scope hard (the [[dt-j2ab]]/[[dt-toqi]] pattern):
  did **not** wire it into the still-`notImplemented` commands or re-touch the
  README — per-command consumption is downstream-ticket work, and the
  `§Precedence` README section already shipped with dt-9x3y. An _exported_
  function with only `_test.go` callers is **not** flagged by `unused` (unlike
  unexported funcs), so the gate stays green with no premature caller.
- **TDD tracer bullet caught the only real RED.** Test 1 (empty path) failed to
  compile (`undefined: LoadWithEnvFile`) = genuine red; the precedence + two
  error-path tests went green on first run because they exercise branches I'd
  just written by _reusing_ the well-tested `Resolve`/`ParseEnvFile`. The value
  there is locking the end-to-end file-open→parse→resolve contract, not
  discovering new logic. Empty `getenv` paths set `DN_HOSTNAME` so the
  hostname fallback never touches the real host (no need for the unexported
  `resolve`'s injected-hostname seam here).
- **`make build` must run from the repo root, not `src/`.** The Makefile lives
  at `/workspace/Makefile` and `cd`s into `src/` itself; running `make build`
  from `src/` gives "No rule to make target 'build'". The persistent-cwd Bash
  tool will strand you in `src/` after a `cd` — prefix with `cd /workspace &&`.

## CI workflow — last acceptance item of the P0 epic (dt-vsi6, 2026-06-06)

- **The P0 scaffolding epic was a tracking container with all four children
  closed but one un-tracked acceptance gap: CI.** dt-vsi6's "Current state &
  gaps" struck through flake/version/Makefile gaps but never listed CI, while
  its acceptance criteria and design §2.12 step 1 both name CI. Lesson: when an
  epic shows in `tk ready` with all children closed, re-read its _acceptance
  criteria_ against reality — the residual scaffolding (here `.github/workflows/ci.yml`)
  is the epic's own work, not a missing child ticket.
- **CI just runs `make build` — don't reimplement the gate in YAML.** The
  Makefile already encodes fmt-check/vet/lint/test/compile (see
  [[dt-prt6]] for the lint stage), so the workflow is one `run: make build`. No
  per-step duplication; the gate stays single-sourced in the Makefile.
- **Go version comes from `src/go.mod` via `actions/setup-go@v5`
  `go-version-file:`**, not a hardcoded string — so CI tracks the `go 1.26.0`
  directive ([[dt-0mdh]] relaxed it to the minor series for nixpkgs parity) and
  can't drift. `cache-dependency-path: src/go.sum` because the module lives under
  `src/`, not repo root.
- **golangci-lint must be on PATH for `make lint`, pinned to v2.12.2.** The
  Makefile's `lint` target shells out to the `golangci-lint` binary, so CI
  installs it from the _version-tagged_ install script
  (`.../golangci-lint/v2.12.2/install.sh … v2.12.2`) — fully pinned, not `HEAD` —
  onto `$(go env GOPATH)/bin`, then appends that dir to `$GITHUB_PATH` (setup-go
  does not add GOPATH/bin to PATH). Version matches the local toolchain and the
  v2 config schema at `src/.golangci.yml`.
- **No pyyaml in this sandbox; `pip` is PEP-668 externally-managed.** Validate
  workflow YAML with `python3 -m pip install pyyaml --break-system-packages`
  (sandbox-only, acceptable) then `yaml.safe_load`. Note the YAML `on:` key
  round-trips through PyYAML as boolean `True`, so read it as `d.get('on',
d.get(True))` when asserting triggers.
- **Flake build deliberately omitted from CI.** Acceptance is "CI runs make
  build" only; a `nix build` job can't be validated here anyway (the FOD
  `unpackPhase` fails under nix-portable's proot per [[dt-0mdh]]). Left for a
  future ticket if real GitHub runners are wired up.

## Flake packaging: buildGoModule (dt-0mdh, 2026-06-06)

- **`go.mod`'s `go 1.26.4` blocked the nix build — relaxed to `go 1.26.0`.**
  nixpkgs-unstable (rev 891eaa7) ships only **go 1.26.3** (both `go` and
  `go_1_26`). With a `go 1.26.4` directive, the nix sandbox build fails either
  way: `GOTOOLCHAIN=auto` tries to download the 1.26.4 toolchain (network is
  off in the sandbox) and `local` errors "requires go >= 1.26.4". The `.4` was
  auto-stamped by the local toolchain at `init` (git-confirmed: present in the
  very first commit), not a real dependency — nothing uses a 1.26.4-only
  feature. Relaxing the directive to the **minor series** keeps both the local
  toolchain (1.26.4) and nixpkgs (1.26.3) satisfied and is the standard way to
  keep CI/nix in sync with whatever patch the system provides. Don't override
  `go` to 1.26.4 from source — that recompiles Go for no benefit.
- **Flakes only see git-tracked files.** `nix build` couldn't find `flake.nix`
  until `git add flake.nix`. Untracked files are not copied into the store, so
  every new flake artifact (`flake.nix`, then the generated `flake.lock`) must
  be `git add`ed before it takes effect — even without committing. Commit
  `flake.lock` for reproducibility (it pins the nixpkgs rev).
- **vendorHash discovery: `nixpkgs.lib.fakeHash` → read the mismatch.** Set
  `vendorHash = nixpkgs.lib.fakeHash`, build, copy the `got: sha256-…` into the
  flake. go.sum was already committed by [[dt-0nl5]]/[[dt-ecf9]] precisely so
  this would be reproducible. Result: `sha256-UfXdjvYdNXPFTm7CPqkIg6NrF9NWt+ffwRgIuyaegHQ=`.
- **Version stamping in a pure build uses `self`, not `git describe`.** git
  isn't available in the build, so mirror the Makefile's `--dirty --always` with
  `version = self.shortRev or self.dirtyShortRev or "dev"` and pass the same
  `-X …/internal/version.Override=${version}` ldflag. Verified `--version` ⇒
  `c92d015-dirty` (not the build-info "dev" fallback from [[dt-ecf9]]).
- **Drop `-buildid=` from the flake ldflags.** buildGoModule sets it by default
  and warns if you repeat it. Keep `-s -w -X …` only. buildGoModule's
  `checkPhase` also runs the full `go test ./...` for free — a bonus gate.
  Don't "fix" this back to mirror the Makefile — the Makefile needs `-buildid=`
  because plain `go build` doesn't add it; buildGoModule does.
- **Verified independently when closing (`nix-portable` only).** Reproduced the
  exact `vendorHash` from go.mod/go.sum without a build sandbox: `cp src → tmp;
GOFLAGS=-mod=mod go mod vendor; nix hash path ./vendor` ⇒
  `sha256-UfXdjvYd…` (the FOD output is literally `go mod vendor -o $out`, and
  the module sources are content-addressed by go.sum, so the NAR hash is
  toolchain-deterministic — recomputed twice, identical). `nix eval` of
  `pname`/`version`/`drvPath` all resolve. **Caveat for this dev sandbox:**
  `nix build` of the FOD fails in `unpackPhase` (`cp: … Permission denied`
  copying the read-only `…-src` store dir) under nix-portable's proot runtime —
  `bwrap` is unavailable (no user namespaces) and `PROOT_NO_SECCOMP=1` doesn't
  help. This is an environment limitation, not a flake defect; use `nix hash
path` to confirm the hash here rather than a full build.

## OS/arch detection — published ≠ supported (dt-ewgz, 2026-06-06)

- **The behavior-4 `linux/mips` case fails _on purpose_, even though the
  downloads API publishes `linux-mips`.** API ref §6.2 lists `linux-mips`,
  `linux-386`, `linux-armv5/6/7`, `ppc64le`, `riscv64`, etc. as real keys — but
  the ticket's acceptance is "supported linux/{amd64,arm64} map; non-Linux and
  unknown arch fail," and behavior 4 explicitly wants `mips` to error. So
  `supportedArch` is a two-entry allow-list (amd64, arm64 = the design §2.1
  production targets x86_64/aarch64-linux), not a transcription of §6.2. The
  principle: dn-tool refuses to install a binary for an arch nobody deploys
  rather than silently pulling an unvetted one. If a future ticket needs 386/arm,
  it widens the map — don't pre-widen it now.
- **`Platform.Arch` == Go arch for both supported targets, so no translation
  table is needed yet.** `amd64→amd64`, `arm64→arm64`. The §6.2 armv5/6/7
  divergence (key `armv7` vs URL path `arm-7`, and GOARM not being in the
  `(goos,goarch)` signature) only bites if those arches are ever added — that
  belongs to [[dt-hts2]] (download URL resolution), the ticket this unblocks.
- **Scope held by _omission_ again** (same pattern as [[dt-j2ab]]/[[dt-toqi]]):
  resisted adding a `DownloadKey()`/URL-path helper to `Platform`. The
  `linux-amd64` key and `linux/amd64` path construction is dt-hts2's job;
  building it here would pre-empt that ticket's tests. The struct stays a pure
  data holder. `goos`/`goarch` are injected (not read from `runtime.*`) so the
  mapper is a pure function with zero env access — closes upstream S4 (bash used
  `arch(1)`, not `uname -m`).

## slog setup: JSON to stderr + level filtering (dt-zxwf, 2026-06-06)

- **`NewLogger` has no error return, so "invalid level" can only mean a
  documented fallback, not a failure.** The fixed signature `NewLogger(io.Writer,
LogOptions) *slog.Logger` rules out surfacing a bad `DN_LOG_LEVEL`. `parseLevel`
  trims + lowercases and `switch`es debug/warn/error, with the `default` case
  catching both empty and unknown → `slog.LevelInfo`. The behavior-3 table pins
  `""` and `"bogus"` to the same info threshold as `"info"`. (This is the same
  signature-forced-decision shape as [[dt-9x3y]]'s "empty live wins is
  inexpressible".)
- **slog renders levels UPPERCASE in JSON** — the record's `level` field is
  `"INFO"`/`"WARN"`, not lowercase. Tests assert `rec["level"] == "INFO"`. Don't
  expect the `DN_LOG_LEVEL` input casing to survive into the output.
- **Scope boundary with [[dt-0jfp]] held by _declaring but not branching_.**
  `LogOptions.Text` exists (the interface contract dt-0jfp extends) but `NewLogger`
  ignores it — output is always JSON here. So `NewLogger(&buf, {Text:true})` still
  emits JSON, leaving dt-0jfp a genuine red test for its TextHandler branch +
  `--log-text` wiring. An unused _struct field_ is not flagged by `unused` (unlike
  an unused func/var), so the gate stays green. Same pre-emption-avoidance pattern
  as [[dt-j2ab]]/[[dt-toqi]].
- **Behavior 4 ("stderr in production") is a one-line `main()` wiring, closing the
  [[dt-egz4]] handoff.** `slog.SetDefault(NewLogger(os.Stderr, {Level:
os.Getenv("DN_LOG_LEVEL")}))` makes the api client's `slogLeveledLogger` (over
  `slog.Default()`) emit JSON-to-stderr honoring the level — retry chatter now
  obeys `DN_LOG_LEVEL` for free, as dt-egz4 predicted. Uses live `os.Getenv` at
  process start; env-file precedence for the level is later command-wiring's
  concern (the env-file isn't parsed until cli reads `--env-file`). stdout stays
  reserved for `WriteResult` — nothing in `output` couples the two writers.
- **Can't smoke-test internal-package wiring from `/tmp`.** A throwaway
  `go run /tmp/x.go` importing `internal/output` fails with "use of internal
  package not allowed" — Go's internal rule is by import path, not file location.
  The unit tests (buffer + NDJSON decode) are the verification; the `os.Stderr`
  arg is covered by compilation + review.

## Unenroll failure invariant — honest message (dt-f7nx, 2026-06-06)

- **The invariant was already enforced; this ticket was the _messaging_ + the
  missing failure tests.** [[dt-2t72]] gated `os.RemoveAll` strictly behind a
  successful/404 delete, so "retain local on failure" was structurally done and
  green. dt-f7nx's real delta is a single package const, `unenrollFailureAdvisory`,
  appended to the delete-failure error. Don't go looking for a behavior change in
  the removal logic — there isn't one.
- **`fmt.Errorf("...: %w; %s", hostID, err, advisory)` keeps the chain.** The
  honest suffix is a plain `%s` _after_ the `%w`, so `errors.Is(err,
context.DeadlineExceeded)` still crosses it. Putting the advisory before `%w`
  or using a second `%w` (Go 1.20 multi-wrap) both work, but single-`%w` +
  trailing `%s` is the least surprising and what the deadline test pins.
- **"exit ≠ 0" needs no `output.ExitError`.** `Unenroll` returns a _plain_ error;
  `main.exitWithError` maps any non-`ExitCoder` to `CodeError` (1) — see
  [[dt-icq8]]. The test asserts the consequence directly in-package via
  `output.ResolveExitCode(err)` (≠ `CodeOK` and ≠ `CodeNoOpAssert`) rather than
  booting the CLI. Reserve `ExitError` for the _distinct_ code-2 assert-changed
  no-op ([[dt-4h21]]); failures stay plain.
- **Deadline path is testable without sleeping.** The `fakeDeleter` now returns
  `ctx.Err()` when the context is done (a real ctx-bounded api.Client does the
  same), so a `context.WithDeadline(…, time.Unix(0,0))` (already past) drives the
  deadline branch deterministically — no timers, no flakiness. This is the seam
  for the §2.5 "delete fails within the deadline" case and the module-D5
  `TimeoutStopSec` counterpart.

## Exit-code mapping + ExitCoder wiring (dt-icq8, 2026-06-06)

- **urfave/cli v3 auto-exits _inside_ `Command.Run` when an Action returns a
  `cli.ExitCoder`.** The default error handler calls `HandleExitCoder` →
  `cli.OsExiter` (= `os.Exit`) before `Run` returns. So a test whose stub action
  returns `output.ExitError(...)` will kill the test binary unless it first
  overrides `cli.OsExiter` (and restore via `t.Cleanup`). Plain (non-ExitCoder)
  errors are _not_ auto-exited — `HandleExitCoder` is a no-op for them — so they
  fall through and `main` maps them. Behavior-4 ("deferred cleanup runs on error")
  is therefore cleanest tested with a plain error: `Run` returns it, the defer
  already ran, no `OsExiter` dance.
- **Two surfaces, one mapping.** `output.ResolveExitCode(err) int` is the pure
  source of truth (nil→0, `cli.ExitCoder`→its code via `errors.As`, else 1) and is
  table-tested directly. `main.exitWithError` does the same resolution but routes
  through `cli.HandleExitCoder` for its _side effects_ (prints the message to
  `cli.ErrWriter`, calls `cli.OsExiter`). They agree by construction; the pure one
  is what new code should call when it just needs the number.
- **`ExitError` preserves the chain (`Unwrap`), `cli.Exit` does not.** Defined a
  private `exitError{err,code}` with `Error/ExitCode/Unwrap` so `errors.Is/As`
  cross it — `cli.Exit(msg,code)` flattens to a string message and loses the
  wrapped error. Guard `ExitError(nil,…)→nil` to avoid the typed-nil-interface
  trap. Compile-time guard is a package-level `var _ cli.ExitCoder =
(*exitError)(nil)`, not a `var _ = ` inside a test func (staticcheck QF1011 +
  revive unused-`t` both fire on the in-test form).
- **Lives in `internal/output`, not `internal/exit`.** Matches the ticket's
  primary suggestion and dodges revive stutter: `output.ExitError` is clean,
  `exit.ExitError` would stutter (and renaming to `exit.Error` diverges from the
  ticket name). Cost: `output` now imports `urfave/cli/v3` — acceptable, output is
  the process's external-contract package (stdout result + stderr logs + exit
  code). Handoff: [[dt-4h21]] emits the assert-changed no-op via
  `output.ExitError(err, output.CodeNoOpAssert)`.
- **Unknown-command exit is 3, by framework design** (`help.go:319`
  `return Exit(errMsg, 3)`) — an `ExitCoder` my wiring honors verbatim. Req 8's
  0/1 mapping is about _action_ success/failure; usage errors keep urfave's 3.

## JSON result writer (dt-ccmn, 2026-06-06)

- **Closes the [[dt-2t72]] handoff: `Result` was left verbatim, not redefined.**
  dt-2t72 pre-created `internal/output` with _only_ the `Result` struct to bridge
  a dep-graph gap and warned "don't redefine `Result`". dt-ccmn honored that — it
  adds _only_ `WriteResult`; the struct already matched the ticket's documented
  interface exactly, so no edit to it. The graph gap is now fully resolved.
- **`json.NewEncoder(w).Encode(r)` gives "one object + one newline" for free.**
  The behavior-2 requirement (single JSON object, a trailing newline, nothing
  else) is exactly `Encoder.Encode`'s contract — it writes one value followed by a
  single `\n`. No manual `Marshal` + `w.Write(b)` + `w.Write([]byte("\n"))`. The
  test pins it with `json.Decoder.More() == false` after one decode, so a future
  switch to something that emits trailing data would fail.
- **stdout/stderr separation is structural, not enforced at runtime.** `WriteResult`
  takes an injected `io.Writer` (stdout in prod, a `bytes.Buffer` in tests); the
  slog logger (dt-zxwf) takes its own writer (stderr). Nothing in `output` couples
  the two, so the "result on stdout, logs on stderr" contract holds by wiring, and
  tests capture each independently. `Changed` (always present, no omitempty) is the
  exit-2 signal dt-4h21 reads.

## Unenroll delete + local removal (dt-2t72, 2026-06-06)

- **New `internal/unenroll` package, not folded into `enroll`.** The ticket
  allowed either; chose a separate package since unenroll is the inverse command
  and `run` (dt-n5p5) will compose enroll+unenroll — keeps `enroll` from becoming
  a god-package. Design §2.9's structure is "tentative" and didn't forbid it.
- **Created `internal/output` with _only_ the `Result` struct.** The dt-2t72
  interface returns `output.Result`, but the output package didn't exist and
  dt-ccmn (which owns it) is **not** a dependency of this ticket — a graph gap.
  Resolved by defining just the struct (matching dt-ccmn's documented
  action/changed/hostId/network schema verbatim); dt-ccmn now only adds
  `WriteResult` + writer tests. The struct is the shared contract, defined once,
  so both tickets coexist with no duplicate-type conflict. Watch for this when
  picking up dt-ccmn: don't redefine `Result`.
- **Local removal is gated strictly behind a successful delete — that _is_ the
  §2.5 invariant.** `os.RemoveAll(<root>/<network>)` runs only after
  `api.DeleteHost` returns nil, so a delete failure retains local config (no
  orphan). This previews dt-f7nx (the failure _messaging_/invariant ticket) but
  the gating itself is just correct ordering; covered by
  `TestUnenrollDeleteFailureRetainsLocal`.
- **Behaviors 1 (2xx) and 2 (404) collapse at this layer.** `api.DeleteHost`
  already maps 404→nil (dt-255n), so from `unenroll`'s view both are "delete
  returned nil → remove local". The fake `HostDeleter` returns nil for both; the
  distinct 404 handling is tested in `internal/api`, not re-tested here.
- **`Deps{API HostDeleter, ConfigRoot string}` — `ReadHostID` is called
  directly, not mocked.** The not-enrolled path is exercised with a `t.TempDir()`
  that lacks `dnclient.yml`, so the real `dnclient.ReadHostID` returns
  `ErrNotEnrolled` (matched via `errors.Is`) — no need to inject a host-id reader.
  Only the API delete is an interface seam.
- **gofmt struct-tag alignment bit me.** `make build`'s `fmt-check` failed on
  `result.go` because hand-written struct fields with a trailing line comment
  weren't gofmt-aligned. Run `gofmt -w` (or `make fmt`) before the gate on any
  hand-authored struct with tags + comments.

## Config precedence: live env over env-file (dt-9x3y, 2026-06-06)

- **`Resolve` is a thin per-key source chooser over the existing `load`.** Reused
  the [[dt-mhir]] `Load`/`load(getenv, hostname)` split verbatim: `Resolve(fileVars,
getenv)` wraps `resolve(fileVars, getenv, os.Hostname)`, which builds a merged
  lookup closure and hands it to `load`. All defaulting/typing stays in `load` —
  the merge layer only picks the source. No duplication of the §2.3 defaults.
- **The fixed `getenv func(string) string` signature makes "empty live wins"
  inexpressible — so empty live falls through to the env-file.** The ticket's
  parenthetical wanted an explicitly-empty `DN_X=` to win, but `func(string)
string` cannot tell set-but-empty from unset (both yield `""`), and neither can
  `os.Getenv` in production (only `os.LookupEnv` distinguishes them, and the
  signature is fixed). Decided + documented: `merged(key) = getenv(key) if
non-empty else fileVars[key]`. An empty live value is treated as unset →
  env-file → default. Rationale lives in the `resolve.go` godoc and a new README
  "Configuration precedence" section (acceptance required godoc + README). This is
  the same signature-forced-decision pattern as [[dt-mhir]]'s injectable hostname.
- **Test seam:** table-driven over `(fileVars, envVars) → field`, reusing the
  existing `mapEnv`/`emptyEnv`/`fixedHostname` helpers from `config_test.go`. The
  empty-live cell asserts the file value is used (since `mapEnv` returns `""` for a
  key mapped to `""`, it naturally exercises the fall-through).

## Read host_id from dnclient.yml (dt-0nl5, 2026-06-06)

- **New `internal/dnclient` package; `gopkg.in/yaml.v3` is now a _direct_ dep
  (v3.0.1).** First YAML use in the repo. `go mod tidy` also pulled
  `fatih/color`/`mattn/go-isatty`/`golang.org/x/sys` as indirects (transitive via
  the existing retryablehttp/cleanhttp chain, surfaced now) — expected, not new
  surface of ours. go.sum is committed (the flake dt-0mdh needs it for
  `vendorHash`).
- **Decode only `host_id`, not the whole dnclient schema.** A one-field anonymous
  struct (`struct{ HostID string \`yaml:"host_id"\` }`) — the file's schema is
owned by `dnclient`, not us (API ref §5 note), so modeling more would couple us
to a foreign format. yaml.v3 ignores unknown keys by default, so real configs
with `pki:`/etc. parse fine.
- **`ErrNotEnrolled` is a package `var` sentinel, wrapped with `%w`.** Missing
  file → `fmt.Errorf("%s: %w", path, ErrNotEnrolled)` so dt-2t72's unenroll branch
  uses `errors.Is`, never string-matching (same contract as api's `*APIError` /
  [[dt-4b0e]]). Crucially the _other_ failures (missing field, malformed YAML)
  must NOT match it — each has its own test asserting `!errors.Is(err,
ErrNotEnrolled)`, so "not enrolled" and "enrolled-but-broken-config" stay
  distinct decisions for the caller.
- **`os.ErrNotExist` is the only not-enrolled signal.** `os.ReadFile` error is
  split: `errors.Is(err, os.ErrNotExist)` → `ErrNotEnrolled`; any other read error
  (perms, IO) → wrapped `reading <path>` error, _not_ not-enrolled — a permission
  failure on a present file shouldn't read as "host isn't enrolled".
- **Empty `host_id` is treated as missing.** `doc.HostID == ""` covers both an
  absent key and an explicit `host_id:` (null/empty) — yaml.v3 leaves the field
  zero in both, so one check handles both malformed-field cases.
- **`configRoot` injected (default `/etc/defined` lives in the caller, not here).**
  Tests pass `t.TempDir()`; the function never hardcodes `/etc`, so no real-FS
  dependency in unit tests.

## Enroll request building + required-param validation (dt-j2ab, 2026-06-06)

- **Behavior 4 ("tun device = network name") is satisfied by an _absence_, not a
  field.** The ticket lists it as a TDD behavior for `buildCreateRequest`, but
  [[dt-255n]] already established the v2 create-host body has no tun/network-name
  field (API ref §4.1) — the tun device is named at `dnclient enroll` time, not
  in the POST. Reconciled by _not_ adding the field and locking the invariant
  with `TestBuildCreateRequestOmitsTunField`: marshal the request, assert no
  `tun`/`tunName`/`deviceName`/`networkName` key and that `DN_NETWORK_NAME`'s
  value doesn't leak into the body. This prevents a future change from smuggling
  the network name into the request to "fix" the literal ticket wording.
- **Required-param validation names the _first_ missing one, in a fixed order.**
  API key → network ID → role ID, via a `switch` with empty-string cases, each
  returning `missingParam("DN_*")`. Errors carry the `DN_*` env-var name (what an
  operator actually sets), not the Go field name. Req 2 requires roleID even
  though the API marks it optional — so it's validated here, not delegated.
- **Scope split with dt-wleh held firm.** `CreateHostRequest` already has
  `IsLighthouse`/`IsRelay`/`StaticAddresses`/`ListenPort`, but this task leaves
  them zero-valued (regular-host path; `listenPort` 0 = auto-select). dt-wleh
  extends `buildCreateRequest` to populate them + add the lighthouse/relay
  validation gates. Setting them here would have pre-empted that ticket's tests.
- **`DN_IP_ADDRESS` → `[]string{ip}` only when set** (v2 `ipAddresses`, not the
  deprecated v1 scalar). Unset leaves the slice `nil` so `omitempty` drops it —
  same nil-is-omitted contract [[dt-toqi]] relies on for `tags`/`staticAddresses`.
- **Unexported `buildCreateRequest` isn't flagged unused** because same-package
  `_test.go` exercises it (the [[dt-z99h]] pattern). The enroll command wiring
  (dt-pe29/dt-brug) becomes its real caller.

## Typed config fields: arrays + port bounds (dt-toqi, 2026-06-06)

- **Empty/unset JSON arrays resolve to `nil`, not `[]string{}`** — and that's
  load-bearing downstream. dt-255n's `CreateHostRequest` puts `omitempty` on
  `tags`/`staticAddresses`, so a `nil` slice omits the field from the POST body
  entirely (a regular host sends a minimal body). Returning `[]string{}` would
  serialize as `[]` and send an empty array. `parseJSONArray("")` returns
  `(nil, nil)` deliberately; the empty-is-nil test locks it in.
- **Port `0` is a valid value, not "unset".** Bounds check is `0 <= p <= 65535`
  (0 = auto-select, matching dt-255n sending `listenPort` always, no omitempty).
  Negative and >65535 error with the var name; non-numeric falls through from
  `strconv.Atoi`. The lighthouse/relay "non-zero port required" rule is _not_
  here — that's a per-command validation (dt-j2ab), this task only rejects
  out-of-range bytes.
- **Bools/timeout were already done in [[dt-mhir]]** (`strconv.ParseBool` /
  `time.ParseDuration`); dt-toqi only added the two arrays + port bounds. The
  dt-mhir scope-boundary note ("arrays left unparsed, that's dt-toqi's job") is
  now closed.
- **Handoff to dt-druh (config→env-file serializer):** `Tags`/`StaticAddrs`
  must be re-emitted as JSON (`json.Marshal` → `["a","b"]`) so the next
  `ParseEnvFile`→`Load` round-trips. Combined with the dt-kiqk note (re-quote
  empty/whitespace values), the serializer needs both quote-on-write _and_
  JSON-encode-arrays to survive a parse.

## env-file parser, SEC3 (dt-kiqk, 2026-06-06)

- **SEC3 passes the moment you do nothing.** The behavior-4 test
  (`K=$(rm -rf /)`, backticks, `$VAR`, `;`/`&&`) went green immediately — the
  parser never expands, so the literal-byte assertion held on first run. That's
  the point: the security property is the _absence_ of an expansion code path,
  not a feature to add. The test exists to lock the absence in so a later
  "convenience" interpolation can't sneak in unnoticed. Keep it.
- **Value-trimming is a deliberate spec gap fill.** The ticket's format rules
  say "trim surrounding whitespace around KEY" and are silent on the value.
  Chose to `TrimSpace` the value too, _then_ strip one layer of matching
  quotes — because the quote rule (`K="a b"` → `a b`) only makes sense if
  unquoted whitespace is otherwise stripped; quotes are how you opt back into
  intentional leading/trailing spaces. Ordering matters: trim → unquote, so
  `K = "  x  "` → `  x  `. dt-druh's serializer must round-trip this: any value
  with leading/trailing space (or that's empty) has to be re-quoted on write,
  or the next parse will mangle it.
- **Unbalanced/mismatched quotes stay literal, not an error.** `K="oops` and
  `K="x'` keep the raw bytes rather than failing — the only malformed-line
  errors are "no `=`" and "empty key" (per the ticket). `unquote` guards on
  `len>=2` and first==last char, so a lone quote or empty string passes
  through.
- **`ParseEnvFile(io.Reader)` not `(path)`** — pure reader→map, so tests use
  `strings.NewReader` and the command layer owns file-open + the precedence
  overlay (dt-9x3y: live env wins over file). Last duplicate key in the file
  wins (plain map overwrite); not specified, but the only sane default.

## Retry transport wiring (dt-egz4, 2026-06-06)

- **`retryPolicy` dropped straight into `CheckRetry` with zero glue.** dt-4b0e's
  decision to define `retryPolicy` with the stdlib `(ctx, *http.Response, error)
→ (bool, error)` signature paid off exactly as planned: `rc.CheckRetry =
retryPolicy` compiles, no adapter, no extra import. `execute`/`do` are
  untouched — `New` just hands them `rc.StandardClient()` (a plain `*http.Client`
  whose RoundTripper retries underneath). The overall ceiling stays the per-call
  context (`DN_API_TIMEOUT`), never a transport setting.
- **Wiring retry made two committed tests slow — that's the signal, not a
  regression.** dt-z99h/dt-ihil wrote `TestDoChecksStatusBeforeBody` (500) and
  `TestListAllSurfacesErrorMidPagination` (5xx mid-page) against the no-retry
  `http.DefaultClient`; once retry is live they do full prod backoff (1+2+4+8s ≈
  15s). Fix: a white-box `fastClient(baseURL, retryMax)` helper (1ms/2ms waits)
  built from the same `newRetryableClient`, and route just those two through it.
  Intent is preserved (error surfaces, `out` untouched, no silent truncation).
  **All 4xx tests are unaffected** — `retryPolicy` returns `false` for 4xx, so
  retryablehttp returns the response normally and `count==1` still holds.
- **retryablehttp buffers the request body for replay via the `io.Reader`
  case.** A body from `http.NewRequestWithContext(…, bytes.NewReader(b))` reaches
  `FromRequest` as `io.NopCloser(*bytes.Reader)` — not a bare `*bytes.Reader` —
  so it falls through to `getBodyReaderAndContentLength`'s `io.Reader` branch,
  which `io.ReadAll`s it into a replayable buffer. POST retries resend the body
  correctly; no manual `GetBody` plumbing needed.
- **Exhaustion and context both surface as a wrapped error through
  `StandardClient`.** With `ErrorHandler` nil, a persistent 5xx returns `(nil,
"giving up after N attempt(s)")` (body drained) — so `execute` reports an error
  (not a typed `*APIError`) on budget exhaustion, which is all Req 9 needs. A
  context deadline returns `req.Context().Err()` wrapped with `%w`, reachable via
  `errors.Is(err, context.DeadlineExceeded)` even through the `*url.Error` that
  `http.Client.Do` adds. Tested with a hang-until-ctx-done handler so the bound
  is the deadline, not the backoff schedule.
- **Logger routed through slog via `*slogLeveledLogger`, `New` passes
  `slog.Default()`.** retryablehttp's `Logger` is an `interface{}` matched against
  `LeveledLogger`; the adapter maps Debug/Info/Warn/Error onto slog (slog's
  variadic `...any` already takes key/value pairs). A nil logger leaves
  `rc.Logger = nil` (silent) — used in tests to avoid stderr noise. Once
  [[dt-zxwf]] sets the default slog handler, retry chatter obeys `DN_LOG_LEVEL`
  for free.

## API endpoint methods (dt-255n, 2026-06-06)

- **`CreateHostRequest` models the documented v2 body only — no tun/network-name
  field.** The ticket text lists "tun/network name" among the struct's fields, but
  the API reference §4.1 body has no such field. The tun device name (matching
  `DN_NETWORK_NAME`) is a `dnclient`-invocation concern — `dnclient enroll`/`run
-name %i` — not part of the create-host POST. Adding it to the JSON body would
  fail the "expected JSON body" assertion and send an undocumented field. The
  enroll command (dt-j2ab) maps config→`CreateHostRequest`; `DN_IP_ADDRESS`
  becomes a single `ipAddresses` entry (v2 superseded the v1 scalar `ipAddress`).
- **`do` already unwraps the nested response envelopes for free.** The
  create-host response is `{data:{host,enrollmentCode}}` and downloads is
  `{data:{dnclient,versionInfo}}`. Because `do` unmarshals the inner `data` object
  into `out`, modeling `HostAndCode`/`Downloads` with matching json tags (`host`,
  `enrollmentCode`, `dnclient`, `versionInfo`) means no second unwrap layer is
  needed — the typed methods are one `do` call each.
- **`DeleteHost` calls `execute`, not `do`.** Delete returns `{data:{}}` with
  nothing to decode, and the 404-as-success branch needs the raw `*APIError`
  (via `errors.As` on the status) before any envelope parse. `do` would try to
  unwrap an empty `data` — `execute` returns the bytes/typed error directly, so
  the 404→nil / other-4xx→`*APIError` logic (consumed by [[dt-2t72]] unenroll) is
  clean. This is the same `execute`-vs-`do` split [[dt-ihil]] set up.
- **Request-body shape: `omitempty` on optionals, explicit `listenPort`.** Bools
  (`isLighthouse`/`isRelay`) and slices (`tags`, `staticAddresses`, `ipAddresses`)
  use `omitempty` so a regular host sends a minimal body; `listenPort` is sent
  always (0 = auto-select is a meaningful value, not "unset"). `ListHosts` reuses
  the `listAll` cursor walker from [[dt-ihil]] — no name filter exists server-side,
  so callers match `name` client-side over the full network list.

## Cursor pagination + execute refactor (dt-ihil, 2026-06-06)

- **`do` was split into `execute` + `do`.** `execute(ctx, method, path, body)
([]byte, error)` now owns request building, bearer auth, and the
  status-check-before-body D4 invariant, returning raw success-body bytes (or a
  typed `*APIError`). `do` unwraps `{data}` on top; `listAll` decodes
  `{data:[]json.RawMessage, metadata}` on top. The two dt-z99h/dt-4b0e invariants
  survived verbatim — status checked before the success body is read, and `out`
  is never populated on failure — because `execute` returns `(nil, err)` on
  non-2xx and the decoders only run on the success branch. All pre-existing
  `client_test.go` assertions pass unchanged.
- **`do` switched from streaming decode to `Unmarshal` of buffered bytes.**
  `execute` does `io.ReadAll`; `do` then `json.Unmarshal`s. Functionally
  identical to the old `json.NewDecoder(resp.Body).Decode`, but now the body is
  read once and reusable — required so `listAll` can read `data` and `metadata`
  from the same response.
- **`listAll` defaults `pageSize=500`** (the §2.4 max) only when the caller
  hasn't set one, to bound round-trips for enroll's full-list name match (the
  host-list endpoint has no name filter). Caller filters are copied into a fresh
  `url.Values`, never mutated; the cursor is threaded via `query.Set("cursor",
nextCursor)` each iteration. Loop terminates on `!hasNextPage ||
nextCursor == ""`.
- **Mid-pagination failure already surfaces** without retry because `httpClient`
  is still `http.DefaultClient`. dt-egz4 swaps in `retryablehttp` as the injected
  client; `execute` needs no change — it returns whatever the transport yields.
  dt-255n builds typed list methods (e.g. ListHosts) on `listAll`.

## golangci-lint gate config (dt-prt6, 2026-06-06)

- **The toolchain is golangci-lint v2 (2.12.2), not v1.** The config schema
  changed: `version: "2"` is required, the linter set is chosen via
  `linters.default` (`standard` = errcheck/govet/ineffassign/staticcheck/unused)
  plus `linters.enable`, formatters moved to a separate top-level `formatters:`
  block, and exclusions live under `linters.exclusions.rules`. The v1 example in
  the golang skill (`linters.enable` list + `linters-settings`) will not parse.
  Config lives at `src/.golangci.yml` (the lint target runs `cd src && golangci-lint run`).
- **Behavior 4 ("forbid `_ =` to mute errors") is a review convention, not
  `errcheck.check-blank`.** Enabling check-blank would flag the project's own
  deliberate idioms — `_ = resp.Body.Close()` (api/client.go) and `_, _ =
w.Write(...)` in httptest handlers — breaking `make validate` and contradicting
  committed code. The gate keeps errcheck at default strictness (every unchecked
  error is flagged) and documents the no-`_ =` rule in the config header; humans
  enforce it. Don't reach for check-blank.
- **revive's stutter check fights a deliberate API name.** revive's `exported`
  rule flags `api.APIError`/`APIErrorItem` as stutter, but those names are load-
  bearing (see the dt-4b0e note below — `errors.As(err, &apiErr)`). Suppressed via
  a _scoped_ exclusion (`path: internal/api/`, `text: stutters`) so the rule stays
  live everywhere else, rather than renaming another ticket's public type or
  dropping the rule globally.
- **Specifying `revive.rules` replaces revive's whole default rule set** — so to
  tune one rule you'd have to re-list them all. The scoped `exclusions.rules`
  message-match avoids that: keep revive's defaults, suppress just the one
  message in one path.
- **Verify the gate isn't a no-op.** Dropped a temp `os.Setenv(...)` (unchecked)
  into a package and confirmed errcheck flagged it before trusting the green
  result. revive's `exported`/`package-comments` also caught genuinely-missing
  doc comments in `internal/version` and `cmd/dn-tool/main.go` — fixed those (Go
  convention: exported identifiers and packages get doc comments).

## API error typing + retry policy (dt-4b0e, 2026-06-06)

- **The retry policy ships before the retry transport, deliberately.** dt-4b0e
  owns "4xx terminal" but dt-egz4 owns wiring `retryablehttp`. Resolved by
  defining `retryPolicy(ctx, *http.Response, error) (bool, error)` using only
  stdlib types — it matches `retryablehttp.CheckRetry`'s underlying signature, so
  dt-egz4 assigns it directly (`client.CheckRetry = retryPolicy`) with no
  conversion and no import added here. It's tested in isolation (table + a
  cancelled-ctx case), so the no-retry guarantee is real now, not deferred to
  whenever the transport lands. Until then `httpClient` is still
  `http.DefaultClient` (one request, never retried), so the httptest "count==1"
  assertions hold both before and after dt-egz4.
- **D4 invariant survived the envelope parse.** The non-2xx branch now _does_
  read the body (to parse `{"errors":[…]}`), which seems to contradict the
  dt-z99h note "returns before touching the body". The invariant that actually
  matters is narrower: `out` is never populated on failure. The body is read
  into a local `*APIError` only; `out` is untouched. The existing
  `TestDoChecksStatusBeforeBody` (junk 500 body) still passes — junk fails the
  `json.Unmarshal`, leaving `Errors` nil.
- **Parse failures are swallowed by design.** A non-JSON / unreadable error body
  must still yield a usable `*APIError` (status only), so the `io.ReadAll` and
  `json.Unmarshal` errors are intentionally ignored — the typed error with the
  HTTP status is more useful to callers than a decode error. This is the one
  place ignoring an error is correct; it's guarded by the non-JSON-body test.
- **Wrap with `%w`, not the bare error.** `do` returns
  `fmt.Errorf("%s %s: %w", method, path, apiErr)` so call sites recover the type
  via `errors.As(err, &apiErr)` and branch on `apiErr.Has(code, path)` — enroll's
  orphan check (`ERR_DUPLICATE_VALUE`/`name`) depends on this, never on string
  matching.

## API client core (dt-z99h, 2026-06-06)

- **D4 fix is an ordering invariant, not a feature.** `do` returns on a non-2xx
  `resp.StatusCode` _before_ touching the body, so a 5xx with a junk payload
  never reaches the JSON decoder and never populates `out`. The test asserts
  this two ways: the call errors, and a pre-seeded `out` field is left
  unchanged. Keep this ordering when dt-4b0e adds typed errors — parse the
  error envelope only on the failure branch, after the status check.
- **Scope boundary with dt-4b0e / dt-egz4.** Non-2xx currently yields a plain
  `"unexpected status N"` error and `httpClient` is `http.DefaultClient` (no
  retry). Typed `code`/`path` errors + 4xx-no-retry are dt-4b0e; retry/backoff
  bounded by `DN_API_TIMEOUT` is dt-egz4. Both extend `do` rather than replace
  it — dt-egz4 will likely swap in `retryablehttp.StandardClient()` as the
  injected `httpClient`.
- **`Content-Type` is conditional, `Accept` is not.** Header is set only when
  `body != nil` (a bodyless GET sends no `Content-Type`); `out == nil` skips
  decoding so callers that ignore the response body work. `nil` body / `nil`
  out are both first-class.
- **Unexported `do` isn't flagged unused** because same-package `_test.go`
  files exercise it — no need to export it prematurely just to satisfy lint.
  Endpoint methods (dt-255n) will be its real callers.

## Config struct + env loader (dt-mhir, 2026-06-06)

- **Testable hostname fallback without widening the public API.** The ticket
  fixes the signature as `Load(getenv func(string) string)`, yet behavior 3
  needs the `os.Hostname()` fallback to be injectable. Resolved by keeping the
  public `Load(getenv)` as a thin wrapper over an unexported
  `load(getenv, hostname func() (string, error))`; tests call `load` with a
  stub hostname. Use this pattern for the other config layers that need to
  inject a system dependency while preserving a clean public signature.
- **Scope boundary with dt-toqi.** `DN_TAGS` / `DN_STATIC_ADDRESSES` are
  declared as `[]string` fields but left unparsed here — JSON-array decoding is
  dt-toqi's job (the field comments say so). Bools/port/duration _are_ parsed
  here (behavior 5 requires the module-only bools to populate), returning
  wrapped errors on malformed input rather than silently defaulting.
- **`APITimeout` has no single default.** §2.3 lists "~30s enroll / ~10s
  unenroll" — per-command, not global. `Load` leaves it `0` when unset; the
  command layer is expected to pick the right deadline. Don't invent a global
  default here.

## CLI skeleton (dt-ecf9, 2026-06-06)

- **urfave/cli is v3** (`github.com/urfave/cli/v3` v3.9.0). v3 has no `App`
  type — the root program is a `*cli.Command`, subcommands nest in
  `Commands`, and handlers are `func(context.Context, *cli.Command) error`.
  The [reference](research/urfave-cli-reference.md) v2→v3 table is accurate;
  follow it rather than v2 idioms found online.
- **`Command.Run` does not call `os.Exit`.** It returns the Action's error.
  `cli.HandleExitCoder` only acts on errors implementing `ExitCoder` (it
  prints + calls `OsExiter`); plain errors must be surfaced by the caller.
  `main()` currently prints plain errors to stderr and exits 1 — proper
  0/1/2 exit-code mapping is deferred to dt-icq8.
- **`--version` is enabled by setting `Command.Version`**; it auto-registers
  `--version`/`-v`. We feed it `version.Current()`.
- **Flag scoping:** root `Flags` are global/inherited; a flag placed only on a
  subcommand's `Flags` is local to it. `--force` lives on `enroll` only.
- **go.mod stays minimal:** urfave/cli/v3 is stdlib-only, so `go mod tidy`
  adds no indirect requires (testify/yaml get _downloaded_ as cli's own test
  deps but are not recorded in our go.mod). `go.sum` is committed — the flake
  packaging (dt-0mdh) needs it for `vendorHash`.
- **Testing the CLI without running it:** `newApp()` returns the root command
  with no I/O side effects, so tests introspect `.Commands`/`.Flags` directly
  and only call `app.Run` (with buffers wired to `Writer`/`ErrWriter`) for
  `--version` and stub-error behavior.
