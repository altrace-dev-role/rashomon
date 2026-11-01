# AutoSteer lessons from the 10-22 limit test (living document)

Each lesson gives:
- Evidence: what happened, with its record in LOG.md or approaches/.
- Mechanism: what AutoSteer does about it, phrased as harness behaviour (a guard, sensor, injection or workflow stage), not as advice to the model.
- Models: which models it applies to.

Status is "supported" when this run's evidence backs it, and "hypothesis" when it needs more runs. One run per arm is an anecdote.

## A. Specification (most leverage)

### A1. A testable spec mattered more than the model. Status: hypothesis (downgraded from "supported" at W3 close, C18).
- Evidence:
  - W3's spec named exclusions (not 0o777, not `except (A, B):`, "unless the module binds that name").
  - S matched all 34 hidden findings. On the 1,335-file stdlib, though, S emitted 322 PY3-BUILTIN findings O and B did not: 106 of them are real bugs (binding forms the spec calls assignments), and 216 come from my spec gap on parameters. The 34-finding oracle could not see either (C17).
  - So the clauses the spec stated explicitly were met by every arm, and the arms diverged exactly where the spec relied on enumeration from memory. The lesson is narrower than first written: explicit clauses transfer across models; implicit enumerations do not.
  - In W1 the request was phrased as a negated spec, and the same model inverted it (S0 correctness 0).
  - The arms were not bare (C16): all three had the owner's global CLAUDE.md and plugins.
- Mechanism: a spec-hardening stage before dispatch, run by the strongest model or a human. It rewrites the request into positive acceptance statements and adds:
  - the near-misses that must not fire;
  - the precision and format of every number;
  - the exact runtime and command used for verification.
  - The dispatch is rejected if any acceptance statement is untestable.
- Models: all. It helps the smaller models most.

### A2. The controller's own spec defects cause "agent failures". Status: supported (C1, C3, C15).
- Evidence:
  - W1's polarity (C1);
  - W2's reference contradicted the documented contract (C3);
  - W3's oracle demanded unrounded readiness the spec never asked for (C15).
- Mechanism:
  - every reference outcome is checked against the repo's docs and its dependent tests before use;
  - every oracle is validated red/green and mutation-tested before it judges anything.

## B. Steering (correct while it happens)

### B1. The acceptance-mismatch trigger works. Status: supported, W1.
- Evidence: one nudge of about 90 words, "your handoff does not meet the stated acceptance criterion", took S from correctness 0 to 2. It gave no hint of the answer.
- Mechanism: a T8 sensor compares the acceptance statements against the handoff's claims and receipts (fail asked, pass reported) and injects the gap.
- Models: Sonnet observed; Opus untested on this trigger.

### B2. Steering must be defeasible. Status: supported, W2.
- Evidence: the agent overturned the controller's wrong reference with evidence (a documented contract plus a dependent test). Enforcing the reference would have shipped a breaking change.
- Mechanism:
  - an agent may answer any steer with evidence (docs, a failing dependent test);
  - AutoSteer records that as "steer contested", re-checks its reference, and never re-sends the same steer blindly.

### B3. Steering text must be calm and self-contained. Status: supported (F13, C4).
- Evidence:
  - Sonnet called the owner's coercive SessionStart context ("EXTREMELY_IMPORTANT", "YOU DO NOT HAVE A CHOICE") a prompt injection;
  - an internal label ("W1") in a steer cost the agent a search.
- Mechanism: a lint on steering text that rejects:
  - capitalised imperatives;
  - unexplained references or labels;
  - any claim not backed by a receipt.

## C. Environment (prevent rather than instruct)

