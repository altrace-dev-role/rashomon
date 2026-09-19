# Chain view and rule-match layer

Status: proposed, revision 2. Sign-off: approve the pull request that carries
this file; a comment may scope the approval to Part 1 alone.
Scope of change: Go, in `internal/report`, `internal/shape`, `internal/hook`,
`internal/store`, `cmd/rashomon`; store schema 3 for Part 2 only. No new
dependency, no new witness, no other harness.

Revision 2 follows the first review, which found that revision 1 overstated
two things. It attributed wire evidence to individual calls when the proxy
join is by session window and hostname, not by call; and it called a match
against permission rules "authorized scope", read those rules in the wrong
order, got cross-scope precedence backwards, treated `dontAsk` as a bypass
mode, snapshotted rules that change during a session, and would have
evaluated a hook-rewritten call against its pre-rewrite input. Every one of
those is corrected below, and each has an acceptance item that fails under
the old behaviour.

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
holds. The second is a rule match, recorded at the time it was computed, and
it is named that way everywhere because a rule match is not authorization:
the inputs Claude Code consults that this tool cannot see are listed in Part
2, and an allowed tool can still be used beyond what the user meant.

## What exists and is reused unchanged

- Join keys on every record: `tool_use_id`, `session_id`, `prompt_id` (null
  before the first input), `agent_id` inside a subagent call, and `seq`, the
  run's total order.
- The three-way reconciliation: declaration, execution (`outcome`,
  `exit_code`, `duration_ms`, `executed_digest`), and the transcript's
  `tool_result` set; plus the proxy join when `--proxy-store` names a store.
- The proxy join's granularity: rows are attributed to a session by the run
  id they carry, or by the session's time window when they carry none, and
  to a host by name. Nothing on the wire carries a `tool_use_id`. The join is
  the window; this document builds on that and does not pretend otherwise.
- Coverage decided at run time and read back by `report`, never re-derived
  from today's configuration. The coverage record is already written once per
  hook invocation, resolving the settings files each time.
- One reader of `tool_input`: `shape.Derive`. Nothing else looks inside it,
  and nothing from inside it is persisted.
- The per-project host baseline (`internal/baseline`).
- The settings layers and their locations (`settings.Locations`).

## Non-goals

- No new witness. Nothing here observes files, processes or sockets; the only
  evidence from outside the client remains the proxy's store.
- No per-call wire attribution. Until the wire carries a correlation key,
  "this host was reached during this session" is the strongest true
  statement, and the report says exactly that.
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
| `hosts_observed_in_session` | destinations join | per declared host: `observed_in_session`, `not_observed_in_session`, or `unknown` |
| `wire_attribution` | constant | always `"session"`: the wire evidence above is a property of the session and the host, not of this call |
| `ssh_hosts` | declaration | rendered under `not observable`, never joined |
| `rule_match` | Part 2 | `null` on a schema 1 or 2 record: rendered `unknown` |

`hosts_observed_in_session` is `observed_in_session` for a host only when the
destinations join counts at least one row for that host as this session's --
in window, and not inherited from another run. It is `unknown` for every host
when the proxy store is absent, unreadable, or the run recorded no window,
which are exactly the conditions under which the destinations section says
`not observed` with a reason. It is `not_observed_in_session` only when the
store was read, the window applied, and the host was not among this
session's rows.

Two calls in one session that name the same host therefore carry the same
value for it, whatever each call did. The section header states this once:
"host reached during the session; which call reached it is not recorded."

### Rendering

- Text: `rashomon report --chain` adds a `chains` section per session. The
  default text report is unchanged.
- JSON: `Session` gains `chains`, always present, `[]` for a run with no
  declarations. Additive; every existing key keeps its meaning.
- `--redact` applies to chains exactly as to destinations: hostnames become
  the keyed digest.
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
  window. Both links read `observed_in_session`; both read
  `wire_attribution: session`; no field on either link claims the row.
  Break: attribute the row to the declaration closest in time.
