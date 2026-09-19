# Chain view and rule-match layer

Status: proposed, revision 3. Sign-off: approve the pull request that carries
this file; a comment may scope the approval to Part 1 alone.
Scope of change: Go, in `internal/report`, `internal/shape`, `internal/hook`,
`internal/store`, `internal/settings`, `cmd/rashomon`; store schema 3 for
Part 2 only. No new dependency, no new witness, no other harness.

Revision history. Revision 2 corrected the first review's findings: wire
evidence attributed to individual calls, "authorized scope" for what is a
rule match, the wrong rule-evaluation order, cross-scope precedence
backwards, `dontAsk` treated as a bypass mode, a start-of-session snapshot of
rules that change mid-session, and evaluation of a hook-rewritten call
against its pre-rewrite input. Revision 3 corrects the second review's
findings: wire attribution described as run-id-capable when the join is by
time window alone, a verdict change attributed to rewrites alone, `unknown`
too narrow for partially readable policy and unsupported command structure,
and two wrong assumptions about existing code -- the redaction digest and the
settings loader -- recorded below.

## Why

`rashomon` already reconciles three records of one session -- what the agent
declared, what ran, and what the wire saw -- and says in fixed vocabulary what
it does not know. Two things a reader still cannot get from a report:

1. A timeline per prompt of what the agent asked for, with each piece of
   evidence about each call kept apart: recorded here, in the transcript, on
   the wire -- and, for the wire, at what granularity.
2. Which permission rule, as written on disk at that moment, matched each
   call when it was declared and when it ran.

Neither is a judgement. The first is a rendering of records the store already
holds. The second is a rule match, recorded with the time and the policy it
was computed against, and it is named that way everywhere because a rule
match is not authorization: the inputs Claude Code consults that this tool
cannot see are listed in Part 2, and an allowed tool can still be used beyond
what the user meant.

## What exists and is reused unchanged

- Join keys on every record: `tool_use_id`, `session_id`, `prompt_id` (null
  before the first input), `agent_id` inside a subagent call, and `seq`, the
  run's total order.
- The three-way reconciliation: declaration, execution (`outcome`,
  `exit_code`, `duration_ms`, `executed_digest`), and the transcript's
  `tool_result` set; plus the proxy join when `--proxy-store` names a store.
- The proxy join's granularity, exactly as implemented: rows are attributed
  to a session by the session's time window and to a host by name. The
  proxy's `run_id` is the agent's own identifier and shares no namespace with
  a Claude Code session id, so the report leaves it empty on purpose -- "the
  window is the join" (`internal/report/destinations.go`). Nothing on the
  wire carries a `tool_use_id`. Two sessions whose windows overlap cannot be
  told apart on the wire. The wire layer already distinguishes a host that
  was reached from one whose every row was a dial failure
  (`Destination.Unreached`).
- Coverage decided at run time and read back by `report`, never re-derived
  from today's configuration. The coverage record is already written once per
  hook invocation, resolving the settings files each time.
- One reader of `tool_input`: `shape.Derive`. Nothing else looks inside it,
  and nothing from inside it is persisted. Its tokenizer splits a command
  line as a shell would, without expanding anything.
- The per-project host baseline (`internal/baseline`).
- The settings layers and their locations (`settings.Locations`).

## Assumptions about existing code, corrected

Two assumptions revision 2 made about the code were wrong, and both matter to
the claims below.

- **Redaction is not keyed.** `--redact` renders a host as an unkeyed SHA-256
  truncated to eight hex characters plus the host's last label
  (`internal/report/redact.go`), and the README says exactly that: it is not
  a privacy boundary, anyone holding a candidate hostname can hash it and
  compare, and the keyed digest is a different thing, used by `forget --host`
  inside the store. Revision 2 of this document wrongly called the report's
  redaction a keyed digest. This specification describes it as it is, and
  the chain section inherits it unchanged, limitation included.
- **The settings loader is unbounded.** `settings.Load` calls `os.ReadFile`
  with no size limit. Part 2 reads the settings files on every hook
  invocation, so an explicit cap is a prerequisite, not an existing property:
  a file over the cap is treated as `unreadable`, never parsed, and the
  loader returns before allocating. The coverage path, which already reads
  these files per call, gains the same protection as a side effect.

## Non-goals

