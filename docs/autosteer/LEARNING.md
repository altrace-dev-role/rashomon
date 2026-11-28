# Transcript learning: design

Owner: Luis. Status: proposed, for review. Date: 2026-11-28.
Answers issue 204 (what confirms or refutes a lesson) in section 6.

## 0. The question and the answer this design is built to give

**Question.** Can AutoSteer measurably reduce repeated mistakes for developers who already have
sophisticated hooks and workflows?

**How the design answers it.** Every lesson is a hypothesis about one recurring mistake. For each
session and each lesson, a keyed coin decides whether the lesson is shown or held back. In both
arms the same rule then scores the same outcome: did the mistake happen again when the session had
the chance to make it? The answer to the question is the pooled difference in recurrence between
the two arms, with an interval. A user's own hooks are present in both arms, so the difference is
AutoSteer's effect on top of their setup.

A green test after a lesson is never evidence (section 6.1). A falling recurrence count on its own
is not evidence either, because a mistake seen three times often does not show a fourth time with no
lesson at all (regression to the mean). Only the comparison with held-back sessions is evidence.

## 1. What exists, and what this design keeps or replaces

| Piece on main | Keep, extend or replace |
|---|---|
| `internal/events.Event`, the agent-neutral event kinds (`events/event.go`), mapped from Claude Code only in `hookio.EventKind` | Keep. It is the vocabulary of the trace (section 2). |
| `graph.SessionLog` (`internal/graph/sessionlog.go`): the `Call`, `Prompt`, `Note`, `Claim` and `Verdict` builders under session-content consent (scope 6), with caps. Only overlay groups are appended today. | Extend: wire the call, prompt and note writers on the hook path. This is the live content source. |
| `internal/seed`: the Claude Code JSONL parser and five correction kinds (`seed/class.go`), used by `rashomon audit` | Replace the parser with a `trace` adapter (section 2). Its correction kinds become episode detectors (section 3). |
| `internal/learn`: `Lesson`, the ledger with an HMAC manifest, `approve`/`withdraw` with a terminal code, `Pick`/`PickRide` delivery at the prompt, `Delivered`, `Tier.Next` with Wilson95, `Candidate` (no caller) | Keep the ledger, approval and signatures. Extend `Lesson` and the label row. Replace `Tier.Next`'s treated-only bound with the arm comparison (section 7). `Candidate` gets its first caller, the miner. |
| `learn.Judge` (confirm by written path) | Not used for lessons. It is risk R7; a red test pins that the lesson rule never confirms by a written path. |
| `internal/arm` (keyed per-session draw per feature) | Keep. The lesson holdout is one feature per lesson (section 7.1). |
| The steer router, pattern `learned-fact` (nudge, delivered only at UserPromptSubmit today) | Extend: trigger-time delivery at PostToolUse, PostToolUseFailure and prompt, plus quiet-point deferral (section 5). |

## 2. The trace: one agent-neutral model of a session

The miner, the label rule and the measurement read one type and never an agent's file format.

```go
// package trace (new, imports only events)
type Trace struct {
    Session string
    Agent   events.Agent  // {Name, Version}
    Repo    string        // repository id (baseline.RepoID), "" when unknown
    Steps   []Step        // in seq order
}

type Step struct {
    Seq     int64
    At      time.Time
    Kind    events.Kind   // prompt, act_declared, act_landed, act_failed, stop, boundary, ...
    Actor   Actor         // agent, subagent(id), human
    Act     *Act          // for act_* kinds
    Text    string        // a prompt, or the agent's closing message; content scope only
    Human   *HumanSignal  // interrupt, reject, block, pushback
    Note    *NoteRef      // a note AutoSteer delivered or held back at this point
}

type Act struct {
    Class   ActClass   // exec, edit, write, read, search, fetch, plan, delegate, other
    Tool    string     // the agent's own tool name, kept as data, never matched in core code
    Program string     // exec only: the basename of the first word after env and wrappers
    Argv    []string   // exec only: the masked command, split
    Paths   []string   // repository-relative paths the act names or writes
    Status  Status     // ok, failed, interrupted, unknown
    Exit    *int
    Output  string     // the tail kept by the session log (8 KiB; runner output whole)
    Diff    string     // edits only
}

type Source interface {
    // Sessions lists the sessions in the window, newest first, for one repository or all.
    Sessions(ctx context.Context, w Window) ([]Ref, error)
    Read(ctx context.Context, ref Ref) (Trace, error)
}
```

