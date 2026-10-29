# AutoSteer prescan: importing gate history before the first session

Status: proposal, revision 3. B0 to B4 approved as a bounded reporting and
shadow-mode experiment. Adopting seeding is a separate decision. Nothing here is built.
Date: 2026-10-29.
Inputs: a review of [Photobombastic/paxel-local](https://github.com/Photobombastic/paxel-local)
(MIT, one Python file `paxel.py`, 2,891 lines), main at `d1106a0`, and the Phase 2 lab
results on `lab/autosteer-phase2` (PR #91).

## 0. Revisions

### Revision 3: decisions and fixes

**The investment case, restated.** The cold-start justification of revision 1 does not
hold (see revision 2 below). What remains is two things:
- **Historical reporting**, useful on its own.
- **An experiment** in whether imported history can safely improve gate policy. Its
  benefit is not established, and the experiment has to measure it.

**Decisions recorded:**
1. **B0 to B4 approved**, as reporting plus a shadow experiment. Seeding is not enabled
   because results look inconclusive. It is enabled only if a separate adoption
   decision finds a measured benefit and a bounded harm (§7.2).
2. **Seeding stays report-only** if safe adoption is not shown. Revision 2's D38
   fallback is withdrawn. Any D38 change is its own policy proposal, with examples and
   tests.
3. **Labelling:**
   - The primary labeller is a maintainer who knows the repository's intended checks.
   - They never see which output is live and which is replay.
   - A second reviewer adjudicates ambiguous cases.
   - One labelling session may serve several evaluations, but each has its own rubric (§7.3).

**Fixes:**

| Fix | Where |
|---|---|
| The safety criterion is a threshold set in advance, met by the upper confidence bound, with sessions correlated within a project accounted for. "Not distinguishable from zero" is dropped | §7.2 |
| A positive benefit is measured: unnecessary blocking verdicts removed, and needed ones added | §7.2 |
| The nuisance proxy is dropped ("never ran again" proves nothing). Each policy difference is counted mechanically and labelled for whether blocking was warranted given the task and the evidence | §7.2 |
| Exit codes do not establish infrastructure failure. Timeout, 126 and 127 are their own outcome classes, of unknown cause | §6.1, §8.1 |
| The shadow policy has an exact rule for which history it uses as live sessions arrive, and the backtest simulates exactly that rule | §6.4, §7.2 |
| "Classified once per file" is clarified: transitions are kept from classify's ordered runs, and verdicts are computed at every Stop point | §6.3, §6.7, §7.1 |

### Revision 2

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

This supports review point 1 more strongly than review put it. Section 6.4 defines
the shadow policy, and §7.2 measures it in both directions.

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

v1 is a release that **imports gate history and shows what it found.** It also runs
seeding of the existing rules in shadow, so the policy can be measured. Adopting
seeding is a separate decision, after v1.

| In v1 | Out of v1 (§8) |
|---|---|
| Claude Code transcripts only | Codex, Pi, opencode and Gemini translators |
| Current project only | `--all` across every project |
| Gate inventory per family, with evidence grades | Fragile paths and write-time nudges |
| A `rashomon scan` report: what was found, and the coverage | Steer ranking from history |
| Idempotent import with a session-identity invariant | Regression and recovery statistics as inputs to steering (the report may show them as descriptive) |
| Seeding the existing index rules, in shadow only, with its evaluation | Enabling seeding; any D38 change; any new steer pattern; any RulesVersion bump |

The promise v1 makes is concrete and testable: **on the first session, Rashomon can
show which checks this project has established**, from history, and report what
seeding them would have changed. Whether to act on them is decided from the evaluation.

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
   │ 4 shadow: Evaluate computes the live verdict and the verdict under policy P1 (§6.4),
   │   and logs the difference; the agent and the user see the live verdict only
   ▼
   adoption of P1: a separate decision, from the §7.2 evaluation
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
  - `failed(exit)` when the error text starts with an anchored `Exit code N`, N other
    than 124, 126 and 127.
  - `timeout` for exit 124, and `not_run` for 126 and 127. Each is its own class
    **of unknown cause.** A timeout can come from a code regression, and 126/127 from
    a change that broke permissions or removed a tool. They are never labelled
    "infrastructure", and v1 does not fold them into `failed`. Live regression-location
    already excludes 126 and 127 (`regressedHere`); v1 does not change that.
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
| `greens_verified`, `reds_verified`, `timeouts`, `not_run` | `Verified` run outcomes by class (report only in v1) |
| `transitions` | counts of green→red, red→green and green→red→green episodes per session, from classify's runs **in seq order** (report only). Transitions are read from the ordered run list, never from end states. Across the files of one resumed session, the order is file order and then seq, and a transition that spans two files is graded `Partial` |
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

**The shadow policy, P1 ("bridge until live learning").** This is the exact policy
v1 shadows, the one the backtest simulates, and the only one an adoption decision would
enable. Other policies are not evaluated in v1, to keep the comparison to one.

- **The seed is frozen at scan time.**
  - **What it contains:** sessions not recorded live (DEDUP-2). After install, live
    sessions never enter it.
  - **Rescans:** a rescan adds only sessions that are not recorded live and not
    already in the ledger.
  - **Currentness:** rule 4 is evaluated over the imported sessions only.
- **The seed's established set** at time `t` is the families meeting rules 1 to 4 at `t`.
  Rule 3 ages entries out against `t`.
- **The live index is unchanged.** It learns from live sessions at every Stop, exactly
  as today (`stop.go:69`).
- **Policy at a point `t`:**
  - If the live index has learned (`indexLearned(live, t)`), **the seed is ignored
    entirely** and the live policy applies unchanged.
  - Otherwise, if the seed's established set at `t` is not empty, the point is
    evaluated in learned mode: Required = established in the seed.
  - Otherwise, prior mode applies: every gate that ran is Required, as today.
- **What P1 can change.** P1 acts only while live learning has not begun, which for
  most projects is the first one or two sessions. It never adds requirements; it can
  only make non-seeded gates advisory during that window. So P1's possible benefit is
  bounded and small, and its possible harm is a loosening. The evaluation measures both.
- **The seed's lifetime is bounded.** It ends at whichever comes first: live learning
  begins, or rule 3 ages out every seed entry.

**Shadow in v1.** B3 ships P1 in shadow mode:
- **No change to verdicts.** At every Stop, Evaluate computes the live verdict and the
  P1 verdict. Only the live verdict reaches the agent, the recap and the report's verdicts.
- **A local log of differences.** Every point where the two differ is logged locally,
  digest-only, with each changed gate's family, state and Required under both policies.
  The scan report and `rashomon status` show the counts.

**Visible without changing verdicts.** The scan report can name the established checks
("established here: 3 checks, last run 2 days ago"). That delivers v1's reporting
promise whatever the adoption decision.

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
| Classify, for the seed and report | **one pass per file**, O(n). Its `runs` come back in seq order, each with its own outcome, so every transition is kept (§6.3); nothing is read from end states alone | none |
| Evaluate, for shadow verdicts and the backtest | **at every Stop point** of a session, as live does. That is O(turns × n) per session, bounded by a cap of 500 turns per session; past it, the remaining turns are not evaluated and are counted | counted, never estimated |
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

Compare per (session, family), **at every gate-run point and at every Stop point**,
not only at session end. A replay that matched end states but missed a
green→red→green episode would pass an end-state comparison. This comparison runs on
the B2 corpus offline, so per-point Evaluate is affordable there.

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
grade, are labelled from the raw transcript under rubric A (§7.3). Both live and replay
are scored against the labels, so a bug they share shows up as both being wrong.

**Exit criterion for B2:**
- zero known/known disagreements;
- an evidence-gap rate reported per reason;
- at least 95% agreement of replay with the annotations on `Verified` observations
  (the threshold is proposed and set in B0);
- DEDUP-1 and DEDUP-2 passing.

### 7.2 Shadow evaluation of P1, B3e

**What P1 can change, mechanically.** P1 acts only while the live index has not
learned. In that window, prior mode makes every gate that ran Required, and P1
Required = seed-established. So **every difference is one kind: a gate that live
requires and P1 makes advisory.** P1 adds no requirement. A difference can change four
things:
- the done verdict (`recordGreen`);
- the turn summary and value line;
- the obligation ledger;
- whether a "done" claim is checked against that gate (`claims.go:340`).

Each difference is counted mechanically, with no judgement, as a policy difference.
Whether a difference is a benefit or a harm needs judgement, and that comes from labels.

**The simulation is exactly P1.** For a project with imported history:
1. Choose cutoffs T1, T2, and so on (at most one per 30 days).
2. At each cutoff, the seed is a scan of the sessions before *T*, frozen as in §6.4.
3. The sessions after *T* are replayed in order as if Rashomon had just been
   installed: a fresh live index, updated by Observe at every Stop (`stop.go:69`).
4. At every Stop point, the live verdict and the P1 verdict are computed.
5. The P1 window ends when the replayed live index learns.

Nothing else is simulated, so the backtest evaluates the feature that adoption would enable.

**Labels (rubric B, §7.3).** Each difference becomes a question: *given the task and the
evidence available at that point, was it warranted for this check's state to block
the turn from reading verified?* The answer is one of three:
- **Yes, warranted.** P1 would have hidden a failure that mattered. **Harm.**
- **No, not warranted.** P1 would have removed an unnecessary blocking verdict. **Benefit.**
- **Can't tell.** Reported, and **counted as harm** in the safety bound.

There is no proxy label. A check that never runs again proves nothing: it could mean
abandonment, missing history, or a problem left unresolved.

**Independence.**
- **Within a session:** differences inside one session are strongly correlated. At
  most one difference per session is labelled: the first in the session that changes
  the done verdict, or else the first.
- **Within a project:** sessions inside one project are correlated too. The project
  is the cluster.

**Criteria, fixed in B0 before any data is seen:**
- **Safety.**
  - Choose a maximum acceptable hidden-failure rate *H*.
  - The one-sided 95% upper bound (Clopper-Pearson, unit: labelled session) on
    (harm + can't tell) / labelled must be below *H*.
  - **And** a project-cluster bootstrap upper bound must also be below *H*.
  - For scale: zero harms in *n* independent labels gives an upper bound of about 3/*n*.
    40 labels bound the rate at about 7.5%, *H* = 5% needs at least 60 harm-free labels,
    and *H* = 2% needs about 150. Correlation makes these minimums larger.
- **Benefit.**
  - Unnecessary blocking verdicts removed, per first-session-after-install.
  - The lower 95% bound must be above a minimum *B*, set in B0. This keeps P1 from
    being adopted for a benefit too small to matter.
- **Coverage.** At least *P* distinct projects, so one repository cannot carry the result.

**Outcomes:**
- **Adopt** only when all three criteria hold. That decision is separate, recorded
  with the evaluation report.
- **Inconclusive** (too few labels, or bounds too wide) means P1 is not adopted. The
  scan report stays.
- **Unsafe** means P1 is not adopted. A different policy, including any D38 change,
  would be its own proposal, with examples and tests.

**Expected yield, stated now.** P1's window is the first one or two sessions per
cutoff, so each project gives few labelled units, and reaching *H* = 5% needs at least
60 harm-free ones across *P* projects. On the team's own repositories alone, the likely
outcome is **inconclusive**. That is an acceptable result: v1's reporting does not
depend on it.

### 7.3 Labelling

- **Primary labeller:** a maintainer of the repository being labelled, who knows its
  intended checks. Labels for a project come from someone who knows that project, and
  are read from transcripts on the owner's machine, with the owner's consent.
- **Blinding:**
  - The labeller sees the raw transcript and the question, never which output came
    from live or replay (rubric A), nor which policy produced which verdict (rubric B).
  - Rubric B presents "a check whose state blocked this turn", without saying that
    another policy would not have blocked it.
- **Second reviewer.** Every "can't tell", and a random 20% of the rest, is labelled
  again by a second reviewer, who adjudicates disagreements. Agreement (Cohen's κ) is
  reported per rubric.
- **One session, separate rubrics.** One labelling session may cover several
  evaluations, but each has its own rubric, and rubric A is done before rubric B for a
  given session.

| Rubric | Used by | Question | Answers |
|---|---|---|---|
| **A, facts** | B2 (§7.1) | For each gate run: which check (command and directory), its outcome (pass, fail, timeout, not run, unknown); per check, the last observed state and whether the session's transcript is complete | Factual fields; no judgement |
| **B, warranted blocking** | B3e (§7.2) | Given the task (the session's prompts up to the point), the agent's final message, the checks and their states, and the writes, was it warranted for this check's state to block "verified"? | Yes / No / Can't tell, plus a descriptive reason (relevant to the change, pre-existing and unrelated, obsolete check, environment or flaky, other). The reason is reported, not used for the decision |
| **W1** | W1 detector precision | As W1 defines it | As W1 defines it |

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
- **Test failures separate from failures of other or unknown cause.** Only
  `failed(exit)` runs whose `gateout` class is an assertion or test failure count
  against a path. Timeouts (124), not run (126/127), kills, and network or
  environment output are kept as their own classes **of unknown cause**, never
  labelled infrastructure. A timeout can be a code regression, and 126/127 can be a
  change that broke permissions or removed a tool. They are reported, and counted for
  or against a path only once enough labelled evidence classifies them.
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
| **B3** | Seed file, seed rule (§6.4), **P1 in shadow only**, `rashomon scan` and `--dry-run`, consent, purge, forget, disclose | Acceptance: with a fixture history, the shadow log shows the P1 verdicts expected at each Stop; live verdicts are unchanged; once the live index learns, the shadow log shows no further differences |
| **B3e** | The P1 simulation (§7.2), labels under rubrics A and B (§7.3), and the evaluation report | The report: mechanical difference counts, the safety and benefit bounds, coverage, and κ. **No enable switch is built in v1** |
| **B4** | Report polish; `BenchmarkScanFile`, the RSS and wall-time rows (§6.7) | Perf gates on darwin and linux |

v1 is B0 to B4. It needs no RulesVersion bump, adds no steer pattern, and changes no
live verdict. Adopting P1 is a separate decision taken on the B3e report. Enabling it
would be a small follow-up change, reviewed on its own.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Seeding loosens verdicts | P1 is shadow-only in v1; every difference is a loosening, counted and labelled; adoption needs a pre-set upper bound below *H* (§7.2) |
| A small sample hides harm | Clopper-Pearson upper bound plus a project-cluster bootstrap, at most one label per session, *P* projects minimum; "can't tell" counts as harm |
| The labels are a proxy for the wrong thing | Rubric B asks whether blocking was warranted, with no "never ran again" proxy; blind labelling; adjudication; κ reported |
| The backtest evaluates a different policy from the feature | The simulation is P1 exactly (§6.4, §7.2), and B3's acceptance test checks that the shadow stops when live learning begins |
| Exit codes misread as infrastructure | 124, 126 and 127 are their own classes of unknown cause (§6.1) |
| One session counted twice | §6.2 rules, DEDUP-1 and DEDUP-2 acceptance tests, no cross-source count |
| Replay agrees with a bug it shares with live | Independent annotation (§7.1) |
| Format drift | Supported-version list; unknown versions graded `Unsupported` and never seeded |
| Upgrade trap | §6.6: rescan, stop counting what cannot be re-derived, bounded by the 30-day window |
| Memory and time | §6.7 bounds and perf gates |
| Privacy | Text stays in memory; the seed is digests; new consent unit; current project by default; purge covers it |
| Over-claiming | Report language is "last observed", "established here", and association only; the description and causal claims are kept apart |

## 11. Decisions

Recorded (revision 3):
1. B0 to B4 approved, as a bounded reporting and shadow-mode experiment.
2. Seeding stays report-only unless the §7.2 criteria are met. Inconclusive means it
   is not adopted. Any D38 change is a separate policy proposal.
3. Labelling as in §7.3.

Open, for B0:
1. The values of *H*, *B* and *P*, fixed before any data is seen. Suggested starting
   points: *H* = 5%, *B* = one unnecessary blocking verdict removed per ten first
   sessions, *P* = 8.
2. The B2 agreement threshold (95% proposed) and *K* for currentness.
3. Which repositories form the evaluation corpus, and their maintainers as labellers.
