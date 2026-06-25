---
id: dt-9lem
status: closed
deps: [dt-jbme]
links: [dt-nutn, dt-iwl2, dt-jbme]
created: 2026-06-07T19:04:13Z
type: chore
priority: 3
assignee: Andre Silva
parent: dt-3gvq
tags: [unenroll, refactor, architecture]
---
# Give unenroll its own API-key precondition + narrow its interface

Deepen the unenroll module: drop its *config.Config dependency (it only ever reads NetworkName) in favour of a narrow Input{NetworkName, APIKey}, and move the DN_API_KEY precondition out of the cmd layer into the module so it self-enforces its own interface. Makes enroll and unenroll consistent (both modules own their key check). Surfaced by an architecture review (deepening candidate 2); see Design for the grounding that ruled out an api-client-level check.

## Problem

`unenroll.Unenroll` takes a whole `*config.Config` but reads only `cfg.NetworkName` (`internal/unenroll/unenroll.go:46-47,56,65`) — a test must build a full Config to set one field. Meanwhile its "DN_API_KEY required" precondition lives up in the command layer (`cmd/dn-tool/unenroll.go:23,45` — `errMissingAPIKey`), so a caller must know to gate the key before calling, and enroll (which checks the key in-module, see dt-nutn / `newEnrollInput`) and unenroll disagree on where that knowledge lives. The key requirement is part of unenroll's interface (a precondition for correct use) but is enforced outside the module.

## Grounding (what ruled out alternatives)

- `api.New(cfg) *Client` returns no error and stores the key; `execute` sets the bearer header on every request (`internal/api/client.go:133,168`).
- `install` shares that same client for `ListDownloads`, and per design §2.3 the key is "Required for enroll/unenroll" — NOT install. So the key requirement is **per-operation, not per-client**: pushing the check into `api.New` would wrongly break a keyless `install`. Rejected.
- A lazy check inside `DeleteHost` would fire only after `ReadHostID`, changing error precedence; design Req 4 ("require a key **before attempting** unenrollment") and dt-577o (which leans on `errMissingAPIKey` firing before `ReadHostID`) both want the check first. So the precondition stays at the front of the flow. Rejected.

## Decision (scoped via architecture-review interview)

**The unenroll module owns its precondition.** Pass a narrow `Input{NetworkName, APIKey}`; the module checks the key first, then proceeds. Chosen over keeping the guard at the cmd layer because it makes the module self-enforce its own interface and makes enroll/unenroll symmetric (the report's "they disagree" complaint). Accepted cost: the key appears twice — in `Input` for the check, and inside the injected `HostDeleter` the client already holds.

## Design

`internal/unenroll/unenroll.go` — narrow input; precondition moves in and stays first:

```go
// Input is the validated request data for Unenroll: the network whose host
// record to remove, and the management API key required before any remote call.
type Input struct {
	NetworkName string
	APIKey      config.Secret
}

// ErrMissingAPIKey reports the absence of DN_API_KEY, which unenroll requires
// before attempting unenrollment (design Req 4 / §2.3). Checked before ReadHostID
// so a keyless unenroll fails with this actionable error rather than a
// not-enrolled one.
var ErrMissingAPIKey = errors.New("DN_API_KEY is required to unenroll")

func Unenroll(ctx context.Context, in Input, deps Deps) (output.Result, error) {
	if in.APIKey == "" {
		return output.Result{}, ErrMissingAPIKey
	}
	hostID, err := dnclient.ReadHostID(deps.ConfigRoot, in.NetworkName)
	// … delete-then-remove, §2.5 invariant unchanged; reads in.NetworkName
}
```

`Deps{API HostDeleter, ConfigRoot}` stays as-is — injected collaborators, distinct from the request data in `Input`.

`cmd/dn-tool/unenroll.go` — delete the `errMissingAPIKey` var and the `if cfg.APIKey == ""` guard; `runUnenroll` just wires the input:

```go
return unenroll.Unenroll(ctx, unenroll.Input{NetworkName: cfg.NetworkName, APIKey: cfg.APIKey},
	unenroll.Deps{API: api.New(cfg), ConfigRoot: configRoot})
```

## Where

- `internal/unenroll/unenroll.go` — add `Input` + `ErrMissingAPIKey`; `Unenroll` takes `Input`, reads `in.NetworkName`/`in.APIKey`.
- `cmd/dn-tool/unenroll.go` — remove `errMissingAPIKey` and the guard; wire `Input`.

## Interactions

- **dt-577o (run shutdown):** the precondition still fires before `ReadHostID`, so the "key required" outcome is preserved — only the error's home moves from the `main` package to `unenroll.ErrMissingAPIKey`. Any `errors.Is` against the old cmd-package var (run/cmd tests) must repoint.
- **dt-nutn (candidate 1):** consistency partner — both leave each module owning its `DN_API_KEY` check. Linked.

## Tests (replace, don't layer)

- `internal/unenroll/unenroll_test.go` — callsites build a 2-field `Input` instead of a full `*config.Config`; the missing-key case becomes a module-interface test (`TestUnenroll_MissingAPIKey`).
- `cmd/dn-tool/unenroll_test.go` — drop the cmd-level `errMissingAPIKey` assertion (now the module's responsibility); keep only integration coverage that the wired command propagates it.

## Acceptance criteria

- `Unenroll`'s signature is `Unenroll(ctx, Input, Deps) (output.Result, error)`; it no longer takes `*config.Config`.
- `Unenroll` references no `config.Config`; it reads `in.NetworkName` and `in.APIKey` only.
- The `DN_API_KEY` precondition (`unenroll.ErrMissingAPIKey`) lives in the unenroll package and is checked before `ReadHostID` (Req 4 ordering preserved).
- The cmd-layer `errMissingAPIKey` var and `if cfg.APIKey == ""` check are removed; cmd only wires `Input`.
- The §2.5 invariant (local config removed only after the delete succeeds or reports 404) is unchanged.
- enroll and unenroll are consistent: each module owns its `DN_API_KEY` precondition.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 2), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.5 (unenroll/shutdown invariant), Req 4 (require API key before attempting unenrollment), §2.3 (DN_API_KEY required for enroll/unenroll, not install).
- Sibling: dt-nutn (candidate 1, enroll create-path input). dt-577o (run shutdown-unenroll error identity).