**Two sources, one type.**
- `trace/record`: the store's own record plus the session log. This is the live source. It is
  agent-neutral already, because the hooks write it through `events.Event`.
- `trace/claudecode`: Claude Code's JSONL transcripts. It is used for backfill (sessions before
  install, which makes lessons available on day one) and for what a hook cannot see: interrupts and
  the human's words. The parser in `internal/seed` moves here, and `seed` becomes a user of it.

A second agent is a new adapter package. `TestCore_NoAgentKeys` is extended to `internal/trace`
(outside its adapters), `internal/episode` and `internal/learn`: they may not import an adapter or
hold a Claude Code key.

**ActClass mapping** is the adapter's job and lives in a data table per adapter, not in core code.
For example, Claude Code's Edit, MultiEdit and NotebookEdit map to `edit`. Core code reasons only
about classes.

## 3. How a mistake is found: episodes

An **episode** is one observed occurrence of a mistake: a short span of steps with a negative signal
and, when it exists, what resolved it. Detectors are pure functions over a `Trace`.

### 3.1 Six episode kinds (structural, not a pattern list)

The kinds are fixed, because each one needs its own opportunity rule (section 6). What a kind
matches is learned from data: programs, paths, error shapes. Nothing in a detector names a
language, a runner or an error string.

| Kind | Negative signal | Resolution captured |
|---|---|---|
| `act_failure` | An act fails (status failed, nonzero exit or tool error), and the same act class and program run again later in the session. One-off failures are not episodes. | The first later success of the same act class and program: the argv delta and the edits between the failure and the success. |
| `gate_regression` | A gate (any exec the record already classes as test, build, lint or typecheck) goes green, then edits happen, then it goes red. | The edits between red and the next green. |
| `human_correction` | Interrupt, reject or block (structural signals from the adapter). Pushback text is a per-locale lexicon file, data, off unless present. | The act that follows the correction. |
| `unbacked_claim` | The turn ends with a done claim, and no gate ran after the last edit (`claim-vs-record` already computes this). | None. The lesson is the gate. |
| `self_revert` | An edit restores a file to a content hash it had earlier in the session. | None. |
| `retry_loop` | The same act (same digest) repeats after an identical failure, k ≥ 2. | The act that breaks the loop. |

### 3.2 The failure key

Episodes group by a key. The key is also what a lesson carries, which is ruling Q1(c) of issue 204.

```go
// package episode
type Key struct {
    V     int      // key schema version, 1
    Kind  Kind     // one of the six above
    Act   ActSig   // {Class, Program, Verb}; Verb is the first non-flag argv word, "" otherwise
    Scope Scope    // where it happens
    FP    string   // failure fingerprint: template id, "" for kinds without output
    FPV   int      // fingerprinter version
}

type Scope struct {
    Unit string // a global unit id when the repo is indexed (graph), else ""
    Dir  string // else the deepest common directory of the episode's paths, at most 3 levels
    Ext  string // the dominant file extension, as data (".py", ".tf", ".proto", ...)
}
```

- **Fingerprint (FP), without patterns.** The output tail of a failed act is reduced to a
  template by an online log-template miner. This is Drain (He et al., ICWS 2017): a fixed-depth
  parse tree that masks variable tokens (numbers, hex, paths, quoted strings, identifiers that vary
  across occurrences) and clusters lines by similarity. The FP is the id of the template of the
  most specific failing line. That line is chosen generically, as the line in the tail whose
  template is rarest among templates seen in successful outputs of the same program. So "error
  wording" is learned per repository from its own output, in any language or tool. Templates are
  stored as data (`templates.json`), versioned, and stable once assigned.
- **Scope** uses the code map's unit when the repository is indexed. Otherwise it uses a directory
  and an extension, which works in any repository.
- **Key equality** is exact on Kind, Act and FP. On Scope it means the same unit, or the same
  directory prefix.

### 3.3 Episode record

