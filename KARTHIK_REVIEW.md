# Codex support, phases 2 and 3: items for Karthik's review

Branch `codex-adapter`, local only. Phase 2 makes the recorder take Codex
hook payloads under `--harness codex`. Phase 3 installs for Codex; its
decisions are at the end, and it changes no record format. Without the flag nothing changes:
H-144 pins every byte the Claude Code paths produced before the work began.

Four items touch the record format or `docs/store-schema.json`. Each is in
the branch as written below, and each is **provisional until you sign off**.
For each: what changed, why, and what the alternative would cost.

## 1. An execution's `outcome` may be empty in a v4 record

**What.** Under Codex every execution is written with `"outcome": ""`, and
`exit_code` and `is_interrupt` null. `docs/store-schema.json` adds `""` to
the `outcome` enum and says when it appears.

**Why.** Codex fires a plain `PostToolUse` for a call that failed. In the
capture (`test/fixtures/codex`), `python3 missing.py` failed and its payload
carried no failure event, no exit code and no interrupt flag: it reads
exactly like a success. Recording `ok` is the error this record exists to
prevent.

**Why empty, and not a new word like `"unobserved"`.** Every reader already
treats an empty outcome as unobserved, because a v1 record decodes to one:
`report/account.go` counts it in `outcome_unobserved`, `report/chains.go`
renders "outcome unobserved", the timeline files it under unknown, and test
bending and `spend` leave it alone. No reader changes. A new word would reach
the `default` branch of each of those switches, so each would need a change,
and the account count would silently skip it until then. The writer has
never produced `""` before, so the schema change is real, but the reading
side is not new.

**What it costs.** `""` in a v4 record now means two things: a v1 record
read back, or a Codex call. Neither is a success, and the report says the
same thing for both. If the two ever need telling apart, that is item 3.

## 2. `prompt_id` holds a Codex `turn_id`

**What.** Under Codex a declaration's `prompt_id` is the payload's `turn_id`.
A subagent's declaration takes the `prompt_id` of the main agent's most recent
declaration in the run instead of its own `turn_id`. The key and its type are
unchanged; the schema's description of `prompt_id` says so.

**Why.** `turn_id` names a turn the way `prompt_id` does: the same id runs
from the `UserPromptSubmit` that opens a turn, through every Pre/Post call in
it, to the `Stop` that closes it. All three readers of `prompt_id` (the turn
digest, recap's once-per-turn key, the report's chains) group by it as a turn.

