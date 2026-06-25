---
id: dt-ccmn
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cq78
tags: [output, json]
---
# JSON result writer + Result type

Define the command `Result` type and a writer that emits exactly **one JSON object to stdout** per command (design §2.8). The `Changed` field is the signal `--assert-changed`/exit-2 reads ([EXIT.assert](dt-4h21.md)), so settle the schema here as the contract both this epic and the exit-code epic share.

## Public interface

```go
// internal/output
type Result struct {
    Action  string `json:"action"`            // "enroll" | "unenroll" | "install" | ...
    Changed bool   `json:"changed"`
    HostID  string `json:"hostId,omitempty"`
    Network string `json:"network,omitempty"`
    // extend per command as needed; keep additive
}
func WriteResult(w io.Writer, r Result) error  // w is stdout in production
```

Example: `{"action":"enroll","changed":true,"hostId":"host-…","network":"defined"}`.

## Behaviors (TDD order)

1. **Result marshals to the documented shape** — fields/JSON tags exactly as above.
2. **One object to the writer** — `WriteResult` writes a single JSON object (+newline) and nothing else.
3. **`omitempty` fields omitted** — a result without `HostID` has no `hostId` key.
4. **stdout/stderr separation** — result goes to the provided writer (stdout); logging (OUT.slog) is unaffected.

## Test strategy

`WriteResult(&buf, r)`; unmarshal `buf` and assert field values + absence of empty fields. Keep the type small and additive.

## Acceptance

- Exactly one JSON result object per command on stdout, matching the schema.
- `Changed` is populated by every command for the exit-code layer.

## References

- Design: [Req 7](../docs/dn-tool-design.md#7-output-and-observability), [§2.8 Output contract](../docs/dn-tool-design.md#28-output-contract).
- Consumed by: [EXIT.assert](dt-4h21.md) (`Changed` → exit 2).

Parent epic: [dt-cq78](dt-cq78.md).

## Notes

**2026-06-06T19:07:13Z**

Added WriteResult(w io.Writer, r Result) error to internal/output. Result struct already existed (pre-defined by dt-2t72 to bridge a dep-graph gap; left verbatim, NOT redefined, per that learning). Implementation is a one-liner: json.NewEncoder(w).Encode(r) — Encode writes exactly one JSON value + a single trailing newline, satisfying the 'one object to the writer' behavior for free. Tests (result_test.go, table of 3) cover: documented field/tag shape, single-object-+-one-newline (json.Decoder.More()==false), and omitempty (hostId/network absent when empty; action/changed always present). Writer is injected so stdout/stderr separation is structural — slog logging (dt-zxwf) is untouched. Changed is the exit-2 signal for dt-4h21. make build green.