```go
type Episode struct {
    ID         string    // keyed digest of (session, first seq, key)
    Key        Key
    Session    string
    From, To   int64     // seq span
    At         time.Time
    Evidence   []int64   // seqs that show the negative signal
    Resolution *Resolution
    Source     string    // "record" or "transcript:<agent>"
}

type Resolution struct {
    ArgvDelta []string  // tokens added to or removed from the failing command
    Before    []ActSig  // acts run before the successful retry that were not run before the failure
    Edited    []string  // paths edited between failure and success
    Diff      string    // the combined diff, capped, content scope only
}
```

## 4. How a lesson is formed

### 4.1 Mining (offline, no model)

`rashomon learn mine` runs outside any hook, at a terminal or on a schedule. It also runs
incrementally from `SessionEnd`, bounded to that session.

1. It reads traces from `trace/record`, plus `trace/claudecode` with `--backfill`, and runs the
   detectors. It appends to `learn/<project>/episodes.ndjson`.
2. It groups episodes by key and computes the existing `learn.Evidence` (episodes, sessions, days,
   consistency, contradictions).
3. **Candidate gate.** It feeds the existing `learn.Candidate`: at least 3 episodes, 3 sessions and
   2 days, consistency at least 0.75, and no contradictions. Consistency is the share of episodes
   whose resolution agrees with the modal resolution (the same `Before` act, the same `ArgvDelta`
   or the same edited unit). A contradiction is a session where the modal resolution was applied and
   the failure happened anyway.
4. It writes a **proposed** lesson (section 4.3). Proposed lessons are never delivered; a person
   approves with the existing `learn approve`.

### 4.2 Lesson text without a model

Text comes from a template filled with data from the episodes. It is a statement of fact, because
the render rules ban imperatives and "you":

```
rashomon record: lesson L7, from 5 sessions since 2026-11-10: 'npm test' under 'web/src'
failed with 'Cannot find module <*> from <*>'; in 4 of 5 the run passed after
'npm run build' ran first.
```

Every value comes from the key and the resolution. The template set is one per kind (six), and
there is no per-language wording.

### 4.3 Lesson v2

`Lesson` gains fields. The JSON schema becomes `lessons/2`, and `lessons/1` rows read as keyless.

```go
type Lesson struct {
    ID             string
    Tier           Tier
    Text           string
    TextSource     string        // "template", "model", "human"
    SinceUnixMS    int64
    ApprovedUnixMS int64
    // v2
    Key        *episode.Key     // nil for hand-written lessons: those can only ever be unresolved
    Mode       Mode             // preventive or corrective (section 5.2)
    Episodes   []string         // ids of the episodes that justify it
    HoldoutBP  int              // per-lesson holdout share, default 2500 (section 7.1)
    RetiredUnixMS int64
    RetiredWhy    string        // "harm", "futile", "scope_gone", "superseded", "person"
}
```

### 4.4 Optional model at index time (opt-in)

`rashomon learn propose --model` reads the episodes of proposed lessons and asks a model for two
things: clearer wording, and, when the modal resolution is a diff, a one-line summary of it. The
output is stored as data on the proposed lesson (`TextSource: "model"`) and needs the same human
approval. No model runs in a hook. It is off unless the person runs the command, and the command
says what it sends.

## 5. When and how a lesson is delivered

### 5.1 The moments a note can reach the agent

Facts from the code on main:

| Moment | Channel | Today |
|---|---|---|
| UserPromptSubmit at turn start | additionalContext | Preflight and graph brief; the one learned-fact lesson rides here |
| UserPromptSubmit taken into a running turn | additionalContext | Same path; the note lands next to the human's new words |
| PostToolUse (after a successful act) | additionalContext | Blast callers, unit brief at first edit, conflicts, platform rules |
| PostToolUseFailure (from 2.1.304) | additionalContext | Regression notes |
| SessionStart at compact | additionalContext | The continuity briefing |
| PreToolUse | none (the router has no channel) | Prints nothing |
| Stop | person only (flag) | The end-of-turn line |
| Subagent calls | shadowed (`unverified_channel`) | Not shown, but charged to the budget |

### 5.2 Trigger-time delivery

A lesson is delivered at the moment its key says the mistake is about to be possible, not at
every prompt. There are two modes:

- **Preventive** (default for every kind except `retry_loop`). The lesson is delivered at the
  first **opportunity signal** in the session, before the risky act:
  - the prompt (or its graph brief) names a path in the key's scope;
  - or the first act in scope lands (a read or search in scope is the preferred point, before any
    edit);
  - or for `unbacked_claim`, the first edit of the turn.