- No new witness. Nothing here observes files, processes or sockets; the only
  evidence from outside the client remains the proxy's store.
- No per-call wire attribution, and no per-session attribution beyond the
  time window. Until the wire carries a key that this tool also holds, "a
  connection to this host was observed inside this session's window" is the
  strongest true statement, and the report says exactly that.
- No verdict of "authorized", "safe", or "violation". A rule match says which
  rule matched; it does not say what the user meant.
- No inference from a permission mode about what should or should not have
  run. Modes are rendered as the payload named them.
- No enforcement. The hook path records; it never blocks. Exit code 2 stays
  impossible.
- Rule matches never feed coverage. `coverage.state` is about whether the
  recorder was watching, and stays that.
- Claude Code only.

## Part 1: chain view

### Definition

A chain is the set of declarations in one session that share a `prompt_id`,
ordered by `seq`. Declarations whose `prompt_id` is null form one chain
labelled `before first input`; they are never merged into a neighbour. A
session has as many chains as distinct prompt ids, in order of first `seq`.

Subagent declarations (`agent_id` non-null) are rendered inside their chain,
grouped by `agent_id` and labelled with `agent_type`. Which parent call spawned
them is NOT recorded, and the chain does not infer it from timing: the group
reads `spawned by: not recorded`.

### Link: evidence kept apart

One declaration renders as one link. Its evidence fields are separate and
are never folded into a single "strength" value, because the fields come from
sources of different independence and one of them is not per-call.

| field | source | meaning of null |
| --- | --- | --- |
| `seq`, `tool_use_id`, `recorded_at_unix_ms` | declaration | -- |
| `tool_name`, `verb_class`, `program`, `permission_mode`, `agent_id`, `agent_type` | declaration | as the record's own null |
| `executed` | execution record for the id | `null`: no execution record. Rendered `unrecorded`, never `denied`: the README's three-way ambiguity holds |
| `executed.outcome`, `exit_code`, `duration_ms` | execution record | a schema 1 record renders outcome `unobserved` |
| `result_in_transcript` | the id's transcript group | `null`: transcript unreadable |
| `rewritten` | `executed_digest != shape.digest` | `null`: no execution record, or `executed_digest` empty |
| `hosts_declared` | declaration `hosts` | as the record: null when none named, never `[]` |
| `hosts_observed_in_window` | destinations join | per declared host: `reached_in_window`, `attempted_in_window`, `not_observed_in_window`, or `unknown` |
| `wire_attribution` | constant | always `"time_window"`: the wire evidence above is a property of the session's window and the host, not of this call, and not of this session alone when windows overlap |
| `ssh_hosts` | declaration | rendered under `not observable`, never joined |
| `rule_match` | Part 2 | `null` on a schema 1 or 2 record: rendered `unknown` |

`hosts_observed_in_window` per declared host:

- `reached_in_window`: the join counts at least one in-window row for the
  host that was not a dial failure.
- `attempted_in_window`: in-window rows exist for the host and every one of
  them is a dial failure -- a connection was attempted and did not succeed.
- `not_observed_in_window`: the store was read, the window applied, and no
  in-window row names the host.
- `unknown`: the proxy store is absent or unreadable, or the run recorded no
  window -- exactly the conditions under which the destinations section says
  `not observed` with a reason.

Two calls in one session that name the same host carry the same value,
whatever each call did. Two sessions whose windows overlap and name the same
host carry the same value for the same rows. The section header states both
once: "connections observed inside this session's time window; which call,
and which of two overlapping sessions, is not recorded."

### Rendering

- Text: `rashomon report --chain` adds a `chains` section per session. The
  default text report is unchanged.
- JSON: `Session` gains `chains`, always present, `[]` for a run with no
  declarations. Additive; every existing key keeps its meaning.
- `--redact` applies to chains exactly as the destinations section applies it
  today: the unkeyed truncated digest described above, plus the last label.
- No count of missing links. A chain covers recorded declarations; the ids
  the transcript holds and this store does not stay in `missing_from_store`.

### Acceptance

Headless, in the style of the existing items: each must be shown to fail
under a named break before it passes, and `sweep.py` gains one break per
item.