- **H-32 -- unknown is not not-observed.** With no proxy store, every
  `hosts_observed_in_session` entry reads `unknown`. With a store and a
  window, a declared host absent from this session's rows reads
  `not_observed_in_session`. Break: default the absent-store case to
  `not_observed_in_session`.
- **H-33 -- rewritten degrades to null.** `rewritten` is null when the
  execution record's `executed_digest` is empty or there is no execution
  record; true only for a non-empty digest that differs. Break: treat empty
  as different.
- **H-34 -- redaction covers chains.** A canary hostname declared by a
  fixture call appears nowhere in `report --chain --redact`, text or JSON.
  Break: skip chains in the redactor.
- **H-35 -- no inferred parentage.** A subagent group reads `spawned by: not
  recorded` even when exactly one agent-class declaration precedes it.
  Break: attribute by nearest preceding agent-class call.

### Effort

About one week: `internal/report/chain.go`, text and JSON rendering, six
acceptance items and their sweep breaks. No schema change, no hook-path
change.

## Part 2: rule-match layer, version 1

### The question it answers

"Which permission rule on disk matched this call when it was declared, and
which matched the input that actually ran?" -- nothing wider. The rules are
Claude Code's own `permissions.allow`, `permissions.deny` and
`permissions.ask`; the matching is this tool's reimplementation of Claude
Code's documented evaluation; and the result is a rule match, recorded at
the moment it was computed.

### What is visible and what is not

Visible at each hook invocation:

- the four settings files as they are at that moment: managed, local,
  project, user. This includes `.claude/settings.local.json`, which Claude
  Code appends to during a session when the user picks "don't ask again" for
  a command or a domain -- so the rules a later call is matched against are
  not the rules an earlier call was;
- `permission_mode`, from the payload, per call;
- at `PostToolUse`, the input as it actually ran, after any hook rewrote it.

Not visible, and therefore never inferred:

- command-line grants such as `--allowedTools` and `--disallowedTools`;
- session-only approvals: Claude Code does not write a file-modification
  approval to disk, and a one-time approval writes nothing;
- the built-in read-only command list, the working-directory read
  allowance, and MCP tool and connector controls, all of which let a call
  run with no rule matching it;
- what Claude Code's own evaluation concluded. This tool matches rules; it
  does not observe the decision.

Consequently `no_rule_match` means no rule on disk matched. It does not mean
the user was prompted, and `no_rule_match` beside an execution record does
not mean the user said yes.

### Resolution at each hook invocation

There is no snapshot and no copy of the rules. At every `PreToolUse` and
`PostToolUse` invocation the hook path reads the settings files -- as the
coverage record already does -- and matches the call against the rules it
finds. Two things are persisted, and only these:

- on the declaration, and on the execution record, a `rule_match` object
  (below) carrying a verdict and the keyed digest of the matching rule;
- in the run's `coverage.ndjson`, a `policy` record whenever the canonical
  rule set's digest differs from the last one this run recorded: the first
  at `probe start`, then one per change. It is a revision log, and it holds
  no rule text.

| `policy` field | meaning |
| --- | --- |
| `type`, `schema_version` | `policy`, 3 |
| `recorded_at_unix_ms`, `session_id`, `install_id` | as every record |
| `layers` | per layer `managed`, `local`, `project`, `user`: `present`, `absent`, or `unreadable` |
| `default_mode` | `permissions.defaultMode` as resolved by layer precedence, with the layer that set it; null when unset |
| `rule_counts` | `allow`, `deny`, `ask`, `unparsed` -- counts of the user's rules, not of this instrument's measurements, so zero is honest |
| `rules_digest` | HMAC under the per-install key of the canonical rule set |

Rule text can carry a full command line or a path. It never reaches a record,
a file in the store, stdout or stderr. The no-content promise in the README
gains one sentence saying so, and H-41 holds it.

### Matching

Rules from all four layers are merged into one list. Layer precedence does
not apply to the rule arrays: a deny rule from any scope blocks an allow rule
from any other, in Claude Code's own words, "because deny rules from any
scope are evaluated before allow rules." Layer precedence applies to the
scalar `defaultMode`, which is recorded with its layer and otherwise unused.

