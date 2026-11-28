# Transcript learning: design, second version

Owner: Luis. Status: v2, for review. Date: 2026-11-28.
v1 is commit 30238061 on this branch. Review: Karthik's comment on #379.
Issue 204 is answered in section 7.

Built so far: PR 1 #381 (`internal/trace`, Claude Code adapter, `audit` byte-identical) and PR 2
#382 (`internal/episode`).

## 0. The question, and how the design answers it

**Can AutoSteer measurably reduce repeated mistakes for developers who already have sophisticated
hooks and workflows, without making the agent worse overall?**

- Every lesson is a hypothesis about one recurring mistake.
- For every (lesson, session) pair where the lesson's trigger fires, a keyed coin decides whether the
  lesson is shown or held back. The trigger fires the same way in both arms.
- One rule scores both arms the same way, over the same window: did the mistake recur?
- The answer is the difference between the arms, pooled across lessons, with an interval that
  respects clustering by developer and repository.
- It is reported beside guardrails: task completion, time, tokens and human corrections. A
  reduction bought by making sessions worse is not a yes.
- A user's own hooks run in both arms, so the difference is AutoSteer's effect on top of their
  setup.

What is never evidence:
- **A green test after a lesson.** It scores the task, not the lesson.
- **A falling count.** A mistake seen three times often does not show a fourth with no lesson
  at all (regression to the mean).
- **A treated share on its own.** Only the comparison with held-back pairs counts.

## 1. What changed from v1, and why

| v1 | v2 | Reason |
|---|---|---|
| The effect is computed only over sessions with an opportunity | The effect counts every pair where the trigger fired (intention to treat); the opportunity rate per arm is reported beside it | A lesson can change whether the opportunity happens (point 2) |
| Label written once, at the Stop of the delivering turn | The label only moves up over the whole session and is appended where it moves. A lesson is given again after each compaction. Tier decisions read only closed sessions | A session has many Stops and can run for days (point 7) |
| 25% holdout; looks counted in treated outcomes; a futility rule | 50% holdout while on probation, 10% once proven; looks counted by the smaller arm. "Inert" parks a lesson and never retires it. An offline pooled fit | Power is about 4 in 10 at the last look; futility as written almost never fires (points 3, 4 and goal 4) |
| Newcombe and Mantel–Haenszel intervals | A cluster bootstrap over developers, stratified by repository. Below 8 developers, the interval is reported as unavailable, not narrow | Sessions of one developer are not independent (point 4) |
| Drain templates | A digest of the fixed words of one line under a versioned masking rule, computed at mining time | Drain depends on order and widens over time (point 6). Built in PR 2 |
| `learn propose --model` inside the binary | The binary writes request files; a plugin skill calls the model and writes response files | No network code in the binary (point 8, hard line) |
| Content under `learn/` not on the consent screen | Every kept item is named, purged and secret-checked (section 10) | Point 9, hard line |
| PR 4 described as "no behaviour change" | PR 4 is the one release-level consent change, with a version Karthik assigns | Point 10, hard line |
| Mining at SessionEnd from the first line | Mining resumes from a mark, under a time budget | Transcripts reach 270 MB (point 11) |
| Sessions only | Team history (git, reviews, tickets, wiki), arriving as files, plus one brief that gathers everything known | Point 1 and goal 1: part of the loop |
| Lessons only | Every note we ship is measured the same way, with guardrails | Goal 2 and question E |
| Budget fix PR 3 | Dropped: the charge does not happen (`decide.go:340`, `budget.go:270`) | Karthik's read was right |

## 2. The loop, end to end

```
 sources ──► trace ──► episodes ──► candidates ──► lessons (approved) ──► brief / trigger ──► agent
   │  (sessions,         (pure        (gate:          (knowledge rows,        (one ranked     │
   │   git, reviews,      detectors,   3 episodes,     tiers, keys,            brief; trigger │
   │   tickets, wiki)     keys)        3 sessions,     test)                   notes; arms)   │
   │                                   2 days)                                                ▼
   └──────────────◄── effects.json (offline fit) ◄── labels (both arms) ◄── record + trace ◄──┘
```

