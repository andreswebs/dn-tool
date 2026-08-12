---
id: dt-nrq1
status: closed
deps: []
links: [dt-cjgq, dt-nljm, dt-2efx]
created: 2026-08-12T17:39:20Z
type: bug
priority: 1
assignee: Andre Silva
tags: [adr, errors, output]
---
# Do not emit an error envelope for non-failure statuses (ADR 0002)

Regression against the ADR conformance adoption epic (dt-69kx), found in real
output after that epic closed. Related: dt-2efx (declared the sentinels),
dt-nljm (built the envelope boundary), dt-cjgq (the golden harness that did not
catch it).

## The defect

The exit boundary emits a JSON error envelope for **every** non-nil error, so
two non-failure statuses are reported to consumers as failures.

`internal/command/run.go:69-75`:

```go
func exitCode(err error, w io.Writer) int {
    if err == nil {
        return output.CodeOK
    }
    output.EmitError(w, err)
    return output.ExitCodeFor(err)
}
```

### Case 1: a graceful signal shutdown (observed in production output)

```json
{"schema_version":1,"ok":false,"error":{"code":"signal_shutdown","message":"terminated by signal","hint":"the process was terminated by a signal after a graceful shutdown; 130 for SIGINT, 143 for SIGTERM"}}
```

A clean container stop is not a failure. `docs/dn-tool-design.md` Req 5 is
explicit: "a clean container stop is reported as the signal code, **not as a
failure**". ADR 0005 scopes the envelope to failures ("**Failures** render on
stderr as an error envelope"), and ADR 0001 puts 130/143 in a "terminated by
SIGINT/SIGTERM" class distinct from the 64-78 failure range.

### Case 2: the `--assert-changed` no-op (reproduced 2026-08-12)

```console
$ dn-tool --assert-changed enroll     # local config present, a clean no-op
exit=1
stdout: {"schema_version":1,"ok":true,"action":"enroll","changed":false}
stderr: {"schema_version":1,"ok":false,"error":{"code":"no_change","message":"no change made","hint":"the command succeeded and changed nothing; drop --assert-changed if a no-op is an acceptable outcome"}}
```

**Two contradictory envelopes for one successful invocation**: `ok:true` on
stdout and `ok:false` on stderr, about the same command.

This case is a direct violation of prose in ADR 0002, which names it:

> Result and status are decoupled. When a command exits with a result-class code
> (1, or a documented sub-code in 2-63, per ADR 0001), the full result envelope
> is still written to stdout first; **the error value that carries the exit code
> prints nothing itself.**

### Impact

Any consumer branching on `ok` treats every normal container stop and every
assert-changed no-op as a failure, which is exactly the ambiguity the envelope
was introduced to remove. An agent reading stderr sees a fabricated failure
where none occurred.

## Second defect: the hints are documentation, not remediation

ADR 0002 defines the hint as "an optional remediation hint written for the
consumer, not the developer". Two hints describe the exit-code mapping instead:

- `internal/run/errors.go:19-21` (`signal_shutdown`): "the process was
  terminated by a signal after a graceful shutdown; 130 for SIGINT, 143 for
  SIGTERM". There is nothing to remediate.
- `internal/run/errors.go:28-30` (`daemon_exit`): "the dnclient daemon exited
  with its own status, which dn-tool propagates verbatim; 70 when it had no
  clean status". Describes a mapping, not an action.

Both trace to the dt-2efx spec, which argued the per-invocation exit-code
variance should be "stated in the hint rather than hidden behind a single
misleading number". That was the wrong home for it: the variance belongs in
`schema`, which already carries it as
`passes_through_child_exit_status` (`internal/command/schema*.go`, per dt-3xxb).

## Why a numeric predicate will not work

Do not gate the envelope on the exit code's numeric range:

- `signal_shutdown` produces 130/143, which are **above** 64 yet not failures.
- `daemon_exit` propagates the daemon's status N verbatim, which **is** a real
  failure of the wrapped process but may be any value, including 1-63.

The error itself must declare its class.

## Target shape