- **Corrective** (for `retry_loop`, and a fallback for the others). The lesson is delivered at the
  first PostToolUseFailure whose key matches, which is the point where the agent already stops to
  handle a failure. It is labelled separately, because the first occurrence has already happened.

Matching is a pure function in `internal/learn`. It is data-driven from the approved lessons'
keys and reads only the current step and the record:

```go
// Trigger returns the lessons this step opens, in rank order, at most one.
func Trigger(active Active, step trace.Step, given func(id string) bool, scope ScopeOf) []Match
```

The hook calls it from the PostToolUse and PostToolUseFailure steps and from the prompt path. The
result is a `steer.Finding` with pattern `learned-fact` and a kind of `preventive` or
`corrective`, and it goes through `steer.Route` like every other note. So budgets, consent, channel
gates, holdout and logging all apply unchanged.

### 5.3 How notes interrupt a session's thinking

Notes reach the agent while it is still working at three points: after any act (PostToolUse),
after a failed act (PostToolUseFailure), and with a prompt typed into a running turn. Each can
break the agent's train of thought instead of correcting it. These are the ways, from the code
and the runs on record, with the rule this design adopts for each.

1. **Mid-burst interruption.** The agent is making a planned series of edits (A, then B, then C).
   A note after edit A (blast callers, unit brief) pulls it to the callers before B and C, and the
   change is left half done. *Rule:* a note that is not about a failure waits for a quiet point.
   A quiet point is a read or search act, a failed act, or the turn boundary. "During a burst"
   means the last two acts were edits or writes. The router already has a `deferred` state, and
   this uses it.
2. **The note lands on the human's words.** On a prompt taken into a running turn, the note sits
   next to the person's new message and reads as part of it. A lesson there can outrank what the
   person just asked. *Rule:* no lesson is delivered with a mid-turn prompt unless the prompt
   itself names the lesson's scope.
3. **A stale fact triggers repair of a non-problem.** The blast note already says when "the graph
   does not hold the present content of the edited file". A lesson whose scope has moved does the
   same thing. *Rule:* a lesson whose scope no longer resolves (unit gone, directory gone) is
   retired as `scope_gone`, never delivered.
4. **A deliberate red read as a regression.** In a test-first workflow, the agent writes a failing
   test on purpose. A regression note or a corrective lesson then tells it something "broke", and
   it reverts the test. *Rule:* a red run whose failing output names only files written in this
   turn (a new test) is not a regression and not a corrective trigger. This is recorded as an
   episode-kind exclusion, and it applies to the existing regression note too.
5. **Duplication with the user's own hooks.** Users with sophisticated hooks already inject their
   own context. Two notes on the same fact, from two hooks, make the agent respond twice or argue
   with one. *Rule:* the design measures this rather than guessing (section 8). The adapter records
   which other hooks were installed at session start (names only), as a covariate.
6. **Meta-talk.** The agent quotes the note to the person ("rashomon says…") or explains itself
   to the hook instead of working. *Signal:* the closing message quotes note words; this is
   counted.
7. **Note storms.** Up to 3 notes in a window is the budget today, but a failure followed by a
   lesson, a regression note and a blast note in three consecutive acts is still a storm. *Rule:* a
   lesson counts against the same window, and at most one lesson is delivered per prompt.
8. **Budget spent on what no one saw.** Subagent calls are shadowed but charged
   (`budget.go:163-164`). A burst of subagent activity can exhaust the parent's window, so a
   lesson that matters never prints. *Rule:* shadowed notes stop counting against the budget. This
   is a small fix to the existing router, and it gets its own PR.
9. **Forced continuation.** The Stop channel can carry `stopContinue`, which would force the agent
   to keep going. It is never used, and this design keeps it that way: a lesson never blocks or
   extends a turn.
10. **Imperative wording read as a new instruction.** The render rules already ban imperatives,
    "you", "must" and "should", so a note reads as a record and not as an order. Lesson templates
    stay inside those rules (section 4.2).

Each of these is measured, not assumed (section 8.2).

## 6. How we tell a lesson helped (issue 204)

### 6.1 The label rule `learned-fact/1`

This is ruling Q1: option (a) as the mechanism, with (c) as the content of its row. The rule enters
the repository as the one row of the `label_rule` table, and the writer is its only evaluator
(MSR 13).

