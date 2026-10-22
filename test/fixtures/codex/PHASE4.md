# Phase 4: live test with the Codex CLI

Run on 2026-10-22 against Codex CLI 0.163.0, model `gpt-6-luna`, with the
branch built from `ba32d25` to a separate path. The test store was separate
too (`RASHOMON_HOME`). The September install (binary, `~/.claude/settings.json`,
store) was not touched: checksums match before and after, and its store holds
no Codex session.

## How it was run

`watch --harness codex` installed three entries into `~/.codex/hooks.json`, and
the user trusted them in `/hooks`. Four prompts were sent, one per turn, in a
throwaway folder:

| turn | prompt | what Codex did |
| --- | --- | --- |
| 1 | add `multiply(a, b)` to `calc.py` with apply_patch | `pwd && rg …`, `cat calc.py && find …`, `apply_patch`; the file was changed |
| 2 | run `python3 calc.py` | printed `5`, exit 0 |
| 3 | run `python3 missing.py` | "can't open file", exit 2 |
| 4 | list the files with one subagent | `spawn_agent`, `wait_agent`; the subagent ran `rg --files` |

The ground truth is Codex's own rollout files in `~/.codex/sessions`.

**Not the default setup.** The plain `codex` TUI is a client of a shared
app-server daemon, and the hooks run in the daemon (finding 3). The daemon had
been started without `RASHOMON_HOME`, so the test ran a private app-server with
it set and attached the TUI with `codex --remote unix://…`. Turn 1 ran with
that server started from the home directory, so the session's cwd was `~`, not
the folder: the agent's first call ran `rg --files` over the home directory
(read-only sandbox). Turns 2–4 ran in a second session with the server started
from the folder. The records are correct for both sessions; the cwd is what
Codex sent.

## Checks

