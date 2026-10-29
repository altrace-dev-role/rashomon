# AutoSteer prescan: importing gate history before the first session

Status: proposal, revision 2, for review. Nothing here is built.
Date: 2026-10-29.
Inputs: a review of [Photobombastic/paxel-local](https://github.com/Photobombastic/paxel-local)
(MIT, one Python file `paxel.py`, 2,891 lines), main at `d1106a0`, and the Phase 2 lab
results on `lab/autosteer-phase2` (PR #91).

## 0. What changed in revision 2

Review asked for a narrower first release and six changes. Rereading the code for them
also corrected a premise of revision 1.

**Corrected premise: what seeding does.** Revision 1 said a new project "requires
nothing for weeks" and that seeding would make families Required from the first
session. Both were wrong:
- **Fresh install is fail-closed.** Until the index has established any family, every
  gate that runs in a session is Required (`internal/verify/evaluate.go:94`, D38:
  `Required = !learned || idx.Required(...)`).
- **Gates come only from this session's runs** (`gateStates(runs, ...)`,
  `evaluate.go:89`). The index cannot require a check that did not run.
- **The cold start is short.** A family is established after two sessions with the last
  within 30 days. For a repo whose tests run in most sessions, that is the second
  session. "Weeks" holds only for families run rarely.

So the only behaviour seeding changes is the switch from **prior mode** (every gate
that ran is Required) to **learned mode** (established families Required, the rest
advisory). That switch:
- **Changes:** the done verdict (`recordGreen`), the turn summary and value line
  (`recap/value.go:349`), the obligation ledger, and which gates a "done" claim is
  checked against (`verify/claims.go:340`).
- **Does not change:** regression-location steering, which ignores Required
  (`verify/steer.go:69`).
- **Can loosen verdicts.** A red gate that is not established becomes advisory. If an
  obsolete workflow is established and the current test suite is not, a real failure
  stops blocking "done".

This supports review point 1 more strongly than review put it. Section 6.4 sets
seeding up to be measured in both directions before it is turned on.

**The six review points**, and where each is handled:

| # | Review point | Where |
|---|---|---|
| 1 | Seeding is a behaviour change and needs its own evaluation | §6.4 shadow mode, §7.2 time-split backtest |
| 2 | Fragile paths stay experimental, with exposure, an infra/test split, and association only | §8.1, out of v1 |
| 3 | Uncertainty is in the data model, per observation | §6.1 evidence grades, §6.3 "last observed" |
| 4 | Deduplication and repeated scans are an invariant, before seeding | §6.2, acceptance tests in §9 B2 |
| 5 | Semantic equivalence, not byte-for-byte, plus independent annotation | §7.1 |
| 6 | RulesVersion lifecycle defined now | §6.6 |
| + | Bounded memory is explicit and performance-tested | §6.7 |

## 1. The goal of v1

v1 is a release that **imports gate history, shows what it found, and seeds the
existing rules, conservatively and only after measurement.**

| In v1 | Out of v1 (§8) |
|---|---|
| Claude Code transcripts only | Codex, Pi, opencode and Gemini translators |
| Current project only | `--all` across every project |
| Gate inventory per family, with evidence grades | Fragile paths and write-time nudges |
| A `rashomon scan` report: what was found, and the coverage | Steer ranking from history |
| Idempotent import with a session-identity invariant | Regression and recovery statistics as inputs to steering (the report may show them as descriptive) |
| Seeding the existing index rules, shadow first, enabled after the backtest | Any new steer pattern, and any RulesVersion bump |

The promise v1 makes is concrete and testable: **on the first session, Rashomon knows
which checks this project has established**, shows them, and (once the backtest
passes) uses them the way it would after two live sessions.

## 2. How paxel-local scans

paxel-local profiles a person from their local agent transcripts, with no network.

| Stage | What it does | Where in `paxel.py` |
|---|---|---|
| Discover | Fixed home paths for six harnesses: Claude Code `~/.claude/projects`, Codex `~/.codex/sessions`, Gemini `~/.gemini/tmp`, Pi `~/.pi/agent/sessions`, opencode `~/.local/share/opencode/storage`, Cursor `state.vscdb` plus `~/.cursor/projects/*/agent-transcripts` | `discover_sources` 340, constants 320-337 |
| Normalise | Each format becomes Claude-shaped event dicts, so one loop handles every source | `iter_events` 481, translators 533-1120 |
| Clean | Strips injected wrappers; drops Codex automation runs; joins argv-list commands | `strip_injections` 123, 572-583, `_cmd_str` 148 |
| Aggregate | One pass of counters: tool categories, errors, "recoveries", edits per file, shell writes and test runs by regex | `main` 1248-1810, `_SHELL_TEST_RE` 173 |
| Git | `git log --numstat` as a subprocess; repos deduped by root-commit SHA | `git_churn` 203 |
| Score and output | Rule-based scores, archetype, HTML profile; `narrative_input.md` holds raw prompt text | 1817-2891 |

**How it handles failures.** A failure is a boolean `is_error`, counted per session
(1456):
- **No command link.** Results are never joined to their `tool_use` by id; the Codex
  translator drops `call_id`.
- **No exit codes, output, command identity or location.**
- **Weak recovery.** "Recovery" is any later tool call in the session (1493).

It measures how a person works. It cannot say which gate failed, where, or whether it
was fixed.

## 3. What to take, and what to leave

**Take, as knowledge ported to Go:**
- **Its source map and format quirks.** These are mostly for the later harness
  translators. For Claude Code, the one that matters is a `sessionId` reused across
  resumed sessions (§6.2).
- **The design:** one normalised shape, small translators.
- **Its MIT fixtures.** Use them with a `THIRD_PARTY_NOTICES` entry, after the W3
  scrubber (`test/tools/scrub`).
- **`_SHELL_TEST_RE` as a gap list** for `shape`'s runner tables (`tox`, `nox`,
  `gotestsum`, `cargo nextest`, `bazel test`, `mix test`, `deno test` and others).
  `shape` stays the authority; each gap is its own card.
