# AutoSteer prescan: importing gate history before the first session

Status: revision 4, in B0. B0 to B4 approved as a bounded reporting and shadow-mode
experiment. Adopting seeding is a separate decision. The reader, replay validation and
scan report (B1, B2, B3a) proceed now. The evaluation protocol (§7.2) gates conclusions
about P1 only, not the reporting feature.
Date: 2026-10-29.
Inputs: a review of [Photobombastic/paxel-local](https://github.com/Photobombastic/paxel-local)
(MIT, one Python file `paxel.py`, 2,891 lines), main at `d1106a0`, and the Phase 2 lab
results on `lab/autosteer-phase2` (PR #91).

## 0. Revisions

### Revision 9: rulings on B2b

Historical replay is **validated as reconstruction, not yet as a basis for seeding.**
The paired-fixture equivalence is strong evidence that replay reproduces what the
recorder would have made of the recorded inputs. The synthetic SessionStart and the
missing rewrite evidence limit what it proves about the sessions themselves. **The key
and the confidence semantics below are correctness requirements that must be met
before any seeding.** They are not reporting details.

| Point | Ruling |
|---|---|
| Real install key | Required before B3a. The key is injected into the isolated replay explicitly. Tests show that the same key gives the live family identity, that a different key does not, and that using the real key redirects no write into live storage |
| Synthetic SessionStart | Accepted as **replay scaffolding, not evidence of complete capture**. A transcript proves the session existed; it does not prove every relevant event was kept. The marker lets the recorder evaluate. It never raises historical coverage to Verified |
| SessionEnd | Not assumed to be a kill. Its occurrence cannot be established from a transcript, so it is **unknown lifecycle evidence** (`session_end_unknown`). B2c checks whether leaving it out changes any reported metric |
| Rewrite evidence | Enforced downstream, not only flagged. While `rewrite_evidence_absent` holds, a replayed run "matches recorded inputs" and is never "verified to match executed inputs". Seed eligibility must read this distinction; report-only analysis may go ahead |
| Two kinds of confidence | **Reconstruction fidelity** (did replay reproduce the recorder's view of the recorded inputs) and **source completeness** (lifecycle, capture, rewrites) are separate fields. Neither stands in for the other |
| Replay globals | The clock variable, the Stop tap and the environment overrides must not let two replays interfere. Cleanup is certain on error, panic and cancellation, with tests (B2c) |
| fsync cost | Now a **product constraint**. A representative scan is measured before B3a counts as usable. If it is too slow, the durable recorder path stays the equivalence reference, and a faster scratch-store mode is validated separately against it |

**Added to B2c:** merging files must keep cross-file causal order:
- a subagent's calls fall between its parent call's use and result;
- each result comes after its use;
- a resume's new turns come after the turns it repeats.

Where the order cannot be established and could change a verdict, the affected points
lose confidence.

### Revision 8: what B2b established

**Equivalence holds on both fixtures.** The captured payloads and the rebuilt timeline,
each fed through the real entry points into its own isolated store under the same
injected clock, leave zero differences in order, field by field, outputs included:
- 2.1.280: 65 records, 24 outputs.
- 2.1.312: 48 records, including a regression steer note, 17 outputs.

**Normalisation is exactly two items:**
- `duration_ms`, null on replay, by ruling;
- steer line ids, which are random and are compared by ordinal.

**cwd holds through the recorder.** On 2.1.312:
- the `cd sub && …` call's Pre and Post stay associated (declaration, terminal and
  execution adjacent);
- all three unittest runs share one location digest and one gate family, live and
  replayed alike.

**Per-Stop verdicts.** Each Stop is equal to the live one, and is the StopView the
recorder itself computed. Cutting the transcript at a Stop gives the same verdict. A
result after a Stop does not reach that Stop. A Stop with a gap at or before it is
*evaluated*, never Verified, and names its first gap.

**Corrections to earlier revisions:**
- **SessionStart is synthesized, one per session, at its first event.** Without the
  liveness marker, coverage reads unverified and every verdict is empty, so revision
  6's "out of scope" was wrong. SessionEnd is never replayed: a killed session fired
  none, and replaying one would claim a clean close. Both are coverage flags:
  `probe_start_synthesized` and `probe_end_not_replayed`.
- **The host guard is pinned on replay** (entrypoint "cli", the ambient agent and CI
  variables unset). Otherwise an imported transcript's path would stamp every
  declaration `harness_id: unknown`.
- **Final messages:**
  - Replay keeps the recorder's own head and tail (256 KiB and 64 KiB) and holds at
    most 16 MiB in total for Stops not yet replayed.
  - A Stop over that budget is unsupported (`final_message_budget`) and taints what
    follows.
  - A message the live payload could not carry is `final_message_too_large`.
- **Rewrite evidence:** no transcript field records an executed input. The 2.1.312
  `wireToolInputs` differs from the model's input only by a default Claude Code adds.
  Every replayed session carries `rewrite_evidence_absent`.

**Must fix before B3a: the key.** B2b replays under a pinned key and install id, so
its digests are comparable only between replay worlds. A seed is only useful if its
family keys equal the live install's (§4). B3a must therefore replay under the real
install key:
- read-only access to the real key;
- passed in memory where possible;
- never left on disk outside the install's own store.

A test must show that a seeded family equals the family the live recorder gives the
same command.

**Owed to later items:**
- **Cost (B4):** every entry point fsyncs per event, about 3 to 5 ms an event, so a
  20,000-call session takes minutes.
- **Turn cap (B3):** §6.7's 500-turn cap is not implemented yet.
- **Interrupts (B3a):** `scan` must watch for interrupts between events, since the
  recorder's signal handling swallows them during an entry point.

**Live code touched (for reviewers):**
- `entryNow`, a package clock variable, replaces `time.Now` on the five entry paths;
  it is `time.Now` outside a replay.
- `stopSeen`, a nil tap in `recapTurn`, does nothing outside a replay.
- Three existing mutants were re-anchored to match.

### Revision 7: rulings on B2a

| Point | Ruling |
|---|---|
| `duration_ms` | Omitted on replay: the recorder stores null, as for an unmeasured call. This is an **explicit exception to record equivalence**, named in B2b's comparison. It is never estimated from transcript timestamps. A guard test fails if any non-test code outside the recorder's write path reads the stored duration, so any future use of duration in a verdict forces this ruling to be revisited |
| Subagent `agent_type` | Recovered only through an unambiguous link: the child's agent id to exactly one parent Agent call, and that call's input type. Never from message text or file names. A missing or ambiguous link stays unsupported. It is established on a fixture with **two differently typed subagents**, captured before B2c |
| Harness location | The B2b replay harness lives in `cmd/rashomon` and reuses the live Stop and catch-up code there (`recapTurn`, `stopView`), with no package extraction. Isolation (a temporary store root, nothing written outside it) and injected historical time are explicit and tested |
| Invisible input rewrites | On replay, the declared and executed inputs both come from the transcript's single input, so the recorder's rewrite check cannot fire. **A replayed Verified means "consistent with the transcript", not "equivalent to what executed"**, whenever a PreToolUse hook could have rewritten the input. Each replayed session carries a "rewrite evidence absent" coverage flag. B3e keeps such cases out of safety conclusions unless evidence resolves them |
| Final-message memory | Bounded across the whole replay, not only per Stop. Replay keeps exactly what the live recorder keeps (its head and tail capture). Where a replay cap removes evidence the live recorder would have had, the claim evaluation that needed it is downgraded with a reason, never run on truncated text |

**B2b acceptance gate:**
- ordered record equivalence with the captured-payload replay, with the documented `duration_ms` exception;
- correct cwd handling through the recorder: the `cd sub && …` Pre/Post pair stays associated, and it resolves to the same gate family and location as live;
- per-Stop verdicts that use only prior, untainted evidence.

### Revision 6: what B2a established

**The live recorder never calls `gateout`.** No live record holds an output
classification. So the replay feeds the recorder exactly the payloads it would have
received and adds nothing. Output classes from `OutputHead` and `OutputTail` are
report-only, never a record. §5.2's "through `hook.Locate`, `shape` and `gateout`"
should read "through the recorder's own entry points".

**Field completeness, from both paired fixtures (39 events, 0 unsupported).**
`internal/replay.Fields` is the table as code. `TestREPLAY5_` in `cmd/rashomon`
established "not read" by running the real entry points (`hook`, `post`, `recap`,
`probe`) in process against a store. It removed or altered each field and checked that
the whole store and every output stayed the same, under both host-guard settings.
`TestREPLAY6_` shows the rebuilt payloads and the captured ones leave identical stores
and identical outputs on both fixtures.

| Event | Rebuilt equal | Equal after normalisation | Not read | Read, not in the transcript (unsupported) |
|---|---|---|---|---|
| UserPromptSubmit | `session_id`, `cwd`, `hook_event_name` | `transcript_path` (the caller's path) | `prompt`, `prompt_id`, `permission_mode`, `scratchpad_dir` (prompt text is never rebuilt) | none |
| PreToolUse | `session_id`, `cwd` (from the use line), `prompt_id`, `permission_mode`, `tool_name`, `tool_use_id` | `tool_input` (same JSON value; Edit key order differs), `transcript_path` | `effort`, `scratchpad_dir` | `agent_id`, `agent_type` on subagent calls |
| PostToolUse | `session_id`, `cwd` (from the result line), `tool_name`, `tool_use_id` | `tool_input`, `transcript_path`, Bash `tool_response` (its two presence bits) | `prompt_id`, `permission_mode` (supplied anyway), other tools' `tool_response`, `effort`, `scratchpad_dir` | none; `duration_ms` is omitted (below) |
| PostToolUseFailure | `session_id`, `cwd`, `prompt_id`, `permission_mode`, `tool_name`, `tool_use_id`, `error`, `is_interrupt` | `tool_input`, `transcript_path` | `effort`, `scratchpad_dir` | `agent_id` on subagent calls |
| Stop | `session_id`, `cwd` (the last assistant line's), `last_assistant_message` | `transcript_path` | `prompt_id`, `permission_mode`, `stop_hook_active`, `background_tasks`, `session_crons`, `effort`, `scratchpad_dir` | none |

SessionStart and SessionEnd are out of scope: they write a liveness marker, and a
transcript has no moment for either.

**`duration_ms` is read and stored, never read back, and not in a transcript.**
B2a omits it, so the recorder stores null as it does for an unmeasured call. B2b
compares execution records without it. (Ruling pending; the alternative makes every
PostToolUse unsupported.)

**Stop rule.** A Stop is placed only after a main-agent message with
`stop_reason == "end_turn"`, with no later message, interrupt marker or unreadable line
in that turn. Anything else is an **unknown** boundary, with a reason, and it taints
what follows. The rule gives the same boundaries with every hook-written line removed
(`TestREPLAY3_`).

**Taint.** Each event carries the earliest gap at or before its position:
- an unsupported event;
- an unknown Stop;
- an evicted call;
- an unreadable line.

B2b must not call a verdict Verified when the point is tainted by a gap that could
have changed it.

**New evidence differences for §7.1:**
- A PreToolUse hook that rewrote a call's input is invisible: the transcript keeps the
  original.
- The `cwd` drifts after `cd`: the Pre's comes from the use line, the Post's from the
  result line.
- Subagent calls need `agent_type`, which no transcript line holds. It may be
  recoverable by joining the sidechain's `agentId` to the parent's Agent call, whose
  result names the agent and whose input names the `subagent_type`. That needs a
  subagent fixture first.

### Revision 5

| Point | Ruling | Where |
|---|---|---|
| Use order and result order | Each call carries `UsePos` and `ResultPos`. The replay places declarations and executions by position, and ORDER-1 says no point sees a later result | §6.1, §9 B2 |
| Project-level safety is its own quantity | One cutoff per project by a fixed rule, and a fixed observation window. A harmful project is one with any harmful evaluated session in that window. The 59-project figure bounds the proportion of harmful projects, not the session harm rate. The design-effect estimate stays secondary | §7.2 |
| Current-format coverage | A paired 2.1.312 fixture before B2, with a passing check, a genuine failure, a recovery, a write and a Stop, and a test that the scrub kept call ids, order, cwd relations and result semantics | §9 |

### Revision 4: B0 rulings

| Point | Ruling | Where |
|---|---|---|
| P1's duration is conditional | Eligibility is re-evaluated at every point against the frozen observations, so the seed never extends its own window. P1 ends when the live index learns or when the newest imported `Verified` run of every seed family is more than 30 days old, whichever is first. The evaluation reports P1's actual duration in sessions and in days | §6.4 |
| The seed rule | It mirrors the live rule exactly: two distinct `Verified` sessions, the last within 30 days. Revision 3's "a verified green" and "current within *K*" rules are dropped from P1 and become a separate hypothesis, H-cur, that is never tuned on the evaluation corpus | §6.4 |
| One label per session can miss harm | The labelled unit is the session, and **every** affected check in it is inspected. A session is harmful if any warranted block was removed. A sampled-difference rate, if reported, is labelled as such and makes no session-level claim | §7.2 |
| Zero events and clustering | The bootstrap is dropped. The primary bound is at project level: a project is harmful if any of its sessions is, with Clopper-Pearson over projects. A secondary session-level bound uses a design effect with an intra-project correlation fixed in advance, never estimated from the data | §7.2 |
| Caps and coverage | A Stop beyond the 500-turn cap is **unevaluated**. A session with any unevaluated Stop, or any evidence below `Verified`, inside its P1 window is excluded from the safety count, never counted as harmless, and is reported as coverage loss | §6.7, §7.2 |
| *H* | *H*<sub>screen</sub> = 5% is an exploratory screen only. *H*<sub>adopt</sub> is chosen by the owners from the consequences of a missed block. The backtest cannot meet it on its own: adoption also needs a prospective evaluation | §7.2 |
| *B* | 0.1 removed unnecessary blocks per eligible first session, with every eligible first session in the denominator, including those with no differences | §7.2 |
| *P* | 8 projects for the pilot. Projects from outside the team are required before any broader adoption | §7.2 |
| Replay agreement | Exact agreement on supported, fully observed (`Verified`) cases; any reproducible mismatch blocks B2 until it is resolved. Unsupported evidence is reported as coverage loss, separately | §7.1 |
| Backtest vs behaviour | The report states that a backtest estimates which recorded blocking verdicts P1 would remove. It does not show how agents would behave without those blocks | §7.2 |

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

**Use order and result order are different (revision 5).** B1 emits calls in
`tool_use` order, which is the right order for identity. But the evidence a point can
see depends on when each result **became available**. Calls overlap (parallel tool
uses, background runs), so one sequence number per call cannot carry both. Each call
therefore carries two positions in its file:
- `UsePos`, where its `tool_use` block is;
- `ResultPos`, where its `tool_result` block is (none when `Missing`).

Each is (line, block index within the line). The replay (B2) builds its record from
these positions, never from the call order:
- a declaration at `UsePos`, as PreToolUse writes one before the call runs;
- an execution at `ResultPos`, as PostToolUse writes one after it;
- a Stop at the position where its turn ends, defined in B2 and checked against the
  Stop payloads of both paired fixtures.

**Invariant ORDER-1:** a point at position *p* (a Stop, or a gate run's execution)
sees exactly the records whose position is before *p*. **No point ever sees a later
result.** A call whose result comes after a Stop is, at that Stop, a declaration
without an execution, as it would be live. ORDER-1 is an acceptance test in B2, with
fixtures of overlapping calls whose results arrive on either side of a Stop and of a
write.

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

The seed rule mirrors the live rule exactly (B0 ruling), applied to `Verified` imported
evidence only. A family is seed-established at point `t` when both hold:
1. `sessions_verified >= 2` (the live `RequiredSessions`), **and**
2. its newest `Verified` run is within 30 days of `t` (`RequiredWindowMS`, inclusive,
   as live).

The `Verified` grade is a data-quality filter, not a policy change.

**H-cur, a separate hypothesis.** Stricter rules, such as "a verified green exists" or
"the family ran in one of the last *K* imported sessions", are not part of P1. They are
a separate hypothesis, H-cur. They are not tuned on the evaluation corpus, and they
would be evaluated as their own policy.

**The shadow policy, P1 ("bridge until live learning").** This is the exact policy
v1 shadows, the one the backtest simulates, and the only one an adoption decision would
enable. Other policies are not evaluated in v1, to keep the comparison to one.

- **The seed is frozen at scan time.**
  - **What it contains:** sessions not recorded live (DEDUP-2). After install, live
    sessions never enter it.
  - **Rescans:** a rescan adds only sessions that are not recorded live and not
    already in the ledger.
- **The seed's established set** at time `t` is the families meeting rules 1 and 2 at
  `t`. It is re-evaluated at every point against the frozen observations. **Frozen
  membership is never permanent eligibility:** rule 2 ages each family out 30 days
  after its newest imported `Verified` run, and nothing in P1 refreshes that date.
- **The live index is unchanged.** It learns from live sessions at every Stop, exactly
  as today (`stop.go:69`).
- **Policy at a point `t`:**
  - If the live index has learned (`indexLearned(live, t)`), **the seed is ignored
    entirely** and the live policy applies unchanged.
  - Otherwise, if the seed's established set at `t` is not empty, the point is
    evaluated in learned mode: Required = established in the seed.
  - Otherwise, prior mode applies: every gate that ran is Required, as today.
- **What P1 can change.** P1 never adds requirements; it can only make non-seeded gates
  advisory while it is active. So its possible harm is a loosening, and the evaluation
  measures both the harm and the benefit.
- **P1's duration is conditional.** P1 ends at whichever comes first:
  - the live index learns (two live sessions of one family within 30 days), or
  - every seed family has aged out by rule 2, so the seed's established set is empty.

  In a project that runs its checks in most sessions, that is one or two sessions. In
  a project with sparse sessions, P1 can stay active for up to 30 days after the newest
  imported `Verified` run, and never longer. The evaluation reports the distribution of
  P1's duration in sessions and in days, rather than assuming it.

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
| Evaluate, for shadow verdicts and the backtest | **at every Stop point** of a session, as live does. That is O(turns × n) per session, bounded by a cap of 500 turns per session | Stops past the cap are marked **unevaluated**, never estimated. A session with any unevaluated Stop in its P1 window is excluded from the safety count and reported as coverage loss (§7.2) |
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

**Exit criterion for B2 (B0 ruling):**
- **Exact agreement** between replay and live on every supported, fully observed
  (`Verified`) case. A reproducible mismatch blocks B2 until it is resolved: as a
  bug fixed on one side, or as an evidence difference added to the table above, with
  the state replay must give for it.
- **Exact agreement with the annotations** on `Verified` observations, after
  adjudication. Each disagreement is resolved as an implementation bug, which is fixed,
  or a label error, which the second reviewer corrects. None stays unresolved.
- **Unsupported and `Partial` evidence** is reported as coverage loss per reason, and
  kept separate from agreement.
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
1. **One cutoff per project**, chosen by a rule fixed before any data is seen (revision
   5): *T* is the start of the latest session such that the seed built from the
   sessions before *T* has a non-empty established set and at least one session starts
   at or after *T*. This is the most history a real install would have. Several
   cutoffs per project would make "a harmful project" depend on how many were taken,
   so there is one.
2. At the cutoff, the seed is a scan of the sessions before *T*, frozen as in §6.4.
3. The sessions after *T* are replayed in order as if Rashomon had just been
   installed: a fresh live index, updated by Observe at every Stop (`stop.go:69`).
4. At every Stop point, the live verdict and the P1 verdict are computed.
5. **The observation window** is from *T* to the end of P1: the replayed live index
   learns, or every seed family ages out, and at most 30 days after *T* in any case.
   Sessions after the window are not evaluated.

Nothing else is simulated, so the backtest evaluates the feature that adoption would enable.

**Eligible first sessions.** A session is eligible when it falls in a P1 window: after
a cutoff, before the replayed live index learns, and while the seed's established set
is not empty.
- **Fully evaluated:** every Stop in the window evaluated, and every observation in it
  `Verified`.
- **Coverage loss:** any other eligible session. It is excluded from the safety count
  (never counted as harmless), counted as zero benefit, and reported with its reason.

**Labels (rubric B, §7.3). The unit is the session.** For each fully evaluated eligible
session with at least one difference, **every** affected check is inspected. That is
every (session, check) pair where live required the check and P1 made it advisory, at
any Stop. Each pair gets one answer to *given the task and the evidence available at
that point, was it warranted for this check's state to block the turn from reading
verified?*:
- **Yes, warranted.** P1 would have hidden a failure that mattered.
- **No, not warranted.** P1 would have removed an unnecessary blocking verdict.
- **Can't tell.**

The session is **harmful** if any of its pairs is "yes" or "can't tell". Its **benefit**
is the number of its pairs answered "no" that changed a done verdict. A session with no
differences is harmless and has zero benefit. It still counts in both denominators,
because P1 changed nothing there.

There is no proxy label. A check that never runs again proves nothing: it could mean
abandonment, missing history, or a problem left unresolved. If a sampled-difference rate
is ever reported (one pair per session, for cost), it is labelled as a rate over
sampled differences and makes no session-level safety claim.

**Clustered, rare outcomes.** With zero observed harms, resampling gives zero in every
sample, and Clopper-Pearson assumes independent trials, which sessions of one project
are not. So the method is fixed now:
- **Primary: the proportion of harmful projects.** Under the protocol above (one
  cutoff, one fixed observation window), **a project is harmful if any harmful session
  is among its fully evaluated eligible sessions in that window.** The primary quantity
  is the proportion of harmful projects under this protocol, bounded with a one-sided
  95% Clopper-Pearson over projects, which are independent repositories. With zero
  harmful projects out of *P*, the bound is about 3/*P*.
  - **This is not a session harm rate.** It answers how likely it is that a project,
    observed this way, has at least one harmful session. Its value depends on the window
    and on how many eligible sessions a project has, which is why both are fixed.
  - A project with no fully evaluated eligible session in its window is not counted in
    *P*. It is reported as coverage loss, never as a harm-free project.
- **Secondary: a session-level bound with a fixed design effect.** Use
  *n*<sub>eff</sub> = *n* / (1 + (*m̄* − 1)·*ρ*), where *m̄* is the mean number of
  eligible sessions per project and *ρ* = 0.5 is fixed in B0, never estimated from the
  data (with zero events it cannot be). Then Clopper-Pearson on *n*<sub>eff</sub>.
  This is the only estimate of the session harm rate. It is reported, stays secondary,
  and is never used alone or in place of the primary.

**Thresholds (B0):**
- **Safety screen.** The primary bound must be below *H*<sub>screen</sub> = 5%. This is
  an exploratory screen, not an adoption bar.
  - Zero harmful projects in 59 gives a bound of 4.95% on **the proportion of harmful
    projects** under this protocol. That says nothing directly about the session harm
    rate, which only the secondary estimate addresses. So the screen needs at least 59
    independent projects with at least one fully evaluated eligible session each, far
    more than the pilot's *P* = 8.
  - **With the pilot alone, the safety screen cannot pass.** It can only fail (a harmful
    project is found) or be inconclusive. This is stated so nobody reads a pilot result
    as a pass.
- **Adoption bar *H*<sub>adopt</sub>.** Set by the owners from the consequences of a
  missed warranted block, before any data is seen. It is not proposed here. A
  backtest alone cannot meet it (see "What a backtest shows", below).
- **Benefit floor *B* = 0.1.** Removed unnecessary blocks (pairs answered "no" that
  changed a done verdict) per eligible first session. **Every** eligible first session
  is in the denominator: those with no differences, and those with coverage loss
  counted as zero. The one-sided 95% lower bound must be above *B*, with sessions
  clustered by project (the same design effect).
- **Coverage *P* = 8 projects** for the pilot. Projects outside the team are required
  before any broader adoption.

**What a backtest shows.** It estimates which recorded blocking verdicts P1 would have
removed, and whether a person judges those blocks warranted. It does not show how an
agent would have behaved without those blocks, for example whether it would then have
claimed "done" and stopped. Any adoption therefore also needs a **prospective
evaluation** under the randomised holdout (`internal/arm`), designed as its own study.
The evaluation report carries this paragraph verbatim.

**Outcomes of B3e:**
- **Screen fails:** a harmful project is found. P1 is not pursued. A different policy,
  including any D38 change, would be its own proposal.
- **Inconclusive:** too few projects, or the benefit bound is below *B*. This is the
  expected pilot result. P1 is not adopted, and the scan report stays.
- **Screen passes:** at least 59 independent projects, none harmful, and the benefit
  bound above *B*. That justifies proposing a prospective study, not adoption.

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
| **B0** | Rulings: the protocol in revision 4 (done here); still open for B3a: the consent unit, the seed path, whether `history` may import `verify` internals (Q0-style), and the fixture licence notice if paxel fixtures are reused (B1 uses synthetic fixtures of its own, so it needs none) | Written here and in APPROACH-NOTES |
| **B1** | `internal/history` Claude reader: `HistCall`, id join, outcomes, grades (§6.1), version list, the bounds in §6.7, `FuzzHistCalls`, scrubbed fixtures | Fuzz floor +1. Property: each `tool_use` gives exactly one `HistCall`, and no `Missing` call is ever `ok` |
| **B2** | Positions on each call (`UsePos`, `ResultPos`, §6.1); replay through `hook.Locate`, `shape` and `gateout` into records placed by position; Stop points; per-session summaries; merge and ledger (§6.2) | §7.1 criteria against both paired fixtures (2.1.280 and 2.1.312); ORDER-1; DEDUP-1 and DEDUP-2 over the §6.2 fixtures, including scanning the same tree twice and a tree with copies, resumes and subagent files |
| **B3a** | Seed file and import ledger, `rashomon scan` and `--dry-run` report (§6.5), consent, purge, forget, disclose | Acceptance: a fixture history gives the expected established families, grade counts and coverage. Scanning twice gives an identical seed file |
| **B3b** | P1 in shadow only (§6.4) | Acceptance: with a fixture history, the shadow log shows the P1 verdicts expected at each Stop; live verdicts are unchanged; once the live index learns, or every seed family ages out, the shadow log shows no further differences |
| **B3e** | The P1 simulation (§7.2), labels under rubrics A and B (§7.3), and the evaluation report | The report: mechanical difference counts, the safety and benefit bounds, coverage, and κ. **No enable switch is built in v1** |
| **B4** | Report polish; `BenchmarkScanFile`, the RSS and wall-time rows (§6.7) | Perf gates on darwin and linux |

**Status of B1.** `internal/history` reads one transcript file:
- It joins each `tool_use` to its `tool_result` by id and emits calls in `tool_use`
  order.
- It sets outcomes and grades per §6.1 and enforces the §6.7 limits, each of which
  degrades a grade rather than growing memory.
- It shares the report's denial rule (`report.IsDenial`).
- It is tested against the real 2.1.280 fixture, with `FuzzHistCalls` and 13 mutants
  (`HIST-1`).

Owed:
- **A fixture for the current Claude Code version.** Only 2.1.280 is supported, so
  transcripts from later versions (2.1.312 is current here) grade `Unsupported` until
  a scrubbed fixture of each is added.
- **The `BenchmarkScanFile` and RSS rows,** which are B4's.

v1 is B0 to B4. It needs no RulesVersion bump, adds no steer pattern, and changes no
live verdict. **Order:**
- **Proceeding now:** B1, B2 and B3a, the independently useful reporting path.
- **Waiting on the protocol:** B3b and B3e wait only for the §7.2 protocol to be frozen
  (*H*<sub>adopt</sub>, *ρ*, the corpus and its labellers). The protocol gates
  conclusions about P1, not the reporting feature. Adopting P1 is a separate decision taken on the B3e report. Enabling it
would be a small follow-up change, reviewed on its own.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Seeding loosens verdicts | P1 is shadow-only in v1; every difference is a loosening, counted and labelled; adoption needs a pre-set upper bound below *H* (§7.2) |
| A small sample hides harm | The session is the labelled unit, with every affected check inspected; a primary project-level Clopper-Pearson bound; a secondary design-effect bound with *ρ* fixed in advance; "can't tell" counts as harm; the pilot can only fail or be inconclusive |
| Truncated or partial sessions counted as harmless | Unevaluated Stops and evidence below `Verified` are coverage loss, excluded from the safety count and reported |
| A backtest read as a behavioural result | The report states its limit verbatim; adoption needs a prospective holdout study |
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

Recorded:
1. B0 to B4 approved, as a bounded reporting and shadow-mode experiment (revision 3).
2. Seeding stays report-only unless adopted separately. Inconclusive means not
   adopted. Any D38 change is a separate policy proposal (revision 3).
3. Labelling as in §7.3 (revision 3).
4. The B0 rulings in revision 4: P1's duration and seed rule, H-cur as a separate
   hypothesis, session-level labels, project-level bounds, coverage exclusions, *H*,
   *B*, *P*, and exact replay agreement.

Open; these gate B3b and B3e only:
1. *H*<sub>adopt</sub>, set by the owners from the consequences of a missed block.
2. Confirming *ρ* = 0.5 for the secondary bound.
3. The evaluation corpus: the pilot's 8 projects, the route to projects outside the
   team, and each project's maintainer as labeller.
