---
name: rashomon-stop
description: 'Stop recording: remove the rashomon hook entries, keeping the store and every completed run. Use when the user says "stop rashomon", "rashomon detach", "turn the recorder off", "stop recording my session".'
---

1. Resolve the binary: `~/.local/bin/rashomon`, then `~/go/bin/rashomon`,
   then `command -v rashomon`. If no binary exists, nothing of ours can be
   watching — say so; never build or install from here.

2. The user runs `detach`, not you: it refuses an agent session, and a `!`
   line typed into Claude Code counts as one. Tell the user to run this in a
   terminal outside Claude Code, with `$RASHOMON` written out as the full
   path you resolved:

       $RASHOMON detach

   It removes only this install's eight entries (recording plus the
   exception-only recap) and leaves every other entry in
   `~/.claude/settings.json` with its value as found (indentation may
   change). The store and its history survive — completed runs still render
   their true coverage afterwards.

3. If `detach` refuses there because the store is gone or the id is
   unknown, the user runs the form it names, in that same terminal, rather
   than editing the file:
   - `$RASHOMON detach --install <id>` with the id printed at install time
   - `$RASHOMON detach --all` when the store (and the id with it) is gone

4. Once the user says it ran, confirm with `$RASHOMON status`, which only
   reads and so runs here. After a plain `detach` the eight entries
   read `absent`; after `detach --all` with the store deleted they read
   `unknown` — with no install id nothing can be called ours, so `unknown`
   is the expected confirmation there, not a failure.
