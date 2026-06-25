---
id: dt-iwl2
status: closed
deps: [dt-jbme, dt-nutn]
links: [dt-nutn, dt-41ww, dt-9lem, dt-jbme]
created: 2026-06-07T20:13:03Z
type: chore
priority: 3
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, refactor, architecture]
---
# Extract remote-record lookup behind a findRemoteRecord seam in enroll

Untangle the enroll create path: extract the no-name-filter list-and-match lookup into an internal findRemoteRecord helper so Enroll reads the §2.4 'remote present?' truth source as one call, while the orphan/force decision (rows 3-4) stays in the state machine. Internal seam, no interface or behavior change. Surfaced by an architecture review (deepening candidate 5).

## Problem

The create path in `Enroll` (`internal/enroll/state.go:66-88`) tangles three concerns inline:

1. **Lookup** — `ListHosts` + client-side name match (the no-name-filter workaround, reference §4.2).
2. **Orphan decision** — found and `!Force` → fail with guidance (§2.4 row 3).
3. **Force recovery** — found and `Force` → `DeleteHost` by id (§2.4 row 4).

The "API has no name filter" workaround is a self-contained concept mixed into the state-machine body. Deletion test: remove the helper and the list-and-match loop reappears inline, re-tangled with policy.

## Decisions (scoped via architecture-review interview)

1. **Internal function extraction, not a port.** One implementation (list-and-match). Per seam discipline, a strategy interface with a second create-and-detect adapter is NOT justified now: the design chose list-and-match (§2.4), create-and-detect can't return an id (so `--force` delete can't work behind it), and a no-`hosts:list`-scope deployment is not a current need. Revisit only if that deployment becomes real.
2. **Orphan/force decision stays in Enroll.** The seam does lookup only; rows 3-4 remain visible in the state machine, consistent with dt-nutn's "keep the state machine whole".

## Design

Add an unexported helper in `internal/enroll` (new `lookup.go` or alongside `state.go`):

```go
// findRemoteRecord answers the §2.4 "remote present?" truth source, hiding the
// no-name-filter list-and-match workaround (reference §4.2). A nil host means
// absent. A list error leaves presence unknown, so the caller must abort rather
// than risk creating a duplicate.
func findRemoteRecord(ctx context.Context, api API, networkID, name string) (*api.Host, error) {
	hosts, err := api.ListHosts(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("listing hosts for network %s: %w", networkID, err)
	}
	for i := range hosts {
		if hosts[i].Name == name {
			return &hosts[i], nil
		}
	}
	return nil, nil
}
```

`Enroll` create path becomes (orphan/force decision unchanged, just reads `existing`):

```go
existing, err := findRemoteRecord(ctx, deps.API, networkID, name)
if err != nil {
	return output.Result{}, err
}
if existing != nil {
	if !deps.Force {
		return output.Result{}, fmt.Errorf("remote host record %q (id %s) already exists but no local config is present (orphaned enrollment); re-run with --force to delete the stale record and enroll afresh", existing.Name, existing.ID)
	}
	if err := deps.API.DeleteHost(ctx, existing.ID); err != nil {
		return output.Result{}, fmt.Errorf("deleting stale host record %s: %w", existing.ID, err)
	}
}
```

## Where

- `internal/enroll/state.go` (or a new `internal/enroll/lookup.go`) — add `findRemoteRecord`; `Enroll` calls it in place of the inline `ListHosts` loop.

## Compose with dt-nutn

dt-nutn (candidate 1) edits the same orphan-match region: it changes the match source from `cfg.Hostname`/`cfg.NetworkID` to `in.Request.Name`/`in.Request.NetworkID`. The two compose cleanly — `findRemoteRecord` takes `networkID, name` params, so the caller passes whichever source applies. Whichever ticket lands first, the other rebases its call site. No conflict in intent.

## Tests (replace, don't layer)

- `internal/enroll/orphan_test.go` — tests through `Enroll`; survives unchanged (Enroll's interface is stable). Must not churn.
- Optional focused `findRemoteRecord` tests for match edge cases (no match, single match, multiple hosts in the network) — additions, not a layer duplicating the orphan tests.

## Acceptance criteria

- `findRemoteRecord` exists in `internal/enroll`; `Enroll` calls it instead of the inline `ListHosts` + match loop.
- The orphan/force decision (rows 3-4) remains in `Enroll`, not behind the seam.
- `Enroll`'s signature is unchanged; no behavior change — `orphan_test.go` passes unchanged.
- A `ListHosts` error still aborts enrollment (presence-unknown → no create).
- No strategy interface / second adapter is introduced.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 5), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.4 (enrollment state machine, list-and-match vs create-and-detect), reference §4.2 (host-list endpoint has no name filter).
- Sibling: dt-nutn (candidate 1, same `Enroll` body). dt-41ww (the Create-time duplicate gap this extraction surfaced).


## Notes

**2026-06-07T21:15:20Z**

Sequencing: implement SECOND (depends on dt-nutn, dt-jbme). Write findRemoteRecord's call site against the final in.Request.NetworkID / in.Request.Name form (post dt-nutn) so no rebase. Then dt-41ww lands on top.
