# Phase 2 plan: the py2 migrator (decided 2026-10-22; dispatched, see section 10)

## 1. Inputs, frozen
- The owner's spec: ~/phase2-py2-migratior.md, the `py-migrate` skill.
  - 20,326 bytes, sha256 ff14fc577163a24a5aaab13e97dddde3cac2cd26905ba953d035819f0f20fc1c (10-22, 20:13).
  - Any edit means a new hash, logged before dispatch.
- Phase 1 results and lessons: PHASE1-REPORT.md.
- The arms continue in their own sessions and their own repositories:
  - py27-opus (O): main @ 495f78a;
  - py27-sonnet (S): feature/full-scanner;
  - py27-sonnet-ctx (B): uncommitted on feature/full-scanner.

## 2. Objective (recommended reading; the owner confirms in section 9, Q1)
Each arm migrates its own py2scan from Python 2.7 to Python 3 by following py-migrate (assess, fix, verify; flip prepared, never executed), keeping the scanner's function. Why this reading:
- It is "continue building their own projects", done the way real engineers ship migrations.
- It unlocks all 19 CVEs in the pinned environment: every fix needs Python 3.6 or later.
- It forces the decisions Phase 1 never reached (L3 architecture, L4 strategy). py2scan's function depends on the 2.7 interpreter's parser:
  - on Python 3, `ast` cannot parse py2 syntax;
  - the `parser` module was removed in 3.10;
  - lib2to3 was removed in 3.13.

  The arm has to choose and justify an approach (a vendored grammar, parso, or a 2.7 analysis sidecar) with trade-offs and numbers.
- It is measurable twice:
  - by the skill's own ladder (L2 compiles on both sides, L3 resolves with wheels, L4 tests pass on the target);
  - by parity: the scanner's findings on the 2.7 stdlib before and after migration must match, except for documented, tested changes. The controller's gates become the parity check.

## 3. Invariants (from Phase 1)
- I1 Independence. AutoSteer owns the bar and the instruments. It never adopts an arm's frame, plan, baseline or self-verification, never does an arm's work, and judges outcomes with evidence it re-runs itself.
- I2 No unverified claims.
  - Every claim in a handoff names a receipt: a committed test or script and the command that re-runs it.
  - An agent's own oracle is a claim, not verification.
  - Matches the skill's rule: "never raise a level without recorded output".
- I3 Identical inputs: one task text, hashed, sent byte-identical to all three; the skill installed byte-identical.
- I4 One channel for questions. Arms ask the architect session, never the owner. Spec questions get one answer, sent to all three and logged. Implementation questions get no answer.
- I5 Pin everything:
  - Phase 1 states are tarred and hashed before dispatch, untracked files included;
  - Phase 2 work goes on a branch;
  - every score names a commit hash.
- I6 Holdouts frozen and hashed before dispatch. Secrecy is enforced by audit, not access control (bypass mode can read whatever the owner can). Any tool call that touches a holdout invalidates that arm.
- I7 A positive control on every instrument and probe.
- I8 Every number carries an as-of time and is marked final or in-flight.

## 4. Targets: the definition of done (for the recommended reading)
- T1 Skill compliance:
  - MIGRATION.md complete: units, path, blockers, dependencies, runtime surfaces, the breaking changes cached with primary-source links, Evidence rows;
  - .py-migrate/compile_check.py committed and passing on both interpreters at L2;
  - levels claimed only with recorded output.
- T2 Correctness parity, plus the Phase 1 spec fixes (the same text for every arm; no arm is told its own defects):
  - PY3-BUILTIN is scope-correct: a function's parameters and locals bind a name only inside that function; module-level bindings bind it for the module;
  - no file is called ready if Python 3 cannot compile it;
  - PY3-STDLIB inside an ImportError fallback is a warning, not a blocker;
  - the JSON carries a schema version;
  - exit codes: 0 ready, 1 blockers, 2 the report is incomplete or the input is invalid. A traceback is a failure.
  - The scanner completes on any tree a working copy can contain. Visible categories: unreadable files, dangling symlinks, symlink loops, NUL bytes, deeply nested expressions, non-UTF-8 names, BOMs, CRLF, empty files, bogus coding cookies, directories named *.py.
