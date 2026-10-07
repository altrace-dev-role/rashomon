---
name: rashomon-report
description: 'Render the rashomon report: declarations, executions, per-transcript accounting and coverage for recorded Claude Code sessions. Use when the user says "rashomon report", "show what the session did", "session recording report", or asks what rashomon captured.'
---

Render what the recorder captured. This command reads; it writes nothing and
creates no store.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `command -v rashomon`. If no binary exists there is nothing to
   report — say so and point at `/rashomon`; never build or install from a
   read-only command.

2. Pick flags from what the user asked for, passing through any they named:
   - default: `$RASHOMON report` — text for a terminal
   - a specific session: `--session <id>`
   - machine-readable: `--json`
   - to share outside the team: `--redact` (hostnames become keyed digests
     and the agent's prose summary is dropped)
   - which prompt produced which calls: `--chain` (lists each call under its
     prompt, with the hosts the call named; JSON always carries this)
   - every call in order: `--timeline` (lists every call, main agent and
     subagents, in the order they were recorded, by seq; the `#N` seqs the
     end-of-turn line and the `test runs` block cite are its rows; JSON
     always carries this)

3. Show the output. If it is long, show the coverage block and the
   per-transcript accounting in full and summarize the id lists; never
   summarize a number into a different claim. When reading it for the user,
   keep the report's own distinctions:
   - `without execution` is not a list of denials — it holds denied, failed,
     and unrecorded calls together, each with its permission mode.
   - `unknown` and `not read` are never `0`.
   - `masked exit status`, and a test run `with exit status masked`, is
     neither a failure nor a pass: a pipe or a later command set the call's
     exit status, so the runner's own result was never recorded.
   - `coverage: unverified` with `probe_absent` is expected for any session
     the recorder joined mid-way (including the session that ran `watch`).

If the report says "no sessions recorded", say so and point the user at
`/rashomon` to start recording — do not run `watch` yourself from here.
