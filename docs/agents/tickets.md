# Ticket workflow

Work is tracked as tickets managed with the `tk` CLI. Files live in `.tickets/`
as markdown (frontmatter + body). The current breakdown is one epic per design
requirement; tasks are added under epics as work begins.

```bash
tk ls                    # List all tickets
tk ready                 # List tickets with all deps resolved (ready to start)
tk blocked               # List tickets blocked by unresolved deps
tk show <id>             # Show full ticket details
tk dep tree <id>         # Show dependency tree
tk start <id>            # Mark as in_progress
tk close <id>            # Mark as closed
tk add-note <id> "..."   # Append a note
```

IDs are auto-generated with a `dt-` prefix and accept partial matching. Use
`tk add-note` to record scope changes and traceability (e.g. which upstream
finding a change addresses) so the rationale survives outside the diff.
