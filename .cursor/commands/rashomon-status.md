# rashomon status

Say what the rashomon recorder has installed on this machine.

Scope first, stated to the user whenever it matters: rashomon records
**Claude Code sessions**. Cursor's own sessions are not supported yet, and no
command here changes that.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `/opt/homebrew/bin/rashomon` or `/usr/local/bin/rashomon`, then
   `command -v rashomon` (keep only an absolute path). If none exists, the
   answer is "rashomon is not installed on this machine" — say that and
   stop. Never build or install anything from this command.
2. Run `rashomon status` and show the output as-is. It reads; it writes
   nothing and creates no store. Entries read `present`, `absent`,
   `unreadable`, or `unknown` — on a machine with no store every event reads
   `unknown`, which is the correct answer, not a failure.