- **The adversarial-invariant test discipline.**

**Leave:**
- **Scores and HTML.** These profile a person, which is not our product.
- **`narrative_input.md`.** It writes raw prompt text, and Rashomon's record is digest-only.
- **`subprocess git`.** `os/exec` is banned (TRU-40, H-17).
- **Cursor's SQLite.** No CGO, and the driver was removed so the binary could claim no
  networking package.
- **Regex test detection and boolean error counting.**

## 4. Why Go, inside the binary

A family is `runner digest @ location digest` (`verify/classify.go:47`). Both are HMACs
under the install key (`shape/shape.go:305`), derived by `shape.DeriveWithDirs` and the
cd fold in `hook.Locate` (`internal/hook/handle.go:341`). An import is only useful if
its keys equal the live keys. Calling the same functions makes them equal by
construction. A Python port of `shape`'s tables would drift on the next table change.

The binary's constraints, all of which v1 keeps:

| Constraint | Source | Consequence for v1 |
|---|---|---|
| Standard library only | `go.mod` has no `require` | The reader is `encoding/json` plus `linescan` |
| No networking package | TRU-40 | Local transcripts only |
| No `os/exec` | TRU-40 / H-17 | No git history (§8.3) |
| `CGO_ENABLED=0` | `release_targets_test.go` | No SQLite |
| Digest-only record, record scope | `consent.RecordScope` | The seed file holds digests, grades and counts, never text or clear paths |

Rashomon already reads Claude transcripts in Go (`internal/report/transcript.go`). It
joins transcript calls to records by `tool_use_id` for the report's cross-check, reads
lines up to 64 MiB through `linescan`, and detects denials by `deniedPrefix`. v1 needs
a new reader mode, not a port.

## 5. Pipeline

```
transcripts of the current project (~/.claude/projects/<project dir>/**.jsonl)
   │ 1 reader (bounded): tool_use ⨝ tool_result by id, grades each observation
   ▼
HistCall stream, one file at a time
   │ 2 replay through the live code: hook.Locate, shape, gateout → store.Run per file
   │   classify once per file (no per-point Evaluate in v1)
   ▼
per-session family summaries, merged across files by session id and tool_use_id
   │ 3 dedupe against the import ledger and the live store
   ▼
seed file: verify/seeds/<FileName(key)>.json   +   `rashomon scan` report
   │ 4 shadow: Evaluate computes the live verdict and the seeded verdict, and logs the difference
   ▼
   enabled after the backtest: Required = live rule OR seed rule (no cross-source count)
```