The subagent rule is the part to look at. Codex gives a subagent a `turn_id`
of its own (`…a3d4…` in the capture, against the parent's `…97f9…`), and
nothing a subagent sends names the turn that spawned it. The digest groups a
turn by `prompt_id` alone on the rule that a subagent's calls carry the
parent's (H-89). Copied across, a Codex subagent's calls would form a turn of
their own, and the parent's turn would say "subagents: 0", a false zero.

**How.** `store.LatestMainPromptID` reads the last 256 KiB of the run's
`records.ndjson` without a lock and returns the `prompt_id` of the last
declaration with no `agent_id`. With none in the window, the subagent keeps
its own `turn_id`.

**Where it is wrong.** For a subagent still calling tools after the user has
started a new turn: its later calls land in the new turn. The alternatives:

- Pin each subagent to its turn at `SubagentStart`, in a small per-run file.
  That is exact, but it adds an entry point and a file to the store layout.
- Copy `turn_id` as given, and accept the false zero.

## 3. Nothing records which harness wrote a run

**What.** No record carries the harness. A reader cannot tell a Codex run from
a Claude Code one, except by the shape of `transcript_path`.

**Why it is left out.** Recording it means a new key on coverage records, or
on every call record: a schema change and, by the rules in `record.go`, a
version decision. Phase 2 needs neither, because every Codex-specific reading
(an empty outcome, an unresolved hook entry) already renders correctly
without knowing the harness.

**When it would be needed.** When the report reads Codex transcripts (phase
3), or if item 1's double meaning of `""` ever has to be told apart.

## 4. Coverage under Codex reuses `hook_entry_unresolved`

**What.** Under Codex, `Resolve` reads none of Claude Code's configuration.
The hook entry is `unknown`, so every Codex run is `unverified` with the
existing reason `hook_entry_unresolved`. No new reason code, so no schema
change. It is listed here because it is a choice about the coverage
vocabulary.

**Why.** Reading Claude Code's settings for a Codex run is wrong in the
dangerous direction. On a machine with both installed, our Claude Code entry
reads as present, and the Codex run would be `verified` on the strength of a
file Codex never opens (H-145 installs both and checks this).

Phase 3 changes the reason "unresolved" is true, not its truth. `watch
--harness codex` now writes Codex's `hooks.json`, but coverage still does not
read it. Our entry being in the file does not mean Codex runs it. Codex skips
an entry until the user trusts it in `/hooks`, records that trust against a
hash of the entry, and the hash recipe is Codex's own. Trust is also per
entry: a PreToolUse that fires shows nothing about whether PostToolUse is
trusted. Until trust can be read, "unresolved" is the honest word.

**The alternative.** A new reason, say `harness_config_unread`, would name
the cause more precisely, at the cost of a schema enum entry and
`store.Reasons()`. A further one, once trust can be read: a `hook_entry`
value such as `present_codex`, set only when our entry is in `hooks.json`
*and* trusted, which would let a Codex run read `verified`.

## Decided in phase 2, not format changes

These were decided with the user rather than left to you; they are listed so
nothing is a surprise.

- **recap prints nothing under Codex.** How Codex reads a Stop hook's stdout
  is unmeasured, and an output it took for an instruction would change what
  the agent does.
- **An unknown `--harness` value stands down.** It writes nothing, prints one
  fixed stderr line and exits 0.
- **`apply_patch` is classed as a write but gets no file label** until a
  capture shows its input.
- **The three Codex tool names are classed without a harness check.** No
  Claude Code tool carries them.

## Not in phase 2 or 3

- The report reading Codex transcripts. The final-message quote and the H-10
  accounting read Claude Code's transcript format.
- `SessionEnd`: Codex did not fire it in the capture, so the run-time
  unterminated-entry check never runs for a Codex session.

## Phase 3: installing for Codex

`watch --harness codex` writes `PreToolUse` -> `hook`, `PostToolUse` ->
`post` and `SessionStart` -> `probe start`, each with `--harness codex`, into
`$CODEX_HOME/hooks.json` (or `~/.codex/hooks.json`). The marker, install id
and tamper rules are the Claude Code installer's. `detach --harness codex`
removes exactly those. No record, schema or reason code changes.

### Decisions made with the user, each reversible

- **A. No recap entries under Codex.** `Stop` and `UserPromptSubmit` are not
  installed. recap returns before doing anything under Codex, so the entries
  would cost two trust prompts and a process per turn for nothing. *To
  reverse:* add both to `install.CodexEvents` with `recap` and `recap
  pending`. They would need trusting again once recap speaks.
- **B. A missing Codex directory is refused, not created.** `watch` names the
  directory and `CODEX_HOME`. *To reverse:* create it. The cost is entries
  that never run on a machine without Codex.
- **C. `[features] hooks = false` is not checked.** This build has no TOML
  reader. `watch` and `status` both say the setting was not checked. *To
  reverse:* a TOML dependency, or a narrow reader for one key in two files.
- **D. `status` shows `codex:` only where there is Codex**, meaning the
  directory exists. A rashomon Codex entry lives inside it, so that covers an
  install too. A Claude Code-only
  user's status is unchanged. *To reverse:* always print it.

### Left to you

- **Coverage stays unverifiable for Codex** until trust can be read (item 4,
  above). Or add `present_codex`.
- **One install id for both harnesses.** The store is shared, so the same id
  sits in `settings.json` and `hooks.json`. Each file's detach touches only
  its own entries.
- **A moved binary means trusting again.** `watch` refreshes the path, which
  changes the hash, and says so. The alternative is refusing to rewrite, and
  telling the user to.
- **Tamper rules unchanged.** `async` or `commandWindows` added to our Codex
  entry is not treated as an edit, as Claude Code's rules ignore keys they do
  not install. `async: true` would run the recorder in the background. Whether
  that matters is unmeasured.
- **Open, for phase 4:** whether `/hooks` rewrites `hooks.json` (see
  `test/fixtures/codex/README.md`).