For a lesson L with key K, delivered (or held back) at seq d in session S:

- **Opportunity:** a step after d at which K's opportunity predicate holds.
- **Recurrence:** a step after d at which K's recurrence predicate holds.
- **Label:** if a recurrence happens, `refuted`; else, if an opportunity happens, `confirmed`;
  else `unresolved`.
- **A lesson with no key** is always `unresolved`. Nothing in the record can test it, so the rule
  never promotes it.

What counts as an opportunity and a recurrence for each kind of key:

| Kind | Opportunity (after d) | Recurrence (after d) |
|---|---|---|
| `act_failure` | An act with K.Act (class, program, verb) in K.Scope | That act fails and its output's fingerprint is K.FP |
| `gate_regression` | An edit in K.Scope followed by a run of the gate | Green, then edit in scope, then red with fingerprint K.FP |
| `human_correction` | An act of K.Act's class in K.Scope | The same correction kind follows such an act |
| `unbacked_claim` | A turn that edits files and ends | That turn ends with a done claim and no gate after the last edit |
| `self_revert` | An edit in K.Scope | An edit in scope restores an earlier content hash |
| `retry_loop` | A failure of K.Act | The identical act repeats after an identical failure |

For **corrective** lessons, delivered at the first occurrence, the opportunity is the next act of
K.Act after d, and the recurrence is the same failure again. Corrective labels are a separate
stratum and never pool with preventive ones.

**Why R7 cannot happen here.** The rule never reads which paths were written in order to confirm.
A red test pins it: a lesson whose text names a path, in a session that writes that path and has
no opportunity, labels `unresolved`, not `confirmed`.

### 6.2 When it is written

This is ruling Q2. The label is written at the Stop of the session that delivered (or held back)
the lesson: one label per (lesson_id, session_id), written once. A second Stop rewrites nothing,
and a test pins that.

The rule is a pure function in `internal/learn`:

```go
func Evaluate(l Lesson, t trace.Trace, at int64) Outcome
type Outcome struct {
    Label         steer.Label
    Rule          string // "learned-fact/1"
    EvidenceSeq   int64  // the recurrence, else the first opportunity, else 0
    Opportunities int
}
```

The call from the Stop belongs to Karthik's hook path, as he said on the issue.

### 6.3 The label row v2

This is ruling Q4. The pin test `TestP6_LabelRowIsLabelWriter1sContract` changes in the same pull
request, red first.

```json
{"v":2,"lesson_id":"L7","session_id":"…","at_unix_ms":0,
 "label":"refuted","rule":"learned-fact/1","evidence_seq":412,
 "arm":"treatment","mode":"preventive","delivered_seq":398,"opportunities":2}
```

`arm` is `treatment`, `holdout` or `steer_holdout`. A session the whole-steer holdout already holds
back gets `steer_holdout`.

### 6.4 Holdout outcomes

This is ruling Q3. When a lesson's arm says holdout, the trigger still fires and the router logs the
line with delivery `holdout`, at the same seq the lesson would have been shown. That gives the
holdout outcome the same `d`, so treated and held-back sessions are scored from the same moment.
Their labels are stored with `arm`, and `Tier.Next` never counts them.

## 7. Statistics: from labels to "it helped"

### 7.1 Randomisation

- **Unit:** the (lesson, session) pair. The arm is `arm.Assign("lesson/"+L.ID, session,
  L.HoldoutBP)`, so lessons draw independently, and one session can be treatment for L1 and
  holdout for L2.
- **Share:** 25% holdout per lesson by default. That is more than the steer-wide 10%, because the
  per-lesson comparison needs it (section 7.3).
- Sessions in the whole-steer holdout are labelled `steer_holdout` and excluded from per-lesson
  effects.

### 7.2 Effect

For a lesson, over sessions with an opportunity (confirmed or refuted labels):

- p_T is the recurrence share in treatment, and p_H the recurrence share in holdout.
- **The effect is p_H − p_T**, with a Newcombe hybrid-score 95% interval.

The feature-level answer to the question in section 0 is the **Mantel–Haenszel pooled risk
difference** across all lessons, stratified by lesson, reported with its interval. Preventive and
corrective are reported separately.