- **H-30 -- chain set equality.** The union of link ids over every chain of a
  session equals the session's recorded declaration id set, `before first
  input` included. Break: drop the null-prompt chain.
- **H-31 -- shared host, no attribution.** Two declarations in one session
  name the same host; the proxy fixture holds one row for it inside the
  window. Both links read `reached_in_window` and `wire_attribution:
  time_window`; no field on either link claims the row. Break: attribute the
  row to the declaration closest in time.
- **H-32 -- overlapping sessions, no attribution.** Two runs with
  overlapping windows each declare the same host; the fixture holds one row
  inside both windows. Each run's link reads `reached_in_window`; neither
  report claims the row as its own or marks it inherited. Break: assign the
  row to the run whose start is nearest.
- **H-33 -- unknown, not observed, attempted, reached.** With no proxy store,
  every entry reads `unknown`. With a store and a window: a declared host
  with no in-window rows reads `not_observed_in_window`; one whose only
  in-window rows are dial failures reads `attempted_in_window`; one with a
  non-failed row reads `reached_in_window`. Break: default the absent-store
  case to `not_observed_in_window`; second break: render a dial failure as
  reached.
- **H-34 -- rewritten degrades to null.** `rewritten` is null when the
  execution record's `executed_digest` is empty or there is no execution
  record; true only for a non-empty digest that differs. Break: treat empty
  as different.
- **H-35 -- redaction covers chains.** A canary hostname declared by a
  fixture call appears nowhere in `report --chain --redact`, text or JSON.
  Break: skip chains in the redactor.
- **H-36 -- no inferred parentage.** A subagent group reads `spawned by: not
  recorded` even when exactly one agent-class declaration precedes it.
  Break: attribute by nearest preceding agent-class call.

### Effort

About one week: `internal/report/chain.go`, text and JSON rendering, seven
acceptance items and their sweep breaks. No schema change, no hook-path
change.

## Part 2: rule-match layer, version 1

### The question it answers

"Which permission rule on disk matched this call when it was declared, and
which matched the input that actually ran, against the rules on disk when the
post hook observed it?" -- nothing wider. The rules are Claude Code's own
`permissions.allow`, `permissions.deny` and `permissions.ask`; the matching is
this tool's reimplementation of Claude Code's documented evaluation; and each
result is a rule match, recorded with the time it was computed and the
digest of the policy it was computed against.

### What is visible and what is not

Visible at each hook invocation:

- the four settings files as they are at that moment: managed, local,
  project, user. This includes `.claude/settings.local.json`, which Claude
  Code appends to during a session when the user picks "don't ask again" for
  a command or a domain -- so the rules a later call is matched against are
  not the rules an earlier call was;
- `permission_mode`, from the payload, per call;
- `cwd`, from the payload, which path rules need;
- at `PostToolUse`, the input as it actually ran, after any hook rewrote it.

Not visible, and therefore never inferred:

- command-line grants such as `--allowedTools` and `--disallowedTools`;
- session-only approvals: Claude Code does not write a file-modification
  approval to disk, and a one-time approval writes nothing;
- the built-in read-only command list, the working-directory read
  allowance, wrapper stripping, and MCP tool and connector controls, all of
  which let a call run, or stop it, with no rule matching it;
- the rules in effect at any instant other than the two hook invocations. A
  policy that changed between them is visible only as two different digests;
- what Claude Code's own evaluation concluded. This tool matches rules; it
  does not observe the decision.

Consequently `no_rule_match` means no rule on disk matched at that
invocation. It does not mean the user was prompted, and `no_rule_match`
beside an execution record does not mean the user said yes.

### Resolution at each hook invocation

There is no snapshot and no copy of the rules. At every `PreToolUse` and
`PostToolUse` invocation the hook path reads the settings files -- as the
coverage record already does, now under the size cap -- and matches the call
against the rules it finds. Persisted, and only these:

- on the declaration, and on the execution record, a `rule_match` object
  (below);
- in the run's `coverage.ndjson`, a `policy` record whenever the canonical
  rule set's digest differs from the last one this run recorded: the first
  at `probe start`, then one per change. It is a revision log, and it holds
  no rule text.

| `policy` field | meaning |
| --- | --- |
| `type`, `schema_version` | `policy`, 3 |
| `recorded_at_unix_ms`, `session_id`, `install_id` | as every record |
| `layers` | per layer `managed`, `local`, `project`, `user`: `present`, `absent`, `unreadable`, or `oversized` |
| `default_mode` | `permissions.defaultMode` as resolved by layer precedence, with the layer that set it; null when unset |
| `rule_counts` | `allow`, `deny`, `ask`, `unparsed` -- counts of the user's rules, not of this instrument's measurements, so zero is honest |
| `rules_digest` | HMAC under the per-install key of the canonical rule set read from the readable layers |

Rule text can carry a full command line or a path. It never reaches a record,
a file in the store, stdout or stderr. The no-content promise in the README
gains one sentence saying so, and H-46 holds it.

### Matching

Rules from all readable layers are merged into one list. Layer precedence
does not apply to the rule arrays: a deny rule from any scope blocks an allow
rule from any other, in Claude Code's own words, "because deny rules from any
scope are evaluated before allow rules." Layer precedence applies to the
scalar `defaultMode`, which is recorded with its layer and otherwise unused.

Evaluation order is Claude Code's: deny, then ask, then allow; the first match
decides, and specificity does not reorder it.

| verdict | condition |
| --- | --- |
| `deny_rule_match` | a deny rule matches |
| `ask_rule_match` | no deny matches; an ask rule matches |
| `allow_rule_match` | no deny or ask matches; an allow rule matches |
| `no_rule_match` | every layer was readable or absent, the input was fully evaluable, and no rule matched |
| `unknown` | anything else; the reason is recorded, from the closed list below |

Grammar supported in version 1: bare tool (`Read`); exact (`Bash(npm run
test)`); prefix, both spellings (`Bash(git *)`, `Bash(git:*)`); domain
(`WebFetch(domain:example.com)`); path (`Edit(src/**)`, `Read(~/.zshrc)`),
with an `Edit` rule covering every file-writing tool and the four
documented prefixes resolved as Claude Code documents them -- `//` from the
filesystem root, `~/` from home, `/` from the settings source's directory,
and bare or `./` from `cwd`; and a glob in the tool-name position of a deny
or ask rule (`"*"`, `mcp__*`). A rule the parser cannot classify, including
any `!` negation pattern, is kept as `unparsed`, counted, and forces
`unknown` on any call it might cover.

### Conservative fallbacks: when the verdict is `unknown`

A parsed rule is not enough. Claude Code matches a `Bash` rule against each
subcommand of a compound command independently, after splitting on `&&`,
`||`, `;`, `|`, `|&`, `&` and newlines, and applies deny and ask rules to a
match inside a subshell, a command substitution or a control-flow body. A
matcher that understood `Bash(git *)` but not `git status && curl ...` would
conclude `allow_rule_match` for a command Claude Code would not allow.
Version 1 therefore concludes only what it can evaluate completely, and
records why when it cannot:

| reason | condition |
| --- | --- |
| `no_policy_readable` | at least one layer is present and none is readable |
| `policy_partially_readable` | a present layer is `unreadable` or `oversized`; a readable deny rule still yields `deny_rule_match`, since a deny is conclusive whatever the unreadable layer held, but no other verdict can be concluded |
| `unparsed_rule_may_apply` | an unparsed rule names the tool, or a tool glob that covers it |
| `unsupported_input_structure` | the command line contains a separator, subshell, command substitution, backtick, control-flow keyword, redirection into a command, or a wrapper prefix; a bare-tool rule (`Bash`, `"*"`) still yields its verdict, because it does not depend on structure, and a deny or ask rule matching a top-level subcommand still yields its verdict, because Claude Code applies those when any subcommand matches |
| `path_context_unavailable` | a path rule needs `cwd` or a settings source directory the invocation does not have |
| `input_unavailable` | the post payload carried no input; `executed_digest` is empty in the same case |
| `evaluation_failure` | any recovered failure inside matching |

### Two matches per call, each stamped

Claude Code evaluates permission rules against the input as it stands after
`PreToolUse` hooks have run, and a hook may rewrite that input. This tool's
declaration is the input before any rewrite. And the rules themselves can
change between the two hooks. So each call carries two independent matches,
and each match says what it was computed against:

```
"rule_match": { "verdict": ..., "reason": ... | null, "rule": <keyed digest> | null,
                "policy_digest": <rules_digest at this invocation>,
                "evaluated_at_unix_ms": ... }
```

- the declaration carries `rule_match` computed at `PreToolUse`, on the
  declared input, against the rules on disk at that invocation;
- the execution record carries `rule_match` computed at `PostToolUse`, on
  the input that ran, against the rules on disk at that invocation -- which
  are not necessarily the rules in effect when execution began.

`rule` is the HMAC of the matching rule's canonical text under the
per-install key: enough to say two matches hit the same rule, not enough to
recover it.

Invariants the hook path keeps:

- `shape.Derive` stays the one reader of `tool_input`: matching is added
  inside it, and the post handler's existing digest derivation gains the
  executed-input match the same way.
- Matching is bounded: rule count capped, input already capped at 8 MiB by
  the stdin limit, glob matching without backtracking blow-up, settings files
  under the new size cap.
- Any failure inside matching is recovered into `unknown` with
  `evaluation_failure`. Exit code 2 stays impossible; the fault-injection
  points H-1 exercises gain one inside matching.

### Report

A `rule_matches` section per session:

- `policy`: how many revisions the run recorded, and for the latest: the
  layers and their states, the default mode and its layer, the rule counts,
  the digest. `not recorded` for a run with no `policy` record, in which case
  every verdict below reads `unknown`.
- `by_verdict`: counts of this store's declarations per declared verdict and
  per executed verdict, with `unknown` broken down by reason. Counts of
  records this store holds, so zero is honest.
- `deny_rule_match_executed`: the ids whose EXECUTED match is
  `deny_rule_match` and which have an execution record. Rendered with the
  permission mode and both policy digests beside each. The line is a
  discrepancy between this tool's matcher and the fact that the call ran; the
  section says that it may be this matcher's error, an input this tool cannot
  see, a rule added after execution began, or a decision Claude Code made for
  reasons the docs cover mode by mode -- and that this tool cannot tell which.
- `verdict_changed`: the ids whose declared and executed verdicts differ,
  each with `input_changed` (from `rewritten`, null when unknowable) and
  `policy_changed` (the two policy digests differ). The two are independent:
  an unchanged input under a rule added mid-call is `input_changed: false,
  policy_changed: true`.
- `unknown`: ids, grouped by reason.

Host rules: `WebFetch(domain:...)` rules give network tools a match like any
rule. A host named on a `Bash` command line has no rule grammar in Claude
Code, so its only signal remains the project baseline (`new for this
project`), and the section says so.

JSON: `Session` gains `rule_matches`; chain links carry both matches.
`--redact` leaves the section untouched: it holds ids and digests only.

### Acceptance

- **H-37 -- order is deny, ask, allow.** A call matched by both an ask rule
  and an allow rule reads `ask_rule_match`; matched by a deny rule and an
  allow rule, `deny_rule_match`. Break: evaluate allow before ask.
- **H-38 -- deny wins across scopes.** A user-level deny with a project-level
  allow reads `deny_rule_match`; so does a managed deny with a local allow.
  Break: apply layer precedence to the rule arrays.
- **H-39 -- rules are read when the call is declared.** Append an allow rule
  to `.claude/settings.local.json` between two declarations of the same
  call: the first reads `no_rule_match`, the second `allow_rule_match`, their
  `policy_digest` values differ, and the run holds exactly two `policy`
  records. Break: cache the rules read at `probe start`.
- **H-40 -- the executed input decides the executed match.** A fixture
  `PreToolUse` hook rewrites a command that a deny rule matches into one an
  allow rule matches: the declaration reads `deny_rule_match`, the execution
  record `allow_rule_match`, `rewritten` is true, `verdict_changed` lists the
  id with `input_changed: true, policy_changed: false`, and
  `deny_rule_match_executed` is empty. Break: use the declared match for that
  line.
- **H-41 -- unchanged input, changed policy.** A call declared under an
  allow rule; a deny rule for it is appended to the settings before the post
  hook runs. The declaration reads `allow_rule_match`, the execution record
  `deny_rule_match`, `rewritten` is false, `verdict_changed` lists the id
  with `input_changed: false, policy_changed: true`, and the two
  `policy_digest` values differ. Break: derive `policy_changed` from
  `rewritten`.
- **H-42 -- unparsed forces unknown.** A rule in an unrecognised grammar that
  names the declared tool makes that tool's verdicts `unknown` with
  `unparsed_rule_may_apply`, and `rule_counts.unparsed` says so. Break: drop
  unparsed rules silently.
- **H-43 -- unsupported structure forces unknown.** With only `Bash(git *)`
  in allow, the command `git status && curl example.com` reads `unknown`
  with `unsupported_input_structure`; with a bare `Bash` allow rule the same
  command reads `allow_rule_match`; with `Bash(curl *)` in deny it reads
  `deny_rule_match`. Break: match the prefix rule against the first token
  only.
- **H-44 -- partial policy forces unknown.** With the managed layer present
  but unreadable and a project-level allow rule that matches, the verdict is
  `unknown` with `policy_partially_readable`; with a project-level deny rule
  that matches, it is `deny_rule_match`. Break: skip unreadable layers
  silently.
- **H-45 -- oversized settings are unreadable, not parsed.** A settings file
  over the cap makes its layer `oversized`, the verdict `unknown` with
  `policy_partially_readable`, and the loader returns without reading past
  the cap; the hook exits 0. Break: remove the cap.
- **H-46 -- no rule text, no rule file.** A rule carrying a canary string
  reaches no record, no file under the store directory, no report output,
  no stdout and no stderr. Break: write the matching rule into
  `rule_match.rule`; second break: write a compiled copy of the rules into
  the run directory.
- **H-47 -- no fault reaches the agent.** The five panicking faults of H-1,
  injected inside matching, exit 0 with the declaration recorded and its
  verdict `unknown` with `evaluation_failure`.
- **H-48 -- no rule match is not a prompt.** A declaration with
  `no_rule_match` and an execution record renders as `no rule on disk
  matched; ran`, and no output anywhere renders it as prompted, approved, or
  granted. Break: label it `approved`.
- **H-49 -- modes are rendered, not interpreted.** For a `deny_rule_match`
  executed under `dontAsk`, under `bypassPermissions`, and under a mode name
  the test invents, the line renders the mode verbatim and no other output
  differs between the three. Break: suppress the line for a mode the code
  considers a bypass.

### Effort

About three weeks: the settings size cap; rule parsing, structure detection
and matching in `internal/shape` (with direct tests beside the acceptance
suite, as the tokenizer has); the `policy` record and per-call resolution in
`internal/hook`; the report section; thirteen acceptance items and their
sweep breaks; the schema file update with `TestStoreSchemaMatchesTheAllowlists`
extended to the new record and fields; and the one-sentence README change for
the no-content promise.

## Sequencing

Part 1 first. It changes no schema and does not touch the hook path, so it
ships on its own and is the demonstration: one prompt, its calls, what the
client recorded about each, and which hosts the wire saw inside the window.

Part 2 second, behind schema 3. It touches the hook path, so it carries the
exit-2 constraint and H-47 before anything else, and the settings size cap
before that.

## Schema

`schema_version` 3: the declaration gains `rule_match`, the execution record
gains `rule_match`, and `policy` is a new record type in `coverage.ndjson`;
`docs/store-schema.json` gains all three, every key required,
`additionalProperties: false`, the reason list as an enum. A reader meeting a
schema 1 or 2 record renders the verdicts as `unknown`. A reader meeting a
record version it does not know continues to skip it.

## Open decisions

1. `--chain` as a flag (proposed) or the chain section always on in text.
   JSON carries `chains` either way.
2. The version-1 grammar and the structure detector above versus wider ones.
   Proposed: ship the subset, let `unparsed` and `unsupported_input_structure`
   count what they miss, widen from evidence.
3. Reading the settings files on every hook invocation (proposed: the
   coverage path already does, and under the cap they are small) versus
   caching by modification time.
4. The settings size cap. Proposed: 1 MiB per file.
5. Hosts named on `Bash` command lines: baseline only (proposed) or a
   `rashomon`-owned allowlist file, which is a new configuration surface.
6. Whether `deny_rule_match_executed` also renders in the default text
   report, outside the section.
7. Ship schema 3 with Part 2 only (proposed), keeping Part 1 on schema 2.
8. Whether to pursue per-call wire attribution later. It needs a correlation
   key that both the wire and this store hold, which the proxy does not emit
   today; it is a change to the other product, not to this one.

## Sign-off

Approving this pull request approves building Part 1 immediately. Part 2 is
approved for implementation as specified here, revision 3; a review comment
may hold it back while the open decisions are settled. Each part lands as its
own pull request against the acceptance items above, reported the way every
H-item is: the command that ran it and its output, and the break that made it
fail first.
