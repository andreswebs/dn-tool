---
id: dt-cmn7
status: closed
deps: [dt-koaf, dt-i0yx, dt-3gvq]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 1
assignee: Andre Silva
tags: [run, lifecycle, container, signals]
---

# Container lifecycle command (run)

Requirement 5 / run command. Single command for the full container lifecycle: install -> enroll -> exec dnclient run in the foreground; on SIGTERM/SIGINT unenroll the host then exit; propagate the daemon's termination outcome to dn-tool's exit status.

## Design

Composes install + enroll + unenroll under a signal handler suited to the container process model. dnclient run is the foreground process; its exit status maps to dn-tool's.

## Acceptance Criteria

run installs, enrolls, then runs the daemon in foreground; termination signal triggers unenroll then exit; daemon exit outcome propagated to dn-tool exit status.

## Notes for a fresh agent

- `run` is the **container / foreground counterpart** to the three systemd units in §2.7 — it must not assume a systemd context (no `systemctl`, no `Type=notify` semantics). It is the path for containers and pipelines.
- Because there is no systemd here, the module's reboot-vs-poweroff and `DN_SKIP_UNENROLL` policy (§2.7) does **not** apply: on `SIGTERM`/`SIGINT` `run` always unenrolls, then exits. (If a skip-on-stop knob is ever wanted for containers, it would be a separate decision — flag it, don't silently add it.)
- It composes the same building blocks as the standalone commands: [install (dt-koaf)](dt-koaf.md) → [enroll (dt-i0yx)](dt-i0yx.md) → run daemon → [unenroll (dt-3gvq)](dt-3gvq.md). Reuse them; don't fork the logic.
- `dnclient run` is the foreground child: forward signals to it, wait, and map its exit status onto dn-tool's (per [dt-zwgc](dt-zwgc.md)). Make sure unenroll's bounded deadline fits inside the container's grace period.

## References

- Design: [Req 5 Container lifecycle command](../docs/dn-tool-design.md#5-container-lifecycle-command), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.7 NixOS module shape](../docs/dn-tool-design.md#27-nixos-module-shape-servicesdnclient) (the systemd path it parallels), [§2.12 step 6](../docs/dn-tool-design.md#212-build--migration-order).

**2026-06-06T22:19:37Z**

Epic complete: all three children closed (dt-flal compose lifecycle, dt-n5p5 signal->unenroll, dt-r2ks propagate daemon exit status). internal/run/run.go composes install->enroll->daemon->unenroll-on-signal; daemon exit status mapped via daemonExitCode (see learnings, dt-r2ks). 'run' command wired in cmd/dn-tool/main.go. make build green. Closing the tracker; no remaining work.
