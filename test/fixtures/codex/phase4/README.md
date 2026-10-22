# Codex rollout lines from the phase 4 live test

Lines from the three rollout files Codex CLI 0.163.0 wrote during the phase 4
live test on 2026-10-22 (`../PHASE4.md`), kept as input for the report's Codex
outcome reader (H-149).

| file | session |
| --- | --- |
| `rollout-01a16455-….jsonl` | session 1: `pwd && rg …` (exit 2), `cat …` (exit 0), the `apply_patch` edit |
| `rollout-01a1645b-….jsonl` | session 2: `python3 calc.py` (exit 0), `python3 missing.py` (exit 2), the subagent spawn and wait |
| `rollout-01a1645c-….jsonl` | the subagent: `rg --files` (exit 0) |

## What was kept

Only `event_msg` / `item_completed` lines whose item is a tool item:
`CommandExecution`, `FileChange`, `CollabAgentToolCall` and
`SubAgentActivity`. Messages, reasoning and every other line were dropped.

Within each kept line, the item's `type`, `id`, `exit_code` and `status` are
as Codex wrote them; they are the only item fields the reader takes. Every
other item field keeps its key with its value replaced: `"[scrubbed]"` for a
string, list or object, `0` for a number, `null` left as it was. The command,
its output, the working directory and the diff are gone. The line's thread,
turn and timestamps are kept, and the capturing user's home directory was
replaced with `/home/user`, though no kept value held it.

`secrets_test.go` beside the fixtures scans every file in this directory for
home directories, email addresses and credential shapes, and checks that
these lines keep nothing but the four fields.

## What they show

- Each tool call's `item.id` is exactly the `tool_use_id` Codex sent the
  hooks: all eight calls of the two sessions match.
- A command item carries an integer `exit_code` and a `status` of
  `completed` or `failed`. A file change carries a `status` and no exit code.
- The subagent spawn's id appears on a `SubAgentActivity` item, and so does
  `subagent-completed-…`, which is not a call. The wait's id appears on a
  `CollabAgentToolCall`.

The hook payloads of phase 4 were not captured. H-149 rebuilds them from the
phase 4 records and these rollouts: the ids, turns, tools and commands are
phase 4's, and the tool output is a placeholder the recorder never reads.
