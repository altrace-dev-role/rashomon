---
name: stop
description: 'Stop recording: remove the rashomon hook entries, keeping the store and every completed run. Use when the user says "stop rashomon", "rashomon detach", "turn the recorder off", "stop recording my session".'
---

1. The binary is `${CLAUDE_PLUGIN_ROOT}/bin/rashomon` — it ships with this
   plugin, so there is nothing to search for. Set `$RASHOMON` to that path.

2. If recording is coming from THIS plugin rather than from a settings
   install (`$RASHOMON status` says which), the way to stop it is to disable
   the plugin — `detach` only removes settings.json entries and leaves this
   plugin's alone, on purpose: a plugin's entries are not detach's to remove.
   Tell the user to disable the plugin instead of running `detach` in that
   case.

3. Otherwise, run `$RASHOMON detach`. It removes only this install's eight
   settings entries and leaves every other entry in `~/.claude/settings.json`
   with its value as found (indentation may change). The store and its history survive — completed
   runs still render their true coverage afterwards.

4. If `detach` refuses because the store is gone or the id is unknown, use
   the forms it names rather than editing the file:
   - `$RASHOMON detach --install <id>` with the id printed at install time
   - `$RASHOMON detach --all` when the store (and the id with it) is gone

5. Confirm with `$RASHOMON status`. After a plain `detach` the eight settings
   entries read `absent`; after `detach --all` with the store deleted they
   read `unknown` — with no install id nothing can be called ours, so
   `unknown` is the expected confirmation there, not a failure.
