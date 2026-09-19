# Chain view, sensitive-file labels, and rule-match layer

Status: proposed, revision 7. Sign-off is per part. Part 1 (chain view) is
approved to build with the changes this revision carries. Part 2
(sensitive-file labels) is the first thing this track builds, ahead of Part
3. Part 3 (rule-match layer) is held until the three owed measurements are
reported inside their budgets, and gets its own approval by comment.

Scope of change. Part 1: `internal/report`, `internal/wire`, `cmd/rashomon`
(flag only) -- the other track, after Phase B. Parts 2 and 3:
`internal/shape`, `internal/hook`, `internal/settings`, and the report
sections that render them -- this track. Store schema 3 is being cut on the
Phase B branch and is reserved, not defined, here. No new dependency, no new
witness, no other harness.

Numbering. H-30 is taken by Phase B's denial item, and Phase B is not yet on
the shared remote, so every H-number below is provisional from H-31 and is
grepped against the merge target (`H-[3-6][0-9]`) before it becomes the
contract.

Revision history. Revisions 2 through 4 corrected three external reviews:
per-call wire attribution the join cannot support, "authorized" for a rule
match, evaluation order, cross-scope precedence, `dontAsk`, a start-of-session
snapshot, hook-rewritten inputs, verdict changes attributed to rewrites alone,
`unknown` too narrow, the `/path` anchor, an allow concluding over an
unevaluable deny, and an in-window distinction built on an out-of-window
aggregation. Revision 5 takes the maintainer's three passes: `internal/wire`
in scope; chains per transcript and prompt; links consuming the suppressed,
client-plane-aware host view; the cost premise wrong by a factor of four;
nothing coupling the matcher to the client's observed behaviour; the unknown
rate unmeasured; `policy` concurrency unspecified; `shape.Derive` unable to
host matching; two settings sources the matcher never reads; three semantics
the docs contradict; stale claims about the tree; the labels ordered first;
and two measurements taken, one of which rejected a mechanism revision 4
relied on. Revision 6 takes an internal review of revision 5: the
sign-off now names the pull request it asks for a comment on, so the
reference survives the squash merge; the `--settings` clause is attributed to
its real source and measurement 2's population is pinned to match; and H-69
bounds the filesystem access Part 3 introduces into the hook path, which
revision 5 observed and left uncovered. Revision 7 takes the maintainer's three
rulings on revision 6: the chain's hosts render under the keyed digest Phase
B introduced, not the unkeyed one; the default text report carries a
`chains: N` line so the flag is discoverable; and the schema reservation is
fields only -- `file_label` moves onto the Phase B branch, where it should
have been, and the `policy` record type is not reserved at all, because a
structure from an unapproved part does not belong in a published contract.

## Why

`rashomon` already reconciles three records of one session -- what the agent
declared, what ran, and what the wire saw -- and says in fixed vocabulary what
it does not know. Three things a reader still cannot get from a report:

1. A timeline per prompt of what the agent asked for, with each piece of
   evidence about each call kept apart, and the wire's evidence at its true
   granularity.
2. Whether a call touched a file whose shape says "secret" -- without the
   store ever holding the path.
3. Which permission rule, as written in the files this tool can read, matched
   each call when it was declared and when it ran.

None is a judgement. The first renders records the store already holds. The
second is a closed-vocabulary label derived at hook time. The third is a rule
match against the files this tool can see, recorded with the time and the
policy revision it was computed against, and named that way everywhere
because a rule match is not authorization: Part 3 lists the inputs Claude
Code consults that this tool cannot see, and an allowed tool can still be
used beyond what the user meant.

## What exists, as it is

- The declaration carries `tool_use_id`, `session_id`, `prompt_id` (null
  before the first input), `agent_id` and `agent_type` (null outside a
  subagent call), `transcript_path`, `permission_mode`, and `seq`. The
  execution and terminal records carry `tool_use_id`, `session_id` and a
  nullable `seq`; the coverage record carries no `seq`. An execution is
  placed in a chain only through its declaration.
- Three tool-event invocations, not two: `PreToolUse` writes the declaration;
  `PostToolUse` AND `PostToolUseFailure` each write an execution record and a
  coverage record. The post payload carries no `permission_mode`
  (`internal/hook/post.go`).
- The accounting equation is per transcript (`internal/report/report.go`): a
  nested `claude -p` inherits the parent's session id and writes its own
  transcript.
- The proxy join: rows attributed to a session by time window and to a host
  by name; `run_id` left empty on purpose ("the window is the join",
  `internal/report/destinations.go`); nothing on the wire carries a
  `tool_use_id`; overlapping sessions are indistinguishable on the wire.
- The wire layer (`internal/wire/wire.go`, `summarise`) folds rows by
  `request_id` -- a refused dial is a verdict row plus a `dial_failed_*` row
  under one id -- and sets `Destination.Unreached` over every row of a host,
  in window and out. When no stamp parses, `WindowApplied` is false, every
  row is reported and none is inherited.
- The destinations section (`buildDestinations`) suppresses forgotten hosts,
  classifies loopback (never proxied) and the client plane (the client's own
  model traffic), and only then reports what the session reached.
- Phase B (commit 7f7cb06 on its branch) names denials exactly:
  `denied_by_user`. A chain that renders a missing execution as "unrecorded,
  never denied" would contradict the section above it once that lands.
- Coverage decided at run time and read back by `report`. Per hook invocation
  the coverage path resolves ONE file -- the user's `settings.json`,
  `settings.UserPath()`, up to three attempts (`internal/hook/coverage.go`,
  `Resolve`). Managed, project and local layers are read only by `watch` and
  `detach`. The `start` record carries the session's `cwd`; the `call` and
  `post` records carry each invocation's `cwd`.
- `shape.Derive` is the one reader of `tool_input`: for a shell tool it
  tokenises the command line and records `program` with `path.Base`; for
  every other tool it digests the whole input. It does not read `file_path`
  or `url`. Its tokenizer treats a newline as whitespace.
- `settings.Load` returns an empty document, with no error, for a missing
  file; absent and present-but-empty are indistinguishable at the loader. It
  reads unbounded. `settings.DefaultLocations` derives the project and local
  paths from a `cwd` it is given.
