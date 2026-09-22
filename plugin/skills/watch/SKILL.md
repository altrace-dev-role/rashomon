---
name: watch
description: 'Start recording what this Claude Code session asks to run. Use when the user says "rashomon", "start rashomon", "claude rashomon", "watch this session", "record my session", asks to turn the recorder on, or wants an easier way to launch recorded sessions. For reports use /rashomon:report; to stop use /rashomon:stop; for install state use /rashomon:status.'
---

This plugin ships disabled: enabling it is the only thing that starts
recording, and installing it (adding the marketplace, or having it enabled by
policy) does not, by itself.

1. The binary is `${CLAUDE_PLUGIN_ROOT}/bin/rashomon` — it ships with this
   plugin, so there is nothing to resolve and nothing to build. Set
   `$RASHOMON` to that path.

2. If this skill is reachable at all, the plugin is enabled, and its five
   hook entries (PreToolUse, PostToolUse, PostToolUseFailure, SessionStart,
   SessionEnd) are already configured — enabling the plugin is what installs
   them, the same way `watch` installs the settings-file form. There is
   normally nothing further to run. Confirm with `$RASHOMON status`, which
   reports this plugin's entries and a settings install separately: if both
   read live, relay the `overlap` line and its `rashomon detach` resolution.

3. Do NOT run `$RASHOMON watch` from here as a matter of course: it writes
   the settings-file form of these same five entries, and it refuses when
   this plugin already provides them, naming the plugin and pointing at
   disabling it — because installing both would record every tool call
   twice, and nothing at the hook layer can undo that once both origins are
   live. If a user specifically wants the settings-file form instead of this
   plugin (for example, to keep recording after disabling the plugin), point
   them at `$RASHOMON watch` directly rather than running it for them, so
   they see the refusal and the reason if the plugin is still enabled.

4. Tell the user, briefly, and say the SCOPE first — it is the part they are
   actually agreeing to:
   - Recording is USER-WIDE AND OPEN-ENDED, not this session and not this
     project. Once enabled, every session from now on is recorded —
     including unrelated repositories and client work — until the plugin is
     disabled. Someone who is under an agreement not to instrument a
     particular codebase needs to know that before saying yes, not after.
   - Recording covers tool calls from this point on. A session the recorder
     joined mid-way reports its coverage as `unverified` (reason
     `probe_absent`) — that is honest accounting, not a failure. Sessions
     started after this plugin was enabled are eligible for verified
     coverage, because the probe is in place from their first moment; the
     report is the proof.
   - `/rashomon:report` renders what was recorded; `/rashomon:stop` removes
     a settings install (not this plugin's own entries — disable the plugin
     for that); `/rashomon:forget` erases records.

Do not edit `~/.claude/settings.json` by hand, and do not run `watch` or
`detach` as a substitute for enabling or disabling this plugin — the two
origins are independent, and mixing them up is exactly what the `overlap`
line in `status` exists to catch.
