# rashomon

Arm the rashomon recorder from Cursor.

Be precise with the user about what this does and does not do: rashomon
records **Claude Code sessions only** ("Claude Code only: the hooks, the
store and the report describe Claude Code sessions and nothing else" —
README). Running this from Cursor installs the
recorder for the machine's *Claude Code* sessions. **This Cursor session is
not recorded**, and rashomon has no hook into Cursor's agent loop today.
Never imply otherwise.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `/opt/homebrew/bin/rashomon` or `/usr/local/bin/rashomon`, then
   `command -v rashomon` (absolute paths only). If none exists and Go is
   available and this is the rashomon repo, build it:
   `go build -o ~/.local/bin/rashomon ./cmd/rashomon` (never from a `go run`
   temp path). Otherwise point the user at the README's install options.
2. Confirm with the user that they want Claude Code sessions on this machine
   recorded, then run `rashomon watch`. Quote back the printed install id and
   the exact `detach --install <id>` undo line.
3. Confirm with `rashomon status` (all five entries `present`), and tell the
   user: recording starts with their next Claude Code session; `rashomon
   report` in any terminal renders what was captured.