1. Add an opt-out marker to `internal/terr`, one method, defaulting to failure so
   an unclassified error stays loud. For example:

   ```go
   // Status is an optional interface for a coded value that carries an exit
   // code without being a failure: the code is the whole contract and no error
   // envelope is written (ADR 0002, "the error value ... prints nothing
   // itself").
   type Status interface{ NonFailure() bool }
   ```

2. `internal/command/run.go`'s `exitCode` consults it: resolve the code as
   today, but skip `output.EmitError` when the error is a non-failure status.
3. Mark exactly two as non-failure:
   - `ErrNoChange` (`internal/command/errors.go:46`, `no_change`, exit 1).
   - `ErrSignalShutdown` (`internal/run/errors.go:19`, `signal_shutdown`,
     130/143). Note `exitErr` (`internal/run/errors.go:36-69`) must forward the
     marker for the wrapped instance, alongside `Code`/`Hint`/`Unwrap`.
4. `ErrDaemonExit` keeps its envelope: a non-zero daemon exit is a genuine
   failure. Rewrite its hint as real remediation, pointing the operator at the
   daemon's own output on stderr rather than restating the code mapping.
5. On a signal shutdown, stderr carries **log records only**. This preserves
   design Req 5, whose unenroll-failure diagnostic
   (`internal/run/run.go`, the `logctx` error log on the shutdown path) was
   always specified as a log rather than something that changes the exit code.

## Open design decision (owner's call, do not assume)

`no_change` and `signal_shutdown` are currently listed in `schema`'s **error**
inventory (`terr.All()` projection), which repeats the same category confusion:
they are statuses, not errors. Options:

- (a) Add a parallel `statuses` list to the schema envelope, keeping the
  code-to-meaning mapping a consumer wants without claiming they are failures.
- (b) Drop them from the inventory and let `exit_codes` plus the per-command
  sets describe them, losing the code-to-meaning mapping.
- (c) Keep them under `errors` with a boolean flag.

Recommendation: **(a)**. Resolve before implementing, because it decides whether
the `schema` golden changes.

## Blast radius (verified 2026-08-12)

Smaller than it looks, because nothing currently guards the rule.

**Golden files pinning the wrong behavior: one.**

- `internal/command/testdata/install-noop-assert-changed/stderr` holds the
  `no_change` envelope and must become empty. Its `stdout` and `exit` (1) are
  correct and stay.
- `internal/command/testdata/schema/stdout` lists `no_change` and
  `signal_shutdown` under `.errors`; it changes only if the open decision above
  moves them.
- **No golden pins the `signal_shutdown` envelope.**
  `internal/command/testdata/run/` captures a clean daemon exit (empty stdout
  and stderr, exit 0), and dt-cjgq deliberately exempted 130/143 from
  in-process coverage because the in-process harness cannot deliver a signal.
  That exemption is why this reached real output unnoticed.

**Tests that break: none identified.** Every test touching these two codes
asserts the exit code or error identity, not the envelope:

- `internal/command/run_test.go:99-103` (`TestExitCode_CodedErrorHonored`) uses
  `ErrNoChange` and asserts only the code, so it passes either way. It should
  gain an assertion that stderr stays empty.
- `internal/command/errors_test.go:73`, `:89-92`;
  `internal/output/exit_test.go:58`;
  `internal/output/exitcodes_test.go:87`, `:94-95`;
  `internal/run/run_test.go:806-814`; `internal/enroll/state_test.go:90` are all
  code/identity assertions and are unaffected.

This is an inspection, not a test run: confirm by running the suite.

## Acceptance criteria

- A clean signal shutdown writes **nothing** to stderr beyond log records, and
  exits 130 (SIGINT) or 143 (SIGTERM).
- `--assert-changed` on a no-op writes the result envelope to stdout, **nothing**
  to stderr, and exits 1.
- No invocation ever emits both an `ok:true` and an `ok:false` envelope.
- Every failure-class error still emits its envelope: one test per class
  (64, 65, 69, 74, 78) plus the unclassified `internal_error` case.
- `daemon_exit` still emits an envelope and still propagates the daemon's status
  N verbatim.
- **The missing guard exists**: a test asserting that a non-failure coded status
  produces an exit code with an empty stderr, so this cannot regress. Add it at
  the `exitCode` boundary, where a signal-class status can be constructed
  directly without needing a real signal.
