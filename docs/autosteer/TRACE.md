# AutoSteer trace (status file)

Companion to PLAN.md (the plan) and VISION.md (the vision, read at every session start). This file is the
machine-checkable status: the requirements-to-code matrix with a status per row, and the owner's rulings.
It is updated at every task close with the task id, the acceptance id, the code path, the test by name and
the live evidence seq. The plan's prose does not change when a row moves; this file does.

Status log (newest first):
- 2026-11-05 00:4x: W-M1.5 done. engine.Tokens splits an initialism from the word after it (HTTPServer -> http, server) and keeps
  a plural s on it (URLs); FieldsOf reads the doc attribute into Summary. Verified at task open: the lower-to-upper split
  already existed (PlatformRead passed before the change), so the task closed the initialism and doc gaps only. Tests:
  TestTokens_CamelCase, TestScope_DocMatches; whole engine package, internal/graph/..., cmd graph path, TestRubricLAW10_
  green through the slot. No new read, no disclosure change. Next: W-M1.6 knowledge importer (internal/graph/knowledge;
  TestImport_LedgerRow, TestImport_DecisionLine, TestImport_ClauseValidates, TestImport_AppliesToResolved,
  TestImport_Idempotent).
- 2026-11-05 00:3x: W-M1.4 done. internal/graph/index runs the resolver of each language the configuration declares (index.Unit.Lang,
  required: gopg.Lang, pylite.Lang) over the files under that language's units, asserts the repo:<rid> node and returns
  index.Result{Store, Header, Files}; the store header is {schema, repo, rev{commit}, producers, records} (PLAN B6) and
  graph.Load refuses another schema; graph.GlobalID, SplitGlobal and RepoNodeID hold g:<rid>#<local> (AD14); baseline.RepoID
  reads the origin url from .git/config without git, through a worktree, normalizes it (NormalizeOrigin), else an ephemeral
  path: form (no local:<root-commit> form: packed objects, see PLAN W-M1.4 as built). rashomon's own .rashomon/graph.json
  lists . and pgraph as Go units; .rashomon/graph.jsonl is ignored. The consent screen names the index's reads and the
  prompt-time read of the graph (TestAI3_). Tests by name in PLAN W-M1.4 as built; all green through the slot with build,
  vet, gofmt, TestGS5 (cmd), TestRubricLAW10_, TestH156d. Deviations pending the owner's word: no root-commit rid form;
  the disclosure paragraph pulled forward from W-M1.9; open question for W-M1.9: the brief evidence file stores matched
  prompt tokens (D160). Not yet run on rashomon itself (W-M1.8 syncs and indexes it). No run has happened. Next: W-M1.5
  (Tokens CamelCase split; FieldsOf reads doc).