## 6. Design

### 6.1 Observations carry their evidence grade (review point 3)

The reader yields one `HistCall` per `tool_use`, and every call carries a grade:

```go
type Grade uint8

const (
    Verified    Grade = iota // id-joined result, outcome known, location known, supported version
    Partial                  // outcome known but location unread/unknown, or exit code absent on an error
    Unsupported              // unknown transcript version, unreadable line nearby, malformed shape
    Missing                  // a tool_use with no tool_result: killed mid-call, or the file was cut
)
```

The grade rules:
- **Outcome.**
  - `ok` when the result is not an error.
  - `failed(exit)` when the error text starts with an anchored `Exit code N`.
  - `denied` when it starts with `deniedPrefix`; a denial is never a failure.
  - `interrupted` from the result's `interrupted` flag.
  - `none` when there is no result.
  - Background runs are `Backgrounded`, as live, with no inferred outcome.
- **Supported versions.** The reader names the Claude Code `version` values it has
  fixtures for. Any other version grades the whole file `Unsupported`.
- **Cut files.** A file whose last line is cut, or that holds a line `linescan`
  stepped over, grades the calls after that point `Partial` at best.

Grades decide what counts:
- **Seeding counts `Verified` observations only.**
- **The report shows all four grades**, as counts per family.
- **Nothing is guessed.** An `Unsupported` file is read for coverage and never
  contributes a family to the seed.

**End of session is "last observed", never "abandoned".** Per family, the summary holds
`last_observed` (state, seq, time) and `session_complete`. A session is complete only
when its newest file ends with an assistant turn and no outstanding `tool_use`. A red
`last_observed` in an incomplete session says nothing about what the agent did next.
The report words it as "the last result we can see was red", and v1 uses it for nothing else.

### 6.2 Session identity and idempotence (review point 4)

**The identity of a session is its Claude Code `sessionId`, the same value the hook
receives as `session_id`.** Live and imported evidence therefore name sessions the same way.

Cases:

| Case | Rule |
|---|---|
| Same scan twice | The seed file holds an import ledger: `HMAC(install key, sessionId)` for every imported session, with its file digests. Rescanning replaces a session's summary; it never adds to it |
| Resumed session (same `sessionId`, several files or appended history) | Files are merged by `sessionId`, and calls are deduped by `tool_use_id` inside the session. A replayed earlier turn copied into the new file contributes nothing new |
| Copied transcript (same session in two project dirs, or a backup) | Same `sessionId`, so merged. If the copies disagree on a `tool_use_id`'s result, that call grades `Unsupported` |
| Subagent and sidechain files | Belong to their parent `sessionId`, with `AgentID` set; never a session of their own |
| Session also recorded live by Rashomon (its run is in the store) | Excluded from the seed. The live record is the better evidence, and the live index has counted it. Such sessions feed only the B2 equivalence corpus |
| Live index plus seed for one family | **No cross-source count in v1.** The seed rule and the live rule are each evaluated on their own session sets, and Required is their OR. One historical session plus one live session does not make two. The live index keeps only `LastSession` (`index.go:110-122`: A, B, A counts A twice), so a sound union would need a session set in the index. That is a later schema change, not v1 |

**Invariant (DEDUP-1):** the seed's session count for a family equals the number of
distinct `sessionId`s, not recorded live, with at least one `Verified` run of that
family, whatever the number of scans, files, copies, resumes or subagent files.

**Invariant (DEDUP-2):** for any one session, the seed and the live index never both count it.

Both are acceptance tests in B2, with fixtures for each row of the table above, before
any seeding code exists.

### 6.3 What the seed file holds

Per project, keyed by `baseline.Key` of the call's cwd. A cwd whose directory is gone
keys on the path, as `baseline` does. Per family (`runner@location`):

| Field | Meaning |
|---|---|
| `kind` | test, build or custom, from `kindOf` |
| `sessions_verified` | distinct sessions with a `Verified` run (DEDUP-1) |
| `sessions_any` | the same, at any grade (report only) |
| `first_seen`, `last_seen` | times of `Verified` runs |
| `greens_verified`, `reds_verified` | `Verified` run outcomes (report only in v1) |
| `last_observed` | state, time and `session_complete` of the newest session (report only) |
| `rules_version` | the RulesVersion this entry was derived under (§6.6) |