- `store.Accepts` admits schema versions 1 and 2; other versions are skipped.
- `host.Canonical` rejects an authority carrying userinfo outright.
- Redaction: an unkeyed SHA-256 truncated to eight hex characters plus the
  host's last label, rebuilding slices rather than mutating them; the README
  says it is not a privacy boundary.
- The acceptance suite's schema test checks the reason enums against
  `store.Reasons()` and `report.Reasons()`; a reason vocabulary that is not
  exported through such a function drifts from the schema silently.
- H-17's forbidden-import list does not include `os`; a hook-path
  `os.Lstat` or `EvalSymlinks` leaves that check green.

## Measurements taken

Both on Claude Code 2.1.278, real sessions, records pasted from the store.

- **A subagent's `PreToolUse` payload carries the parent's `prompt_id`.**
  The parent's `Agent` declaration (`seq` 49, `prompt_id` 318ff481...) was
  followed by the subagent's `Bash` declaration (`seq` 51, `agent_id`
  a53e8ff9..., `agent_type` `general-purpose`, `prompt_id` 318ff481...). Same
  prompt id. The agent-class tool is named `Agent` in this version; older
  payloads name it `Task`. Part 1 groups subagent calls under the parent
  prompt's chain on this measurement, and re-measures when the client version
  changes.
- **A `cd` inside a Bash call changes the payload `cwd` for every later
  invocation.** After `cd sub && pwd` in one call, the next call's `call`
  coverage record carried `cwd` `/tmp/rashomon-e2e/sub`. So "the invocation's
  `cwd` differs from the run's start `cwd`" is not a `/cd` detector: it fires
  after any ordinary `cd` and would mark every `/path` rule `unknown` for the
  rest of the run. Revision 4's detector is withdrawn. Part 3 anchors on the
  run's start `cwd` and states `/cd` as undetectable.

## Non-goals

- No new witness of the agent's behaviour. Nothing here observes what the
  agent's processes do to files, processes or sockets; the only evidence from
  outside the client remains the proxy's store. Part 3 does stat and resolve
  the path a call itself names, in its own input, at hook time: that is
  reading configuration and file metadata to evaluate a rule, and it is
  named here so the non-goal is not read more widely than it holds.
- No per-call wire attribution, and no per-session attribution beyond the
  time window.
- No verdict of "authorized", "safe", or "violation". A rule match says which
  rule matched in the files this tool read; it does not say what the user
  meant, and a label says what a path looks like, not what happened to it.
- No inference from a permission mode about what should or should not have
  run. Modes are rendered as the payload named them.
- No enforcement. The hook path records; it never blocks. Exit code 2 stays
  impossible.
- Neither labels nor rule matches feed coverage.
- No cryptographic claim for any digest here. `policy_revision`,
  `rules_digest` and `rule` are HMACs under the per-install key, which any
  process running as the user can read -- any `Bash` call the agent makes
  included. They are stable pseudonyms that let a reader say "same" or
  "different" without seeing the text. They are not evidence against
  tampering and not a privacy boundary.
- Claude Code only.

## Part 1: chain view

Owned by the other track, after Phase B. At about a week it lands after the
09-24 ship; it is post-v0.1 unless that track scopes it to fit, and this
document does not assume it in v0.1.

### Definition

A chain is the set of declarations in one run that share a
`(transcript_path, prompt_id)` pair, ordered by `seq` -- not by the order
records appear in `records.ndjson` and `spill.ndjson`, which `read.go`
appends in file order. Transcript path is part of the key because a nested
`claude -p` writes its own transcript under the parent's session id. A run
has as many chains as distinct pairs, ordered by each chain's first `seq`, so
prompts interleaved as P1, P2, P1 render two chains, P1 first.

Groups that are not chains:

- `prompt_id` null and `agent_id` null: one group per transcript, `before
  first input`.
- `prompt_id` null and `agent_id` non-null: `prompt not recorded`, never
  `before first input`.
- `transcript_path` empty: one group, `no transcript path`, distinct from
  `transcript unreadable`.
- Declarations the store dropped (`run.Dropped()`): links with every evidence
  field `unknown`; they do not vanish.
- The unattributed bucket (`UnattributedSession`): no chain, no window.

Subagent declarations render inside their chain, grouped on `(agent_id,
agent_type)`; when `agent_type` varies within one `agent_id` the group renders
`agent_type: unknown`. Which parent call spawned a group is NOT recorded, and
the chain does not infer it from `seq` or timing even when exactly one
agent-class declaration precedes it: the group reads `spawned by: not
recorded`.

### Link: evidence kept apart

| field | source | value / meaning of null |
| --- | --- | --- |
| `seq`, `tool_use_id`, `recorded_at_unix_ms` | declaration | -- |
| `tool_name`, `verb_class`, `program`, `permission_mode`, `agent_id`, `agent_type` | declaration | as the record's own null |
| `executed` | execution record(s), or Phase B's denial record | `executed` with `outcome`, `exit_code`, `duration_ms`; `denied_by_user` when Phase B's record says so; `unrecorded` when neither exists. Never "denied" from absence alone |
| `executed.records` | count of execution records for the id | 2 when both `PostToolUse` and `PostToolUseFailure` wrote one; the link's outcome fields come from the record with the higher `seq`, both outcomes are listed |
| `result_in_transcript` | the id's transcript group | `null`: transcript unreadable |
| `rewritten` | `executed_digest != shape.digest`, `Bash` only | `null` for any tool but `Bash`, whose digest covers the whole input and moves on a reworded `description`; `null` when either digest is empty or there is no execution record; never true in those cases |
| `input_digest_changed` | same comparison, non-`Bash` tools | rendered with the note that a changed `description` alone moves it |
| `hosts_declared` | declaration `hosts` | as the record: null when none named, never `[]` |
| `hosts_observed_in_window` | the destinations view, per declared host | one of the values below |
| `wire_attribution` | constant | `"time_window"` |
| `ssh_hosts` | declaration | rendered under `not observable`, never joined |
| `file_label` | Part 2 | `null` on a record without one: rendered `unknown` |
| `rule_match` | Part 3 | `null` on a record without one: rendered `unknown` |

`hosts_observed_in_window` takes its input from `buildDestinations`' output --
after `suppress(forgotten)` and after loopback and client-plane
classification -- never from the raw observation. Per declared host:

