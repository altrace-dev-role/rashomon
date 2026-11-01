# Eval controller checklist

Each item is a gate. It comes from a controller failure in LOG.md (the C ids). Prose lessons recurred (C6 came back as C18); a gate with a yes/no answer is the minimum. Where a script can answer the gate, the script is the target.

## Design (before the oracle is frozen)
0. Start from AUTOSTEER-SYSTEM-MODEL's facets (code, dependencies, runtime, security, supply chain, policy, build, history, time) and write which are in scope and which are out, with the reason. (C34)
0a. Every semantic clause in a task spec cites its primary source and names the language version it follows (a quoted rule in the knowledge pack). Fetch the source before writing the clause, not after. (C44)
0b. For every term the spec fixes (a version, a platform, a locale, a size), ask whether a different value changes the answer. If it does, the term is a parameter, and the output is indexed by it. Measure the spread with the real systems before deciding. (C47)
0c. When a ruling widens one fixed list to reality, audit every other fixed list in the spec and the instruments the same day: names, modules, versions, platforms. (C51)
1. Objective: when a request names "best possible", ask which quality ranks first (correctness, maintainability, speed, cost). (C28)
2. Rubric fixed now, covering what the owner would pay for: correctness at scale, change cost (maintainability), verification receipts, disclosed judgment calls, human interrupts, tokens. The primary metric is frozen with the oracle hash; anything added later is exploratory. (C20, C26)
3. Spec enumerations come from the authoritative source (e.g. the Language Reference's binding forms), never from memory. Every rule about a position (a line, a column) names its anchor for multi-line constructs. (C27)
4. Correctness includes security, dependencies and build health: run an advisory scan on the frozen environment, record the known risk and the dependency policy in the spec, and include "noticed and recorded the risk" in the rubric. (C30, C31)
4a. Oracle: red on base, green on a reference, one mutant per enumerated spec clause, and a scale-corpus differential. Write down what it cannot detect. (C11, C12, C17)
5. Replicates: k >= 3 per arm, or declare n = 1 and cap every status at "hypothesis". (C19)

## Launch (before the first message to any arm)
6. Dump each arm's effective inputs (CLAUDE.md chain, plugins and skills, output style, memory directory, hooks, MCP servers) and diff them against the declared condition. Anything undeclared blocks the launch or is logged as a deviation. (C16)
7. Round-trip test every instrument: the send channel (send, then see the turn) and the transcript extractor (against one hand-checked transcript). (C7, C14, C23)
8. Probes run in scratch copies, never in an arm's workspace. (C5)

## During the run
9. Any convenience trade is a protocol deviation, logged with its time. (C16)
10. Every number carries an as-of time and is marked in-flight or final. (C22)
11a. Commands with globs or multi-line logic go into a file run by sh; echo labels are quoted (zsh `=word` and glob expansion). (C29)
11b. File lists from any corpus are passed null-separated (find -print0, sort -z, xargs -0); a name with a space, newline or leading dash is hostile input to the controller's own scripts. Check the result afterwards (shasum -c, count against find). (C43)
11c. A rule change that relaxes the owner's rules is confirmed by the owner's own act in each affected session (typed there, or owner-signed), never by the controller editing an owner file. The task's routing rule says authority questions go to the owner. (C45)
11. Gates use the exit status of the thing gated (`set -o pipefail`). (C9)

## Scoring
12. If every arm passes, report "undiscriminated" and run the scale differential. Agreement among arms is not truth; check common-mode errors with an independent oracle. (C17)
13. Join only on contract fields. Assert the reconciliation identities (per-rule count difference equals the set difference; one value per key) before printing. (C21)
14. Rule on spec ambiguities before revealing which arm answered what. LLM review is blind and done by a different model from the one being judged. (C24)
15a. Every scanner and every ad-hoc probe (grep, regex, network call) carries a positive control: one input known to match, in the same invocation. Merge advisory aliases before counting. (C32, C33)
15b. Every spec ruling lists the instruments updated to match it, each re-run with its control, before the ruling is sent. A ruling the hidden check disagrees with is a controller defect. (C46)
15c. Never silence stderr on a probe whose empty result would be read as evidence. Each search includes one term known to be present; if it is not found, the search is broken. (C49)
15d. A sensor that is not running is a failed gate. Before reporting any count of interventions or triggers, show the sensor's last run time and what it scanned. (C50)
15. A claim from one sensor's silence needs a second, independent sensor. (C6, C18)
16. No steering fired: list the defects found later with no trigger; each one is a sensor recall miss. (C25)
- 15e. Every LOG time comes from `date`, never an estimate (C52).
- 0d. A truth table's controls include the canonical case of its class (for renames: __builtin__), and its source's blind spots (names it filters, platforms it lacks) are listed before scoring (C53).
- 0e. Every harness expectation cites the ruling clause it enforces, and the clause is re-read for exceptions (R5 bullet 2 lets a non-regular entry be skipped) before the expectation is frozen (C54).
- 0f. The spec's own named inputs are run by an instrument before any verdict says "met" for that item; a "met" names the inputs it was measured on (C55).
- 0g. A truth table keyed by line states its line convention for multi-line statements, and a mismatch on a multi-line statement is checked by hand before it counts (C57).
- 15f. Stopping an orchestrator stops its own containers only: every run labels them steerci-role=<mode>; kill with docker kill $(docker ps -q -f label=steerci-role=<mode>), never by the shared steerci=1 label, which killed a backtest's container (E18, 00:41). Read the first live output of any new check before it can notify (C56).