- 2026-11-05 00:1x: W-M1.3 done. internal/graph/lang/gopg is the Go lang.Resolver over platform.ReadSnapshot: a unit node per
  package directory (unit:go:<dir>), a file node per digests row carrying pgraph's digest, a sym node per symbols row
  (sym:go:<dir>:<kind>:<qualified>, attrs name, symkind, pkg, line, doc) declared by its file, an unplaced sym for an
  interface method the facts name, and edges calls->Calls (PROVEN, certain in the store), may-call->MayCall
  (ALGORITHMIC, conf 0.5, never certain), implements and implements-via-pointer->ResolvesTo (via=pointer), embeds->
  References, a type's declares->Declares; a callee outside the sync (strconv.Itoa) is dropped; two build configs of one
  fact are one edge; Overlay is nil, nil (admitted without overlays) until W-M2.5. Fixture gopg/testdata/shapes synced by
  the branch pgraph (sidecars committed). Red first (undefined Resolver), then green TestGopg_* five, internal/platform
  whole (Snapshot gained Digests), TestEngine_NoLanguageLiterals, vet and gofmt clean. Not yet registered in index.Build
  (W-M1.4 reshapes it per language). No run has happened. Next: W-M1.4 (index per language, store header, repo node,
  GlobalID/SplitGlobal, rashomon's .rashomon/graph.json).
- 2026-11-05 00:0x: W-M1.2 done. platform.ReadSnapshot(ctx, root) reads one sync whole under the walk file's vouch chain:
  the log (live facts, exported as Fact with the v0 predicate and tier spellings), the digests (Files per package) and the
  symbols file (Place rows: id, package, file, line, doc), refusing whole on a flipped byte, a file the walk does not name,
  a symbols file chained to other digests (ErrUnvouched), a malformed row (its own error), and saying ErrNotSynced for no
  walk file or a four-line one. Red first (undefined names, implementation moved aside), then green: TestReadSnapshot_*
  five (one over internal/platform/testdata/snapshot, a sidecar set the real pgraph wrote), internal/platform and
  internal/disclose whole, TestH156d_SnapshotReadsWhatSyncWrote in test/acceptance over a real sync, vet and gofmt clean.
  Disclosure: the pgraph paragraph now says each symbol's file, line and doc sentence is written (TestAI2_); no scope
  bump (a pgraph write beside what the screen names, no new read of user data until W-M1.9 wires graph index).
  Intentional change (principal's call pending the owner's word): the row type is Place, not Symbol, because
  platform.Symbol already exists; no Vouched field (a Snapshot is returned only when vouched); PLAN.md updated in place.
  No run has happened. Next: W-M1.3 (lang/gopg resolver over Snapshot).
- 2026-11-04 23:5x to 2026-11-05 00:0x: W-M1.1 done. pgraph writes .pgraph/pgraph.symbols (id, package, file, line, doc
  first sentence) chained to the digests file by sha256; the walk file names all three; the rashomon platform reader
  accepts the symbols line and still reads the older four-line form. Red first (undefined names), then green on the
  whole pgraph module, its CLI tests and internal/platform (TestSymbols_* three, TestReadVouch_SymbolsLine), vet clean
  in both modules; a real sync of pgraph/testdata/dispatch with the branch binary wrote 38 rows for 38 declares rows
  with the chain matching and a second sync stable. One fixture error caught by the red run: a file at d/d.go under
  module example.com/d has package path example.com/d/d, so the doc fixture moved to the module root. Harness
  adjustment before any run: run.sh now syncs arm R's tree with the pinned pgraph and arm G's with its own (RK_PGRAPH_G,
  pins.env T1_G_PGRAPH_SHA, filled at W-M1.9), because rashomon watch installs no pgraph hook and the plugin does;
  PLAN.md section 7.3 rows R and G carry the note. No run has happened. Next: W-M1.2 (platform.ReadSnapshot).
- 2026-11-04 23:3x: W-M1.0 harness written into test/live/autosteer (run.sh, wrap.sh, hook.sh, cleanup.sh, judge/common.py) and the T1
  pre-registration (t1/prompt.txt frozen as section 7.2; pins.env: tree and arm R at 5f851724, baseline pgraph.log sha pinned, caps;
  consumers.txt; oracle-symbols.txt with four shapes verified against pgraph; testdata/dispatch-reflect; judge a1, a2, a3 (A3 to A5),
  a6, a7, a8, oracle.py, judge.sh). A3 rule corrected before any run: the renderers judged are the PostToolUse blast line and the
  Stop platform line; rashomon report has no platform section. Arm G binary sha and knowledge digests stay empty until W-M1.9.
  23:5x, still before any run: the A3 replay was smoke-tested against the arm R binary on the pinned tree and three
  preconditions it needs were found and pre-registered (one symbol per session because the Stop prints two package lines
  at most; the fixture copies carry a CI step naming go test and an empty test file per package, which the Stop's
  platform row requires; AI_AGENT in the claude-code_<v>_harness form, which the blast note's delivery requires). The
  fixture extraction depth in a3.sh was wrong (module nested one level) and is fixed; oracle shapes re-verified
  unchanged. On the pinned tree both renderers fire and name no symbol: A3 neither, as expected. No run has happened.
  Next: W-M1.1 (pgraph symbols sidecar).

- 2026-11-04: plan approved by the owner (Parts A, B, C). Execution step 1 (PR #281 merge) and step 2 (this
  commit) done. Next: W-M1.0 (harness into test/live/autosteer) and the T1 pre-registration commit.

## 2. Requirements-to-code traceability matrix

Status column: `live`, `built`, `experimental`, `none` as defined in 1.2; `stale` means the truth table disagrees with main. Work-item ids (W-M1.1 etc.) refer to section 6. Acceptance ids (AC-...) refer to section 8. Evidence to mark complete is always: the code path, a passing behavioral test by name, and a live session record (steer log seq, record seq) where the row has runtime behavior.

### 2.1 The nine capabilities of the vision (reset directive section 2) and the loop

| Cap | Behavior AutoSteer must demonstrate | Status | Code involved | Gap | Work items | Acceptance |
|---|---|---|---|---|---|---|
| C1 Understand an ambiguous objective | A prompt naming no file resolves to units, symbols and obligations | experimental | `engine/scope.go` (Scope, FieldsOf, Tokens), `intent.Resolve` | FieldsOf reads sig/value/text/cols only; no doc; Go absent | W-M1.3, W-M1.5 | AC-M1.1 |
| C2 Identify relevant architecture unnamed by the user | The brief names the owning unit and its neighbors before the first edit | experimental | `engine/traverse.go`, `rank.go`, `select.go` | Python fixture only | W-M1.1..M1.6 | AC-M1.1, AC-M1.2 |
| C3 Discover existing abstractions and non-local dependencies across files, services, repos | Reuse target and consumers named; cross-repo through contract ids (`ContractID`, `ServiceMap`, `ResolveConsumers`) | experimental (cross-repo measured 5/5 at n=5 on a fixture) | `graph/ids.go`, `engine/rank.go ReachBonus` | Go, TypeScript absent; consumers in other repos need both repos indexed | W-M1.*, W-M2.3, W-M6.* | AC-M1.2, AC-M6.3 |
| C4 Applicable decisions, conventions, workflows, constraints | Decision and convention lines scoped to the touched unit, with source | experimental (hand-written knowledge) | `index.knowledge`, `engine/facts.go knowledgeFact`, `spec` rules | No importer; no scope beyond applies_to; no supersession | W-M1.4, W-M2.1, W-M5.2 | AC-M1.3, AC-M2.2 |
| C5 Plans consistent with that knowledge | A plan that crosses a boundary or ignores a decision gets a plan-time line | none | `engine/conflicts.go` (arity only, tests only) | Check Conflicts unwired; no plan reader | W-M2.1, W-M2.2 | AC-M2.1 |
| C6 Guidance while implementing | First write in a unit delivers that unit's rules, decisions, importers, lessons | none | `cmd/rashomon/post*.go`, `steer.Route` | No first-write trigger; PreToolUse has no context field (hook channel fact) | W-M2.3 | AC-M2.3 |
| C7 Preserve goals, decisions, progress across compaction and handoffs | The frontier (intents, requirements, decisions, touched, obligations, delivered) survives compact, resume, clear, subagent | experimental (types only) | `graph/session/*` | No persistence, no fold, no boundary subscription | W-M4.1..M4.5 | AC-M4.1..4 |
| C8 Independent verification against objective and architecture | Stop verdict names unmet obligations, failed hidden criteria, architecture conflicts; never the agent's claim | live for gates; none for architecture and objective | `verify/*`, `recap/*`, `engine/conflicts.go` | Architecture rules over the diff; requirement predicates at Stop | W-M3.1..M3.4 | AC-M3.1..3 |
| C9 Learn from validated outcomes | Delivered lines labeled by compliance predicates; lessons promoted or retired by evidence; nothing promoted unsupported | built (ledger), none (runtime) | `learn/ledger.go AppendLabel` (CLI only), `learn/given.go Judge` | Label writer not in production; Judge too weak; no lifecycle edges | W-M5.1..M5.5 | AC-M5.1..4 |
| G/A/D | Every record says which of G, A, D it is | partly ruled (contracts-v2 section 4) | `session/verdict.go Delivered, Followed, Outcome`, steer log HMAC | Not enforced in reports; labels conflate | W-M4.4, W-M5.1, W-M7.3 | AC-X.1 |

### 2.2 The twelve functional scenarios (vision section 6)

| Scenario | Must understand | Evidence sources | Graph classes | Status | Missing | Milestone |
|---|---|---|---|---|---|---|
| S1 Unknown architectural dependency | touched unit, its importers and consumers | structure (calls, imports, contracts) | structure | experimental (Python fixture 5/5 at n=5) | Go, scale, precision measured | M1 |
| S2 Existing functionality | a symbol whose name and doc match the task exists elsewhere | structure (symbol doc, name), judgment (decision "reuse X") | structure, judgment | none (no doc field, no reuse line) | reuse line kind; duplicate detection | M1, M2 |
| S3 Cross-repository change | consumers in another repo through a contract id | structure with ServiceMap | structure | experimental (fixture) | second language PROVEN; repo discovery; refresh | M6 |
| S4 Architectural convention conflict | the diff crosses a declared boundary or contradicts a decision in scope | judgment rules, structure edges, evidence diff | all three | none | rule kinds, diff reader, plan reader | M2, M3 |
| S5 Business process impact | a change to a unit touches a flow (sync call, event, table, job) | structure (exposes, consumes, reads, writes), process templates | structure, process | none (table edges exist in pylite only) | process templates (declared), flow traversal | M3 |
| S6 Long-running session | requirements and decisions survive compactions | evidence (record), session fold | session | none | W-M4 | M4 |
| S7 Multiple agents | a subagent or a peer continues with the frontier | evidence (agent id), session | session | none | subagent boundary events | M4 |
| S8 Historical engineering knowledge | an incident or rejected approach influences a plan | judgment (lesson, decision, supersedes) | judgment | experimental (hand-written lesson 5/5 vs 0/5 at n=5) | importer, lifecycle | M1 (importer), M5 |
| S9 Evolving repository | reasoning over uncommitted edits | overlays (rev commit+ovN) | structure | experimental (TestOverlay 6/6; live not measured) | Go overlay; K6 live | M2 |
| S10 Incomplete or conflicting knowledge | uncertain, stale, contradicted facts are said as such | tiers, Certain, freshness keys, supersedes | all | partly built (tiers, Certain, vouch refusal) | contradiction edges; stale detection in briefs | M5, M6 |
| S11 Enterprise-scale system | partial index, many languages, monorepo | unit-level ALGORITHMIC producers, incremental sync | structure | none | in-binary import parsers, incremental index, size budgets | M6 |
| S12 Cost and efficiency | when context reduces exploration and when it rightly adds work | harness measures (turns, tool calls, cost, correctness) | measurement | built (instrument), no claim | cost per correct outcome row | every milestone's AC |

### 2.3 The 44 PRD rows (test/prd/truth_table.json), each with the behavior, status, code, gap, work and acceptance

Process graph PF-PG:

| Row | Behavior | Status | Code | Gap (from the truth table, verified) | Work | Acceptance |
|---|---|---|---|---|---|---|
| PG1 Gate families | every runner kind classified | partial | `shape/runner.go` | wrapper-hidden lint, typecheck, scan | W-M3.5 | AC-M3.4 |
| PG2 Masked-output classifier | a piped or wrapped red run is read red | partial | `shape/masked.go` | pass read from the last summary line under pipelines; PR #281 covers wrappers | W-DEBT.1 | AC-M3.4 |
| PG3 Gate evaluator | gate state per family from the record | partial | `verify/evaluate.go` | run scope on the record | W-M3.1 | AC-M3.1 |
| PG4 Claim comparator | a claim without a run is contradicted | built | `verify/claims.go` | lint, typecheck, scan claims read no_gate_known | W-M3.5 | AC-M3.4 |
| PG5 Obligation ledger | obligations with state and seqs | partial | `verify/obligations.go` | ledger verb replays from the record; unit scope | W-M3.1 | AC-M3.1 |
| PG6 Tree digest | tree sha at points | built | `hook/treesha.go` | under scope v4 without consent no sha | none (consent design) | n/a |
| PG7 Watched red then green | a red gate owes a green after the write | partial | `verify/obligations.go` | change scope is the session | W-M3.1 | AC-M3.1 |
| PG8 Record-green point | the point a checkpoint is green | partial | `verify/evaluate.go` | forbid_code undecided (content not read) | W-M3.2 | AC-M3.2 |
| PG9 Done flag | done with evidence or named as unbacked | partial | `recap/done.go` | W1 precision measurement | W-M3.6 | AC-M3.5 |
| PG10 Value line | end-of-turn line | partial | `recap/value.go` | (none listed) | W-M7.3 | AC-M7.2 |
| PG11 Contradiction-first report | report leads with contradictions | partial | `report/text.go` | checkpoint claim verdicts; live quote match | W-M7.3 | AC-M7.2 |
| PG12 Integrity markers | user-only flags for test removal, skips, assertion edits | partial | `verify/integrity.go` | edit-marks writer (1.8p) | W-M3.7 | AC-M3.6 |
| PG13 Scopes | turn, subtask, commit, push, PR windows | none | `verify/obligations.go` | all windows and evaluation points | W-M3.1 (unit and turn scope), W-M3.8 (commit, push, PR) | AC-M3.1 |

Platform graph PF-PL:

| Row | Behavior | Status | Code | Gap | Work | Acceptance |
|---|---|---|---|---|---|---|
| PL1 Ingest | load a module without egress | built | `pgraph/ingest.go` | rashomon never runs the producer itself | W-M1.1 (index runs sync when absent) | AC-M1.0 |
| PL2 PROVEN facts | declares, calls, implements | built | `pgraph/extract.go` | no reader beyond family hash | W-M1.2 (engine reads them) | AC-M1.0 |
| PL3 ALGORITHMIC dispatch | may-call by CHA | built | `pgraph/cha.go` | same | W-M1.2 | AC-M1.0 |
| PL4 Trust tiers | tier is identity, never upgrades | built | `pgraph/tier.go` | same | W-M1.2 maps tiers | AC-M1.0 |
| PL5 Fact log | append-only reconciled log | built | `pgraph/log.go` | no compaction | W-M6.4 | AC-M6.4 |
| PL6 Two-speed invalidation | content-hash gate before re-extraction | none | `pgraph/syncroot.go` | per-package hash gate | W-M6.4 | AC-M6.4 |
| PL7 Blast radius | fan-in, fan-out, bounded reverse reachability as two numbers | stale (package-level callers exist in `platform/callers.go`; two numbers none) | `platform/callers.go` | reverse reachability; two numbers; symbol level | this is the first slice's task T1 (section 7), done by the agent under steering | AC-M1.1..3 |
| PL8 Impact and Obligations | strict identifier resolution | partial | `platform/read.go`, `symbols.go` | exact id or qualified name or refusal naming candidates | W-M3.3 | AC-M3.3 |
| PL9 No egress | loader env forced | built | `pgraph/ingest.go` | CI job under unshare -n | W-DEBT.2 | n/a |
| PL10 Stable identity | fingerprints link renames | built | `pgraph/identity.go`, `link.go` | a caller that links across commits | W-M6.4 | AC-M6.4 |
| PL11 Unknown never zero | reflection and cgo boundaries attach unknown fan-in | stale (PlatformRead prints unknown lines; detector none) | `platform/read.go` | fan-in (PL7); boundary detector | T1 carries the first; W-M3.3 the detector | AC-M1.3 |
| PL12 Untracked build config warning | a changed file outside every tracked config warns | none | `pgraph/doc.go` | detector | W-M3.3 | AC-M3.3 |
| PL13 Accuracy publication | precision and recall of dispatch published | partial | `pgraph/accuracy.go` | eval command | W-M6.5 | AC-M6.5 |
| PL14 Read-only demo | proposed | none | none | design | W-M7.4 | AC-M7.1 |

History and learning PF-HD:

| Row | Behavior | Status | Code | Gap | Work | Acceptance |
|---|---|---|---|---|---|---|
| HD1 History from day one | every delivered line has an outcome | partial | `steer/line.go` | label writer in runtime | W-M5.1 | AC-M5.1 |
| HD2 Lessons are hypotheses | tally and holdout per lesson | partial | `learn/learn.go` | per-lesson holdout comparison | W-M5.4 | AC-M5.3 |
| HD3 Episode mining | candidates from recurring failure signatures | none | `learn/learn.go Candidate` | the miner | W-M5.2 | AC-M5.2 |
| HD4 Typed deltas and refusals | five delta kinds, seven refusals | none | `learn/ledger.go` | the kinds | W-M5.2 | AC-M5.2 |
| HD5 Fact lifecycle with approval | observation to approval to reuse to retirement | partial | `learn/learn.go` | miner; mismatch seen at the next prompt | W-M5.2, W-M5.3 | AC-M5.2 |
| HD6 Delivery of learned facts | lessons delivered at the trigger, not only at the prompt | partial | `learn/ledger.go`, preflight | failure-time trigger | W-M2.3, W-M5.3 | AC-M5.3 |
| HD7 Team packs through git | pinned, expiring packs | partial | `cmd/rashomon/learn.go` | sha256 pins, 90-day expiry, org tier | W-M7.5 | AC-M7.3 |
| HD8 Human-correction counts | corrections mined and counted | none | none | miner | W-M5.2 | AC-M5.2 |
| HD9 Instruction-load report | which instruction files loaded | partial | `install/install.go` | capture of what Claude Code loaded | W-M7.3 | AC-M7.2 |
| HD10 Storage bounds and quarantine | caps enforced | partial | `learn/ledger.go` | 16 MiB and 64 MiB caps | W-M5.5 | AC-M5.4 |
| HD11 Scope key | normalized origin | partial | `baseline/baseline.go` | origin as key | W-M4.1 (repo node identity) | AC-M4.1 |
| HD12 Promotion needs a person | tiers personal, repository, organisation; taint | partial | `learn/ledger.go` | tiers and taint | W-M5.3 | AC-M5.2 |
| HD13 Earned, scoped autonomy | design once ED-9 decided | none | none | owner decision ED-9 | deferred (section 11) | n/a |
| HD14 Relax only through a holdout | every raise checked against holdout | none | `learn/ledger.go` | holdout record | W-M5.4 | AC-M5.3 |
| HD15 Slice 0 and E1 | backtest miner | none | `cmd/rashomon/backtest.go` | miner | W-M5.2 | AC-M5.2 |
| HD16 Learner reads content | miner reads bodies under consent | partial | `learn/ledger.go` | content miner | W-M5.2 | AC-M5.2 |
| HD17 Seed from existing sessions | seed command over audit's read path | none | `disclose/paragraphs.go` | seed command | W-M5.2 | AC-M5.2 |

### 2.4 PRD families that bind AutoSteer beyond the 44 rows (mapped at family level; their rows stay Luis's vertical ownership unless re-assigned in section 10)

| Family | Rows | Relevance | Status | Work |
|---|---|---|---|---|
| PF-ST Steer (router, levels, renderer, mirror, log, budget, timing, attendance, consent clamp, agent may only lower, CLI, continuity briefing, fault silence, patterns ST14..17, degradation) | 18 | the delivery channel every milestone uses | live (router, levels, log, budget, patterns); ST12 continuity briefing partial | W-M2.3 (first-write pattern), W-M4.3 (boundary pattern), W-M7.1 |
| PF-IV Intervention (off-track signals, fact ceiling, one-way doors list, ask form, deny is owner's, unattended, measured) | 7 | Check Conflicts' delivery level and the never-block rule (R2) | none beyond R2 | W-M2.4 (conflict lines at nudge level), W-M3.8 (one-way door list at commit, push) |
| PF-SP Spec and rule packs (spec.json, trust, managed, compile-bomb guard, declared facts, init, packs, convention stubs) | 8 | DECLARED tier for rules and decisions | built (spec.json, trust, kinds) | W-M2.1 (boundary and decision rule kinds), W-M5.3 (approval writes DECLARED rows) |
| PF-TR Trust and control (consent, disclose, status, detach, purge, never-write, selfguard, tamper, release, terminal safety, host stand-down, UX contract, hook cost) | 15 | every new read must appear in disclosure; hook cost rows | live | each work item that adds a read adds its paragraph (tripwire) |
| PF-MS Measure (detector precision, labels, demotion, holdout assigner, unbacked-done rate, scorecard, lab) | 14 | the measurement discipline M5 and every AC rely on | partial (labels, Wilson, holdout arm exist) | W-M5.4, W-M8 (eval harness in repo) |
| PF-RC, PF-FD, PF-RG, PF-AU, PF-SH, PF-BR, PF-DL | 61 | rashomon base: record, foundation, receipts, audit, sharing, breadth, delivery | live or Luis's | out of this plan's critical path; PF-BR (Cursor, Codex, SDK) is M7's adapter work; PF-SH is M7.5 |

### 2.5 What this matrix says

Nothing is `live` for the product's intelligence. The runtime (record, verify, steer) is live and reusable as the evidence store, the obligation engine and the delivery channel. The engine and the session graph are experimental and single-language. The gaps cluster into five mechanisms: a second-language producer into the graph (Go via pgraph), a knowledge importer, Check Conflicts wired to the diff, a persisted session frontier, and the label writer in production. Those five, integrated in that order and demonstrated live, are the product.

---


## 13. Rulings (owner, 2026-11-04, on the twelve decisions; the two left to the principal are marked "picked")

1. Plan location: `docs/autosteer/PLAN.md`, `docs/autosteer/TRACE.md` and `docs/autosteer/VISION.md` are committed as the single exception to the no-commit-docs rule. VISION.md is the 11-04 vision verbatim (`~/autosteer_vision.md`), read at every session start; the 10-31 end-state vision stays referenced from PLAN.md's Spec line. Nothing else under `docs/` or `docs-out/` is exempt.
2. Keep 70597cf0.
3. PR #281: merge now (picked). Reason: it is green and mergeable, W-M3.5 builds on it, and every week it waits is another merge of Luis's train through its six files. The three surviving mutants are coverage debt (W-DEBT.1), not behaviour, and stay tracked. The merge is a GitHub write: `set -uo pipefail`, `ls-remote` equals the PR head sha, then `gh pr merge`; never a force-push.
4. T1 baseline is the unaided agent; ablation only after signal at n=3.
5. Go facts enter through pgraph's sidecars; no pgraph import (AD12).
6. Identity migration in `internal/platform` in M6 (W-M6.4).
7. Agent-key lint tripwire in M1 (W-M1.7); adapters in M7 (W-M7.1, W-M7.7).
8. Luis re-sequenced into milestone sets (section 10.2); the owner tells him.
9. The model-assisted proposer stays closed until after M5, with the owner's reservation: if AC-M1.1 fails (the brief's unit line misses on the five pre-registered prompts), the resolver is the next experiment and this ruling is reopened. Recorded as the trigger in section 11.
10. The harness moves into `test/live/autosteer/` with fixtures by sha (AD13).
11. The T1 prompt (section 7.2) and the judge rules (section 7.4) are approved as written; the pre-registration commit freezes them before the first run.
12. n=3 cost ceiling: 2.50 USD mean per run (picked). Basis: the Python fixtures measured 0.305 to 0.472 USD per run; rashomon is a larger Go tree where Sonnet reads eight to fourteen files before its first edit and must run `go test`, so two to five times that is the expected band. Above 2.50 mean at n=3, n=10 waits for the owner's word. At the ceiling, T1 costs at most 20 runs times 2.50 plus 5 ablation runs, about 62 USD on the meter.
13. The T6 fixture (section 7.6) is decided when M6 opens, not now.

Execution order on exit from planning: (1) merge #281; (2) copy this plan to `docs/autosteer/PLAN.md`, the vision to `docs/autosteer/VISION.md`, start `docs/autosteer/TRACE.md` with the section 2 matrix and the section 13 rulings, commit on `autosteer/contracts-v2`; (3) W-M1.0 and the T1 pre-registration commit; (4) W-M1.1 onward in section 6.1 order; no push until the owner's word.

---

