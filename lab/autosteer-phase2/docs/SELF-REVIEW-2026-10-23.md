# AutoSteer Phase 2 self-review, 2026-10-23 01:50 (referee session; in flight, not final)

Sources: ci/ledger.jsonl, rendered as steer/AUTOSTEER-LEDGER.md (102 items; generated, with a gate that refuses estimated times), plus calls.jsonl, notices.jsonl, events.jsonl, ci/backtest/, and the hidden reports under handoff/out.
- Every number here comes from those files as of 01:46.
- Kept under ~/.k-hidden-accept because it discusses the hidden corpora and the truth-table internals.

## 1. Caught, when it acted, missed, and what more it could have caught

| | count |
|---|---|
| Items AutoSteer should catch (O 17, S 26, B 27) | 70 |
| Acted on | 56 (10 within 10 min, 46 late) |
| Found but not yet told (all O; O has not handed off) | 9 |
| Missed: the arm fixed it first, or the referee never called it | 5 |
| No instrument exists | 5 |
| AutoSteer's own errors | 27 |

Per arm:
- O: 8 late, 9 pending.
- S: 4 in time, 20 late, 2 missed.
- B: 6 in time, 18 late, 3 missed.

Timing: the median lag from appearance to act is 100.5 min (56 items); the median fix after an act is 21.5 min (36 items). The arms fix in about 20 minutes once told. The time is lost before they are told.

