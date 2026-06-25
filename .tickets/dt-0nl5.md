---
id: dt-0nl5
status: closed
deps: [dt-mhir]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-3gvq
tags: [unenroll, yaml]
---
# Read host_id from dnclient.yml

Read the remote host identifier from the local `dnclient` config by parsing `/etc/defined/<network>/dnclient.yml` as YAML and reading its `host_id` field (Q28 → A). Replaces the upstream `grep | awk` approach. If no local config exists, fail clearly ("host does not appear to be enrolled"); if the field is absent or the file is malformed, fail clearly.

## Public interface

```go
// internal/dnclient
func ReadHostID(configRoot, networkName string) (string, error)
//   parses <configRoot>/<network>/dnclient.yml (gopkg.in/yaml.v3), returns host_id
//   missing file        -> ErrNotEnrolled (clear "not enrolled" message)
//   missing/empty field -> clear malformed error
```

Inject `configRoot` (default `/etc/defined`) so tests use a temp dir. Dependency: `gopkg.in/yaml.v3`. Decode only the field needed (`host_id`), not the whole schema.

## Behaviors (TDD order)

1. **Valid file → host_id** — well-formed `dnclient.yml` with `host_id: host-…` → that id.
2. **Missing file → not-enrolled error** — no file → sentinel/clear error indicating the host isn't enrolled (consumed by [UNE.delete](dt-2t72.md)).
3. **Missing `host_id` field → malformed error** — file present, field absent → clear error.
4. **Malformed YAML → clear error** — invalid YAML → error, no panic.

## Test strategy

`t.TempDir()` as `configRoot`; write fixture `dnclient.yml` variants. Assert returned id / typed error. No real `/etc`.

## Acceptance

- host_id read from the YAML; missing config → clear not-enrolled error; missing field/malformed → clear errors. No `grep|awk`.

## References

- Design: [Req 4](../docs/dn-tool-design.md#4-host-unenrollment), [§2.6 Host ID retrieval](../docs/dn-tool-design.md#26-host-id-retrieval).

Parent epic: [dt-3gvq](dt-3gvq.md).

## Notes

**2026-06-06T18:57:12Z**

Implemented internal/dnclient.ReadHostID(configRoot, networkName) parsing <configRoot>/<network>/dnclient.yml with gopkg.in/yaml.v3 (now a direct dep, v3.0.1). Decodes only host_id via a small anonymous struct, not the full dnclient schema. Errors: missing file -> sentinel ErrNotEnrolled (var, wrapped with path via %w so callers use errors.Is); missing/empty host_id field -> clear 'missing host_id field' error; malformed YAML -> wrapped 'parsing <path>' error. All non-not-enrolled cases tested to NOT match ErrNotEnrolled so dt-2t72's branch stays correct. TDD: 4 behaviors, t.TempDir fixtures, no real /etc. make build green (vet/lint 0 issues/tests). Consumed by dt-2t72 (unenroll delete).