- **Every call was recorded.** 3 calls in session 1, 5 in session 2 (one of
  them the subagent's): each has a declaration and an execution, no gaps, no
  dropped or unterminated records.
- **The failed call is unobserved, never ok.** `python3 missing.py` has
  `outcome: ""` and no exit code, the same as the passing `python3 calc.py`.
  The report lists both under "outcome unobserved" and says "failed calls: 0".
  `--chain` shows each as "outcome unobserved".
- **The subagent is counted in its parent's turn.** Codex gave the subagent
  its own `turn_id` (`…bb57`). Its `rg` declaration carries the parent's
  (`…f78566`) and its `agent_id`. The digest for that turn reports
  `subagents: {declarations: 1, executions: 1}`, and the report says
  "subagents: 1". `--chain` says "chains: 4" for three turns, because the
  subagent's call is listed under its own rollout file, under the parent's prompt.
- **Coverage** is `unverified` with `hook_entry_unresolved` on every record,
  as designed, plus `run_not_closed`: `SessionEnd` never fired, as in the
  capture.

## The two open questions

**apply_patch fires both PreToolUse and PostToolUse.** It is declared as
`tool_name: "apply_patch"`, verb `write`, no file label, and its execution has
an empty outcome. The patch in the rollout names its file
(`*** Update File: <absolute path>`). Whether the hook's `tool_input` carries
the same text was not captured here, and a file label depends on it. The
declaration's `shape.may_write` is `false` although its verb is `write`.

**`/hooks` does not rewrite `hooks.json`.** Its bytes and mtime were the same
before and after trusting. Starting Codex did not change it either. Trust
went to `config.toml` (logged as a `config/batchWrite`), one entry per hook:

```
[hooks.state."<path to hooks.json>:pre_tool_use:0:0"]
trusted_hash = "sha256:…"
```

The key is the file, the event, and the entry's position in the file, not its
command line. Codex leaves these behind after detach (below).

## Findings

1. **The report's transcript check is wrong for Codex.** Both runs get
   `transcript_mismatch`, "ids in transcript: 0", and every call listed as
   "missing from transcript" and "declared without result". None of that is
   true: every call is in the rollout. The report reads the rollout as a Claude
   Code transcript. This was out of scope for phases 2–3, but the output asserts
   a disagreement that does not exist, and it cannot be suppressed without
   knowing the harness (review item 3).
2. **Ids join through Codex's completion events, not its tool calls.**
   *Corrected; this first said the ids do not join.* A shell or patch call's
   `tool_use_id` (`exec-<uuid>`) is not the `call_id` (`call_<hex>`) of the
   model's tool call in the rollout, because that call is the model's script,
   which may run several tools. It is, exactly, the `item.id` of the rollout's
   `event_msg` / `item_completed` entry for the command or file change; every
   one of the six `exec-` ids in these runs has one. The
   `spawn_agent`/`wait_agent` ids match the tool call's `call_id`.
3. **The hooks run in Codex's app-server daemon, not in the TUI.** The daemon
   is shared and long-lived, so any variable set when launching `codex`
   (`RASHOMON_HOME`, `XDG_STATE_HOME`) never reaches the hooks. A user who sets
   one gets their records in the default store. If the daemon lacks the
   variable and the default store has a different install id, the entries
   stand down and nothing is recorded. Neither `watch` nor `status` can say so.
   With `--remote`, the session cwd also comes from the server's working
   directory.
4. **SessionStart fires with the first turn, not when the TUI starts.** A
   trusted SessionStart did not run on launch; the probe record arrived with
   the first prompt.
5. **The exit code is in the rollout as a field, not in the hook.**
   *Corrected; this first said Codex's tool output text ends `exit_code=N`.*
   That text was printed by the model's own script
   (`` text(`exit_code=${r.exit_code}`) ``), so it appears only when a model
   chooses to print it. The structured value is in the `item_completed` entry
   for each command: `item.type` `CommandExecution`, `item.exit_code` (an
   integer) and `item.status` (`completed` or `failed`), keyed by the same
   `item.id` as the record. A `FileChange` item has a `status` and no
   `exit_code`. By that field, the first call of session 1 (`pwd && rg …`)
   also failed, with exit 2, on unreadable directories under `~`. The hook
   payload has no exit code (all 7 captured PostToolUse payloads, and the
   payload schema in Codex 0.163.0 and 0.164.0-alpha.12), so outcomes stay
   unobserved in the records.
6. **Detach leaves `{}` behind.** Where no `hooks.json` existed before
   `watch`, `detach --harness codex` leaves a 3-byte file, `{}` plus a newline,
   mode 0600. It is neither the absent file nor `{"hooks": {}}`. The three
   `trusted_hash` entries stay in `config.toml`.
7. **The `hook_entry_unresolved` explanation names Claude Code's sources.**
   The report says "the settings file or the plugin could not be read". For a
   Codex run, neither is read.

## Not tested

A second `watch` after a moved binary (re-trust); `async: true`; `hooks = false`
in `config.toml`; the default shared-daemon setup with the store variable set on
the daemon.

## Live check: the end-of-turn line (2026-10-22)

Run against the branch at `5bf981b`, built to the same separate path, with
the same separate store, the same private app-server started from the work
folder, and the TUI attached with `--remote`. `watch --harness codex`
installed four entries, Stop running `recap --harness codex` among them, and
the user trusted all four in `/hooks`. Two prompts, one turn each, in session
`01a16536-fcf0-7211-bbef-b0029df18c86`:

| turn | prompt | what happened | line |
| --- | --- | --- | --- |
| `…072d33` | run `python3 missing.py`, then reply only "Done" | exit 2 | shown |
| `…ea5fdc` | run `python3 calc.py`, then reply only "Done" | exit 0 | none |

**The line appeared for the failed turn and not for the clean one.** recap's
own bookkeeping agrees: turn `…072d33` is recorded as spoken, and `…ea5fdc`
as checked and silent. The user saw it as a one-line warning in the TUI. Its
exact place on screen is still to be confirmed.

**A turn's items are in the rollout before its Stop fires.** Measured two
ways:

- By outcome: with "Done" as the final message, the line can say
  `1 recorded failure` only if recap read the command's `item_completed`
  entry, failed with exit 2, from the rollout. The hook payload has no exit
  code.
- By time, from the rollout and the `hook/started` and `hook/completed`
  events Codex's app-server logged:

  | turn | command item written | Stop hook ran |
  | --- | --- | --- |
  | `…072d33` | 20:04:47.431 | 20:04:48.624 to 48.644 |
  | `…ea5fdc` | 20:05:04.988 | 20:05:05.950 to 05.978 |

  recap's evaluation of the second turn is stamped 20:05:05.971, inside its
  Stop window. Stop ran about a second after the turn's last command item,
  once the agent's reply was written. Both Stops ran in about 20 ms each.

**The line never reached the model.** The session's rollout holds every
message, tool call and tool output Codex sends the model. It contains none of
the line's text (`recorded failure`, `※`, `rashomon:`,
`rashomon report --session`, `systemMessage`): 0 matches for each. The word
"rashomon" appears only inside paths (`~/rashomon-phase4/work`). Turn 2 was
sent after the line appeared, and what Codex sent the model for it holds none
of the line either.

**Detach.** `detach --harness codex` removed the four entries and deleted
`~/.codex/hooks.json`, which watch had created, as H-148 requires. Codex
left four `trusted_hash` entries in `config.toml`. The three for the entries
unchanged since phase 4 carry the same hashes as on 2026-10-21: an identical
entry hashes the same.

The September install was not touched: binary and settings checksums
unchanged, and no live-check run in its store.
