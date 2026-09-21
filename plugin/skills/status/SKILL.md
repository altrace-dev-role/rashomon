---
name: status
description: 'Say what rashomon has installed here: hook entries, store location, install ids, whether hooks are disabled. Use when the user says "rashomon status", "is rashomon running", "is the recorder on".'
---

1. The binary is `${CLAUDE_PLUGIN_ROOT}/bin/rashomon` — it ships with this
   plugin, so there is nothing to search for. Set `$RASHOMON` to that path.

2. Run `$RASHOMON status` and show the output as-is. It reads; it writes
   nothing and creates no store. It reports two origins separately: the
   settings entries `watch` installs, and this plugin's own entries. Each
   reads `present`, `absent`, `unreadable`, or `unknown` — and on a machine
   with no store, every settings-origin event reads `unknown`, because with
   no install id nothing in settings.json can be called ours. That is the
   correct answer, not a failure.

3. If both origins read live, relay the `overlap` line: it names
   `rashomon detach` as the resolution, and it is the settings entries that
   command removes — this plugin's entries are not detach's to touch.

4. If the plugin reads `absent` or `disabled` and the user wanted recording,
   tell them to enable the plugin. If another install's entries share the
   settings file, relay that line — those entries fire here and record
   nothing in this store.
