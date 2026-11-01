# AutoSteer Phase 2: synthesis (2026-10-22 20:38 to 2026-10-23 08:17)

Not for merge. A lab record for review.

Every number below comes from a generated file in this PR:
- AUTOSTEER-RECORD.md, produced by ci/record.py from ledger.jsonl, calls.jsonl, notices.jsonl, events.jsonl, decisions.jsonl and instruments.json;
- the hidden-check reports (hidden/out/*/report.json);
- the battery scores (hidden/postfreeze/extra/score-*.txt).

Times are local (UTC-7).

## 1. What ran

Three agent sessions ("arms") each migrated their own py2scan, a Python 2.7 readiness scanner, from 2.7 to 3.14 while straddling both, under the py-migrate skill:
- O: Opus.
- S: Sonnet.
- B: Sonnet, the blank-context arm.

AutoSteer refereed them. It used instruments it built and ran itself; it never did an arm's work; it named failures, never fixes. Hidden inputs went to the arms as counts only.

The task is P2-task.txt (items 1 to 6); the plan is PHASE2-PLAN.md.

Timeline:
- Dispatch 20:38:08.
- B closed by the owner at 02:26 (D17).
- O and S time-boxed at 02:27.
- Both stopped at 08:17 on the owner's usage cap (D20).

## 2. Final results, measured by the referee

| | S, final ffdbd9e | O, final ab361ee | B, closed at 64ced43 |
|---|---|---|---|
| R1: a file no Python 3 compiles reported free of blockers (hidden corpus, 2,331 files) | 0 | 0 (14 at d34df6a) | 1 (under 2.7) |
| R8: stdlib removals, 413 expected | 413 exact | 223 exact, 131 missing, 59 wrong severity | 2.7: 367 exact, 46 missing, 29 false; 3.14: 201 exact, 212 missing |
| R3: PY3-BUILTIN against bytecode truth | 0 missed, 0 false | 32 missed, 0 false | 2.7: 0 missed, 0 false; 3.14: 236 missed |
| 2b: byte-identical JSON, 2.7 vs 3.14 | corpus yes; 9 of 9 probes; 40 of 43 battery (the other 3 are its failing cases) | corpus, hostile, 9 of 9 probes, 43 of 43 battery | no: 3.14 finds 4,017 against 6,353 on 2.7; 7 of 9 probes differ |
| Held-back 43-case battery, problems | 3 (1 false-ready, 2 tracebacks) | 0 | 4 on 2.7 |
| Held-back hostile tree | clean | 1 (very long line: 2.7 timeout, 3.14 traceback) | clean |
| Item 4: CPython 2.7.18 advisories named | 37 of 37 | 37 of 37 | not re-measured |
| Item 5: a new rule touches at most 2 files | shown (cb0d66f: 2 files) | claimed, no receipt | not measured |
| Item 6: strategy section | delivered 22:09 | delivered 02:57 | not started |
| Items 1 and 2a (ladder, before/after) | not measured by the referee | O reports: L2; 2a fails at HEAD | not measured |
| Source lines (non-test) | 1,977 | 6,362 | 1,415 |
| Parser for 2.7 source | parso 0.7.1 (pinned) plus two text rewrites before parsing | own port of 2.7's tokenizer, pgen parser and ast.c | the host interpreter's ast (3.x cannot parse 2.7) |

Neither remaining arm is finished:
- S passes everything measured except 3 held-back battery cases.
- O passes every soundness check, and every same-output-on-both-interpreters check, on every input we have. It is short on rule content: R8, R3, and R2 (by its own report).

## 3. S against O: product against foundation

S ends closer to the spec. O ends with the stronger foundation. Both statements hold, and the gap between them is mostly feedback exposure, not ability.

| | S | O |
|---|---|---|
| Verdicts with hidden-check results | 6 | 1 (the final one, 08:16) |
| Messages from AutoSteer, all kinds | 15 | 7 |
| Commits after dispatch | 35 | 12 |

The cause was the hold policy. Hidden findings wait for a handoff. S handed off often and was corrected each time; O did not hand off until 02:57, so it worked without hidden feedback for 6 hours.

Without that feedback, O still fixed on its own:
- R5 listing;
- R6 naming (1,457 unnamed to 0);
- the crash of its whole 3.14 side;
- every battery case;
- all 14 files wrongly reported ready.

The architectures explain the rest:
- O's own 2.7 front end makes item 2b true by construction: the same code parses on both hosts. O reports its trees equal 2.7.18's on 1,104 of 1,104 stdlib files; the referee did not re-run that claim. It costs 3 times the code.
- S borrowed a parser. That reduced the job to rules and made it fast to get right. The cost is a permanent dependency on parso 0.7.1 and two text rewrites before parsing (the ur-prefix blanking, and print to pr1nt under print_function). Its last 2b defect was the host's own error text leaking into the JSON. That is the same class of leak O cannot have.
- B parsed with the host. On 3.14 every tree-based rule is lost for any file 3.x cannot parse. B called this "structurally unrecoverable" itself, which is why it was closed.

Comparing the two: one run per arm, no control arm, unequal starts (O started ahead on robustness), and unequal feedback. Read every between-arm difference as descriptive, not causal.

## 4. Merging the two (owner idea, 08:17:08: something fun to open source)

The complementary halves are clear:
- O's front end and soundness machinery: per-version compile verdicts checked against real 3.6 to 3.15, identical output on both hosts.
- S's rule knowledge: R8 413 of 413, PY3-BUILTIN scope-correct with 0 missed and 0 false, PY3-INVALID-SCOPE, PY3-MIXED-STR-BYTES.

A merge would keep O's py27 front end and report schema 2, and port S's rules onto O's tree.

The port is real work. S's rules walk parso's concrete tree, while O exposes 2.7's own ast. S's data tables (removed names and their primary sources) and its tests carry over directly.

The acceptance bar already exists, in this PR's instruments: the hidden check, the probes, the 43-case battery, and the cross-host 2b diff. The merged scanner should pass all of them at once, which neither arm does today.

Owner decisions open before any open-sourcing:
- a licence (no arm chose one; the plan treats it as the owner's legal call);
- authorship and provenance of agent-written code;
- fresh holdouts, because these were published here (D22).

## 5. AutoSteer's own scorecard

Items AutoSteer should catch: 73.

| outcome | count |
|---|---|
| acted on in time (10 min) | 11 |
| acted on late | 50 |
| missed (the arm fixed it first, or never called) | 12 |
| not measurable (no instrument) | 5 |

Timing: the median lag from appearance to act is about 100 min, while arms fix in a median of about 21 min once told. The time is lost before an arm is told.

Counterfactual: had every built public-input instrument run on every commit from dispatch, in-time detection rises from 21% of observable items to 65% on spec-named inputs alone.

AutoSteer's own errors: 32, all with a root cause, a guard and a recurrence link (record section 2).

| root cause | count |
|---|---|
| ground truth wrong | 6 |
| identity and keys | 6 |
| record typed, not derived | 8 |
| verdict prose not derived from evidence | 5 |
| claim with no running instrument | 4 |
| ops and guards | 3 |

Errors that reached an arm: 7 (E05, E07, E15, E17, E28, E31, E32).

## 6. Learnings

1. **Validate the gauge before judging the process.** This is Measurement System Analysis. Every class of AutoSteer error is a claim no controlled instrument had produced. Preflight every instrument on each arm's base, with a positive control, before dispatch: 22 of the 73 items were already present at minute zero.
2. **Derive, never type.** Typed times, labels and statuses went stale or wrong repeatedly (E02, E20, E23, E25, E27, E29, E30, E31). The lessons written as prose came back; the ones turned into code with a control that fails on the old rule did not.
3. **Invariants that need no oracle catch what oracles miss.** Comparing the same input across two hosts (item 2b) found S's message-text leak and B's 3.14 loss without knowing the right answer. Build such checks over every input set, not over a hand-picked subset (E28).
4. **Feedback cadence is part of the treatment.** A hold-to-handoff policy gives more steering to arms that hand off more. Either release a public canary per hidden class (the open O01 design), or require handoffs on a clock.
5. **Task text decays.** Two arms lost the Phase 2 task text to context compaction (L07, L73). A message that names spec items must quote them (E32). The product fix is re-delivery at compaction (O04).
6. **Arms report honestly when asked for receipts.** The claims-table format, with each claim's receipt and result, produced "not done" and "unverified" lines unprompted from all three arms. The referee's job is to re-run, not to re-ask.

## 7. What is in this PR

| path | what |
|---|---|
| docs/SYNTHESIS-2026-10-23.md | this file |
| docs/AUTOSTEER-RECORD.md | every item, error, message, instrument and decision (generated) |
| docs/AUTOSTEER-LEDGER.md | the ledger table (generated) |
| docs/SELF-REVIEW-2026-10-23.md | the referee's self-review and the MSA gap |
| docs/PHASE2-PLAN.md, docs/P2-task.txt | the plan and the task text |
| docs/CONTROLLER-CHECKLIST.md, docs/AUTOSTEER-LESSONS.md, docs/LOG.md | the controller's checklist, lessons and running log |
| ci/ | the visible-input CI, ledger, record generator and data files |
| hidden/ | the hidden checker, truth tables, reports, post-freeze probes, battery and scores, security coverage (holdouts retired, D22) |

Excluded:
- raw input corpora (stdlib, Trac and MoinMoin tarballs), for size;
- raw per-run scan output;
- session transcripts.