### C1. A discoverable environment beat a prose instruction. Status: supported, W3.
- Evidence: all three arms found and used the 2.7 image from the repo's own Dockerfile.test. None verified on the host's Python 3. The engineered CLAUDE.md did not change that outcome. Cost: B 4.51M against S 4.01M cache-weighted units (1.12; B made 218 calls against S's 155), with B's 2 human inputs against S's 1. At n = 1 (C19) no cost effect is claimed. B's correct binding logic is not traceable to its CLAUDE.md, which says nothing about binding (C38).
- Mechanism: AutoSteer's repo bootstrap puts the runtime into the repo as an executable target (a test image and one test command), never only into prose. A T9 sensor still watches for test runs on an interpreter other than the declared runtime.
- Models: all.

## D. Delegation and model choice

### D1. A prose rule to "propose and wait" turns into "never fan out" when no human is present. Status: supported, W3.
- Evidence:
  - all three arms ran zero subagents, and all three cited the owner's CLAUDE.md rule;
  - the ultracode reminder was overridden every time;
  - B blocked 5 minutes for human approval of a peer-dispatched task;
  - S asked the human a merge question that belonged to the dispatcher.
  - Scope of the claim: the rule reached the arms through the owner's global CLAUDE.md, which leaked into every arm (C16). The finding is about what that prose rule does to an unattended session, not an effect of any arm's condition.
- Mechanism: a dispatch envelope with an explicit authority scope, enforced by a PreToolUse guard on Agent and Workflow. It states:
  - what the session may do without asking (implement on a branch, run tests, commit locally);
  - who decides integration (the dispatcher);
  - the delegation budget, which the guard checks before any spawn:
    - model in {sonnet, haiku};
    - at most N agents and T tokens;
    - roles allowed without approval: verify, review, fixtures;
    - roles that need approval: implementation fan-out.

### D2. Subagents pay off in verification, not implementation, at this task size. Status: hypothesis, partly supported.
- Evidence:
  - the task was a single module of about 20 minutes, with shared files (the rules registry), so implementation fan-out would mostly have added integration cost;
  - the highest-value move was independent verification against the spec. Opus did it inline (a differential oracle built from 2.7's lib2to3); neither Sonnet arm did;
  - in this run the hidden spec-only oracle was the only thing that judged correctness independently of the implementer.
- Mechanism: a standard stage after implementation and before handoff.
  - A fresh-context verifier (Haiku or Sonnet) receives only the hardened spec and writes black-box acceptance tests without seeing the code.
  - The implementer must pass them.
  - A differential check against an independent parser is used wherever one exists.

### D3. Route models by role, not by session. Status: hypothesis, all three results in.
- Evidence (final records):
  - all three reached intent 10/10 on the small oracle;
  - cost in cache-weighted token units (output 5x, cache write 1.25x, cache read 0.1x input; the per-model price ratio is a separate multiplier): O 5.82M, S 4.01M, B 4.51M. Output tokens alone (583k, 298k, 272k) overstated O's relative cost and made B look cheaper than S when it cost more (C37);
  - against an arm-independent builtin ground truth (bytecode, scope-correct, 549 real uses): O and B miss 16, S flags 273 non-uses and misses 1. On 12 hostile inputs O handles 11; S and B crash on the same 5 (C35, C36);
  - Opus spent its extra tokens on verification it chose itself: syntax-tree probing, a lib2to3 differential, a bytecode cross-check, and a list of judgment calls disclosed for override. Its distinctive value was the verification and the handoff honesty, not the first draft.
- Mechanism:
  - spec hardening, ambiguity and architecture: the strongest model;
  - implementation: Sonnet;
  - mechanical verification (compile under the target, lint, fixtures): Haiku;
  - the reviewer is always a different model from the implementer, for diversity.

### D4. Skill and policy conflicts cost deliberation. Status: supported, W3.
- Evidence: both Sonnet arms spent reasoning reconciling the superpowers skills (which assume subagent dispatch and full-code plans) with the owner's "inline by default" rule. B coarsened its plan only after deliberating.
- Mechanism: AutoSteer injects the resolved execution mode once, at dispatch ("execution: inline; plan granularity: coarse"), so no model has to adjudicate between conflicting instructions.

### D5. Peer authority is handled inconsistently. Status: supported, W3.
- Evidence: the same model and the same message led to different behaviour: S proceeded, while B asked the human first.
- Mechanism: covered by D1's envelope. Authority is stated explicitly, never inferred.

## F. Evaluation (how the controller judges; added at W3 close)

### F1. When every arm passes, the experiment has not measured anything. Status: supported, W3 (C17).
- Evidence: a 34-finding oracle passed all three arms; a free 1,335-file differential then found 322 disagreements and a real bug class.
- Mechanism: a ceiling rule in the scorer, which reports "undiscriminated" instead of a tie, and a standing scale-corpus differential across arms after every oracle run. Where arms disagree, at least one is wrong; where they agree, an independent oracle (another parser, bytecode) is still needed for common-mode errors.

### F2. Hermetic means the agent's configuration too. Status: supported, W3 (C16).
- Evidence: the owner's global CLAUDE.md, plugins, output style and auto-memory reached every arm; the arms cited the owner's rules and wrote persistent memories.
- Mechanism: an arm preflight that dumps the effective inputs and diffs them against the declared condition before the first message.

### F3. Adjudicate blind, and never let a model judge its own family unaided. Status: hypothesis (C24).
- Mechanism: rulings on spec questions are made before seeing which arm answered what; LLM review runs on anonymised code by a different model.

### F4. The controller's lessons need the same guards as the agent's. Status: supported (C6 repeated as C18).
- Evidence: a written lesson ("absence of evidence is not evidence of absence") was repeated hours later.
- Mechanism: each controller lesson becomes a check in the preflight or the scorer, or it is expected to recur.

## G. Learning layer (memory)

### G1. Native auto-memory caught a correction rashomon could not see. Status: supported, W3 (n = 2 arms).
- Evidence: both Sonnet arms wrote "route decisions to the architect" from the owner's AskUserQuestion answer. rashomon only reads prompts, so a correction inside a tool_result never reaches the detector (F20). Its proposer did judge the controller's one-off dispositions correctly as temporary.
- Mechanism: capture corrections from AskUserQuestion results and permission denials as well as prompts; when a native memory is written in-session, ingest it as a candidate rather than competing with it.

## E. The harness's own defects (rashomon prototype)
| id | defect | fix |
|---|---|---|
| F7 | Memory learns only from corrections that follow an edit. It is blind to "wrong place", "don't disrupt the call", "don't read work calls". | Detect corrections on read-only turns too; classify them as scope, safety or style. |
| F14 | A new task was paired with a different session's edit as a "correction" | Pair across sessions only when the same files or subject overlap |
| F15 | The proposer could not log in on macOS (USER was dropped) | Keep USER, done in the eval build. Add a self-test call at `memory enable`. |
| F16 | A failed proposer call left an empty pending entry | Never create a candidate without a rendering |
| F17 | The cross-session envelope was mined as lesson content | Strip transport metadata at capture |
| F18 | A per-project install warns "hook_entry_absent" on every turn | Recognise project-local settings, or make the install scope explicit |
| F8 | A recorder hook timed out (5.1 s) under load | Find the cause, budget the hook, and record the gap explicitly |
| F19 | Evidence keys normalise digits inside identifiers ("py27" to "pyN"), including the `example` field | Normalise the key only; keep `example` raw |
| F20 | Corrections that arrive as AskUserQuestion answers are invisible to the detector | Read tool_result answers to AskUserQuestion as correction candidates |
| F21 | The explanatory output style's banner leaked into a tool name (one failed call in B) | Output styles that inject decorations need a check that they never reach tool calls |
