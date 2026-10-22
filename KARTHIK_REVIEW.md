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

*Superseded after phase 4: coverage records now carry `harness`. See
"Decided by Rishi, for Karthik's review", decision B, at the end.*

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
  the agent does. *Superseded: see decision F at the end.*
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
  pending`. They would need trusting again once recap speaks. *Partly
  reversed by decision F: Stop is installed; UserPromptSubmit is not.*
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

## Decided by Rishi, for Karthik's review

Made after the phase 4 live test (`test/fixtures/codex/PHASE4.md`). Each is
in the branch as described, and each is open to your review.

### A. Codex failure detection: investigated, nothing built

**Decision.** Parse only the exit code from Codex's `tool_response` at
PostToolUse, matching one exact pattern, and keep nothing else. Anything else
stays unobserved.

**Why it was not built.** There is no exit code to parse:

- In all 7 captured PostToolUse payloads, `tool_response` is a plain string
  holding only the tool's output. The failed `python3 missing.py` (fixture 17)
  differs from a passing call only in its text.
- Codex 0.163.0 (latest stable) and 0.164.0-alpha.12 (latest alpha) embed
  the PostToolUse input schema: 13 fields, `additionalProperties: false`,
  none of them an exit code or status. OpenAI's hooks page describes
  `tool_response` as the tool's model-facing output. No changelog entry adds
  a status.
- The `exit_code=N` text seen in the phase 4 rollout was printed by the
  model's own script, not by Codex.

**What does exist.** Codex's rollout (the session's transcript file) has an
`event_msg` entry of type `item_completed` per command, with `item.type`
`CommandExecution`, `item.id` exactly equal to our record's `tool_use_id`,
`item.exit_code` (an integer) and `item.status` (`completed` or `failed`).
A `FileChange` (apply_patch) item has `status` and no `exit_code`. The
record's outcome stays `""` (item 1). Reading it from the rollout at report
time is decision D.

### B. The harness is stored on coverage records

**What.** Coverage records gain `harness`: `"codex"` on a record Codex's
hooks wrote, absent for Claude Code. It is additive, at the same
`SessionSchemaVersion`. A missing key means Claude Code, which every record
written before it existed already is. H-144 still passes: Claude Code's
records keep their bytes.

**What it changes.** The report reads a run's transcript in its harness's
format. In phase 4 the Claude Code accounting read a Codex rollout, found no
ids, and reported every call missing and the run `transcript_mismatch`,
though every call was in the rollout. A Codex run now gets decision D's
check. A harness value this build does not know is shown as `unknown`, gets
no check, and the report says `transcript check: not supported for this
harness (unknown)`. The report's JSON gains `harness` and
`transcript_check`, both absent for Claude Code.

**For you to look at.**

- `harness` is the first optional key in `docs/store-schema.json`. The
  schema test enforced "no key is optional"; it now names this one key as
  the exception rather than relaxing the rule.
- No version bump. Bumping would change every Claude Code coverage record for
  a key none of them carries. An older reader ignores the key.
- A Codex run recorded before this change still reads as Claude Code's, and
  still shows the false mismatch.
- On coverage records only, not on calls. Every hook invocation writes one,
  so every run that has records has them.

### C. Detach deletes the hooks.json that watch created

**What.** When `watch --harness codex`'s write creates `hooks.json`, it
writes the path to a new store-root file, `codex-hooks-created`.
`detach --harness codex` deletes `hooks.json` only when that file names it
and the document is empty after our entries are removed. In every other case
it writes the file as before: it existed before watch, it holds anything of
the user's, it is a symlink, the marker names another path, or the store is
gone. Phase 4 had detach leave `{}` where there had been no file.

**For you to look at.**

- A new file in the store layout, read and removed without opening the
  store, so detach still never creates a store.
- A stale marker: if the user deletes our `hooks.json` and later makes an
  empty one of their own, a watch and detach would delete it. Only an empty
  file can be deleted this way.

### D. Codex outcomes and the transcript check, read from the rollout

**What.** At report time, for a run with `harness: "codex"`, the report reads
the rollout each declaration names. It takes only `item_completed` entries,
and only their item's `id`, `type`, `exit_code` and `status`. An execution
joins an item only on exact `item.id` = `tool_use_id`, in its own
declaration's rollout:

- `CommandExecution`: exit 0 is ok, any other integer is failed, with that
  exit code;
- `FileChange`: status `failed` is failed;
- anything else stays unobserved.

The outcome is set in memory. The stored record keeps `""`. Measured on
phase 4's lines (`test/fixtures/codex/phase4`, scrubbed): `python3 missing.py`
reads failed (exit 2), and so does session 1's first call, `pwd && rg …`.
Its exit 2 came from unreadable directories under `~`, which no hook could
tell. The session with `missing.py` reports `failed calls: 1`, and its
timeline says `failed (exit 2)`.

The same items drive the transcript check: a recorded call with no item is
missing from the transcript, and a command or file-change item with no record
is missing from the store. All eight phase 4 calls agree. The ids make that
possible because every hooked tool, the agent tools included, has an item
with its id.

**For you to look at.**

- **A file change that completed reads ok.** Decision E, below.
- **The agent tools stay unobserved.** A `CollabAgentToolCall` has a status,
  and the rule does not name it.
- **"Missing from store" covers commands and file changes only.**
  `SubAgentActivity` also names non-calls (`subagent-completed-…`), so the
  agent-tool types are not held to the store. Unmeasured: whether Codex
  writes a `CommandExecution` item for a command that fires no hook, such as
  one the user runs directly. If it does, that command would read as missing
  from the store.
- **An id seen twice with two outcomes is judged neither.**
- **The digest reads the rollout too**, by the same rule, under a time budget
  (decision F), so a turn's line and the report count the same failures.

### E. A file change that completed is ok

**What.** In decision D's rule, a `FileChange` item whose status is
`completed` now reads ok, as `failed` reads failed. Any other status stays
unobserved, and the match is exact (`Failed` is not `failed`). On phase 4's
lines, the `apply_patch` edit reads ok, so session 1 has no unobserved call
left.

### F. The end-of-turn line under Codex

**What.** `watch --harness codex` installs a fourth entry, `Stop`, running
`recap --harness codex`; `detach` removes it with the rest. You trust it in
`/hooks` like the others. Under Codex, recap:

- **speaks at Stop** from the turn digest and the Stop payload's
  `last_assistant_message`, as under Claude Code. Its stdout is one JSON
  `systemMessage`, which Codex shows as a warning, or nothing. Codex's Stop
  output schema allows `decision`, `continue` and `reason`, and those (or exit
  2) would make the agent continue. recap writes none of them;
- **prints nothing on the UserPromptSubmit catch-up**, installed or not.
  There Codex adds plain stdout to the model's context. The entry is not
  installed, and `recap pending --harness codex` exits silently;
- **sets aside `hook_entry_unresolved`** before deciding whether to speak.
  Every Codex turn carries it by design (item 4), and spoken it would put the
  line on every turn. Any other coverage reason still speaks. The report and
  the digest keep it;
- **reads the rollout in the turn digest** by decision D's rule, bounded at
  one second from when the read starts. A rollout not read in time is dropped
  whole, so its calls stay unobserved rather than judged on part of the file.

H-150 runs the installed Stop line through a shell. A turn whose
`python3 missing.py` failed (exit 2, phase 4's rollout) and whose final
message says "Done." prints `1 recorded failure` and the report pointer. A
clean turn, a second Stop for the same turn, a turn whose message names the
failure, and the catch-up print nothing.

**For you to look at.**

- **Stop's timeout is ten seconds, not five.** It is recap's Claude Code
  timeout (`install.RecapTimeout`): recap reads the whole run to find its
  turn. The rollout read gets one second of it, after up to three for stdin.
  A one-line change makes Codex's Stop five, if you want it held to the
  recorder entries' budget.
- **Setting aside `hook_entry_unresolved` is recap's only harness-specific
  judgement.** A Codex turn's line says nothing about coverage unless
  something beyond the permanent reason went wrong.
- **Unmeasured until the live check:** whether a turn's items are in the
  rollout when its Stop fires. If they are not, the line misses that turn's
  failures and says nothing. It never says something wrong. Also: where the
  TUI shows the warning, and that it never reaches the model.

