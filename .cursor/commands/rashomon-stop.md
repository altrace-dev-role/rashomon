# rashomon stop

Remove the rashomon recorder's hook entries from `~/.claude/settings.json`,
keeping the store and every completed run. This stops recording for **Claude
Code** sessions on this machine, and for the Cursor calls that reached the
recorder through those same hooks.

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `/opt/homebrew/bin/rashomon` or `/usr/local/bin/rashomon`, then
   `command -v rashomon` (absolute paths only). No binary means nothing of
   rashomon's can be watching — say so and stop.
2. Run `rashomon detach`. It removes only this install's eight entries and
   leaves every other entry's value as found (indentation may change). If it refuses, use the forms it
   names: `rashomon detach --install <id>` (the id printed at install time)
   or `rashomon detach --all` when the store is gone.
3. Confirm with `rashomon status`: after a plain detach the eight entries read
   `absent`; after `detach --all` with the store deleted they read `unknown`,
   which is the expected confirmation there.