| value | condition |
| --- | --- |
| `forgotten` | `forget --host` suppressed it |
| `loopback` | in `loopbackHosts`; loopback is never proxied |
| `client_plane` | in `clientPlaneHosts`; the client's own traffic reaches it every session |
| `reached_in_window` | at least one in-window, non-inherited request, folded by `request_id`, did not end in a dial failure |
| `attempted_in_window` | in-window requests exist and every one, folded by `request_id`, ended in a dial failure; a success outside the window does not change this |
| `not_observed_in_window` | store read, window applied, no in-window request names the host |
| `unknown` | store absent or unreadable; no window; `WindowApplied` false; no `end` record (the right edge never closed); or any row for this host with an unparseable stamp among parseable ones |

The `internal/wire` change: `summarise` gains per-host `InWindowReached` and
`InWindowFailed`, incremented on the non-inherited branch after the
`request_id` fold; the chain derives reached-versus-attempted from those two
fields and nothing else. `Unreached` keeps its meaning for the destinations
section.

Two calls in one session naming the same host carry the same value; two
sessions with overlapping windows carry the same value for the same rows. The
section header says so once.

### Rendering

- `rashomon report --chain` adds a `chains` section per session (decision
  1). The default text report carries one line, `chains: N`, and keeps the
  listing behind the flag: a view nobody knows exists is the same as one
  never built, and the count is what tells a reader there is something to
  ask for. JSON gains `chains`, always present, `[]` when empty.
- Every rendered list is sorted. `--redact` deep-copies chains and every
  link's host slices, so redaction never mutates the caller's report; hosts
  render as the destinations section's digest, which Phase B made an HMAC
  under the install key in `896fb9c`. Part 1 lands after Phase B, so that is
  the digest it inherits; the "what exists" note above describes the merge
  target as it stands today, before that commit.
- No count of missing links; `missing_from_store` keeps them.

### Acceptance

Each item is shown to fail under a named break before it passes; `sweep.py`
gains one break per item; negatives carry a positive twin.

- **H-31 -- set equality.** The union of link ids over every chain, every
  non-chain group, and every dropped-declaration link equals the run's
  recorded declaration id set. Break: drop the null-prompt group.
- **H-32 -- split and order.** P1, P2, P1 in `seq` order render two chains,
  P1 first, the third declaration in P1. Break: one chain per session.
- **H-33 -- per transcript.** Two declarations sharing a `prompt_id` under
  different `transcript_path`s render in two chains. Break: key on
  `prompt_id` alone.
- **H-34 -- order is `seq`, not file order.** A fixture whose
  `records.ndjson` and `spill.ndjson` hold declarations out of `seq` order
  renders every chain in `seq` order. Break: render in read order.
- **H-35 -- shared host, no attribution.** Two declarations name one host;
  one in-window row. Both links carry a byte-identical entry
  `reached_in_window`; the destinations section counts the row once; no
  field on either link claims it. Break: attribute by proximity in time.
- **H-36 -- the link consumes the destinations view.** A declared loopback
  host reads `loopback`; a client-plane host with in-window client rows reads
  `client_plane`; a host suppressed by `forget --host` reads `forgotten` while
  the section above still suppresses it. Break: derive from `obs.Hosts`.
- **H-37 -- overlapping sessions, no attribution.** Two runs, overlapping
  windows, one row inside both. Each link carries the byte-identical entry;
  neither report marks the row inherited; each section counts it once.
  Break: assign to the nearer start.
- **H-38 -- the seven values.** One fixture per value in the table,
  including no `end` record and one unparseable stamp among parseable ones.
  Break: default the absent store to `not_observed_in_window`; second: render
  a dial failure as reached; third: treat `WindowApplied == false` as
  in-window.
- **H-39 -- the window bounds reached-versus-attempted, on the fold.** A
  successful request BEFORE the window and one refused dial (two rows, one
  `request_id`) inside it: `attempted_in_window`, `InWindowReached` 0,
  `InWindowFailed` 1. Break: derive from `Destination.Unreached`; second:
  count rows instead of folded requests.
- **H-40 -- rewritten is Bash-only and degrades to null.** Null for a
  non-`Bash` tool, for an empty declared digest, for an empty executed
  digest, for no execution record; true only when both `Bash` digests are
  non-empty and differ; an `Edit` whose `description` alone changed renders
  `input_digest_changed`, not `rewritten`. Break: compare digests for every
  tool.
- **H-41 -- redaction covers chains, and keeps them.** A canary hostname
  appears nowhere in `--chain --redact` output; redacted and unredacted
  reports carry the same chain and link counts; two links naming one host
  carry one digest; the unredacted report is byte-identical after the
  redacted render. Break: skip chains; second: redact in place.
- **H-42 -- no inferred parentage.** `spawned by: not recorded` even when
  exactly one agent-class declaration precedes the group. Break: attribute by
  nearest preceding agent-class call.
- **H-43 -- executed values.** A Phase B denial record renders
  `denied_by_user`; an id with neither record renders `unrecorded`; an id
  with both post records renders `records: 2` with both outcomes. Break:
  render every absence as `unrecorded`.

### Effort

About one week: two `internal/wire` counters, `internal/report/chain.go`,
rendering, thirteen items and their breaks.

## Part 2: sensitive-file labels

Owned by this track, built first: it is days, it is the piece Rishi asked
for by name, and it does not wait on Part 3's measurements.

### What it is

A declaration whose tool names a file gains `file_label`, one value from a
closed vocabulary, derived at hook time from the path's basename and suffix
against a fixed pattern list. The path is never stored; the label is. It is a
label, not a finding: it says what the path looks like, not what happened to
it, and it renders beside the link with no adjective.

Version 1 vocabulary: `ssh-key`, `credential-shaped` (tokens, API keys,
passwords, keychains), `cloud-config` (provider credential and config files),
`env-file`, `certificate`, `none`, `unknown`. The pattern list is a fixed
table in `internal/shape`, cited per row to the convention it matches, and
grown only by adding rows.

### Where it reads