Every arrow but the last is offline or at index time. The hook path does three things only: read
`active.json` and `effects.json`, match triggers, and route notes. No model runs in a hook.

## 3. Sources

### 3.1 Session sources (built)

`internal/trace` is the agent-neutral step model. Its adapter, `trace/claudecode`, reads Claude Code
transcripts: `Read` is lean, for `audit`; `ReadContent` adds commands, paths, changes and outputs. A
second source, `trace/record`, reads our own record plus the session log, and arrives with PR 4.

`Step.Origin` says who wrote a line in the person's slot: the person (typed), a peer (another
session), the machine (the client, a hook or a command) or unknown. This is point 12. Pushback
counts only `OriginPerson`.

**One reader.** Karthik's compaction-instruction reader should be built on `trace/claudecode`, not
beside it. The census of record shapes then sets the adapter's Origin rules, including for versions
older than the `origin` mark, in one place.

### 3.2 Team-history sources (new)

These are part of the loop. A separate program, outside the binary (hard line), writes each source
as files the binary reads:

```
.rashomon/sources/<source>/<id>.json      (in the repository, if the team commits them)
<store>/sources/<project>/<source>/<id>.json   (or local only)
```

One neutral record shape for every source:

```json
{"v":1,"source":"review","id":"pr-412#c9","at":"2026-10-02T11:04:00Z",
 "paths":["services/auth/auth/sessions.py"],"commit":"9f2c…",
 "text":"…","links":["ticket:AUTH-88"],"author_role":"reviewer"}
```

