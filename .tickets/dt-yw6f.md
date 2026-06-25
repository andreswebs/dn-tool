---
id: dt-yw6f
status: closed
deps: []
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-vsi6
tags: [scaffolding, docs]
---
# README Acknowledgement section

Write the repo `README.md` with an **Acknowledgement** section crediting the upstream `quickvm/defined-systemd-units` project as inspiration. `dn-tool` is a complete rewrite, not a derivative, so the project keeps `UNLICENSE` (public domain) and ships **no** upstream MIT license file (design §2.1).

## Scope

1. **Acknowledgement section** — short paragraph: this tool is inspired by and reimplements the behavior of the upstream bash `dnctl`; link the upstream and note the rewrite is original work.
2. **Project overview** — one-paragraph description (control-plane CLI wrapping the defined.net API + orchestrating `dnclient`), the command surface (link design §2.2), and the env-var configuration pointer (design §2.3).
3. **License note** — UNLICENSE only; explicitly no upstream MIT carried over.

Follow the documentation skill (README = how-to/reference oriented, not a tutorial).

## Acceptance

- `README.md` has an Acknowledgement section and a UNLICENSE note.
- `make dist` packages `UNLICENSE` + `README.md` (already wired in the Makefile).
- No `LICENSE.upstream`/MIT file present.

## References

- Design: [§2.1 Repository & distribution](../docs/dn-tool-design.md#21-repository-and-distribution), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface).
- Upstream being acknowledged: [defined-systemd-units source](../docs/research/defined-systemd-units.yek.txt), [analysis](../docs/research/quickvm-defined-systemd-units-analysis.md).

Parent epic: [dt-vsi6](dt-vsi6.md).

## Notes

**2026-06-06T19:47:54Z**

Rewrote README.md: project overview (control-plane CLI wrapping defined.net API + orchestrating dnclient), command-surface table (install/enroll/unenroll/run/write-config) + global flags linking design §2.2, configuration pointer to §2.3 (kept the existing precedence section from dt-9x3y), Acknowledgement crediting upstream quickvm/defined-systemd-units as inspiration for an original rewrite (links the analysis), and a License note stating UNLICENSE only with no upstream MIT carried over. Confirmed no LICENSE.upstream/MIT file exists (only UNLICENSE); make dist already packages UNLICENSE+README. make build green. (Minor: IDE flags MD060 table-alignment warnings on the command table — cosmetic only, markdown isn't in the build gate.)