A new function in the audited file, `shape.Label(toolName, toolInput)`,
reading exactly `file_path` for `Read`, `Edit` and `Write`, and
`notebook_path` for `NotebookEdit`, and nothing else. `Bash` calls get no
label in version 1: extracting a file target from a command line is the
recognised-file-command problem Part 3 lists as invisible, and a label from a
guess would be a guess. `Derive` and `executed_digest` are untouched. A path
that is empty, non-string, or absent yields `unknown`; a path that matches no
row yields `none`. Matching is on the basename and suffix only, case-folded,
after `~` expansion when present; no filesystem access.

### Record and report

`file_label` on the declaration, schema 3, nullable -- a field the Phase B
reservation does not yet carry, and the first coordination item with that
track. The report gains a `by_label` count per session and the chain link
carries it. Redaction leaves it untouched: it holds no path.

### Acceptance

- **H-44 -- no path, ever.** A canary path segment in `file_path` and in
  `notebook_path` reaches no record, no report output, no stdout and no
  stderr; the positive twin: the same call carries the expected label. Break:
  store the basename.
- **H-45 -- deterministic and closed.** The same path yields the same label
  across two installs and two runs; every label emitted is in the vocabulary;
  a path matching no row is `none`, an empty path `unknown`; `~/.SSH/ID_RSA`
  labels as `~/.ssh/id_rsa` does. Break: fall back to a substring of the
  path.
- **H-46 -- only where a path is read.** A `Bash` call, an `Agent` call and a
  `WebFetch` call carry `null`; `Read`, `Edit`, `Write` and `NotebookEdit`
  carry a label; the five panicking faults of H-1 injected inside `Label`
  exit 0 with the declaration recorded and the label `unknown`. Break: label
  `Bash` from its first path-like token; second: remove the recover.

### Effort

Days: the table, `shape.Label`, the field, the count, three items.

## Part 3: rule-match layer, version 1

Owned by this track, after Part 2, held for approval until the measurements
below are reported.

### The question it answers

"Which permission rule, in the settings files this tool can read, matched
this call when it was declared, and which matched the input that actually
ran, against those files when the post hook observed them?" -- nothing
wider. The matching is this tool's reimplementation of Claude Code's
documented evaluation, and each result is a rule match recorded with the time
it was computed and the policy revision it was computed against.

### Evidence for every claim

Every grammar, ordering, precedence and anchor claim in this part carries a
Claude Code documentation citation or a live item (L-n) run against a real
session; a claim with neither is implemented as `unknown`. Where the docs
are silent -- managed-source `/path` anchors, `ask` rules reaching `Read` and
`Edit` through a `Bash` command (the docs document deny, and allow for
redirections) -- the behaviour is the conservative one and the text says
"undocumented", not "documented".

Live items owed, in the existing L-n form: about a dozen rule/command pairs
run under a known settings file with a known trust state, each recording
whether the client prompted, ran, or refused, on a stated client version. The
assertion: this matcher never says `allow_rule_match` where the client
prompted or refused. Pairs: deny-then-ask-then-allow ordering; user deny
against project allow; a compound under a prefix allow; a command
substitution under a bare allow with an argument-pattern deny; a path
invocation (`/usr/bin/curl`) under a `Bash(curl *)` deny; `dontAsk` on a
would-prompt call; a `/path` project rule against a file under `.claude/`; a
symlink into a denied directory; `WebFetch(domain:...)` against the domain
and a subdomain; a case-variant path on a case-insensitive filesystem; a
project allow in an untrusted folder; a bare relative pattern after a `cd`
inside a `Bash` call.

### Measurements owed

1. **Added latency.** p99 per `PreToolUse` with 200 rules across four layers,
   on the team's machine class. Budget under 50 ms. Decision 3 turns on it:
   the coverage path reads one file up to three times per invocation today;
   four layers per invocation is up to twelve reads, doubled across both
   hooks.
2. **The unknown rate.** The share of declarations on real stored sessions
   that would read `unknown`, per reason, before any build. Population: real
   stored sessions from plain launches and, once the wrapper named under
   "what is not visible" exists, launches under it, the two reported apart --
   a launch under `--settings` carries a layer this matcher can never read,
   so its unknown rate is not the plain one and averaging them would hide
   the gap the flag opens. On a machine with
   a `.env` or `.ssh` deny rule most `Bash` verdicts may read
   `file_target_not_extractable`; if so, version 1 ships that number rather
   than a matcher that pretends otherwise.
3. **Real settings-file sizes**, before decision 4's cap is fixed.

### What is visible and what is not

Visible at each hook invocation: the four settings files at the paths this
tool derives -- managed at the documented path; user at
`settings.UserPath()`; project and local under the run's START `cwd`, not the
invocation's -- `permission_mode` from the `PreToolUse` payload; both `cwd`s;
and, at `PostToolUse`, the input as it ran.

Not visible, and therefore never inferred:

- **workspace trust.** Project `allow` rules are inert until the folder is
  trusted, and in a `claude -p` or SDK session in an untrusted folder they
  are never applied; a git-tracked `.claude/settings.local.json` is treated
  the same way. Deny and ask apply immediately. The state lives in
  `~/.claude.json` under `projects["<path>"].hasTrustDialogAccepted`, which
  version 1 does not read: an allow match from project or local settings is
  `unknown` with `workspace_trust_unknown`;
- **`--settings <file>`**, which applies above user, project and local and
  below managed, carries its own `permissions.*` and its own `/path` anchor
  (`<directory of file>/path`). That this flag is present by construction
  under the team's own recommended v0.1 wrapper is the maintainer's third
  review comment on pull request #7, and that comment is its only source: it
  is not read off the tree. At revision 6 nothing in this repository passes
  the flag -- `internal/launch/launch.go` execs the user's own argv and
  appends environment variables, injecting no `--settings`, and the only
  mention of it on any branch is the README naming it as a limitation. The
  claim is recorded here as the maintainer's, to be re-checked against that
  wrapper once it exists; **session rules** entered through
  `/permissions`; **MDM or console-managed policy** delivered other than as
  the managed file. `layers.managed: absent` means "no managed file at the
  documented path", never "no managed policy";
- `--allowedTools`, `--disallowedTools`, `--permission-mode`, `--add-dir`;
- session-only approvals and one-time approvals, which write nothing;
- the built-in read-only command list, the recognised file-command list,
  wrapper stripping, the working-directory read allowance, MCP and connector
  controls;
- an in-session `/cd`: the primary working directory can move and nothing in
  the payload says so;