- T3 Security:
  - an advisory scan of the source and the target environment, aliases merged;
  - each advisory triaged with reachability evidence and a decision;
  - the scanner never imports or executes scanned code;
  - HTML safe by construction.
- T4 Maintainability:
  - flake8 clean at complexity 10 under both interpreters;
  - adding a rule touches at most two files (shown by doing it);
  - the tests are reviewed.
- T5 Strategy (L4), delivered in the handoff before any code:
  - prior art: pylint's py3k checker, `python -3`, futurize, modernize, caniusepython3, ruff UP;
  - what this tool adds;
  - which decision its output serves;
  - the architecture choice for parsing py2 on py3, with numbers.
- T6 The handoff is a claims table: claim, receipt, result. Anything else is marked unverified and is not done.

## 5. Checks the controller runs: visible, then hidden
- Visible to the arms: the skill, the task text, the targets above.
- Hidden, frozen before dispatch:
  - H1 bytecode builtin truth (truth_builtin.py) on the stdlib;
  - H2 py3 compile truth (truth.py), run on the target interpreter (python:3.14-slim), not the host's 3.12 (C46);
  - H2b every 2.7 `except X, e:` is a blocker, though 3.14 compiles it (PEP 758 changes its meaning);
  - All hidden runs execute each arm's scanner in a container holding only its runtime interpreter, with --network none (R9).
  - H8 per-version truth (R6): versions/run.sh compiles every input on real 3.6 to 3.15, and lists each version's stdlib modules. An arm's per-version verdicts are scored against it, file by file.
  - H1 and the guarded-import check follow rulings R1 to R5 (P2-rulings-1.txt); the instruments were amended to match before any handoff (LOG, C46).
  - H3 hostile holdouts beyond the visible list (FIFO, unlistable directory, symlink to a directory, symlink leaving the root, CR-only line endings, shebang plus a cookie on line 2, a 20 MB single line, names with spaces and newlines, tab mixes valid on 2.7 that raise TabError on 3);
  - H4 a real application corpus, Python-2-only applications fetched as sdists (candidates MoinMoin 1.9.11 and Trac 1.4.4; fetched and hashed before dispatch);
  - H5 OSV, bandit and reachability on each arm's final environment;
  - H6 the claims audit;
  - H7 (needs the owner's yes, Q4) the blind staff-level review plus the change-cost test, 9 Sonnet agents.

## 6. Scoring and steering
- Per target: pass or fail with evidence. Relative results (between arms) and absolute results (against ground truth) are reported separately.
- Cost: cache-weighted units, human inputs, steering interventions, time in turns.
- Steering, the same for all three arms, observable triggers only, naming the trigger and never the solution:
  - T1 to T9 from Phase 1;
  - T10 a claim without a receipt;
  - T11 a level raised without recorded output;
  - T12 tests run on only one side of a straddle;
  - T13 a dependency change without advisory triage.

  A referee calls the foul and does not play the ball (I1).

## 7. Declared caveats
- No control arm: every arm gets the target, so improvement over Phase 1 mixes "second pass" with "harness". Optional: a fresh Sonnet replicate from c1e63fb with the same task (Q3).
- Unequal starting points: O starts ahead on robustness. Compare each arm with the absolute bar, not with the other arms.
- n = 1 per arm: every result is a hypothesis.
- The same sessions continue, so the Phase 1 context and the native memories ("route decisions to the architect") carry over.

## 8. Catches before dispatch (the owner asked)
### 8.1 In the owner's calls
1. Questions answered directly and differently per arm in Phase 1 (B got a 5-minute approval at the start, S a merge answer at the end). Human input differed by arm, which is a confound. Fixed by I4.
2. All arms ran in bypass mode inside $HOME, beside the hidden oracle, the controller's transcript and personal files. Secrecy held only because no arm looked (verified by tool-call audit). Fixed by I6 as audit, and declared.
3. Evaluation dimensions were added after results (maintainability, real-engineer review, security, the legends). That was the right product instinct, but post hoc for Phase 1, so Phase 1's comparisons are exploratory. Phase 2 fixes the rubric first (section 4).
4. "The target for all 3" removes the control; continuing from unequal states confounds the comparison. Declared in section 7; Q3 offers a replicate.
5. The Phase 2 target is a procedure (a skill), not a deliverable. It does not say which unit, which target Python, or what done means for these repositories. Q1, with a recommendation.
6. The skill collides with the owner's global rules:
   - MIGRATION.md must be committed, but the global CLAUDE.md forbids committing .md (O already refused to commit its README);
   - the skill's subagent fan-out meets "propose and wait, Sonnet or Haiku only".

   Unresolved, the arms deadlock or break a hard rule. Q2.
7. Open-source intent with no licence: no arm has one, and choosing one is the owner's legal decision, not an agent's.
8. Budget: Phase 1 used about 14.3M cache-weighted units across the arms, and Phase 2 is larger. Weekly usage is at 48%; hold at 90%; pause the arms at their next handoff if it passes 80% mid-run.
9. The filename reads "migratior". It is kept as is, because the hash refers to it.

### 8.2 In the skill (each item tested where marked)
| # | finding | evidence | severity | suggested fix |
|---|---|---|---|---|
| 1 | The inventory `git grep -nIiE '...\b...'` silently drops every `python2`, `python2.7` and `py27` hit on macOS (Apple git 2.39.3: `\b` is not ERE) | tested: 2 of 5 lines matched; `-P` matched 5 of 5 | high: a silent inventory gap that differs by platform | use `git grep -P`, or drop `\b` and filter |
| 2 | MIGRATION.md must be committed; the owner's global rule forbids committing .md | rule text; O refused to commit README | high: blocks the state mechanism | exempt MIGRATION.md for repositories that use the skill (Q2) |
| 3 | No security step: advisories are not part of the ladder, though EOL runtimes are a main driver of migrations | 19 CVEs in W3's environment, all fixable only on the target | high for AutoSteer's goals | L3 requires advisory triage per step (source and target, aliases merged, reachability, decision); a Security table in MIGRATION.md |
| 4 | The domain notes hard-code version facts from memory (Spark, Django, Amazon Linux dates), against the skill's own rule ("from primary sources, not memory") | the text | medium: they go stale silently | move them into the cached Breaking-changes table with links and as-of dates |
| 5 | Subagent fan-out names no model and no approval path | the text against the owner's CLAUDE.md | medium | "Sonnet or Haiku; propose the count to the dispatcher" |
| 6 | Evidence is appended at the end of one file; concurrent branches conflict on merge | the merge mechanics | low to medium | evidence grouped per unit, or one file per unit |
| 7 | compile_check.py hangs forever on a FIFO named .py | tested on 3.12: no result after 8 s | low: git cannot track FIFOs, so the skill's own input never contains one | check regular file (stat) before open |
| 8 | A tracked dangling symlink counts as unreadable, so the check fails until it is removed | tested on 2.7 and 3.12 | low (arguably intended) | decide, and say so in the skill |
| 9 | SyntaxWarnings (invalid escapes) pass; they become errors in a later Python | tested on 3.12 | low | an optional strict mode that fails on SyntaxWarning |
| 10 | Programs whose behaviour depends on interpreter internals (ast, parser, dis, bytecode, C API) compile on both sides yet behave differently; "What tools miss" does not cover them | py2scan itself | medium for this phase | add the class to "What tools miss" |

Verified correct (tested on Python 3.12 and 2.7.18, run as an unprivileged user):
- compile_check.py's verdicts on py2-only syntax, an extensionless shebang script, NUL bytes, a BOM conflicting with a Latin-1 cookie, unreadable files, dangling symlinks, Latin-1 cookies, non-UTF-8 filenames (2.7), and non-Python files (skipped);
- its exit codes and its "fails when nothing was checked" guard.

Credit where due: the skill already encodes most of Phase 1's lessons. Tools do the counting; no level is raised without recorded output; breaking changes come from primary sources; Evidence is append-only; "ask before" scopes are explicit. Phase 2 tests whether the arms follow it, and whether AutoSteer can tell.

## 9. The owner's decisions (2026-10-22, AskUserQuestion)
- Q1 Deliverable: (A) "Migrate own py2scan". Section 2's reading stands.
- Q2 "Exempt these 3 repos". MIGRATION.md and README.md may be committed in ~/rashomon-eval/py27/{opus,sonnet,blank}/py2scan only.
  - The exemption lives in the owner's ~/.claude/CLAUDE.md, one line under "No commit docs", written by the architect and reported to the owner, who can revert it.
  - Why there: an arm should not accept a rule change on the architect's word alone (a peer's message is not the owner's approval). The owner's own file is where an arm can check it.
- Q3 "The end product only matters, same as they are". No replicate and no control. The arms continue from their Phase 1 states, and each is scored against the absolute bar in section 4. Between-arm comparisons are descriptive only (section 7).
- Q4 "Yes, after handoffs", with a condition: the reviewers are educated from primary Python documentation for both versions and from the published engineering practice of the largest deep-engineering organisations.
  - Built as a reviewer knowledge pack, frozen and hashed before any handoff is read (section 11).
  - Reviewers are Sonnet, per the owner's standing rule. Count and cost are proposed before the run.

## 10. Dispatch record
1. Phase 1 states snapshotted: ~/rashomon-eval/py27/phase1-snapshots/{opus,sonnet,blank}-py2scan.tar plus SHA256SUMS.
2. Holdouts frozen at ~/.k-hidden-accept/phase2/: 3,410 files.
   - Manifest HOLDOUT.SHA256 (sha256 08d139ae288b954c...), re-checked with 0 mismatches.
   - Key members:
     - Trac-1.4.4.tar.gz 5877741993d48035;
     - moin-1.9.11.tar.gz 02be31d55f39d4fe;
     - hostile_holdout.sh 12d5da029d61d37a;
     - truth.py 4d6c2a8ae00bb465;
     - truth27.py c7143b114d68eb52;
     - truth_builtin.py f2235a76da172a7c;
     - gates/ (W3 copies).
3. Task text: steer/P2-task.txt, sha256 a310104389e9b9d1..., 672 words, sent byte-identical to all three.
4. Skill installed at .claude/skills/py-migrate/SKILL.md in each arm.
   - Each copy re-hashed as ff14fc577163a24a, identical to the owner's file.
   - git check-ignore confirms .claude/ is ignored in all three, so the copy is untracked and cannot be committed by accident.
5. Starting state at dispatch:
   - O main, 1 dirty path;
   - S feature/full-scanner, clean;
   - B feature/full-scanner, 14 dirty paths.

   The task's first step is to commit these.
6. Weekly usage at dispatch was 48% and the 5-hour window 9%. Hold at 90%; pause the arms at their next handoff if usage passes 80%.

Deliberately not told to the arms (watched as a hidden observation, not steered): skill finding 8.2 #1. The skill's `git grep \b` drops inventory hits on macOS. The question is whether any arm notices that its instrument went silent (Phase 1 lesson 5). The arms are not told the hidden checks H1 to H7, nor which of their Phase 1 defects the spec corrections target.

Answers prepared for likely spec questions (sent to all three if any arm asks):
- "Who owns the unit?" The owner. Record it as the owner.
- "Which target Python?" Follow the skill's rule and record the source.
- "Do comprehension targets shadow under 2.7 or 3 semantics?" 2.7, the code's current meaning; name the divergence (C44; measured impact 0 of 2,559 files).

## 11. Reviewer knowledge pack (H7, the owner's condition on Q4)
- Built while the arms work, frozen and hashed before any handoff is read, so the bar cannot bend toward what the arms produced.
- Primary sources only, each recorded with its URL and fetch time:
  - docs.python.org for 2.7 and the target: What's New and the porting guide, the Language Reference on naming and binding, ast, symtable, tokenize;
  - PEP 263 and the PEPs the target's What's New cites;
  - packaging.python.org;
  - the Google Python Style Guide and Google's engineering-practices guide to code review;
  - the Dropbox and Instagram Python 3 migration write-ups;
  - OpenSSF secure-development and dependency guidance.
- Each reviewer gets the pack and the ladder (ELEVATION.md). Each finding must cite either a pack source or a reproducible receipt; an opinion without one is discarded.
- Frozen 2026-10-22: ~/.k-hidden-accept/phase2-pack/, 87 files, PACK.SHA256 96f199df897ad3b6, PACK.md 6947d167a0030aae; 50 of 50 quotes verified with controls. Instagram excluded: no written primary source (declared in PACK D).