Evaluation order is Claude Code's: deny, then ask, then allow; the first match
decides, and specificity does not reorder it.

| verdict | condition |
| --- | --- |
| `deny_rule_match` | a deny rule matches |
| `ask_rule_match` | no deny matches; an ask rule matches |
| `allow_rule_match` | no deny or ask matches; an allow rule matches |
| `no_rule_match` | no rule on disk matches |
| `unknown` | no settings file readable, a parse or evaluation failure, or an unparsed rule that might apply |

Grammar supported in version 1: bare tool (`Read`); exact (`Bash(npm run
test)`); prefix, both spellings (`Bash(git *)`, `Bash(git:*)`); domain
(`WebFetch(domain:example.com)`); path (`Edit(src/**)`, `Read(~/.zshrc)`),
with an `Edit` rule covering every file-writing tool; and a glob in the
tool-name position of a deny or ask rule (`"*"`, `mcp__*`). A rule the parser
cannot classify is kept as `unparsed`, counted, and forces `unknown` on any
call it might cover.

### Two verdicts per call

Claude Code evaluates permission rules against the input as it stands after
`PreToolUse` hooks have run, and a hook may rewrite that input. This tool's
declaration is the input before any rewrite. Matching the declaration alone
would therefore match the wrong input for every rewritten call. So:

- the declaration carries `rule_match.declared`, computed at `PreToolUse` on
  the declared input;
- the execution record carries `rule_match.executed`, computed at
  `PostToolUse` on the input that ran; `unknown` when the payload carried no
  input, which is also when `executed_digest` is empty.

Both are `{"verdict": ..., "rule": <keyed digest> | null}`. The declared
verdict describes intent; the executed verdict describes what ran. They
differ exactly when a rewrite changed which rule applied, and `rewritten`
says whether a rewrite happened at all.

Invariants the hook path keeps:

- `shape.Derive` stays the one reader of `tool_input`: matching is added
  inside it, and the post handler's existing digest derivation gains the
  executed-input match the same way.
- Matching is bounded: rule count capped, input already capped at 8 MiB by
  the stdin limit, glob matching without backtracking blow-up, and the
  settings files read with the size limit the coverage path already applies.
- Any failure inside matching is recovered into `unknown`. Exit code 2 stays
  impossible; the fault-injection points H-1 exercises gain one inside
  matching.

### Report

A `rule_matches` section per session:

- `policy`: how many revisions the run recorded, and for the latest: the
  layers present, the default mode and its layer, the rule counts, the
  digest. `not recorded` for a run with no `policy` record, in which case
  every verdict below reads `unknown`.
- `by_verdict`: counts of this store's declarations per declared verdict and
  per executed verdict. Counts of records this store holds, so zero is
  honest.
- `deny_rule_match_executed`: the ids whose EXECUTED verdict is
  `deny_rule_match` and which have an execution record. Rendered with the
  permission mode beside each. The line is a discrepancy between this tool's
  matcher and the fact that the call ran; the section says that it may be
  this matcher's error, an input this tool cannot see, or a decision Claude
  Code made for reasons the docs cover mode by mode -- and that this tool
  cannot tell which.
- `verdict_changed_by_rewrite`: the ids whose declared and executed verdicts
  differ.
- `unknown`: ids, with the reason class.

Host rules: `WebFetch(domain:...)` rules give network tools a match like any
rule. A host named on a `Bash` command line has no rule grammar in Claude
Code, so its only signal remains the project baseline (`new for this
project`), and the section says so.

JSON: `Session` gains `rule_matches`; chain links carry both verdicts.
`--redact` leaves the section untouched: it holds ids and digests only.

### Acceptance

- **H-36 -- order is deny, ask, allow.** A call matched by both an ask rule
  and an allow rule reads `ask_rule_match`; matched by a deny rule and an
  allow rule, `deny_rule_match`. Break: evaluate allow before ask.
- **H-37 -- deny wins across scopes.** A user-level deny with a project-level
  allow reads `deny_rule_match`; so does a managed deny with a local allow.
  Break: apply layer precedence to the rule arrays.
