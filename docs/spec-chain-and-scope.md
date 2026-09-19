# Chain view and scope layer

Status: proposed. Sign-off: approve the pull request that carries this file.
Scope of change: Go, in `internal/report`, `internal/shape`, `internal/hook`,
`internal/store`, `cmd/rashomon`; store schema 3. No new dependency, no new
witness, no other harness.

## Why

`rashomon` already reconciles three records of one session -- what the agent
declared, what ran, and what the wire saw -- and says in fixed vocabulary what
it does not know. Two things a reader still cannot get from a report:

1. An explanation, per prompt, of what the agent caused: which calls, in what
   order, which of them have evidence from outside the client, and which are
   known only because the client said so.
2. Whether each call was inside the scope the user had granted when it was
   declared.

This document specifies both. The first is a rendering of records the store
already holds. The second adds one field to the declaration record and one
new record type, and it is decided at run time for the same reason coverage
is: a report that judged a past run by today's rules would change its verdict
every time the rules changed.

## What exists and is reused unchanged

- Join keys on every record: `tool_use_id`, `session_id`, `prompt_id` (null
  before the first input), `agent_id` inside a subagent call, and `seq`, the
  run's total order.
- The three-way reconciliation: declaration, execution (`outcome`,
  `exit_code`, `duration_ms`, `executed_digest`), and the transcript's
  `tool_result` set; plus the proxy join when `--proxy-store` names a store.
- Coverage decided at run time and read back by `report`, never re-derived
  from today's configuration.
- One reader of `tool_input`: `shape.Derive`. Nothing else looks inside it,
  and nothing from inside it is persisted.
- The per-project host baseline (`internal/baseline`): earliest-session-wins
  novelty, with the ubiquitous-host list.
- Settings layer precedence, already implemented for `disableAllHooks`:
  managed, then local, then project, then user (`settings.HooksDisabled`).

## Non-goals

- No new witness. Nothing here observes files, processes or sockets; the only
  independent evidence remains the proxy's store. A chain link with no wire
  evidence is rendered as such, not filled in.
- No verdict of "safe" or "violation". `in_scope` means an allow rule
  matched; `would_ask` means Claude Code would have prompted; neither is a
  judgement about harm.
- No enforcement. The hook path records; it never blocks. Exit code 2 stays
  impossible.
- Scope never feeds coverage. A run's `coverage.state` is about whether the
  recorder was watching, and stays that.
- Claude Code only.

## Part 1: chain view

### Definition

A chain is the set of declarations in one session that share a `prompt_id`,
ordered by `seq`. Declarations whose `prompt_id` is null form one chain
labelled `before first input`; they are never merged into a neighbour. A
session has as many chains as distinct prompt ids, in order of first `seq`.

Subagent declarations (`agent_id` non-null) are rendered inside their chain,
grouped by `agent_id`. Which parent call spawned them is NOT recorded, and the
chain does not infer it from timing: the group is labelled with its
`agent_type` and `spawned by: not recorded`.

### Link

One declaration renders as one link:

| field | source | null means |
| --- | --- | --- |
| `seq`, `tool_use_id`, `recorded_at_unix_ms` | declaration | -- |
| `tool_name`, `verb_class`, `program`, `permission_mode`, `agent_id`, `agent_type` | declaration | as the record's own null |
| `execution` | execution record for the id | `null`: no execution record. Rendered as `unrecorded`, never as denied (the README's three-way ambiguity holds) |
| `execution.outcome`, `exit_code`, `duration_ms` | execution record | schema 1 record: outcome `unobserved` |
| `result_in_transcript` | the id's transcript group | `null`: transcript unreadable |
| `rewritten` | `executed_digest != shape.digest` | `null`: no execution record, or `executed_digest` empty |
| `hosts_declared` | declaration `hosts` | as the record: null when none named, never `[]` |
| `hosts_observed` | destinations join | per host: `observed`, `not_observed`, or `unknown` |
| `ssh_hosts` | declaration | rendered under `not observable`, never joined |
| `scope` | Part 2 | `null` on a schema 1 or 2 record: rendered `unknown` |

`hosts_observed` is `unknown` for every host when the proxy store is absent,
unreadable, or the run recorded no window -- the same conditions under which
the destinations section says `not observed` with a reason. It is
`not_observed` only when the store was read, the window applied, and the host
was not among the rows.

### Evidence class

Each link carries a derived `evidence` value, a closed vocabulary ordered by
independence from the client:

1. `observed_on_wire` -- at least one declared host was observed in window.
   The only class that comes from outside the client.
2. `result_in_transcript` -- the client's transcript holds a result.
3. `executed` -- the client's PostToolUse fired and this store recorded it.
4. `declared` -- PreToolUse only.

A link's `evidence` is the strongest class it reaches. Classes 2 and 3 are
both the client's own account, and the text renderer says so in the section
header once, not per link. A call whose tool names no host (a `Read`, a
`git status`) tops out at class 2 or 3 by construction, and the renderer must
not present that as a shortfall.

### Rendering

- Text: `rashomon report --chain` adds a `chains` section per session. The
  default text report is unchanged.
- JSON: `Session` gains `chains`, always present, `[]` for a run with no
  declarations. Additive; every existing key keeps its meaning.
- `--redact` applies to chains exactly as to destinations: hostnames become
  the keyed digest.
- No count of missing links. A chain covers recorded declarations; the ids
  the transcript holds and this store does not stay where they are, in
  `missing_from_store`.

### Acceptance

Headless, in the style of the existing items: each must be shown to fail
under a named break before it passes, and `sweep.py` gains one break per
item.

- **H-30 -- chain set equality.** The union of link ids over every chain of a
  session equals the session's recorded declaration id set, `before first
  input` included. Break: drop the null-prompt chain.
- **H-31 -- unknown is not not-observed.** With no proxy store, every
  `hosts_observed` entry reads `unknown`. With a store and a window, a
  declared host absent from the rows reads `not_observed`. Break: default the
  absent-store case to `not_observed`.
- **H-32 -- rewritten degrades to null.** `rewritten` is null when the
  execution record's `executed_digest` is empty or there is no execution
  record; it is true only for a non-empty digest that differs. Break: treat
  empty as different.
- **H-33 -- redaction covers chains.** A canary hostname declared by a
  fixture call appears nowhere in `report --chain --redact` text or `--json`.
  Break: skip chains in the redactor.
- **H-34 -- no inferred parentage.** A subagent group renders `spawned by:
  not recorded` even when exactly one `Task` declaration precedes it. Break:
  attribute by nearest preceding agent-class call.

### Effort

About one week: `internal/report/chain.go`, text and JSON rendering, five
acceptance items and their sweep breaks. No schema change, no hook-path
change.

## Part 2: scope layer, version 1

### The question it answers

"Was each declared call inside the permission scope the user had granted at
the time?" -- answered from the rules Claude Code itself applies, snapshotted
when the session started and evaluated when the call was declared.

### Policy source

Claude Code's permission rules: `permissions.allow`, `permissions.deny`,
`permissions.ask`, and `permissions.defaultMode`, resolved across the settings
layers in the precedence `HooksDisabled` already implements (managed, local,
project, user), with arrays merged the way Claude Code merges them.

Rule grammar supported in version 1:

- bare tool: `Read`
- exact: `Bash(npm run test)`
- prefix: `Bash(git *)`
- domain: `WebFetch(domain:example.com)`
- path: `Edit(src/**)`, `Read(~/.zshrc)`; an `Edit` rule covers every
  file-writing tool, as Claude Code's own matching does

A rule the parser cannot classify is kept as `unparsed`. A call that an
unparsed rule might cover evaluates to `unknown`, never to `in_scope`.

### Snapshot at run time

At `probe start`, the rules are resolved, compiled to a canonical form, and
recorded as a new record type, `policy`, in the run's `coverage.ndjson`:

| field | meaning |
| --- | --- |
| `type` | `policy` |
| `schema_version` | 3 |
| `recorded_at_unix_ms`, `session_id`, `install_id` | as every record |
| `layers` | per layer `managed`, `local`, `project`, `user`: `present`, `absent`, or `unreadable` |
| `default_mode` | `permissions.defaultMode` as resolved, or null when unset |
| `rule_counts` | `allow`, `deny`, `ask`, `unparsed` -- counts of policy rules, which are the user's configuration and not this instrument's measurements, so zero is honest |
| `rules_digest` | HMAC under the per-install key of the canonical rule set |

Rule text is never written to a record: a rule can carry a path or a host,
which is one step from content. The compiled rule set the hook path needs is
written beside the run's records as `policy.json`, mode `0600`, evicted with
the run. It is a working file, not evidence; the record carries the digest so
a later reader can say whether two runs saw the same rules without being able
to say what they were.

A run with no `policy` record -- an older run, or one whose probe never fired
-- evaluates every declaration to `unknown`, and the report's scope section
says `policy: not recorded`.

### Evaluation at declaration time

`rashomon hook` evaluates the declared `tool_input` against the run's
compiled rules and writes the verdict on the declaration record, schema 3:

```
"scope": { "verdict": "in_scope" | "denied_by_rule" | "would_ask" | "unknown",
           "rule": "<keyed digest of the matching rule>" | null }
```

- A `deny` rule matches: `denied_by_rule`. Deny beats allow, as in Claude
  Code.
- An `allow` rule matches: `in_scope`.
- An `ask` rule matches, or nothing matches: `would_ask` -- Claude Code would
  have prompted. Whether the user then granted the call is what the execution
  record says, not this field.
- No `policy.json`, a parse failure, an evaluation error, or an unparsed rule
  that might apply: `unknown`.

The verdict is what the rules SAY about the call. `permission_mode` sits
beside it on the same record, and the report renders both, because under
`bypassPermissions` or `dontAsk` a call runs whatever the rules say; the
verdict is still worth having, and presenting `in_scope` as "was granted" in
those modes would be a claim the record cannot support.

Invariants the hook path keeps:

- `shape.Derive` stays the one reader of `tool_input`. Evaluation is added
  inside it -- `shape.Derive(input, rules)` returns the shape and the verdict
  together -- so no second reader appears.
- Evaluation is bounded: rule count capped, input already capped at 8 MiB
  by the stdin limit, glob matching without backtracking blow-up.
- Any failure inside evaluation is recovered into `unknown`. Exit code 2 stays
  impossible; the fault-injection points H-1 exercises gain one inside
  evaluation.

### Report

A `scope` section per session:

- `policy`: `recorded` or `not recorded`, the layers that were present, the
  default mode, the rule counts, the rules digest.
- `by_verdict`: counts of this store's declarations per verdict. These are
  counts of records this store holds, so zero is honest.
- `denied_by_rule`: ids, each with its execution status and mode.
- `denied_yet_executed`: the ids for which all three hold -- `denied_by_rule`,
  an execution record exists, and `permission_mode` is not a bypass mode.
  This is the one line in the section that is alarming on its own, and it is
  the only line allowed to read that way.
- `would_ask`: ids, each with execution status: an execution record means the
  prompt was answered yes; none means denied, failed, or unrecorded --
  the README's three-way ambiguity, restated, never collapsed.
- `unknown`: ids.

Host scope: `WebFetch(domain:...)` rules give network tools a scope and are
evaluated like any rule. A host named on a `Bash` command line has no rule
grammar in Claude Code, so its only scope signal remains the project baseline
(`new for this project`), and the section says so rather than inventing one.

JSON: `Session` gains `scope` with the fields above; chain links carry the
verdict. `--redact` leaves the section untouched: it holds ids and digests
only.

### Honesty rules

- The snapshot decides. `report` never re-reads settings to evaluate a past
  run.
- No rule text in any record, in stdout, or in stderr.
- `in_scope` requires a matching allow rule; nothing defaults to it.
- An unparsed rule that might apply forces `unknown`.
- Scope verdicts never change `coverage.state` or its reasons.

### Acceptance

- **H-35 -- the snapshot decides.** Change the settings' rules after `probe
  start`; every verdict for that run is unchanged, and a run with no policy
  record renders every verdict `unknown` with `policy: not recorded`. Break:
  resolve rules at report time.
- **H-36 -- in_scope only by allow.** A declaration matching no rule is
  `would_ask`; broaden the matcher to accept a near-miss and the item goes
  red. Break: prefix-match without the rule's own boundary.
- **H-37 -- unparsed forces unknown.** A rule in an unrecognised grammar that
  names the declared tool makes that tool's verdicts `unknown`, and
  `rule_counts.unparsed` says so. Break: drop unparsed rules silently.
- **H-38 -- no rule text.** A rule carrying a canary string reaches no record,
  no report output, no stdout and no stderr; only `policy.json` in the run
  directory holds it, and the run's eviction removes that file. Break: write
  the matching rule into `scope.rule`.
- **H-39 -- no fault reaches the agent.** The five panicking faults of H-1,
  injected inside evaluation, exit 0 with the declaration recorded and its
  verdict `unknown`. Break: remove the recover.
- **H-40 -- denied yet executed needs all three.** The line lists an id only
  with `denied_by_rule`, an execution record, and a non-bypass mode; each
  of the three removed alone empties it. Break: drop the mode condition.
- **H-41 -- precedence, both directions.** Managed deny over user allow reads
  `denied_by_rule`; user deny over project allow reads `in_scope`, because the
  project layer outranks the user layer. Break: read the user file first.

### Effort

Two to three weeks: rule parsing and matching in `internal/shape` (with its
own direct tests beside the acceptance suite, as the tokenizer has), the
`policy` record and `policy.json` in `internal/store`, the snapshot in
`internal/hook/probe.go`, evaluation in `internal/hook/handle.go`, the report
section, seven acceptance items and their sweep breaks, and the schema file
update with `TestStoreSchemaMatchesTheAllowlists` extended to the new record.

## Sequencing

Part 1 first. It changes no schema and does not touch the hook path, so it
ships on its own and is the demonstration: one prompt, its calls, which had
wire evidence, which are the client's word alone.

Part 2 second, behind schema 3. It touches the hook path, so it carries the
exit-2 constraint and H-39 before anything else.

## Schema

`schema_version` 3: the declaration gains `scope`; `policy` is a new record
type in `coverage.ndjson`; `docs/store-schema.json` gains both, every key
required, `additionalProperties: false`. A reader meeting a schema 1 or 2
declaration renders `scope` as `unknown`. A reader meeting a record version it
does not know continues to skip it.

## Open decisions

1. `--chain` as a flag (proposed) or the chain section always on in text.
   JSON carries `chains` either way.
2. The version-1 rule grammar above versus a wider one. Proposed: ship the
   subset, let `unparsed` count what it misses, widen from evidence.
3. `policy.json` in the run directory (proposed, `0600`, evicted with the
   run) versus re-resolving the settings files on every hook invocation.
4. Host scope for hosts named on `Bash` command lines: baseline only
   (proposed) or a `rashomon`-owned allowlist file, which is a new
   configuration surface.
5. Whether `denied_yet_executed` should also render in the default text
   report, outside `--chain` and the scope section.
6. Ship schema 3 with Part 2 only (proposed), keeping Part 1 on schema 2.

## Sign-off

Approving this pull request approves building Part 1 immediately and Part 2
as specified, with the open decisions settled by review comments on this
file. Each part lands as its own pull request against the acceptance items
above, reported the way every H-item is: the command that ran it and its
output, and the break that made it fail first.