What more could have been caught, if every built public-input instrument had run on every commit from dispatch (one CI round = 8 min, the measured p75):
- **Spec-named inputs only** (the 2.7 stdlib, item 3's hostile list, the arm's own tests): 23 more items in time. In time rises from 10 of 50 observable items (20%) to 33 of 50 (66%).
- **Plus probes invented after dispatch** (hindsight, an upper bound): 8 more, giving 41 of 50 (82%).
- Known-bad state removed: 4,496 item-minutes as of 01:57 (it grows while O's items stay untold). An item never told counts to the arm's own fix, or to now if unfixed (corrected 01:58; the first count ran missed items to now).
- **Held by policy:** 20 items came from hidden inputs and wait for the handoff. Only a policy change moves them; see section 6a.
- **Beyond every built instrument:** 9 observable items no built instrument catches in time.
  - Referee judgement: L01, L45, L61.
  - Unbuilt instruments: L17 (security in CI), L34 and L71 (Evidence content), L35 (handoff coverage), L44 (skill order). L51 (security coverage) is a script as of 01:31.
- **Present at minute zero:** 22 of the 70 were already in the arm's own Phase-2 base.
  - Each arm's base is the replay's first commit: O bf6499e, S df86749, B 0e39731. B committed its base at 20:54:57, 17 min after dispatch.
  - A baseline run on each base, sent with the dispatch message, would have put them in front of the arm at minute zero.
  - B's 3.14 side could not build until 21:31:11. Its 3.14 items are dated from first observation, so their lags are lower bounds.

## 2. My errors: 27, by root cause

| class | what happened | ids | n |
|---|---|---|---|
| A. Claim with no running instrument | Sensors off for 50 min; "met" on hostile inputs never run; a replay run on stale code | E01, E05, E21 | 3 |
| B. Ground truth wrong | Truth blind to four renames; a FIFO rule against R5; a line convention; stdlib self-imports; citations; a typed unconfirmed list | E03, E04, E08, E12, E16, E26 | 6 |
| C. Identity and keys | One thing under two names, or two under one: class label, double count, alias merge, test-id escaping, unlike sets compared, wrong key on L13 | E10, E13, E14, E19, E22, E24 | 6 |
| D. Verdict prose not derived from evidence | No public input named; a judgement call labelled a defect; an omission; "met" over 46 misses | E07, E11, E15, E17 | 4 |
| E. Record typed, not derived | Estimated LOG times; hand class labels; "23:2x" found times; stale receipts; a LOG decision time | E02, E20, E23, E25, E27 | 5 |
| F. Ops and guards | Deletion guard misfired; delete-as-fixed hole; containers killed by a shared label | E06, E09, E18 | 3 |

Who found them:
- Me, before sending: 9 (E03, E06, E12, E13, E14, E16, E19, E22, E26).
- Me, after the fact: 12 (E27 at 01:52).
- The peer audit: 5 (E04, E09, E11, E20 jointly, and E25 for L09).
- An arm: 1 (E07).

Four errors reached an arm: E05, E07, E15 and E17. All four were in one message, the round-1 verdict to S (P2-10, 22:54:24), written by hand before the visible CI existed. None found in the 16 messages since (P2-11 to P2-26). That is not proof there are none.

Lessons written as prose came back:
- E26 is C59 again (a list typed where the data had it).
- E24 is E10 again (wrong class key).
- E23 is C52 again (estimated times).
- E22 is C21 again (unlike sets).

Lessons turned into code with a control have not come back:
- tnorm_map;
- the C58 and E26 controls, each failing on the old rule;
- the ledger lint gate;
- the derived class.

## 3. Staff or distinguished?

Neither yet. Execution is strong senior, and detection and self-correction reach staff level in places. The system as a whole is not staff-level, is far from distinguished, and is not polished.

The bar used:
- **Senior:** thorough, and fixes what it finds, but needs review to be right.
- **Staff:** builds mechanisms that make a class of failure impossible before the work starts. Its claims can be relied on without re-checking, and its own error rate falls by design.
- **Distinguished:** its methods generalize past the task and anticipate failure classes from first principles. Others adopt them.

Staff-grade evidence:
- The hidden/public boundary held. No arm transcript mentions steer/ or the hidden directory, checked with a control term found in all three.
- Every truth repair now ships with a control that fails on the old rule.
- Errors caught before sending: 9, all after 22:48. None found reaching an arm since 22:54.
- Oracles are the real interpreters 2.7.18 and 3.6 to 3.15 (differential testing, not opinion).
- The arms improved under the loop:
  - S went from 5 items not met to 3, 2 of them partial.
  - O cleared 6 of 7 visible items without a second notice.
  - B cleared its builtin and probe items.
  - With one run per arm and no control arm (owner's Q3), this is association, not cause.

Evidence against:
- 27 errors of my own in about 5 hours, 6 of them in the ground truth.
- 46 of 56 acts were late (median 100 min), because the instruments arrived 2 to 3 hours after dispatch.
- 59 checklist items, each born from a failure: a reactive record, not a design.
- Three instruments existed only in the transcript until tonight: the probe runner, security coverage, and the report comparison.
- The ledger, the artifact meant to prove correctness, needed 9 corrections in one pass, and a peer found 3 of my errors before I did.
- Polished means an adversarial reader finds nothing to correct. Tonight a peer did.

## 4. The gap: one hypothesis

AutoSteer checks the arms' claims, but it did not check its own measurement system before using it. Every error class above is the same act: AutoSteer stated something that no pre-built, controlled instrument had produced. The something was a truth entry, a "met", a found time, a class label, or a count.

The textbook name is Measurement System Analysis (MSA), from manufacturing quality (the AIAG MSA manual; gauge R&R). Before you judge a process with a gauge, you show the gauge is:
- present and running (stability);
- unbiased against a reference standard (bias);
- consistent across paths and operators (repeatability and reproducibility).

Its readings are recorded by the instrument, not transcribed. Tonight maps onto it one for one:
- stability: A;
- bias: B;
- reproducibility: C;
- recording: E;
- reporting: D.

In software testing this is the test oracle problem (Barr, Harman, McMinn, Shahbaz and Yoo, IEEE TSE 2015). An oracle is a claim and must itself be tested; mutation analysis measures how adequate it is. In control terms, a steering loop is bounded by its observer, and an unvalidated observer makes the controller confidently wrong.

What the gap is not:
- Python knowledge, speed, or reading the rules. Every truth error was fixed within minutes once seen.
- It is sequencing: the gauge first, then the process.

## 5. What to do, ranked by measured payoff

1. **Preflight gate before any dispatch** (MSA stability and bias).
   - Every instrument exists as a file and runs on each arm's base, with a positive control and a red/green reference.
   - The base findings go in the dispatch message.
   - Payoff: 22 items at minute zero, 23 more in time, about 4,500 item-minutes.
2. **Truth under bias control** (class B, 6 errors).
   - No typed lists: every entry is derived from data.
   - Every citation is checked by a script.
   - Every repair carries a control that fails on the old rule.
   - Every table gets an independent check before it is frozen.
3. **One canonical-key module** (class C, 6 errors).
   - It covers test ids, advisory aliases, line conventions, and finding identity, with one control table.
   - The peer proposed the same independently.
4. **Verdicts rendered from the ledger, not written** (class D, 4 errors; the only class that reached an arm).
   - Each "Met" line is generated from an instrument output and names its inputs.
5. **Every run record carries its instrument's sha256** (class A; would have flagged E21 at once).
6. **Build the missing instruments.**
   - Security in CI, Evidence content, handoff coverage, skill order, the 2a before/after diff, and a rule-addition probe.
   - H7 blind review needs the owner's yes and a cost estimate first.
7. **Replicate before claiming.** Run k >= 3 per arm before any claim that AutoSteer raises agents to staff level (C19).

## 6. Owner decisions open

a. **Mid-run verdicts and the hold policy.** Take the peer's proposal:
   - When the hidden check finds a class, release a public canary of the same class built from a different instance.
   - Keep the hidden instance frozen, and measure whether the arm fixes the class or only the instance.
   - This recovers most of the 20 held items without spending the holdout.

b. **Move hidden-class content out of steer/.** AUTOSTEER-LEDGER.md names the hidden corpora and the battery shapes.
   - No arm has read it so far (measured).
   - Recommended: moving costs nothing, and the risk is an invalid holdout.

c. **B's pin question.** markupsafe 1.1.1 has no 3.14 wheel, and the project rule says never upgrade a pin.

d. **L07 (B lost the task text to compaction).** A compaction-time re-delivery hook is a product feature for AutoSteer. It is not for this run, because the arms' inputs are frozen (C16).

## 7. The peer's readings, verified

| reading | peer | measured | holds? |
|---|---|---|---|
| class is a hand label (I2) | L59, L61 wrongly in time | confirmed; also L57 wrongly late | yes, now derived |
| in time | 9 of 67 | 10 of 70 | corrected |
| observable rate | 42, with 90% possible | 50; 66% on spec-named inputs, 82% with every built public instrument | corrected: the 5 unmeasured items were subtracted, but they were never in the total |
| fix speed | median 11 over 22 | 21.5 over 36 (O's fixes now recorded) | conclusion holds: detection 100 min against fix 21 |
| at base | 18 | 22 at each arm's own base | holds |
| identity class | 5 of 19 | 6 of 26 | holds |
| L09 receipt stale | yes | yes; O fixed L09 at 42e7b7d | fixed |