| Source | Written by | What becomes an episode |
|---|---|---|
| `git` | an exporter over `git log` | A commit, and a later commit within 7 days that reverts it (git's own `Revert` trailer or an inverse diff) or touches the same lines again. Kind `history_revert` |
| `review` | an exporter over the forge's API | A review comment that asked for a change, on paths. Kind `review_finding`, fingerprinted like an output line, scoped by paths |
| `ticket` | an exporter over the tracker | A bug ticket that links a commit or names paths. Kind `incident` |
| `wiki` | an exporter over pages | No episodes. Decisions and conventions become knowledge rows (kind decision or convention), with applies_to from the paths they name |

`Episode.Source` gains `git`, `review`, `ticket` and `wiki`.

### 3.3 How a lesson from a review or a ticket gets tested

In v1, a lesson with no key could only ever be `unresolved`. In v2 every lesson declares one `Test`:

| Test | Meaning | Scored where |
|---|---|---|
| `session_key` | An episode key, as in section 4 | Inside the session (section 7) |
| `source_recurrence` | A key whose recurrence shows up in a source: the same review-finding fingerprint, in scope, on the pull request the treated session's commits went into | When that pull request's review file arrives. The session is linked to the pull request by the commits its record holds (`head_after` in record v5) |
| `predicate` | A spec rule the lesson names (the existing rule kinds) | Inside the session, by the rule's check |
| `none` | Written by hand with nothing testable | Always `unresolved`, and it never moves a tier, as v1 said |

So a lesson that comes from a review is tested where reviews happen: on the next pull request.

## 4. Episodes

### 4.1 Kinds (built in PR 2, extensible)

| Kind | Source | Signal |
|---|---|---|
| `act_failure` | session | An act failed, and the session ran the same kind of act again |
| `check_regression` | session | A command passed, a file was edited, and the same command failed |
| `correction` | session | A refusal, a rule block, an interrupt after an act, or typed pushback (with a lexicon passed in as data) |
| `self_revert` | session | An edit put back an earlier edit's text |
| `retry_loop` | session | An identical call retried after an identical failure, twice or more |
| `unbacked_claim` | record | A done claim with no check after the last edit (the verifier's existing reading); arrives with `trace/record` |
| `history_revert`, `review_finding`, `incident` | sources | Section 3.2 |
| `conflict` | graph | The engine's Check Conflicts (boundary crossed, decision contradicted, protected path, duplicate abstraction), keyed by rule id and scope |

### 4.2 Beyond operational failures (question A)

The core treats a kind as a plug-in with three functions:

```go
// package episode
type KindRule interface {
    Kind() Kind
    // Opportunity: the step gives the session the chance to make this mistake.
    Opportunity(k Key, s StepView) bool
    // Recurrence: the step is this mistake again.
    Recurrence(k Key, s StepView) bool
}
func Register(r KindRule)
```

The miner groups episodes by key, the label rule dispatches on the kind's rule, and the brief ranks
lessons by key. None of them change when a kind is added.

Semantic mistakes enter as new kinds whose detector reads a richer source:

| Semantic mistake | Kind | Detector reads | Opportunity and recurrence |
|---|---|---|---|
| Architectural violation | `conflict` | The engine's conflicts against the Platform graph | An edit in the rule's scope; the same rule's conflict raised after delivery |
| Disregard for a convention | `review_finding` with a convention row | Reviews, plus the Knowledge DAG | An edit in scope; the same finding on the next pull request |
| An incorrect assumption | `assumption` (later) | The agent's stated fact (text) against the record or graph, e.g. "X has no callers" when the graph has some | Text that names the same fact; the record contradicting it |

### 4.3 Keys and fingerprints (built)

A key is `{V, Kind, Act{Class, Program, Verb}, Scope{Dir, Ext}, Detail, FP, FPV}`.

The fingerprint is a digest of one line's fixed words under a versioned masking rule. That line is
the most specific one (the most fixed words) that no passing run of the same act printed; ties go to
the later line. With no baseline, which is a program's first run, no line is excluded.

It is stateless and order-free, and it means the same thing on any machine. Keys are computed when
mining and never frozen into storage. The baseline is rebuilt at mining time from the session log,
fed oldest first.

## 5. Lessons

### 5.1 Mining (offline)

`rashomon learn mine` reads sources, runs the detectors, groups by key and applies the existing
`learn.Candidate` gate: 3 episodes, 3 sessions, 2 days, consistency at least 0.75, no contradiction.
It writes proposed lessons.

**At SessionEnd (point 11)**, mining resumes from a mark per source file: path, size, mtime and the
byte offset reached. It stops at a 2-second budget and leaves the rest for the next end or for
`learn mine`. A file whose size shrank or whose mtime went back starts again from its beginning;
that is the only case of starting over.

### 5.2 A lesson is a knowledge row

Lessons stay in the signed ledger (tiers, approval, quarantine), and each one is mirrored as a
knowledge row (kind lesson) in the judgment store. That way the brief and the graph traverse them
like decisions (section 12). Lesson v2 gains:

```go
Key        *episode.Key   // nil only when Test is predicate or none
Test       Test           // session_key | source_recurrence | predicate | none
Mode       Mode           // preventive | corrective
Episodes   []string
Sources    []string       // ids of source records, when mined from one
TextSource string         // template | model | human
Parked, Retired …          // section 8
```

### 5.3 Wording

The default is a statement-of-fact template filled from the key and the resolution, inside the render
rules (no imperatives, no "you").

A model can reword it outside the binary (hard line, point 8):
1. `rashomon learn propose` writes `requests/<lesson>.json` (the episodes and template text, already
   secret-checked).
2. A plugin skill (`/autosteer:propose`) has the person's own agent write `responses/<lesson>.json`.
3. `rashomon learn ingest` reads the response and runs it through `learn.Message`'s checks. The
   result still needs a person's approval.

## 6. Delivery

### 6.1 Moments, triggers and modes

As in v1:

- **Moments.** Notes reach the model mid-turn only through additionalContext after PostToolUse,
  after PostToolUseFailure (from 2.1.304), and with a prompt typed into a running turn.
- **Modes.** A preventive lesson is delivered at the first opportunity signal; a corrective one at
  the first matching failure.
- **Matching** is a pure `learn.Trigger`, and delivery goes through `steer.Route`.

New in v2:

- **Re-delivery after compaction.** A compaction boundary (`events.Boundary` with form `compact`)
  re-arms every lesson given earlier in the session. It is delivered again at its next trigger, and
  the arm stays fixed for the session.
- **A held-back lesson charges the window like a delivered one** (point 5). The held-back line is
  written with `suppressed_by: arm_off`, which `countBudget` already counts (`budget.go:270`). So
  both arms get the same later notes, and the arms differ in the lesson alone.
- **At most 5 lessons on probation per repository at a time**, ranked by expected effect from the
  offline fit. That bounds what unproven lessons spend of the note budget.

### 6.2 How a note can break the agent's train of thought

The v1 list stands, minus item 8 (the budget charge does not happen). Two of its rules change notes
that ship today: the quiet point (item 1) and the test-first exclusion (item 4). They land on the
harness side as their own changes, measured before and after.

## 7. Labels (issue 204)

### 7.1 The pair and its window

The unit is a **(lesson, session) pair where the trigger fired**. The trigger fires the same way in
both arms, because it reads only what happened before the lesson could act. That gives equal groups
regardless of what the lesson does afterwards (point 2).

The window runs from the first trigger to the end of the session. A session is **closed** when its
SessionEnd is recorded, or when it has been idle for 24 hours. Only closed pairs move tiers.

### 7.2 The label rule `learned-fact/2`

Over the window:

- **Recurrence** (the kind's `Recurrence` holds): `refuted`.
- **Else an opportunity** (the kind's `Opportunity` holds after a trigger): `confirmed`.
- **Else:** `unresolved`.

The label **only moves up**: unresolved, then confirmed, then refuted. A row is appended at each Stop
where it moves, and the latest row per pair is the label. So a mistake in turn 9 refutes a lesson
given in turn 3, and a lesson given again after a compaction keeps its pair.

For the **effect**, a pair is scored as recurred or not recurred whatever its opportunity, so an
unresolved pair counts as "did not recur". Opportunity is reported separately (section 8.2). Tier
rules read the effect, never the treated confirmed share.

R7 holds as before: the rule never reads written paths in order to confirm.

### 7.3 The label row v3

```json
{"v":3,"lesson_id":"L7","session_id":"…","at_unix_ms":0,"seq":412,
 "label":"refuted","rule":"learned-fact/2","evidence_seq":412,
 "arm":"holdout","mode":"preventive","trigger_seq":398,
 "given":[398,1210],"opportunities":2,"closed":false}
```

`given` lists the seqs where the lesson was shown, or would have been, including after compactions.

The pin test changes in the same PR, red first. Karthik will amend the issue-204 answer to Q2 to
match.

## 8. Statistics

### 8.1 Allocation and looks (points 3 and 4)

| Lesson state | Holdout share | Why |
|---|---|---|
| probation | 50% | Equal arms. About 81 pairs per arm detect 0.4 → 0.2 at 80% power and α 0.05 (two-sided); 25% would need about 217 pairs in all |
| proven | 10% | Keeps watching for decay at a small cost |
| parked | 100% (not delivered) | Section 8.3 |

**Looks** happen at 20, 40 and 80 pairs in the **smaller arm**, at α 0.05/3 each.

### 8.2 Effect, with clustering

For one lesson:

- **Effect** = recurrence(holdout) − recurrence(treated), over closed pairs (intention to treat).
- **Opportunity rate** is reported per arm beside it. If the treated opportunity rate is lower and
  the key's act is a check (a command the repository runs with pass/fail, as `check_regression`
  defines it), that is flagged as **avoidance harm**: the agent stopped running the check.

**Intervals** come from a cluster bootstrap that resamples developers (install keys), with
repository strata kept. With fewer than 8 developers, the per-lesson interval is reported as "too
few developers for an interval" rather than as a narrow one.

**Pooled across lessons** (the feature-level answer), the estimate is the stratified difference,
with the same bootstrap.

### 8.3 Decisions

| Transition | Rule |
|---|---|
| probation → proven | At a look, the lower bound of the effect is above 0 and the avoidance flag is off |
| any → demoted | At any look, the upper bound is below 0 (harm), or the avoidance flag is on |
| probation → parked | The interval sits inside (−0.10, +0.10), which needs about 160 pairs per arm, or there is no lower bound above 0 after 160 pairs in the smaller arm |
| any → retired | The scope is gone, the lesson is superseded, or a person retires it |

**Parked is not retired** (point 4: leakage pushes effects to zero, so a zero does not prove a lesson
failed). A parked lesson stops costing note budget, stays in the ledger, and can be re-armed by a
person or when its key's recurrence rate rises.

**Leakage** is measured as well as possible. At session start the adapter reads whether a lesson's
fixed words appear in the session's instruction files (CLAUDE.md, AGENTS.md, memory). It keeps a
yes/no per lesson, never the text. Held-back pairs with leakage are reported separately.

### 8.4 Small samples: the offline pooled fit (goal 4)

`rashomon learn fit` runs offline and writes `effects.json`. Each lesson's effect is shrunk toward
the pooled effect of its kind with an empirical-Bayes beta-binomial (one prior per kind, fitted on
all lessons). The posterior lets the system:

- **promote early**, when P(effect > 0) ≥ 0.975 with at least 20 pairs in the smaller arm;
- **park early**, when P(|effect| < 0.10) ≥ 0.9.

The hook reads only the file. The fit's rule, version and thresholds are pre-registered.

### 8.5 Base rates first

The replay benchmark (section 9) gives each kind's recurrence base rate and how many candidates
pass the gate, from real history. The experiment in section 14 is sized from those numbers, not
from an assumed 0.4.

## 9. The replay benchmark (early, before live delivery)

`rashomon learn replay [--days N]` reads transcripts the way `audit` does: in memory, through
`trace/claudecode`, with nothing stored and no consent change. It prints:

- episodes per kind;
- keys seen in 1, 2 and 3 or more sessions;
- the candidates that pass the gate;
- each candidate's lesson text;
- the base rate at which a key recurs in a later session, given that it occurred.

`--label FILE` writes a blind sample of episodes and candidates to a file the person names. The file
holds no lesson id and no arm, and people mark each one: real mistake or not, useful lesson or not.
`--score FILE` reads the marks back. That gives precision before anything reaches an agent.

The benchmark runs on our own transcripts first, then on design partners' with their consent. It
replaces the dropped PR 3.

## 10. What is kept, and where (hard line, point 9)

Every kept item is named on the consent screen, removed by `rashomon purge`, and passed through the
same secret check as a note (`secretshape`) before it is stored or shown.

| Item | Where | Holds | Cap |
|---|---|---|---|
| Session log: commands, output tails, diffs, prompts, notes | `graph/<rid>.jsonl` (exists) | Content | Existing caps |
| Episodes | `learn/<project>/episodes.ndjson` | Keys, seqs, the masked template, resolution words and paths, the resolution diff (capped at 8 KiB) | 16 MiB |
| Lessons | `learn/<project>/lessons.json`, `active.json` | Lesson text (command words, template words) | As today |
| Labels | `learn/<project>/labels.ndjson` | Ids, seqs, arms | 16 MiB |
| Effects | `learn/<project>/effects.json` | Numbers | 1 MiB |
| Mining marks | `learn/<project>/marks.json` | Paths, sizes, offsets | 256 KiB |
| Source files | `sources/<project>/…` (if local) | Review, ticket and wiki text | 64 MiB |
| Propose requests and responses | `learn/<project>/requests/`, `responses/` | Episode text for the model step | 4 MiB |
| Other hooks (covariate) | In the session's record | The count of non-rashomon hook entries per event, and a keyed digest of each command, never its text | — |
| Leakage flag | In the label row | Yes/no per lesson | — |

Drain state is gone: the fingerprint is stateless (PR 2).

## 11. Measuring every note, and the guardrails (goal 2 and question E)

The analysis is mine; the harness and the screens stay with the team.

- **Every note pattern** gets the lessons' method. Each eligible note is held back at the same step
  on a keyed share, logged with delivery `holdout`, and charged like a delivered note.
- **Per pattern, measured on what the agent does next:**
  - did the agent's next k = 5 acts touch the note's named target, against holdout;
  - the interruption metrics of v1 section 8.2.
- **Guardrails** (non-inferiority, per session, treatment against holdout, every report):

| Guardrail | Definition | Margin |
|---|---|---|
| Task completion | The session's last turn ends with no human correction in its last 3 turns, and every check run after its last edit passed | −5 points |
| Wall time per turn | From prompt to Stop | +10% |
| Tokens per session | From the transcript's usage fields (the adapter gains `Step.Usage`) | +10% |
| Tool calls per turn | — | +10% |
| Human corrections per session | Correction episodes | +0 (no increase) |

A lesson or a note pattern whose effect is positive but breaks a guardrail is flagged, never
promoted silently.

## 12. One brief, and the graphs (goal 1 and question D)

### 12.1 The four graphs

| Graph | What learning puts there | What learning reads |
|---|---|---|
| Platform | nothing | Scope resolution: a key's scope as a unit; the callers and consumers of what a lesson is about |
| Process | Checks (a command a repository runs with pass/fail) as process steps; `check_regression` edges | Which checks cover a scope (opportunity for `unbacked_claim`) |
| Knowledge DAG | Lessons as rows (applies_to its scope, `derived_from` its episodes, supersedes, contradicts); source records as decisions, conventions and incidents | Approved knowledge for the brief |
| Session | Lessons given, labels, re-delivery after compaction | The window, and the trigger state |

Lesson tiers map to knowledge tiers: proposed is INFERRED, approved is DECLARED, proven is EARNED.

### 12.2 One brief for a session's dependencies

This is a retrieval problem:

1. **Candidates:** graph traversal from the request's scope seeds, plus lessons whose key scope
   meets those units, plus source records whose applies_to meets them.
2. **Ranking:** a learned linear ranker fitted offline (weights in a data file). Its features are
   graph distance, recency, evidence strength (the posterior from `effects.json`), source authority
   (declared, earned, inferred) and past usefulness.
3. **Selection:** the existing budget, 6 lines and 1.6 KB.

**Evaluation protocol:**
- *Offline*, on replayed sessions: recall@k of the facts that the session later needed (facts tied
  to an episode that later occurred in the session, or to files it later edited), and precision
  from a blinded sample.
- *Online*: the brief's lines are notes, so section 11 measures them with the same holdout.

## 13. The session graph across compactions (goal 3)

The session graph is one fold over the trace, `Fold(steps) Frontier`, and the existing
`session.Frontier` type is its output. At the root is the person's intent: the first line with
`OriginPerson`. Under it:

- the person's standing instructions;
- open tasks (the adapter maps an agent's plan or todo tool to neutral plan items);
- decisions;
- checks and their state;
- lessons given.

It is re-emitted as the continuity briefing after a compaction. The team's first version is fixed
rules behind that one function, with a labelled set and a measured rate of false lines; mine is
measured against it on the same set.

## 14. The experiment (sized from the replay benchmark)

- **Who:** 5 to 10 developers with their own hooks, on their own repositories.
- **Day 0:** replay and mining over their history; they approve the lessons they agree with.
- **Arms:** per pair, as in section 8.
- **Primary endpoint:** the pooled ITT difference in recurrence, with the cluster-bootstrap interval.
- **Secondary endpoints:** the guardrails, avoidance, interruption, and the effect by whether the
  developer's own hooks touch the same events.
- **Pre-registered** before the first session: the rule version, the looks, the thresholds and the
  endpoints. A change opens a new slice id.

With 5 to 10 developers, a per-lesson interval will often be unavailable (section 8.2). The pooled
answer and the offline fit carry the question.

## 15. Code design

| Package | State | Holds |
|---|---|---|
| `internal/trace` | built (#381) | Steps, origin, signals, denials, tool errors, content fields (#382); gains `Usage` |
| `internal/trace/claudecode` | built | The one Claude Code reader; `Read`, `ReadContent` |
| `internal/trace/record` | PR 4 | Steps from the record and the session log |
| `internal/episode` | built (#382) | Detectors, keys, fingerprints; gains `KindRule` and the registry |
| `internal/source` | new | The source record shape and readers for each source's files; no network |
| `internal/learn` | extended | Miner with marks, lesson v2, `Trigger`, `learned-fact/2`, label row v3, effects, tier rules, propose and ingest |
| `internal/learn/fit` | new | The offline pooled fit and the ranker fit, written as data files |
| `internal/steer` | unchanged by learning | Patterns gain the learned-fact kinds preventive and corrective |
| `cmd/rashomon` | extended | `learn replay`, `mine`, `fit`, `propose`, `ingest`, `report`, `retire` |
| `plugin/skills/propose` | new | The model step, outside the binary |

The tripwires stay:
- `TestCore_NoAgentKeys` covers `trace` outside its adapters, and `episode`, `learn` and `source`.
- `TestNoEcosystemWords` covers `episode`, `learn` and `source`.
- `TestTRU40_*` holds: no networking package and no `os/exec`.

## 16. Pull requests, v2

| # | PR | State | Behaviour |
|---|---|---|---|
| 1 | trace and the Claude Code adapter | #381, open | none |
| 2 | episode: detectors, keys, fingerprints | #382, open | none |
| 3 | `learn replay` (benchmark, labelling export and scoring) | next | A new read-only command, like `audit`; no consent change |
| 4 | **The release-level consent PR**: session-log wiring, `trace/record`, every learning file of section 10, source readers, the hooks covariate, purge rows, the secret check, the consent text | Needs the version number from Karthik | Stores content; asks for consent once |
| 5 | Miner with marks; `learn mine` | after 3 and 4 | Offline |
| 6 | `learned-fact/2`, label row v3, `KindRule` registry | after 2 | Pure; pin test red first |
| 7 | Trigger delivery, per-pair arms, held-back charging, compaction re-delivery | after 5 and 6 | Notes change |
| 8 | Stop and SessionEnd writer call | Karthik's | — |
| 9 | Effects, cluster bootstrap, the offline fit, tier rules, `learn report` | after 6 | Offline |
| 10 | Guardrails and every-note analysis in `measure` | after 9 | Offline |
| 11 | Sources (git, review, ticket, wiki) and the source-recurrence test | after 4 | Offline |
| 12 | One brief: retrieval, the ranker fit, the evaluation protocol | after 11 | Brief changes, measured |
| 13 | Session graph fold, against the team's labelled set | with the team's v1 | Briefing changes, measured |
| 14 | Propose and ingest, plus the plugin skill | any time after 5 | Opt-in |
| 15 | Live slice T5 | last | Experiment |

The order follows Karthik's priority: the complete loop first (3, 4, 5, 6, 7, 8, 9), with sources
inside it (11), and the more advanced pieces after.

## 17. Open points

1. **The consent version for PR 4** (Karthik).
2. **Which forge and tracker exporters come first**, for PR 11. The binary reads files only; the
   exporters are separate programs, and their home is open (a plugin skill, a CI job, or a separate
   tool).
3. **PreToolUse additionalContext.** If a version floor and the channel evidence check cover it, it
   is the best preventive moment. Not assumed.