- **H-38 -- rules are read when the call is declared.** Append an allow rule
  to `.claude/settings.local.json` between two declarations of the same
  call: the first reads `no_rule_match`, the second `allow_rule_match`, and
  the run holds exactly two `policy` records with different digests. Break:
  cache the rules read at `probe start`.
- **H-39 -- the executed input decides the executed verdict.** A fixture
  `PreToolUse` hook rewrites a command that a deny rule matches into one an
  allow rule matches: the declaration reads `deny_rule_match`, the execution
  record `allow_rule_match`, `rewritten` is true, and
  `deny_rule_match_executed` is empty. Break: use the declared verdict for
  that line.
- **H-40 -- unparsed forces unknown.** A rule in an unrecognised grammar that
  names the declared tool makes that tool's verdicts `unknown`, and
  `rule_counts.unparsed` says so. Break: drop unparsed rules silently.
- **H-41 -- no rule text, no rule file.** A rule carrying a canary string
  reaches no record, no file under the store directory, no report output,
  no stdout and no stderr. Break: write the matching rule into
  `rule_match.rule`; second break: write a compiled copy of the rules into
  the run directory.
- **H-42 -- no fault reaches the agent.** The five panicking faults of H-1,
  injected inside matching, exit 0 with the declaration recorded and its
  verdict `unknown`.
- **H-43 -- no rule match is not a prompt.** A declaration with
  `no_rule_match` and an execution record renders as `no rule on disk
  matched; ran`, and no output anywhere renders it as prompted, approved, or
  granted. Break: label it `approved`.
- **H-44 -- modes are rendered, not interpreted.** For a `deny_rule_match`
  executed under `dontAsk`, under `bypassPermissions`, and under a mode name
  the test invents, the line renders the mode verbatim and no other output
  differs between the three. Break: suppress the line for a mode the code
  considers a bypass.

### Effort

About three weeks: rule parsing and matching in `internal/shape` (with its own
direct tests beside the acceptance suite, as the tokenizer has), the `policy`
record and per-call resolution in `internal/hook`, the report section, nine
acceptance items and their sweep breaks, the schema file update with
`TestStoreSchemaMatchesTheAllowlists` extended to the new record and the new
fields, and the one-sentence README change.

## Sequencing

Part 1 first. It changes no schema and does not touch the hook path, so it
ships on its own and is the demonstration: one prompt, its calls, what the
client recorded about each, and which hosts the wire saw during the session.

Part 2 second, behind schema 3. It touches the hook path, so it carries the
exit-2 constraint and H-42 before anything else.

## Schema

`schema_version` 3: the declaration gains `rule_match`, the execution record
gains `rule_match`, and `policy` is a new record type in `coverage.ndjson`;
`docs/store-schema.json` gains all three, every key required,
`additionalProperties: false`. A reader meeting a schema 1 or 2 record renders
the verdicts as `unknown`. A reader meeting a record version it does not know
continues to skip it.

## Open decisions

1. `--chain` as a flag (proposed) or the chain section always on in text.
   JSON carries `chains` either way.
2. The version-1 grammar above versus a wider one. Proposed: ship the subset,
   let `unparsed` count what it misses, widen from evidence.
3. Reading the settings files on every hook invocation (proposed: the
   coverage path already does, and the files are small) versus caching by
   modification time.
4. Hosts named on `Bash` command lines: baseline only (proposed) or a
   `rashomon`-owned allowlist file, which is a new configuration surface.
5. Whether `deny_rule_match_executed` also renders in the default text
   report, outside the section.
6. Ship schema 3 with Part 2 only (proposed), keeping Part 1 on schema 2.
7. Whether to pursue per-call wire attribution later. It needs a correlation
   key on the wire side that the proxy does not emit today, so it is a change
   to the other product, not to this one.

## Sign-off

Approving this pull request approves building Part 1 immediately. Part 2 is
approved for implementation as specified here, revision 2; a review comment
may hold it back while the open decisions are settled. Each part lands as its
own pull request against the acceptance items above, reported the way every
H-item is: the command that ran it and its output, and the break that made it
fail first.
