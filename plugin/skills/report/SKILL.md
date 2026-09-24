---
name: report
description: 'Render the rashomon report: declarations, executions, per-transcript accounting and coverage for recorded Claude Code sessions. Use when the user says "rashomon report", "show what the session did", "session recording report", or asks what rashomon captured.'
---

Render what the recorder captured. This command reads; it writes nothing and
creates no store.

1. The binary is `${CLAUDE_PLUGIN_ROOT}/bin/rashomon` — it ships with this
   plugin, so there is nothing to search for and nothing to build. Set
   `$RASHOMON` to that path.

2. Pick flags from what the user asked for, passing through any they named:
   - default: `$RASHOMON report` — text for a terminal
   - a specific session: `--session <id>`
   - machine-readable: `--json`
   - to share outside the team: `--redact` (hostnames become keyed digests
     and the agent's prose summary is dropped)
   - which prompt produced which calls: `--chain` (lists each call under its
     prompt, with the hosts the call named; JSON always carries this)

3. Show the output. If it is long, show the coverage block and the
   per-transcript accounting in full and summarize the id lists; never
   summarize a number into a different claim. When reading it for the user,
   keep the report's own distinctions:
   - `without execution` is not a list of denials — it holds denied, failed,
     and unrecorded calls together, each with its permission mode.
   - `unknown` and `not read` are never `0`.
   - `coverage: unverified` with `probe_absent` is expected for any session
     the recorder joined mid-way (including the session that enabled this
     plugin).

If the report says "no sessions recorded", say so and point the user at
`/rashomon:watch` to start recording — do not run `watch` yourself from here.