Per project, also:
- **The import ledger** (§6.2).
- **`coverage`:** files, sessions, and calls per grade; unsupported versions seen;
  lines skipped; the transcript window.

The file holds no text and no clear paths. It is capped at 64 KiB like the index, with
eviction by `last_seen`. It lives under `verify/seeds/`, outside the evictable run
store, for the reason `baseline` does. `purge` (#99), `forget` and `disclose` all cover it.

### 6.4 Seeding the existing rule, conservatively (review point 1)

The seed rule mirrors the live rule and is stricter. A family is seed-Required at `now`
when all of these hold:
1. `sessions_verified >= 2` (the live `RequiredSessions`), **and**
2. at least one `Verified` green: the check is shown to pass here, not only attempted,
   **and**
3. `last_seen` is within 30 days (`RequiredWindowMS`), **and**
4. it is **current**: it ran in at least one of the project's last *K* imported sessions
   that ran any gate of the same `kind`. This guards against a migration's one-off
   commands or a workflow the project has since replaced. *K* starts at 3; the
   backtest sets it.

Because seeds age out after 30 days by rule 3, **the seed is a bridge by construction**:
within one window, live evidence has replaced it.

**Shadow first.** B3 ships seeding in shadow mode:
- **No change to verdicts.** Evaluate computes the live verdict and the verdict with the
  seed, and writes neither to the agent.
- **A local log of differences.** Per turn, a `seed_shadow` count of what would differ
  is kept, digest-only, in the store, and the scan report and `rashomon status` show it.

The seed is enabled only after the backtest (§7.2) passes. It is a separate
switch, recorded in consent like the record scope.

**Visible without changing verdicts.** The scan report and the recap can name the
established checks ("established here: 3 checks, last run 2 days ago") before seeding
is enabled. That delivers the v1 promise even while seeding is shadowed.

### 6.5 The command and consent

- **`rashomon scan`.** Reads the current project's transcripts and prints:
  - the established and candidate families, by local display label (never stored);
  - per-family grade counts and green/red counts;
  - "last observed" states, worded as such;
  - coverage, with what could not be read and why;
  - what seeding would do (shadow).
- **`rashomon scan --dry-run`.** Writes nothing.
- **Offered once by `install`.** Transcripts older than 30 days are already gone. It is
  never run from a hook.
- **Consent.** A new consent unit, `history`, and `steer.DisclosureVersion` moves 2 to
  3, naming the directory read. The default scope is the current project. The
  `/dev/tty` prompt is reused from #80. The record scope applies.

### 6.6 RulesVersion lifecycle (review point 6)

What the code does today:
- **The family key holds no RulesVersion.** It is an HMAC of the *derived* runner and
  location (`shape.go:305`). A RulesVersion bump changes a key only for commands whose
  derivation changed.
- **The live index survives bumps the same way.** Its entries carry no
  `rules_version`. An entry whose key no longer derives ages out within 30 days.
- **Recent bumps mostly touched identity.** Judging by their titles, RulesVersion 5, 6,
  8 and 9 changed runner matching or the location fold. Bumps that touch identity are
  common right now.

Defined behaviour for the seed:
1. Each entry records the `rules_version` it was derived under.
2. **On a bump, Rashomon rescans the transcripts still on disk**, offered at the first
   `scan` or `recap` after upgrade. This is safe because of idempotence (§6.2).
3. **Entries that cannot be re-derived** (their transcripts expired) **stop counting
   toward seed-Required** and stay in the report as "derived under rules N". This is
   stricter than the live index, on purpose: imported evidence has no live session
   behind it to re-confirm the key.
4. **The worst case of the upgrade trap is bounded:** the seed stops helping a little
   early. It never makes a stale key Required, and live evidence takes over within one
   window anyway.

**Splitting an `IdentityVersion`** (runner and location tables only) out of RulesVersion
would let non-identity bumps leave seeds alone. Bumps so far mostly touch identity, so
the split would save little now. The proposal is to make it the first time a
non-identity bump would invalidate seeds. v1 itself needs no bump: it adds no pattern
and changes no table.

### 6.7 Bounded memory and performance

Streaming JSONL bounds the read, not the work. v1's limits:

| Thing held | Bound | Over the bound |
|---|---|---|
| Lines | `linescan` 64 MiB max line, 256 KiB initial buffer | stepped over and counted; nearby calls graded `Partial` |
| Outstanding `tool_use` awaiting a result, per file | 1,024 | the oldest drops to `Missing` |
| Output kept per gate call, for `gateout` | 8 KiB head and 8 KiB tail, gate kinds only | truncated; `gateout`'s classes already bound their scan |
| Calls replayed into one `store.Run` | 20,000 per file | the file is summarised as `Partial` from that point |
| `store.Run` | one file at a time; freed after classify | none |
| Evaluate | **once per file at its end (classify, as Stop does), not per point**, so O(n) | none |
| Per-session merged summary | families × fixed-size counters, plus a `tool_use_id` digest set, capped at 20,000 | over the cap, dedupe reverts to per-file and the session grades `Partial` |

**Performance gates** (B4):
- `BenchmarkScanFile` rows for darwin and linux in `budgets.json`.
- A peak-RSS test like the RES rows over a synthetic 50,000-call session and a file
  holding a 178 MiB line (the size measured here before).
- A wall-time budget for 2,000 transcripts.

## 7. Evaluation

### 7.1 Replay accuracy, B2 (review point 5)

**Semantic equivalence by evidence class, not byte equality.**

**Corpus:** sessions Rashomon recorded live whose transcript still exists. The
report's transcript cross-check already finds these.

Compare per (session, family):

| Live state | Replay state | Verdict |
|---|---|---|
| known X | known X | agree |
| known X | known Y ≠ X | **disagreement: zero allowed; each is a bug in one side** |
| known | Unknown / `Partial` | allowed; counted as replay's evidence gap, with the reason named |
| Unknown / not recorded | known | flagged for review: replay claims more than the live record had |

**Where evidence differs.** The hook sees some things a transcript may not:
- the record order of declaration, terminal and execution;
- a declaration for a call that was then denied or never ran;
- the hook's own completeness signal (`Recording`).

Transcripts can also hold calls the hook missed (hook not installed yet, or a gap). Each
known difference is named in a table, with the state replay must give for it (usually
Unknown).

**Independent annotation.** About 40 sessions, chosen to cover each §6.2 row and each
grade, are labelled by a person from the raw transcript. For each, the person records
the families run, each run's outcome, and the last observed state. The person does not
see either implementation's output. Both live and replay are scored against the labels,
so a bug they share shows up as both being wrong.

**Exit criterion for B2:**
- zero known/known disagreements;
- an evidence-gap rate reported per reason;
- at least 95% agreement of replay with the annotations on `Verified` observations
  (the threshold is proposed and set in B0);
- DEDUP-1 and DEDUP-2 passing.

### 7.2 Seeding backtest, B3 (review point 1)

**Data:** projects with at least 20 imported sessions.

**Time split:** seed from sessions before *T*; replay the sessions after *T* in order,
updating the live index as Stop would. For each turn, compute the verdict in four ways:
- live prior mode,
- live learned mode,
- seeded,
- seeded plus live.

Measure the differences in both directions:

| Direction | Event | Counted as |
|---|---|---|
| Tightening | a seed-established family's red or stale now blocks a turn that would have read verified | needed if the family is run again later in that project; **nuisance** if it never runs again (obsolete) |
| Loosening | a non-established red gate goes advisory, so a turn reads verified, or a done claim passes, where prior mode would not | **hidden failure** unless a person labels the gate a one-off |

Every loosening event, and a sample of tightening events, goes to person labels, as W1
does. They are reported with Wilson intervals; a rate with fewer than 30 labels reads
"unmeasured".

**Enable criteria** (proposed, set in B0):
- the hidden-failure rate is not distinguishable from zero on the labels;
- the nuisance rate is under 10% of tightening events;
- otherwise, tighten rules 1 to 4 (raise *K*, require 3 sessions) and re-run.

If loosening cannot be made safe, there is a fallback: seed only established families,
and keep prior mode for the gates that are not established. That fallback needs a
small change to D38's `!learned ||` and its own ruling.

## 8. Deferred: earns its way in through the lab

### 8.1 Fragile paths (review point 2)

Kept as an experiment, not a feature. If built, it is built this way:
- **Exposure, not counts.** For a path *p* and family *F*:
  - The exposure *E(p,F)* is the number of `Verified` runs of *F* with at least one
    write to *p* since *F*'s previous run.
  - The failure rate among those is compared to *F*'s base rate, with an interval. A
    minimum exposure (for example 10) applies before any value is shown.
  - Central files that are edited before every run get a high exposure and so a
    ratio near the base rate, not a high count.
- **Test failures separate from infrastructure failures**, from `gateout`'s class and
  the exit code. Exit 126/127, timeout 124, killed, network or environment patterns are
  infrastructure and never count against a path.
- **Wording is association:** "runs after edits to this file failed 6 of 9 times,
  against 2 of 30 overall". Never "this file breaks the build".
- **Order of use:** first offline in the lab; then as one input to *ranking existing
  findings*; and only after a W4-style holdout shows value, as a write-time nudge (a
  new pattern, a RulesVersion bump, and the §6.6 rescan).

### 8.2 Broad harness support

Codex first, since its format keeps a `call_id` for the id join. Then Pi, opencode and
Gemini, from paxel's source map and fixtures. This waits until B2's accuracy holds for
Claude, and until Rashomon records those harnesses live. A seed nobody can use live is
only a report.

### 8.3 Git and CI history

There is no `git` in the binary. If wanted, it would be a lab tool outside the binary
(paxel's `git_churn` adapts directly, MIT), producing a hidden-input file to be measured
against the transcript seed before anything ships.

## 9. Plan (Lane B)

| Item | Scope | Exit test |
|---|---|---|
| **B0** | Rulings: consent unit, seed path, the B2 and B3 thresholds and *K*, whether `history` may import `verify` internals (Q0-style), the fixture licence notice | Written in APPROACH-NOTES |
| **B1** | `internal/history` Claude reader: `HistCall`, id join, outcomes, grades (§6.1), version list, the bounds in §6.7, `FuzzHistCalls`, scrubbed fixtures | Fuzz floor +1. Property: each `tool_use` gives exactly one `HistCall`, and no `Missing` call is ever `ok` |
| **B2** | Replay through `hook.Locate`, `shape` and `gateout`; per-session summaries; merge and ledger (§6.2) | §7.1 criteria; DEDUP-1 and DEDUP-2 over the §6.2 fixtures, including scanning the same tree twice and a tree with copies, resumes and subagent files |
| **B3** | Seed file, seed rule (§6.4), **shadow mode**, `rashomon scan` and `--dry-run`, consent, purge, forget, disclose | Acceptance: with a fixture history, the first session's shadow log shows the expected seeded verdicts, and live verdicts are unchanged |
| **B3e** | Backtest (§7.2) and person labels; the enable switch | §7.2 enable criteria |
| **B4** | Report polish; `BenchmarkScanFile`, the RSS and wall-time rows (§6.7) | Perf gates on darwin and linux |

v1 is B0 to B4. It needs no RulesVersion bump and adds no steer pattern.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Seeding loosens verdicts | Shadow mode, the loosening count in the backtest, the fallback in §7.2 |
| One session counted twice | §6.2 rules, DEDUP-1 and DEDUP-2 acceptance tests, no cross-source count |
| Replay agrees with a bug it shares with live | Independent annotation (§7.1) |
| Format drift | Supported-version list; unknown versions graded `Unsupported` and never seeded |
| Upgrade trap | §6.6: rescan, stop counting what cannot be re-derived, bounded by the 30-day window |
| Memory and time | §6.7 bounds and perf gates |
| Privacy | Text stays in memory; the seed is digests; new consent unit; current project by default; purge covers it |
| Over-claiming | Report language is "last observed", "established here", and association only; the description and causal claims are kept apart |

## 11. Decisions needed

1. Approve v1 as B0 to B4 with seeding shadowed until B3e passes.
2. The §7.2 fallback, if needed: is changing D38 for seeded projects acceptable, or
   does seeding stay report-only?
3. Who labels for B2's annotation set and B3e. This is the same person-labelling need
   as W1, so one labelling session could serve both.