- what Claude Code's own evaluation concluded.

Consequently `no_rule_match` means: no rule in the four files this tool
located and read matched at that invocation. It does not mean no rule was in
force, it does not mean the user was prompted, and beside an execution record
it does not mean the user said yes.

### Resolution at each hook invocation

No snapshot and no copy. At every `PreToolUse`, `PostToolUse` and
`PostToolUseFailure` invocation the hook path reads the four files -- under
the bounded loader and decision 3's read strategy -- and matches the call
against the rules it finds. Persisted: a `rule_match` object on the
declaration and on each execution record, and a `policy` record in
`coverage.ndjson` whenever ANY recorded policy state differs from the last one
recorded for this run, up to a per-run cap; past the cap no record is written
and every later match carries `policy_revisions_capped`.

**Concurrency.** Hooks run in parallel, each invocation a fresh process, and
coverage records carry no `seq`. The last recorded policy state lives in a
small file in the run directory, read and rewritten inside the run's
append-lock critical section -- the lock H-16 exercises -- and the `policy`
record is appended in the same section (H-65).

| `policy` field | meaning |
| --- | --- |
| `type`, `schema_version` | `policy`, 3 |
| `recorded_at_unix_ms`, `session_id`, `install_id` | as every record |
| `layers` | per layer: `present`, `present_empty`, `absent`, `not_located`, `unreadable`, `unparseable`, `oversized`; `absent` and `present_empty` are told apart by a stat before the load |
| `default_mode_file` | the `permissions.defaultMode` value found, with the layer it was found in, or null. NOT "resolved": since 2.1.257 `auto` and `bypassPermissions` do not take effect from project or local settings, and `--permission-mode` is invisible; the per-call `permission_mode` on the declaration is the effective answer |
| `rule_counts` | `allow`, `deny`, `ask`, `unsupported_form`, `unparsed` |
| `layer_digests` | per layer: an HMAC of that layer's canonical rules, or the layer's state name when it holds no readable rules |
| `rules_digest` | an HMAC over the four `layer_digests` in fixed order |
| `policy_revision` | an HMAC over the whole recorded state above |

`policy` is the first record in `coverage.ndjson` to carry a number. The
coverage record's no-number rule exists so a run whose instrumentation failed
cannot report a count; `rule_counts` count the user's rules, not anything this
instrument measured, so the rule is relaxed for this record type and this
field alone, named here and in the schema, and the report renders the policy
block beneath the run's coverage state, never above it.

Rule text never reaches a record, a file in the store, stdout or stderr
(H-60). The README's no-content promise gains one sentence saying so.

### Matching

