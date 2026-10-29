# AutoSteer prescan: learning a repo's failure history before the first steered session

Status: proposal, for review. Nothing here is built.
Date: 2026-10-29.
Inputs: a review of [Photobombastic/paxel-local](https://github.com/Photobombastic/paxel-local)
(MIT, one Python file `paxel.py`, 2,891 lines, read at its default branch head),
main at `d1106a0`, and the Phase 2 lab results on `lab/autosteer-phase2` (PR #91).

## 1. The problem

AutoSteer starts every project with no history:

- **No required gates for weeks.** The project index (`internal/verify/index.go:21-36`)
  makes a gate family required only after two sessions within thirty days. On a new
  install, nothing is required.
- **No memory across sessions.** The one live pattern, regression-location, needs a
  gate that passed earlier *in the same session*. A repo whose `go test ./...` broke
  in eleven earlier sessions gets the same treatment as a repo that never broke.
- **Detection is the bottleneck.** In Phase 2, 21% of catchable items were caught in
  time, with a median lag of 105 minutes. Once told, agents fixed problems in a median
  of 24 minutes. The synthesis asks for checks that are live from the start.

The history already exists on the user's machine. Claude Code keeps every session's
transcript under `~/.claude/projects` for 30 days by default (`cleanupPeriodDays`).
Each transcript holds every command run, its working directory, whether it failed,
and its output. A **prescan** reads those transcripts once, through Rashomon's own
derivation code, and builds a per-project **prior**. The prior covers which gates
the repo has, how often each fails, which files tend to come before a failure, and
how often agents left a gate red or claimed success over a red gate.

## 2. What paxel-local does

paxel-local makes a "builder profile" of a person from their local agent transcripts.
It sends nothing over the network. Its pipeline:

| Stage | What it does | Where in `paxel.py` |
|---|---|---|
| Discover | Fixed home paths for six harnesses: Claude Code `~/.claude/projects`, Codex `~/.codex/sessions`, Gemini `~/.gemini/tmp`, Pi `~/.pi/agent/sessions`, opencode `~/.local/share/opencode/storage`, Cursor `state.vscdb` plus `~/.cursor/projects/*/agent-transcripts` | `discover_sources` 340, constants 320-337 |
| Normalise | Turns each format into Claude-shaped event dicts (`type`, `message.content` blocks of `tool_use` / `tool_result`, `sessionId`, `cwd`, `timestamp`), so one loop handles every source | `iter_events` 481, `_codex_events` 533, `_gemini_events` 607, `_pi_events` 662, `_opencode_events` 695, `_cursor_*` 762-1120 |
| Clean | Strips injected wrappers (`<system-reminder>`, Codex `<environment_context>`); drops Codex automation runs (SDK originator, or `exec` with no real cwd); joins argv-list commands into a string | `strip_injections` 123, 572-583, `_cmd_str` 148 |
| Aggregate | One pass of counters: tool categories, errors, "recovered" errors, edits per file, shell writes (regex), test runs (regex over about 50 runners) | `main` 1248-1810, `_SHELL_TEST_RE` 173 |
| Git | Runs `git log --numstat` as a subprocess. Dedupes repos by root-commit SHA and guesses the user's author emails | `git_churn` 203 |
| Score | Rule-based scores, archetype, "signature moves", growth edges | `compute_scores` 2058 onward |
| Output | `stats.json`, `report.md`, `profile.html`, and `narrative_input.md` (which holds raw prompt text) | 1817-2891 |
| Tests | Smoke fixtures per source, plus an adversarial invariant suite ("a spammer must not out-score a shipper") | `tests/` |

**How paxel scans failures.** It reads them as a count, not as events:

- A failure is a boolean `is_error` on a `tool_result`, counted per session (1456).
- **No command link.** Results are never joined to their `tool_use` by id, so a
  failure is not tied to the command that failed. The Codex translator drops `call_id`.
- **No exit codes and no output.** Neither is parsed.
- **No command identity or location.** "Which gate failed, where" is not something it
  can answer.
- **Weak recovery.** "Recovery" means any tool call after an error in the same
  session (1493), whether or not the failure was fixed.
- **Weak test detection.** Test runs are found by regex over the command line.

That is enough to profile a person, not to steer an agent. paxel-local measures how
someone works. AutoSteer needs to know which gate broke, where, after which writes,
and whether it was ever fixed.

## 3. What to take from paxel, and what to leave

### Take: knowledge, ported to Go

1. **The source map.** Where each harness keeps sessions, and each format's quirks:
   - Codex automation filtering.
   - Cursor's project slug mapped back to a cwd.
   - Codex argv-list commands.
   - `sessionId` reused across resumed sessions, so use gaps between events, not the
     session's span.
   - Injected wrapper text to strip.

   This is the hard-won part of paxel, and it is what the translators in §6 B6 need.
2. **One normalised event shape, many small translators.** Same design, but our shape
   is richer (§5.1).
3. **Its test fixtures.** `tests/fixtures/{claude,codex,gemini,pi,opencode,cursor}`
   are small synthetic sessions, MIT-licensed. Reuse them with a
   `THIRD_PARTY_NOTICES` entry, after a pass through the W3 scrubber (`test/tools/scrub`).
4. **Its test-runner list as a cross-check.** `_SHELL_TEST_RE` names runners `shape`
   may not cover (`tox`, `nox`, `gotestsum`, `cargo nextest`, `bazel test`,
   `mix test`, `deno test`, `hatch run test`, and others). `shape`'s runner tables
   remain the authority; the list only finds gaps. Each gap is a RulesVersion card,
   not a regex.
5. **Repo identity across clones.** paxel dedupes clones by root-commit SHA. We cannot
   run git (§4), but `.git/config`'s `remote.origin.url` is a plain file read and
   gives the same dedupe for most repos. `baseline.Key` stays the primary key.
6. **The adversarial-invariant discipline.** Every scoring bug kept as a frozen
   profile. The prior needs the same: for example, "a session that never ran a gate
   must not make that gate look healthy".

### Leave

- **Scores, archetypes, HTML and poster.** These profile a person, which is not our product.
- **`narrative_input.md`.** It writes raw prompt text to disk, which breaks Rashomon's
  digest-only record.
- **`subprocess git`.** `os/exec` is banned from the binary (TRU-40, H-17).
- **Cursor's SQLite reader.** The release binary removed its SQLite driver so it could
  claim no networking package (TRU-40's comment). Cursor's JSONL twin has no
  timestamps and no error statuses, so for failures it adds almost nothing.
- **Regex test detection and the boolean error count.** Replaced by `shape`, `gateout`
  and `verify`.

## 4. Does it need converting to Go? Yes, but as a port of knowledge, not of code

It has to be Go, inside the `rashomon` binary, for one reason: **the prior is only
useful if its keys equal the live keys.** A family is `runner digest @ location digest`
(`verify/classify.go:47`). Both digests are HMACs under the install key, computed by
`shape.DeriveWithDirs` and the cd fold in `hook.Locate` (`internal/hook/handle.go:341`).
Both change on every RulesVersion bump (now 9). A Python scanner would have to
reimplement `shape`'s tables and follow every bump, or produce keys that never match.
Calling the same functions makes them match by construction.

The binary's constraints, all of which the prescan keeps:

| Constraint | Source | Consequence |
|---|---|---|
| Standard library only | `go.mod` has no `require` | Each translator is a hand-written `encoding/json` reader |
| No networking package | TRU-40, README sentence pinned by test | No GitHub API and no CI history from inside the binary |
| No `os/exec` | TRU-40 / H-17 (nothing sanctioned) | No `git log`. Git mining happens outside the binary (§8) or not at all |
| `CGO_ENABLED=0`, every release target | `release_targets_test.go` | No SQLite, so no Cursor DB |
| Digest-only record, record scope v4 | `consent.RecordScope` | The prior stores digests, counts and classes, never text or paths in the clear |

Sizing the port: of `paxel.py`'s 2,891 lines, about 750 do discovery and translation.
The rest is scoring and HTML. Rashomon already has a Go Claude transcript reader
(`internal/report/transcript.go`, with `linescan` for lines up to 64 MiB and denial
detection by `deniedPrefix`). So the Claude path needs no port, only a new reader
mode. Each further harness translator is roughly 150-300 lines of Go plus fixtures.

## 5. Design

```
 ~/.claude/projects/**.jsonl ─┐
 (later: codex, pi, opencode) ├─▶ 1 reader ─▶ HistCall stream ─▶ 2 replay ─▶ store.Run (in memory)
                              ┘                                                │
                                                       hook.Locate / shape / gateout (live code)
                                                                               ▼
                                      3 verify.Evaluate + Claims + the episode extractor
                                                                               ▼
                                    4 Prior per project ─▶ verify/projects index seed
                                                       └▶ verify/priors/<key>.json
                                                                               ▼
                                    5 consumers: Required, steer rank, new patterns, W1, W4
```

### 5.1 Reader: transcript to `HistCall`

A new package, `internal/history`, streams each transcript once, in bounded memory,
and yields one `HistCall` per tool use:

```go
type HistCall struct {
    Harness    string          // "claude-code" first
    SessionID  string
    AgentID    *string         // sidechain / subagent file, nil for the main agent
    ToolUseID  string          // the join key paxel never makes
    Seq        int64           // order within the session, from the transcript
    AtUnixMS   int64
    CWD        string          // the event's cwd, which equals the hook payload's cwd
    ToolName   string
    ToolInput  json.RawMessage // passed to shape as the hook would receive it
    Result     ResultKind      // ok | error | denied | interrupted | none
    ExitCode   *int            // Claude: parsed from the "Exit code N" result prefix
    OutputHead []byte          // bounded, in memory only, for gateout; never stored
    OutputTail []byte
}
```

Reading notes:
- **Join `tool_use` to `tool_result` by id.** A use with no result (session killed
  mid-call) is `none`, never `ok`.
- **Denials are not failures.** `deniedPrefix` is matched anchored at the start, as
  `report` already does.
- **Background runs** (`run_in_background`, then `BashOutput`) are marked
  `Backgrounded`, as live ones are. Their outcome is not inferred.
- **Subagent sidechain files** are read with their `AgentID`, so `AgentCount`/writers
  keep working.
- **Version guard.** Every line carries `version`. The reader declares the Claude Code
  versions it has fixtures for. An unknown version is read but counted under its own
  `coverage` bucket in the prior.

### 5.2 Replay: `HistCall` to `store.Run`

For each call:
1. `hook.Locate(key, ToolName, ToolInput, CWD)` gives the shape and location digest.
2. `shape.DeclaredPath` gives the written path.
3. `gateout.Classify(kind, program, OutputHead+OutputTail)` gives the gate output.

From those, emit in memory the same declaration, terminal and execution records the
hook would have written, in the same order (#103's record order). Then hand the
resulting `store.Run` to `verify.Evaluate(run, nil, nil, pt, now)` at each point of
interest. `gateout.Classify` gets its first production caller here.

This reuses the whole verify stack instead of writing a second "what is a gate" engine.
#103's test-only run builder becomes a production package, or this one replaces it.

### 5.3 Analysis: what goes in the prior

Per project (`baseline.Key` from the call's cwd; a cwd whose directory is gone keys on
the path, as `baseline` does), per family:

| Field | Meaning | Feeds |
|---|---|---|
| `sessions`, `first_seen`, `last_seen`, `kind` | the gate inventory | `ProjectIndex` seed: Required from day one |
| `runs`, `red_runs`, exit-code histogram, `gateout` class histogram | how often and how it fails (compile, test, timeout, masked) | steer rank, report |
| `regressions` | green, then writes, then red, with a real exit: the regression-location pattern found offline | rank, W1 candidates |
| `fragile_paths` | top-N `DeclaredPath` digests most often written between a green and the regressing red, with counts | the fragile-write pattern (§5.4) |
| `red_at_end` | sessions that ended with this family red | Stop-time flag, W4 baseline |
| `red_to_green_ms` (median, p90) and calls to recover | how long recovery takes | report, W4 baseline |
| `claim_over_red` | `verify.Claims` over each session's final message: claimed success while this family was red | claim-vs-record prior |
| `writes_without_run` | sessions that wrote files in this family's location and never ran it | the unrun-gate pattern |

Per project:
- `coverage`: transcripts read, lines skipped, unknown versions, and sessions per harness.
- `rules_version`: the RulesVersion the prior was built with.
- `window`: earliest and latest transcript time.

The prior holds no output text, no file paths in the clear, and no prompt text. Test
names, if kept at all, are kept as HMAC like the claim comparator (1.7). It is capped
like the index (64 KiB, `MaxIndexBytes`), with eviction by `last_seen`.

### 5.4 Consumers

Ordered by value for the least new machinery:

1. **Required from the first session.** Seed `ProjectIndex` with
   `Observe(runs, sessionID, at)` per historical session. A family that ran in two
   sessions in the last 30 days is required on the first live session. This changes no
   live code, only the starting state. **This alone fixes the cold start.**
2. **Steer ranking.** `decide.go` ranks findings within the 3-nudge / 5-flag caps. A
   family with a history of regressions or `red_at_end` ranks above one with none.
   This adds a rank input, not a new message.
3. **New pattern: fragile write** (prevention, as Phase 2 asked). When the agent writes
   a path in a family's `fragile_paths`, and that family has not run since, the agent
   gets a nudge at write time: "edits to this file preceded N of this gate's last M
   failures; run X before moving on". This needs a producer, a render template and a
   `pattern` ID. It ships under the holdout arm like regression-location.
4. **Unrun-gate and red-at-Stop flags.** `writes_without_run` and `red_at_end` turn
   the existing ToSteer kinds (subtask-gate, claim-vs-record) from dormant into fed.
5. **W1 detector precision.** W1 needs 30 person-labelled fires per detector before a
   precision reads as measured. The prescan's historical fires are a ready pool of
   candidates to label. That turns weeks of waiting into one labelling session.
6. **W4 baseline.** Per-project pre-AutoSteer rates (`red_at_end`, `claim_over_red`,
   recovery time) are the "before" numbers. These are descriptive, not causal: the
   randomised holdout stays the only causal comparison (§7).

### 5.5 Command and lifecycle

- **`rashomon scan`.** Scans the current project's sessions and prints a short report:
  gates found, failure rates, and top regressions by family label. Labels are local
  display only, never stored.
- **`rashomon scan --all`.** Every project in the transcripts, which is the "all repos"
  case. It writes one prior per project.
- **When it runs.** It can be offered once at install (`internal/install`), because
  transcripts older than 30 days are already gone and every day of delay loses a day
  of history. Never at SessionStart: hooks have millisecond budgets.
- **RulesVersion.** A prior built under another RulesVersion is not used, because its
  keys would not match. `scan` re-derives from the transcripts still on disk. History
  older than the transcripts survives only as the version-free counts, without family keys.
- **Purge, forget and disclose.** The prior is a Rashomon-owned file. `purge` (#99:
  "delete only files rashomon wrote") and `forget` remove it, and `disclose` lists it.
  It lives outside the evictable run store, for the same reason `baseline` does.

### 5.6 Consent

Reading every transcript in the home directory is a new kind of data access: other
projects, possibly from before Rashomon was installed. So:

- A new consent unit, `history`, with a `consent.json` entry. `steer.DisclosureVersion`
  goes from 2 to 3, with the disclosure text naming which directories are read.
- **Default scope is the current project only.** `--all` asks again, on `/dev/tty`,
  as the consent screen (#80) does.
- **Record scope applies.** Under v4 the project root is not recorded, so the prior is
  keyed by the `FileName(key)` digest only.
- No network, as for the whole binary, and the TRU-40 test already enforces it.

## 6. Plan

Lane B (backfill). Each item is about one PR, in the repo's card style.

| Item | Scope | Exit test |
|---|---|---|
| **B0** | Rulings: the consent unit, `--all` default, storage path, whether `history` may import `verify` internals (as Q0 asks for the learner), the fixture licence notice | Written in APPROACH-NOTES |
| **B1** | `internal/history` Claude reader: `HistCall`, id join, exit-code parse, denial, interrupt, sidechain, version guard, `FuzzHistCalls`, scrubbed fixtures | Fuzz floor +1; a property: every `tool_use` gives exactly one `HistCall` |
| **B2** | Replay to `store.Run` through `hook.Locate`, `shape` and `gateout` | **Equivalence gate:** for sessions Rashomon recorded live and whose transcript is still on disk, the replayed families, gate states and regressions equal the live ones, byte for byte. Phase 2's first lesson was to validate the gauge before judging the process. This is that gate, and nothing after B2 merges without it |
| **B3** | Prior file, analysis (§5.3), `ProjectIndex` seeding, `rashomon scan`, purge / forget / disclose, consent | Acceptance: a fresh install plus fixture history means the first session's Evaluate reports the seeded families Required |
| **B4** | `scan` text report, plus budgets rows (`BenchmarkScan` on darwin and linux) | Perf gate |
| **B5** | Rank input in `decide.go`; the fragile-write pattern (producer, template, pattern ID, RulesVersion bump), under the holdout arm | Holdout draw unchanged; caps unchanged |
| **B6** | Translators: Codex (keep `call_id`), Pi, opencode, Gemini, using paxel's source map and fixtures | Per-harness fixture tests. Cursor out of scope (§3) |
| **B7** | W1 candidate export; W4 baseline fields in the outcomes bundle (`share`, behind its existing consent) | Outcomes schema to `outcomes/2` |

B1 to B3 deliver the cold-start fix. B5 is where steering behaviour changes and needs
a lab run. B6 widens coverage to Codex, Pi, opencode and Gemini. The README's "Claude
Code only" stands until B6 lands *and* the hooks support those harnesses. A prior from
Codex sessions only steers if Rashomon records Codex live.

## 7. Risks and how the design meets them

| Risk | Mitigation |
|---|---|
| **Keys that do not match live keys.** Different cwd, cd fold or RulesVersion makes the prior silently useless | B2's equivalence gate; RulesVersion stamp; re-derive on bump |
| **Transcript format drift** | Version guard, coverage bucket, fixtures per supported version; a line that cannot be read is counted, not guessed (as `linescan` does today) |
| **Exit code only from the result text** ("Exit code N" prefix) | Parsed anchored. Absent means `ExitCode=nil`, which `regressedHere` already treats as not a regression |
| **Circularity** (#91's 65% critique: an instrument defining its own ground truth) | The prior never labels its own fires as correct. W1 labels come from a person. W4's causal claim comes from the randomised holdout only |
| **Privacy.** Transcripts hold everything | Text stays in memory; the prior is digests and counts; record scope; new consent unit; purge covers it |
| **Arm B lesson.** History is shaped by the user's own global config | Report coverage per harness and version; never present historical rates as a baseline for "no AutoSteer" in general |
| **Big homes.** 1,858 transcripts and a single 178 MiB line were measured here before | Streaming reader with `linescan` bounds; one pass per file; budgets row in B4; `--all` shows progress |
| **History is not representative.** 30-day window, sessions in deleted directories | `window` and `coverage` in the prior; Required needs the same two-session rule as live |

## 8. Outside the binary: git history and CI

paxel's best idea, git churn, needs `git log`. Inside the binary there are two options:
a pure-Go reader of `.git` (loose objects, packfiles, zlib, deltas, all standard
library but a sizeable project), or nothing. Neither is worth it for v1. Transcripts
already show which writes came before which failure, and that is the signal steering
needs.

If the team wants git or CI history ("fix:" and revert commits, files that change
together, GitHub Actions failures), the right home is a **lab tool**, like
`lab/autosteer-phase2/ci/*.py`. It would run outside the binary, write its findings
as a hidden-input file, and be measured against the transcript prior before anything
ships. Python is fine there, and paxel's `git_churn` can be adapted directly (MIT, with
attribution).

## 9. Decisions needed

1. **Scope of "all the repos".** Every project in local transcripts (this design), or
   also repos across a GitHub org? The org case is a server-side product, outside the
   local-only binary.
2. **B0 rulings** (§6): the consent unit and default scope, the storage path, and the
   import ruling.
3. **The fragile-write pattern (B5).** Approve it as the first new pattern built on the
   prior, or keep the prior to Required-seeding and ranking until W4 reports.
4. **Harness order for B6.** Codex first? It is the most common second harness in
   paxel's source map, and its format keeps a `call_id` for the id join.