- `daemon_exit`'s hint is remediation advice, not a description of the code
  mapping. Review every other sentinel's hint against ADR 0002's
  "for the consumer, not the developer" test while here.
- `internal/command/testdata/install-noop-assert-changed/stderr` regenerated
  (empty) via `go test ./internal/command -update`, and the golden diff reviewed.
- The exec-level suite (`docs/specs/e2e-testing.md`) asserts the signal case,
  since the in-process harness cannot: on SIGTERM, exit 143 with no error
  envelope on stderr.
- Exit codes otherwise unchanged: `internal/output/exitcodes_test.go`,
  `internal/run/signal_test.go`, `internal/run/run_test.go` pass.
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. **Note dt-xyp4**: `make test` is
  currently red on macOS for an unrelated reason, so fix or land that first, or
  verify on Linux.
- TDD per `docs/agents/code-conventions.md`.

## Docs

- `docs/dn-tool-design.md` 2.8: state that a result-class exit (1) and a signal
  exit (130/143) write no error envelope, so the two streams cannot contradict
  each other.
- `docs/specs/e2e-testing.md`: correct the stderr contract row if it implies an
  envelope on every non-zero exit.
- `CHANGELOG.md`, Unreleased / Fixed: a graceful shutdown and an
  `--assert-changed` no-op no longer emit an error envelope, and no longer
  report `ok:false` for a successful command.
- `markdownlint-cli2 --config ~/.markdownlint.yaml --fix <files>` on every
  markdown file touched.

## Do not commit

Leave git commits and branching to the repo owner.

## Notes

**2026-08-12T18:09:41Z**

Fixed. terr gained a second kind of coded value: terr.NewStatus declares a non-failure whose exit code is the whole contract, exposed as the terr.Status interface (NonFailure() bool) with terr.IsNonFailure(err) as the boundary's predicate; New still defaults to failure. internal/command exitCode resolves the code exactly as before but skips output.EmitError for a status. ErrNoChange and ErrSignalShutdown are the only two statuses (pinned by TestNonFailureStatusesArePinned), and run's exitErr forwards NonFailure from its class so the per-invocation 130/143 wrapper keeps the marker. ErrDaemonExit stays a failure with its envelope and its status-N passthrough; its hint was rewritten as real remediation, and ErrSignalShutdown's hint is now empty (nothing to remediate) — the 130-vs-143 variance lives in run's exit_codes registry.

Open design decision resolved as recommended, option (a): schema gained a 'statuses' list parallel to 'errors' (output.StatusInventory / SchemaStatus with code, exit_code, meaning, optional hint); ErrorInventory now projects failures only. This is a schema contract change for consumers reading .errors for no_change/signal_shutdown.

Side effect worth knowing: an unenroll failure during a signalled shutdown used to surface only via the (now suppressed) envelope, so runAndUnenroll logs it and returns nil on that path — which is what design Req 5 always specified ('is logged ... but does not change the signal exit code'). Extracted as logDroppedUnenrollFailure, shared with the daemon-exit path.

Guards added: TestExitCode_NonFailureStatusWritesNoEnvelope at the boundary; golden assertStreamContract now asserts the ABSENCE of an envelope for exit 1 and 130/143 (it previously demanded one, which is why the defect passed review); and a new exec-level suite in e2e/ (go test ./e2e, part of make build) builds the binary and sends a real SIGTERM/SIGINT — asserting exit 143/130, the shutdown DELETE, empty stdout, and no error envelope on stderr. Verified the e2e guard by re-introducing the defect and watching it go red. Skips on hosts the install platform gate rejects.

Goldens: install-noop-assert-changed/stderr is now empty (stdout/exit unchanged); schema/stdout moved the two codes into statuses. Docs updated: design 2.8 (+2.9/2.11), specs/e2e-testing.md (new Layer 1b, stderr contract row, assert-changed and run rows), agents/build-and-validation.md, CHANGELOG Fixed/Added, specs/learnings.md.

Not addressed (deliberately, ticket spec): a termination signal arriving while startup is failing still wraps that failure in the signal status, so its envelope is suppressed and only the signal code is reported. Rare race, pre-existing in the exit-code precedence; worth a follow-up ticket if the silent message matters.