Rules from all readable layers merge into one list; a deny from any scope
blocks an allow from any other (docs: "deny rules from any scope are
evaluated before allow rules"). The list is never truncated: over the cap,
every verdict is `unknown` with `rule_count_exceeded`, because an appended
allow must never push a deny out of the set. Evaluation order is deny, then
ask, then allow; the first match decides; specificity does not reorder it
(docs: manage permissions).

Matching runs on the raw command text, never on `Derive`'s `program`: a deny
rule "doesn't match the same program by path or inside `sh -c`" (docs), and
`Derive` records the program with `path.Base`, so `/usr/bin/curl` must not
match `Bash(curl *)`.

| verdict | condition |
| --- | --- |
| `deny_rule_match` | a deny rule is confirmed to match |
| `ask_rule_match` | every applicable deny is ruled out; an ask rule is confirmed to match |
| `allow_rule_match` | every applicable deny and ask is ruled out; an allow rule is confirmed to match; the rule is not from project or local settings |
| `no_rule_match` | every applicable rule is ruled out in the four located, readable-or-absent files, with the input fully evaluable |
| `unknown` | anything else, with a reason from the closed list |

**The conclusiveness invariant.** A confirmed deny is conclusive on its own.
An ask verdict requires every applicable deny rule to have been ruled out. An
allow verdict requires every applicable deny AND ask rule to have been ruled
out. "Applicable" means every deny or ask rule naming the call's tool or a
tool glob covering it and, for a `Bash` call, every `Read` and `Edit` deny
rule (docs: file commands and redirection targets) and, conservatively and
undocumented, every `Read` and `Edit` ask rule. An applicable rule that
cannot be evaluated against this input is not ruled out, and the verdict is
`unknown`. A bare `Bash` allow concludes only when no argument-pattern deny or
ask rule applies, or every one that does has been ruled out. A `deny_rule_match`
concluded inside a command substitution or subshell is correct -- deny and
ask rules apply there (docs: "an ask rule like `Bash(git clean *)` still
prompts you for `echo \"$(git clean -f)\"`") -- and version 1 may return
`unknown` instead where it cannot evaluate the nesting; it must never return
`allow_rule_match` there.

Grammar in version 1, each cited or owed an L-item: bare tool; exact; prefix
in both spellings (`Bash(git *)`, `Bash(git:*)`, `:*` trailing only),
spanning the whole remaining line of a single subcommand; domain
(`WebFetch(domain:example.com)`), rule and host canonicalised through
`host.Canonical`, exact host only, a subdomain candidate `unknown` until the
L-item settles it; path in gitignore syntax, `*` not crossing `/`, `**` any
depth, an `Edit` rule covering every file-writing tool; a glob in the
tool-name position of a deny or ask rule. Documented forms version 1 does not
implement -- a `*` elsewhere than trailing (`Bash(git * main)`),
`Agent(isolation:*)`, `!` negation patterns -- are counted under
`unsupported_form`, distinct from `unparsed`, which is for text the parser
does not recognise at all. Both are applicable-and-not-evaluable for any tool
they might cover. MCP rules carrying parentheses are skipped by Claude Code
itself and are `unsupported_form` here.

### `shape.Match`

`shape.Derive` cannot host matching -- its signature takes the input alone,
and it reads neither `file_path` nor `url`. Matching is
`shape.Match(toolName, toolInput, ctx)` in the same audited file; `Derive`
and `executed_digest` are untouched. `Match` reads exactly: `command` for
`Bash`; `file_path` for `Read`, `Edit`, `Write`; `notebook_path` for
`NotebookEdit`; `url` for `WebFetch`; nothing for any other tool. Structure
detection for `Bash` runs on the raw `command` string before tokenising. `ctx`
carries both `cwd`s, the per-source anchors, the platform, the deadline. The
executed match takes its `permission_mode` from the declaration, because the
post payload carries none. Any failure inside `Match` is recovered inside
`Match`, before the declaration is appended, so the record still lands with
`unknown`. H-60's canary extends to every field `Match` reads.

### Path rules: anchors, resolution, platform

| pattern | anchor |
| --- | --- |
| `//path` | the filesystem root |
| `~/path` | the home directory |
| `/path` in project or local settings | the primary working directory: the run's START `cwd` (docs: `<primary working directory>/path`) |
| `/path` in user settings | `~/.claude` (docs) |
| `/path` in managed settings | undocumented; `unknown` (decision 5) |
| `/path` in `--settings` | `<directory of file>/path` (docs); that file is not visible, so never evaluated |
| `path` or `./path` | the invocation's `cwd`, which a `cd` inside an earlier `Bash` call moves (measured); owed an L-item |

An in-session `/cd` moves the primary working directory and is not
detectable from the payload; version 1 anchors on the start `cwd` and states
the limit. The project and local layer FILES are located under the start
`cwd` as well; when the nearest ancestor of the start `cwd` holding a `.git`
directory is not the start `cwd` itself -- a subdirectory launch -- the
project and local layers are `not_located`, found by walking parents with
`os.Stat`, never by executing `git`.

Symlinks, per the docs: an allow rule applies only when both the named path
and its resolution match; a deny or ask applies when either matches, and a
deny or ask written through a symlinked directory applies at the real
location. Version 1 resolves accordingly: an empty `cwd` on either side, or
an unexpandable leading `~`, is `path_context_unavailable`; a path that does
not exist -- every file creation -- resolves its nearest existing ancestor
and rejoins the tail, recorded beside the verdict as `path_does_not_exist`;
resolution runs under a deadline and a link-depth cap, and any other error is
`path_resolution_failed`; on Windows and macOS comparisons are case-folded,
on Linux case-sensitive, as a platform default stated in the report and
covered by an L-item; a path that resolves differently at `PreToolUse` and
`PostToolUse` yields two matches and is never reconciled -- that TOCTOU
window is this tool's, and is stated.

### Reasons

The closed list, exported through a `shape.Reasons()` function and added to
the schema enum test beside `store.Reasons()` and `report.Reasons()`:

| reason | condition |
| --- | --- |
| `no_policy_readable` | a layer is present and none is readable |
| `policy_partially_readable` | a present layer is `unreadable`, `unparseable` or `oversized`; a confirmed deny still concludes |
| `policy_not_located` | project or local layer `not_located`, or the start record absent |
| `policy_revisions_capped` | the run's cap was reached |
| `rule_count_exceeded` | the merged list exceeds the cap; nothing truncated, nothing concluded |
| `workspace_trust_unknown` | the matching allow rule is from project or local settings |
| `unparsed_rule_may_apply` | an `unparsed` or `unsupported_form` rule names the tool or a glob covering it |
| `unsupported_input_structure` | the raw command contains a separator, subshell, substitution, backtick, control-flow keyword, redirection or wrapper prefix, and an applicable argument-pattern rule could not be ruled out |
| `file_target_not_extractable` | a `Bash` call with no unsupported structure while a `Read` or `Edit` deny or ask rule exists |
| `host_not_canonical` | a `WebFetch` authority `host.Canonical` refuses |
| `path_context_unavailable` | an anchor or input path cannot be established |
| `path_does_not_exist` | beside a verdict: the named path was evaluated after ancestor resolution |
| `path_resolution_failed` | resolution error, deadline, or depth cap |
| `input_unavailable` | the post payload carried no input |
| `evaluation_failure` | any recovered failure, including the matching deadline |

**Matching under a deadline.** Every file read and path resolution runs under
a matching deadline well inside the 5-second hook timeout; exceeding it is
`unknown` with `evaluation_failure` and the hook exits 0 on time, so a slow
settings layer can never become a SIGTERM that flips coverage to `unverified`
(H-64).

### Two matches per call, each stamped

```
"rule_match": { "verdict": ..., "reason": ... | null, "rule": <pseudonym> | null,
                "policy_revision": ..., "rules_digest": ...,
                "evaluated_at_unix_ms": ... }
```

The declaration's match is computed at `PreToolUse` on the declared input
against the files at that invocation; each execution record's match at its
own post invocation on the input that ran, against the files at that
invocation -- not necessarily the rules in effect when execution began.
`policy_revision` names the full recorded state; `rules_digest` beside it
tells a rule-content change from a state-only change (an empty layer's
readability flip changes `policy_revision` alone; a non-empty layer's changes
both, because `layer_digests` carry a state marker). `rule` is the pseudonym
of the matching rule's canonical text: equal for two calls hitting one rule
on one install, different across installs, not recoverable.

### The settings size cap

`settings.Load` stays unbounded and keeps serving `watch`, `detach` and the
coverage path's hook-entry probe -- a cap there would make `watch` refuse to
install when the user's file crossed it and flip every run on that machine to
`unverified`. A separate bounded loader, in the `io.LimitReader` shape the
stdin path already uses so the bound is observable, serves `shape.Match`'s
policy reads. Stated plainly: a settings file over the cap makes that layer
`oversized` and its matches `unknown`, and changes nothing about coverage.
The cap's value follows the size measurement (decision 4).

### Report

A `rule_matches` section per session, beneath the run's coverage state:

- `policy`: revisions recorded and whether capped; for the latest, the layer
  states, `default_mode_file` with its layer, the counts, the digests; `not
  recorded` when absent, in which case every verdict reads `unknown`.
- `by_verdict`: declared and executed counts per verdict, `unknown` broken
  down by reason.
- `matcher_disagreements`: ids whose EXECUTED match is `deny_rule_match` and
  which have an execution record. Not a verdict -- under `bypassPermissions`
  the whole list is normal. Each carries the mode, both revisions, the four
  possible causes (this matcher's error; an input this tool cannot see; a
  rule added after execution began; a decision Claude Code made under the
  mode's documented semantics) and `which: not determinable`. Renders only
  here (decision 7).
- `verdict_changed`: ids whose declared and executed verdicts differ, with
  `input_changed` (from `rewritten`, `Bash` only, null otherwise),
  `policy_changed` and `rules_changed`.
- `unknown`: ids by reason.

Forbidden strings: no rendered output of this section contains `violation`,
`unauthorized`, `bypass` outside the verbatim mode name, or `should not
have` -- tested in the forbidden-strings form the Phase B work uses.

Host rules: `WebFetch(domain:...)` matches like any rule; a host named on a
`Bash` command line keeps the project baseline as its only signal (decision
6, closed).

### Acceptance

- **H-47 -- order.** Ask and allow both matching reads `ask_rule_match`; deny
  and allow, `deny_rule_match`. Break: allow before ask.
- **H-48 -- deny across scopes.** User deny with project allow reads
  `deny_rule_match`; managed deny with local allow likewise. Break: layer
  precedence on the arrays.
- **H-49 -- read at declaration.** An allow rule appended to the local file
  between two declarations of one call: `no_rule_match` then
  (trust aside, using a user-settings allow) `allow_rule_match`, two
  `policy_revision` values, exactly two `policy` records however many
  `PostToolUse` and `PostToolUseFailure` invocations intervene. Break: cache
  the rules from `probe start`.
- **H-50 -- the executed input decides.** A fixture `PreToolUse` hook
  rewrites a denied command into an allowed one: declaration
  `deny_rule_match`, execution `allow_rule_match`, `rewritten` true,
  `verdict_changed` with `input_changed: true`, `matcher_disagreements`
  empty; the executed match carries the declaration's `permission_mode`.
  Break: use the declared match for the list; second: read the mode from the
  post payload.
- **H-51 -- unchanged input, changed policy.** A deny appended before the
  post hook: declaration `allow_rule_match` (user-settings allow), execution
  `deny_rule_match`, `rewritten` false, `verdict_changed` with
  `input_changed: false, policy_changed: true, rules_changed: true`. Break:
  derive `policy_changed` from `rewritten`.
- **H-52 -- unsupported and unparsed are counted apart.** `Bash(git * main)`
  counts under `unsupported_form`, an unrecognised token under `unparsed`,
  and each forces `unknown` for a call it might cover. Break: drop either
  silently; second: count both as `unparsed`.
- **H-53 -- structure never lets an allow past an unevaluated deny.** With
  only `Bash(git *)` in allow, `git status && curl example.com` reads
  `unknown`, and so does the two-line form, from the raw string. With a bare
  `Bash` allow and no applicable deny or ask, the compound reads
  `allow_rule_match`. With a bare `Bash` allow and `Bash(curl *)` in deny,
  `echo "$(curl example.com)"` reads `unknown` or `deny_rule_match`, never
  `allow_rule_match`. With `Bash(curl *)` in deny, `git status && curl
  example.com` reads `deny_rule_match`. Break: let a bare-tool allow conclude
  over an unevaluated argument-pattern deny; second: detect structure on the
  tokenized form.
- **H-54 -- path and wrapper invocations do not match.** With `Bash(curl *)`
  in deny, `/usr/bin/curl example.com` does not read `deny_rule_match`, and
  neither does `sh -c 'curl example.com'`. Break: match against `Derive`'s
  `path.Base` program.
- **H-55 -- partial policy and layer states.** Managed unreadable with a
  matching user allow reads `unknown` with `policy_partially_readable`; with a
  matching project deny, `deny_rule_match`; `unparseable`, `present_empty`
  and `absent` are recorded as themselves. Break: skip unreadable layers;
  second: record an empty or unparseable file as `absent`.
- **H-56 -- not located.** A run whose start `cwd` is a subdirectory of a
  repository holding a matching deny in `.claude/settings.json`: project
  layer `not_located`, verdict `unknown` with `policy_not_located`, never
  `no_rule_match`. Break: locate from the invocation `cwd`; second: record
  the unopened file as `absent`.
- **H-57 -- trust is unknown.** A matching allow rule from project settings,
  and one from local settings, each read `unknown` with
  `workspace_trust_unknown`; the same rule in user settings reads
  `allow_rule_match`. Break: conclude allow from a project rule.
- **H-58 -- oversized, observably bounded, matching only.** A user settings
  file over the cap: layer `oversized`, verdict `unknown`, the loader having
  read no more than the cap (asserted on the reader, not the file length);
  coverage state and hook-entry resolution unchanged; `watch` still
  installs. Break: read the whole file and check its length; second: cap
  `settings.Load`.
- **H-59 -- a revision is any change of recorded state.** Empty managed
  file readable, unreadable, readable, rules unchanged: three records, three
  `policy_revision` values, one `rules_digest`, the middle match carrying the
  middle revision. The same with a non-empty layer: three revisions, two
  `rules_digest` values, `rules_changed: true` on the middle match. Break:
  record only on `rules_digest` change; second: omit the state marker.
- **H-60 -- no rule text, and pseudonyms behave.** Premise guard first: the
  canary rule matches the fixture call and `rule` is non-null. Then: the
  canary reaches no record, no file under the store, no report output, no
  stdout, no stderr; a canary in `command`, `file_path`, `notebook_path` or
  `url` likewise; two calls on one rule carry equal `rule` values, the same
  rule on another install a different one. Break: store the rule text;
  second: write a compiled copy into the run directory; third: hash without
  the install key.
- **H-61 -- path anchors.** `Edit(/src/**)` in project settings, trust aside,
  is evaluated against `<start cwd>/src/a.ts` and not `<start
  cwd>/.claude/src/a.ts`; `Read(/secrets/**)` in user settings matches
  `~/.claude/secrets/x` and not `<start cwd>/secrets/x`; after a `cd` inside
  a `Bash` call, a later `/path` rule still anchors at the start `cwd` and a
  bare relative pattern anchors at the invocation's `cwd`. Break: anchor at
  the settings file's directory; second: anchor `/path` at the invocation's
  `cwd`.
- **H-62 -- symlinks, new files, platforms.** `Read(./project/**)` allowed
  (user settings) and `Read(~/.ssh/**)` denied: `./project/key` linking to
  `~/.ssh/id_rsa` reads `deny_rule_match`; `./project/plain` reads
  `allow_rule_match`; a `Write` to `./project/new/file.txt` with no `new/`
  reads `allow_rule_match` with `path_does_not_exist` beside it; a chain past
  the depth cap reads `path_resolution_failed`; on the case-insensitive
  platform default `~/.SSH/id_rsa` reads `deny_rule_match`. Break: evaluate
  the named path only; second: treat non-existence as failure; third:
  compare case-sensitively everywhere.
- **H-63 -- no fault reaches the agent.** The five panicking faults of H-1
  injected inside `Match` exit 0 with the declaration recorded and its
  verdict `unknown` with `evaluation_failure`. Break: recover after the
  append.
- **H-64 -- slow policy is unknown, not a timeout.** A layer served by a
  fixture that blocks past the matching deadline: `unknown` with
  `evaluation_failure`, exit 0 before the hook timeout, coverage `verified`.
  Break: read without a deadline.
- **H-65 -- one policy, one record, under contention.** Twenty concurrent
  invocations under one unchanged policy: exactly one `policy` record; a
  policy changing once, in order: exactly two; past the cap:
  `policy_revisions_capped` on every later match and no further record.
  Break: compare against the last line of `coverage.ndjson` outside the lock.
- **H-66 -- no rule match is not a prompt.** `no_rule_match` beside an
  execution record renders `no rule in the read files matched; ran`, and no
  output renders it as prompted, approved or granted. Break: label it
  `approved`.
- **H-67 -- modes rendered, words forbidden.** An executed `deny_rule_match`
  under `dontAsk`, under `bypassPermissions`, and under an invented mode name:
  the `matcher_disagreements` entry renders the mode verbatim, the four
  causes and `which: not determinable`, and nothing else differs; no rendered
  output contains a forbidden string. Break: suppress the entry for a mode
  the code considers a bypass; second: render `should not have run`.
- **H-68 -- the reasons are the code's reasons.** `shape.Reasons()` equals
  the schema enum, and every reason the implementation can emit is in it.
  Break: add a reason to the code and not to the function.
- **H-69 -- the filesystem reach of the hook path is bounded by the call's
  own input.** Over a fixture run, every path the hook path stats or
  resolves is one the call's own `tool_input` named, or an ancestor or link
  target reached from it under the documented resolution; no path derived
  from a settings file, an environment variable, the transcript or the store
  is stat'd; no path is ever opened for its contents, the settings layers
  excepted, which are read through the bounded loader alone; and resolution
  stops inside the matching deadline and the link-depth cap. Break: resolve
  a path taken from anywhere but the call's input.

  This item stands alone; it does not extend H-17's forbidden-import check.
  H-17 asserts a property of the import graph -- `go list -deps` over the
  recorder, against a list of packages that give code a path to a socket --
  and the property here is behavioural: not which packages are linked, but
  which paths a running hook touches. The two are not interchangeable, and
  the obvious way to make H-17 carry it does not work: `os` cannot join the
  forbidden list, because the store, the settings loader and the coverage
  path all require it, so adding it would fail the check on code that
  predates Part 3 and says nothing about paths either way. What H-17 does
  establish stays true and is relied on here -- Part 3 adds no networking
  package, and `os/exec` remains forbidden, so nothing on this path can
  reach a filesystem through a child process either.

### Effort

About four weeks after approval, after Part 2: bounded loader and layer-state
detection; `shape.Match` with raw-string structure detection, the invariant,
path anchoring, resolution and platform rules; the `policy` record with its
lock-protected last-state file and cap; the report section; twenty-three items
and their breaks; the live items; the README sentence. Schema 3 is on the
Phase B branch.

## Schema

Schema 3 is cut on the Phase B branch, and this document reserves nothing on
its own. The reservation carries `host_source` on declaration hosts;
`rule_match`, nullable, on the declaration and the execution record; and
`file_label`, nullable, on the declaration. `file_label` was missing from the
reservation as first written and is being added on the Phase B branch before
that pull request opens, so Part 2 rebases onto it and adds nothing to the
schema at all.

Each is required only under `schema_version == 3`, through an `allOf` keyed
on the version, so no record already on disk fails the published contract,
and `store.Accepts` admits 3.

The `policy` record type is NOT reserved, and the rule is worth stating
because it is the general one: reserve a field whose only commitment is that
it may be null; never reserve a structure. `policy` is a whole record type
with named fields belonging to a part that has not been approved, and putting
that shape in the published contract would freeze a design that may still
move. Part 3's own pull request adds it, once Part 3 is approved.

This track's branches add behaviour to that schema, not schema.

## Decisions taken

1. `--chain` is a flag; JSON carries `chains` regardless.
2. The grammar as written; `unsupported_form`, `unparsed` and
   `unsupported_input_structure` count what is missed.
3. Per-invocation reads of all four layers only if the measured p99 added
   latency is under 50 ms; otherwise a modification-time cache.
4. A 1 MiB cap, fixed after real settings-file sizes are measured.
5. Managed-source `/path` anchors stay `unknown`; the docs have no row.
6. Hosts named on `Bash` command lines: baseline only. The allowlist-file
   alternative is a product non-goal; closed.
7. `matcher_disagreements` renders only in its section.
8. One schema-3 bump on the Phase B branch, reserving `host_source`,
   `rule_match` and `file_label` -- fields, and only fields. `policy` is not
   reserved; Part 3's pull request adds it on approval.
9. Per-call wire attribution: later; proxy-side.

## Ownership and order

- This track, in this order: Part 2 (days, now); the #5/#6 rework; Part 3's
  measurements and live items, then Part 3 after its approval.
  `internal/shape`, `internal/hook`, `internal/settings`.
- The other track: Phase B (keyed redaction, the release block, schema 3, the
  per-call diff), then Part 1, post-v0.1. `internal/report`, `internal/wire`,
  `internal/baseline`, release, the schema file.

## Sign-off

Part 1 is approved to build against H-31 through H-43. Part 2 is approved to
build against H-44 through H-46 and starts now. Part 3 is approved for
implementation once the three measurements are reported inside their budgets,
by a further comment on the pull request that carried this document: #7,
<https://github.com/altrace-dev-role/rashomon/pull/7>. The number and the
link are written out because a squash merge leaves this file in `main` with
no other pointer to that thread, and the comment is where the approval
lives. Each part lands as its own pull request, reported the way every
H-item is: the command that ran it and its output, and the break that made
it fail first.
