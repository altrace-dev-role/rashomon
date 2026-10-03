# rashomon report

Render what the rashomon recorder captured from this machine's **Claude Code**
sessions. Cursor's own sessions are not supported yet: its calls may appear
under a session named `unattributed`, and what was recorded for them can be
wrong. Say so if the user expects this session in the output.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `/opt/homebrew/bin/rashomon` or `/usr/local/bin/rashomon`, then
   `command -v rashomon` (absolute paths only). No binary means there is
   nothing to report — say so and stop; never build or install from a
   read-only command.
2. Run `rashomon report`, passing through any flags the user named:
   `--session <id>`, `--json`, `--redact` (share outside the team),
   `--chain` (each call under the prompt that produced it), `--timeline`
   (every call in seq order; the `#N` seqs the end-of-turn line and the
   `test runs` block cite are its rows).
3. Keep the report's own distinctions when summarizing: `without execution`
   is not a list of denials; `unknown` and `not read` are never `0`;
   `coverage: unverified` with `probe_absent` is expected for any session the
   recorder joined mid-way.
