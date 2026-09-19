---
name: rashomon-stop
description: 'Stop recording: remove the rashomon hook entries, keeping the store and every completed run. Use when the user says "stop rashomon", "rashomon detach", "turn the recorder off", "stop recording my session".'
---

1. Resolve the binary as the `rashomon` skill does (PATH, then
   `~/.local/bin/rashomon`, then build/install).

2. Run `$RASHOMON detach`. It removes only this install's five entries and
   leaves everything else in `~/.claude/settings.json` byte for byte as
   found. The store and its history survive — completed runs still render
   their true coverage afterwards.

3. If `detach` refuses because the store is gone or the id is unknown, use
   the forms it names rather than editing the file:
   - `$RASHOMON detach --install <id>` with the id printed at install time
   - `$RASHOMON detach --all` when the store (and the id with it) is gone

4. Confirm with `$RASHOMON status` and show the entries as absent.
