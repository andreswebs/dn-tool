---
id: dt-41ww
status: closed
deps: [dt-jbme, dt-iwl2]
links: [dt-iwl2]
created: 2026-06-07T20:13:03Z
type: bug
priority: 3
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, bug]
---
# enroll create path ignores Create-time ERR_DUPLICATE_VALUE; doc comments falsely claim Has-based detection

The enroll state machine detects an existing remote record only via list-and-match before CreateHostAndEnrollmentCode. A duplicate created in the TOCTOU window between ListHosts and Create surfaces as a generic 'creating host record' error, not the orphan guidance. APIError.Has("ERR_DUPLICATE_VALUE","name") exists and is api-tested but is never called by enroll — yet client.go:41 and endpoints.go:83 both document that the state machine reads via APIError.Has. Found during an architecture review (deepening candidate 5).

## Two distinct defects

### 1. Doc/impl mismatch (definitely fix)

Two doc comments assert behavior that does not exist:

- `internal/api/client.go:41` — "enroll's orphan detection keys on 400 ERR_DUPLICATE_VALUE at path \"name\"".
- `internal/api/endpoints.go:83` — "A 400 ERR_DUPLICATE_VALUE on path \"name\" surfaces as a *APIError the enroll state machine reads via APIError.Has".

The enroll state machine (`internal/enroll/state.go`) never calls `APIError.Has`; detection is purely list-and-match (`state.go:69`). `Has` is exercised only by `internal/api/client_test.go:133` and `internal/api/endpoints_test.go:105`. The comments mislead a future maintainer into thinking the create path already recognizes duplicates.

### 2. TOCTOU gap on Create (behavior — confirm desired)

Between `ListHosts` (`state.go:69`) and `CreateHostAndEnrollmentCode` (`state.go:90`), a concurrent enroll of the same hostname (two hosts sharing `DN_HOSTNAME`, or a skipped/failed pre-check) can create the record first. `Create` then returns `400 ERR_DUPLICATE_VALUE`/`name`, which surfaces as `"creating host record: …"` — NOT the actionable orphan guidance ("re-run with --force"). The design's "create-and-detect remains a fallback" (§2.4) is documented but unwired. Low likelihood, but the error is unhelpful when it happens.

## Fix

Wire the existing structured-error seam into the create branch:

```go
hc, err := deps.API.CreateHostAndEnrollmentCode(ctx, req)
if err != nil {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Has("ERR_DUPLICATE_VALUE", "name") {
		// A record appeared after the pre-check (TOCTOU, or pre-check skipped).
		// Surface the §2.4 orphan guidance instead of a generic create error.
		return output.Result{}, fmt.Errorf("remote host record %q already exists but no local config is present (orphaned enrollment); re-run with --force to delete the stale record and enroll afresh", req.Name)
	}
	return output.Result{}, fmt.Errorf("creating host record: %w", err)
}
```

And correct the two doc comments to describe the actual mechanism: list-and-match is the primary detection; the Create-time `ERR_DUPLICATE_VALUE` is the backstop for the TOCTOU/skip case.

Note on `--force`: under `--force` the pre-check already deletes any found record, so a Create-time duplicate means a record was recreated in the race window — the same guidance (retry) is the safe response; do not auto-delete-and-retry in a loop here.

## Independence

This is independent of dt-iwl2 (the `findRemoteRecord` lookup extraction): the duplicate handling lives in the Create branch, not in the lookup helper. Either can land first. dt-iwl2 left the create-path behavior untouched precisely so this gap is owned here.

## Tests

- `internal/enroll/` — add a state-machine test: `Create` returns an `*api.APIError` carrying `ERR_DUPLICATE_VALUE`/`name` → `Enroll` returns the orphan-guidance error (assert via `errors.Is`/message), not a bare create error.
- No api-package test change needed — `Has` is already covered there.

## Acceptance criteria

- `internal/api/client.go:41` and `internal/api/endpoints.go:83` describe list-and-match as the actual primary detection (no false claim that enroll reads via `Has`).
- A Create-time `400 ERR_DUPLICATE_VALUE`/`name` from `CreateHostAndEnrollmentCode` produces the §2.4 orphan-guidance error, not a generic "creating host record" error.
- Other create errors are unaffected (still wrapped as "creating host record: …").
- A state-machine test covers the Create-time-duplicate path.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 5), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.4 (list-and-match vs create-and-detect fallback).
- Sibling: dt-iwl2 (the lookup extraction that surfaced this).


## Notes

**2026-06-07T21:15:20Z**

Sequencing: implement LAST of the three (depends on dt-iwl2, transitively dt-nutn + dt-jbme). The Create-branch duplicate guard uses in.Request.Name (exists only after dt-nutn) and sits on the extracted-lookup structure (dt-iwl2). Chained after dt-iwl2 to avoid a same-file merge in enroll/state.go, though the two edits are in different branches.
