---
id: dt-nutn
status: closed
deps: [dt-jbme]
links: [dt-9lem, dt-iwl2, dt-jbme]
created: 2026-06-07T18:38:33Z
type: chore
priority: 3
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, refactor, architecture]
---
# Concentrate enroll create-path input behind a validated enrollInput value

Locality cleanup of the enroll state machine: replace the bare api.CreateHostRequest + scattered cfg reads on the create path (§2.4 rows 2-4) with one validated value object, killing the cfg.Hostname/req.Name duplication. Interface and state machine deliberately unchanged. Surfaced by an architecture review (deepening candidate 1); see Design for the two decisions that scoped it.

## Problem

After `buildCreateRequest` returns a bare `api.CreateHostRequest`, the create path in `Enroll` reaches back into `cfg` four more times and re-derives the hostname it already mapped:

- `internal/enroll/state.go:69` — `ListHosts(ctx, cfg.NetworkID)`
- `internal/enroll/state.go:74` — `if h.Name != cfg.Hostname` (duplicates `req.Name`)
- `internal/enroll/state.go:97` — `DNClient.Enroll(ctx, cfg.NetworkName, …)`
- `internal/enroll/state.go:105` — `Result{… Network: cfg.NetworkName}`

The create-path inputs come from two sources at once (the built request AND raw `cfg`), and `cfg.Hostname` duplicates `req.Name`. Deletion test: remove the value object and the scattered reads + duplication reappear in the state-machine body.

## Decisions (scoped via architecture-review interview)

These two choices were made deliberately and bound the scope; do not expand beyond them without re-deciding:

1. **Keep the state machine whole.** `Enroll` keeps taking `*config.Config`; all four §2.4 rows stay in the module. The no-op (row 1) is NOT lifted to the caller. Consequence: this is a locality cleanup, not an interface shrink — `Enroll`'s signature is unchanged.
2. **Keep `validateRoles` up front.** Role-coherence validation keeps firing before the `ConfigExists` no-op check, so a contradictory role config (`IS_LIGHTHOUSE && IS_RELAY`, etc.) fails loud even on an already-enrolled host. Role validation and create-path param validation remain two concerns at two points, by design.

## Design

Add an unexported value object and constructor in `internal/enroll` (internal seam — not exposed at `Enroll`'s interface):

```go
// enrollInput is the validated create-path input for §2.4 rows 2-4: the mapped
// create-host request plus the network name driving the dnclient tun device and
// the result. Produced only after the row-1 no-op check, so the already-enrolled
// path needs none of the required params it validates.
type enrollInput struct {
	Request     api.CreateHostRequest
	NetworkName string
}

func newEnrollInput(cfg *config.Config) (enrollInput, error) {
	req, err := buildCreateRequest(cfg) // required-param checks + mapping, unchanged
	if err != nil {
		return enrollInput{}, err
	}
	return enrollInput{Request: req, NetworkName: cfg.NetworkName}, nil
}
```

Keep `buildCreateRequest` as the wrapped mapping helper (preserves its detailed v2-endpoint doc comment; minimal churn).

`Enroll` body after the change — `cfg` is referenced in exactly three spots (`validateRoles`, `ConfigExists`, `newEnrollInput`); the create path reads only from `in`:

```go
if err := validateRoles(cfg); err != nil {            // decision 2: stays first
	return output.Result{}, err
}
if dnclient.ConfigExists(deps.ConfigRoot, cfg.NetworkName) {
	return output.Result{Action: "enroll", Changed: false}, nil
}
in, err := newEnrollInput(cfg)
if err != nil {
	return output.Result{}, err
}
hosts, err := deps.API.ListHosts(ctx, in.Request.NetworkID)
// … orphan match: if h.Name != in.Request.Name  (duplication gone)
hc, err := deps.API.CreateHostAndEnrollmentCode(ctx, in.Request)
// … deps.DNClient.Enroll(ctx, in.NetworkName, hc.EnrollmentCode.Code.Reveal())
return output.Result{Action: "enroll", Changed: true, HostID: hc.Host.ID, Network: in.NetworkName}, nil
```

Note: `newEnrollInput` retains the `DN_API_KEY` required-param check that lives in `buildCreateRequest` today, even though `APIKey` is consumed by `api.New(cfg)` at the command layer rather than placed in the request — it is the create-path "do we have credentials" gate. (The sibling question of where unenroll's API-key precondition lives is a separate ticket — deepening candidate 2.)

## Where

- `internal/enroll/enroll.go` — add `enrollInput` + `newEnrollInput`; keep `buildCreateRequest`.
- `internal/enroll/state.go` — `Enroll` create path reads from `in` instead of `cfg`.

## Tests (replace, don't layer)

- `internal/enroll/validate_test.go` — survives untouched; `validateRoles` does not move.
- `internal/enroll/fields_test.go` — retarget from `buildCreateRequest` to `newEnrollInput`: same assertions on `.Request` and the required-param error cases, plus one asserting `.NetworkName` is carried.
- `internal/enroll/state_test.go`, `orphan_test.go`, `create_test.go` — survive unchanged; `Enroll`'s interface is stable, so they must not churn (their stability is the upside of decision 1).

## Acceptance criteria

- `Enroll`'s signature is unchanged: `Enroll(ctx, *config.Config, Deps) (output.Result, error)`.
- The create path (rows 2-4) references no `cfg.*` field directly; it reads `in.Request.*` and `in.NetworkName`. `cfg` is touched only by `validateRoles`, `ConfigExists`, and `newEnrollInput`.
- The `cfg.Hostname` orphan-match read is gone (uses `in.Request.Name`).
- `validateRoles` still runs before the `ConfigExists` no-op check (decision 2).
- Already-enrolled no-op still succeeds with no `DN_API_KEY`/`DN_NETWORK_ID`/`DN_ROLE_ID` set (row-1 behavior, design Req 2).
- The §2.4 state-machine tests pass unchanged; `buildCreateRequest`'s tests are retargeted to `newEnrollInput`, not duplicated.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 1), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.4 (enrollment state machine), Req 2 (already-enrolled no-op makes no changes), Req 3 (lighthouse/relay rules → `validateRoles`).
- Sibling: deepening candidate 2 (give unenroll its own API-key precondition) — to be ticketed separately.


## Notes

**2026-06-07T21:15:20Z**

Sequencing: implement FIRST of the three enroll-body tickets (after dt-jbme). Establishes the enrollInput 'in' value as the create-path source that dt-iwl2 and dt-41ww both read from. Chain: dt-nutn -> dt-iwl2 -> dt-41ww, linear to avoid conflicts in enroll/state.go.
