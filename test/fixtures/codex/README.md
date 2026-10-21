# Codex hook payloads

Real hook payloads from the Codex CLI, kept as test input for Codex support.
`fixtures_test.go` beside this file decodes them with `internal/hook`'s
payload types.

## Where they came from

Captured on 2026-10-21 from the Codex CLI running model `gpt-6-luna`, with a
pass-through hook that wrote each payload it received to a file and changed
nothing. Five tasks, in two sessions:

| files | session | task |
| --- | --- | --- |
| 01–03 | first | `hi` |
| 04–10 | second | add a function to `hello.py` that adds two numbers |
| 11–14 | second | run `python3 hello.py` |
| 15–18 | second | run `python3 missing.py`, which failed |
| 19–28 | second | use a subagent to list the files in the folder |

The edit task made no edit. `hello.py` already had the function from an
earlier, unrecorded run, and the agent's only calls were `rg --files` and
`cat hello.py`.

Each file is named `<hook_event_name>-<NN>.json`. `NN` counts across every
event in the order the hooks fired, so sorting by number gives the whole
timeline. The capture's own filenames carried a nanosecond timestamp, and
that timestamp decided the order.

One edit was made to the files: the capturing user's home directory was
replaced with `/home/user` everywhere it appeared. Every other byte is as
captured, including the lack of a trailing newline.

These are payloads, not records. Unlike anything in the store, they contain
prompts, file contents and tool output.

## What they show

- **No exit code in any payload.** The failed `python3 missing.py` fired a
  plain `PostToolUse` (17), not a failure event, and nothing in it carries a
  status. The only place the code appears is the agent's own closing message
  (18).
- **`tool_response` is a plain string**, not an object: the command's output
  for `Bash`, and a string holding JSON for the subagent tools (21, 27).
- **`SessionEnd` never fired.** Neither session produced one.
- **The subagent spawn message is encrypted.** The `message` in the spawn
  call's `tool_input` (20, 21) is an opaque `gAAAAA…` token, not the task
  text.

## Open questions

- **Does `/hooks` rewrite `hooks.json`?** Unmeasured; to be measured in phase 4
  against a real Codex session. Codex's 0.163.0 binary carries the message
  "failed to serialize hooks.json", so it can write the file, and its trust
  state (`trusted_hash`) appears to live in `config.toml`. If `/hooks` does
  rewrite `hooks.json`, the tamper rules still recognise our entries by their
  command line, but an entry Codex marked disabled inside the file would
  still read as present in `status`.

- **Do `apply_patch` edits fire `PreToolUse` or `PostToolUse`?** Untested.
  These files hold no `apply_patch` call. An earlier conclusion that such an
  edit fires no hook came from misreading this capture, and should not be
  relied on.