Pre/post counts (the audit baseline in `seed.json` against counts after install) are shown as
description only and never as the effect.

### 7.3 Tier policy (replaces the treated-only bound)

| Transition | Condition |
|---|---|
| proposed → probation | A person approves (exists). |
| probation → proven | At a planned look, the lower bound of the effect is above 0. Looks happen at 20, 40 and 80 treated outcomes, each at a Bonferroni-corrected level (98.3% intervals), so peeking does not inflate false promotions. |
| any → demoted (harm) | At any look, the upper bound of the effect is below 0: the lesson makes recurrence more likely. It stops being delivered at once. |
| probation → retired (futile) | At 80 treated and 25 holdout outcomes, the upper bound is below 0.05. |
| any → retired | The scope is gone, the lesson is superseded, or a person retires it. |

**Sample size, stated honestly.** With a baseline recurrence of 0.4 per opportunity and a true
lesson effect bringing it to 0.2, a two-sided test at 0.05 with 80% power needs about 80 outcomes
per arm. A single lesson reaches that slowly. The feature-level pooled estimate across ten lessons
reaches it in weeks for a team. So the claim "AutoSteer reduces repeated mistakes" comes first,
pooled, and per-lesson promotion comes later. Futility and harm rules keep bad lessons from waiting
that long.

### 7.4 Invalidation

- A lesson is retired as `scope_gone` when its scope no longer resolves at index time.
- It becomes dormant when its key's opportunity has not occurred in 60 days of sessions.
- It is superseded when the miner mines a new lesson with the same key and a different modal
  resolution: the new one starts at proposed, and the old one is retired only on approval of the
  new one.

## 8. Measuring the cost of interrupting

### 8.1 Why

A lesson can lower recurrence and still make sessions worse, by derailing them. The question is
about developers with sophisticated workflows, who will notice derailment first.

### 8.2 Interruption metrics

These are computed per delivered note and per held-back note at the same seq, in the next k = 5
steps, from the trace:

- **Scope jump:** the share of the next k acts outside the scope of the previous k acts.
- **Self-revert:** a revert within k acts after the note.
- **Human interrupt or reject** within k acts after the note.
- **Acts to next green gate**, in a turn where a gate ran.
- **Meta-talk:** the closing message quotes the note's words.

Each is reported as treatment minus holdout, like the effect. A lesson with a positive effect and
a significant interruption cost is flagged to the person in `rashomon learn report`, not silently
promoted.

## 9. Code design

### 9.1 Packages

| Package | New or changed | Holds | Imports |
|---|---|---|---|
| `internal/trace` | new | `Trace`, `Step`, `Act`, `Source` | `events` |
| `internal/trace/record` | new | `Source` over the store record and the session log | `trace`, `store`, `graph` (session log reader only) |
| `internal/trace/claudecode` | new | `Source` over Claude Code JSONL; the ActClass table; the parser moved out of `seed` | `trace`, `spend` (discovery) |
| `internal/episode` | new | the six detectors, `Key`, `Scope`, the Drain fingerprinter | `trace` |
| `internal/learn` | changed | `miner.go`, `trigger.go`, `rule.go` (the label_rule table and `Evaluate`), `effect.go` (Newcombe, MH, tier policy), `lessons/2`, label row v2 | `trace`, `episode`, `arm`, `steer` (labels only) |
| `internal/seed` | changed | `audit`'s cards, now over `trace/claudecode` | `trace/claudecode` |
| `internal/hook` | changed | wires `SessionLog.Call`, `Prompt` and `Note` under session consent; calls `learn.Trigger` at Post and PostFailure | `learn`, `graph` |
| `internal/steer` | changed | `learned-fact` kinds preventive and corrective; quiet-point deferral; shadowed notes not charged | none new |
| `internal/disclose` | changed | the consent wording (section 9.4) | none new |
| `cmd/rashomon` | changed | `learn mine`, `learn propose`, `learn report`, `learn retire` | `learn` |

`TestCore_NoAgentKeys` covers `trace` (outside its adapters), `episode` and `learn`. A new test
fails if any of them imports a `trace/<agent>` package.

### 9.2 Files in the store

All files are under `learn/<project>/` and signed by the existing `state.sum` manifest:

