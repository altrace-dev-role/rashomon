---
name: rashomon-status
description: 'Say what rashomon has installed here: hook entries, store location, install ids, whether hooks are disabled. Use when the user says "rashomon status", "is rashomon running", "is the recorder on".'
---

1. Resolve the binary as the `rashomon` skill does (PATH, then
   `~/.local/bin/rashomon`, then build/install).

2. Run `$RASHOMON status` and show the output as-is. It reads; it writes
   nothing and creates no store — a machine where nothing is installed
   answers "nothing is installed here" without minting an install identity.

3. If entries read `absent` and the user wanted recording, point them at
   `/rashomon`. If another install's entries share the file, relay that
   line — those entries fire here and record nothing in this store.