- `episodes.ndjson` (append only, capped at 16 MiB; oldest first out)
- `templates.json` (Drain state, versioned)
- `lessons.json` (`lessons/2`), `active.json` (as today, plus keys)
- `labels.ndjson` (row v2)

Content (outputs, diffs, prompts) stays in the session log under `graph/`. Episodes hold seqs that
point into it, plus the capped resolution diff.

### 9.3 Hook-path budget

`Trigger` is a map lookup on (ActClass, Program) followed by a scope test, with no I/O beyond
`active.json`, which the prompt path already reads. It must stay inside the existing K7 PostToolUse
budget, and a benchmark pins it.

### 9.4 Consent text

Ruled: session content is allowed. The 1.0 screen says prompts, command text, tool output and
file contents are never stored. The pull request that wires `SessionLog.Call`, `Prompt` and `Note`
on the hook path changes that wording in the same diff and bumps `disclose.ScopeVersion`. The new
paragraph says what is kept (command text, output tails of 8 KiB, runner output, diffs and prompts,
all capped), where it is kept (`graph/` in the store), that it stays until `rashomon purge`, and
that `learn mine` reads it. Our side writes the wording.

## 10. Pull requests

Each PR is mergeable alone, red tests first, with no behaviour change before PR 6.

1. **trace: the agent-neutral model and the Claude Code adapter.** Move `seed`'s parser into
   `trace/claudecode`. `audit` output is byte-identical (a golden test). Extend the no-agent-keys
   test.
2. **episode: detectors, keys and the fingerprinter.** Pure functions and fixtures in three
   ecosystems that share no code (a Python project, a TypeScript project, Terraform), to show that
   nothing is language-bound. No storage.
3. **Router fix: shadowed notes stop charging the budget** (section 5.3, item 8). Small and
   independent.
4. **Capture: wire the session log's call, prompt and note writers on the hook path**, with the
   consent wording and the scope bump in the same diff.
5. **Miner and `rashomon learn mine` (with `--backfill`).** Writes episodes and proposed v2
   lessons with template text. `learn` lists them.
6. **Issue 204 rule: `label_rule` table, `Evaluate`, label row v2.** The pin test turns red first,
   and the R7 false-confirm test lands. Pure functions in `internal/learn`.
7. **Trigger-time delivery.** `learn.Trigger`, per-lesson arms, holdout logging at the same seq,
   quiet-point deferral, and the TDD-red and mid-turn-prompt exclusions.
8. **Stop label writer call.** Karthik's hook path, idempotent per (lesson, session).
9. **Effect and tier policy.** Newcombe, Mantel–Haenszel, planned looks, harm, futility, and
   `rashomon learn report`.
10. **Interruption metrics** in `learn report` and in `measure`.
11. **Optional `learn propose --model`**, opt-in.
12. **Live slice T5:** the repeated-mistake experiment (section 11).

PRs 1 to 3 can go in parallel. PR 6 depends on 2. PR 7 depends on 5 and 6. PR 8 depends on 6 and 7.

## 11. The experiment that answers the question

- **Who:** 5 to 10 developers with existing hooks and workflows, on their own repositories, in any
  languages.
- **Day 0:** install, then `learn mine --backfill` over their past transcripts. Lessons are
  proposed on day one from history, and they approve the ones they agree with.
- **Weeks 1 to 4:** normal work. Per-lesson 25% holdout, the label writer at every Stop.
- **Primary endpoint:** the pooled MH risk difference in recurrence per opportunity, preventive
  lessons, with its 95% interval. The question's answer is yes if the lower bound is above 0.
- **Secondary endpoints:** interruption cost (section 8.2); the effect stratified by whether the
  user had their own hooks touching the same events.
- **Pre-registered** before the first session: the rule version, the looks, the thresholds and the
  endpoints. Changing any of them after a session opens a new slice id, as the existing harness
  rules require.

## 12. Open points for review

1. **PreToolUse context.** Some Claude Code versions accept `additionalContext` on PreToolUse.
   If verified, it is the best preventive moment, before the act rather than after the previous
   one. The router keeps PreToolUse closed until a version floor and the channel evidence check
   cover it.
2. **The pushback lexicon** is English in `seed` today. As data it can grow per locale; structural
   signals (interrupt, reject) carry the design without it.
3. **The 25% holdout share** delays help for one session in four. Teams may want it lower once the
   pooled effect is established. That is a measured decision, not a default.
