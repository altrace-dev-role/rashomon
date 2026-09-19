# rashomon

`rashomon` records what a Claude Code session *asked* to run, and the evidence that
the recorder was running while it did. It captures identifiers and the derived
shape of each tool call. It never captures content.

A declaration is not an execution. Claude Code's `PreToolUse` hook fires before
the permission flow resolves, so the store contains records for calls the user
went on to deny. That is deliberate: a declaration is the agent's stated intent,
and intent is worth keeping whether or not it was granted. Every record carries
`permission_mode` so that a later comparison can exclude what was never allowed
to run. `PostToolUse` closes the loop: an execution record says a declared call
ran. A declaration with one ran; a declaration without one was denied, or
failed, or had its execution go unrecorded — and nothing here claims to know
which of the three.

The store exists to be joined against. Every declaration carries
`tool_use_id`, `session_id`, `prompt_id`, and, inside a subagent call,
`agent_id`; executions and terminals carry `tool_use_id` and `session_id`;
coverage and gap records are per session. Without those
keys the capture is a pile of anonymous shapes and nothing downstream can
integrate it.

Point it at an observing proxy's store as well and it will join the two
records: what the session said it would do, against what actually left the
machine. That comparison is the one thing neither half can produce alone — a
host reached by a command that never named it appears in no transcript and in
no hook log, because nothing in the session knows about it.

Claude Code only: the hooks, the store and the report describe Claude Code
sessions and nothing else, and no other agent harness is recorded. The Cursor
command files in this repository operate the tool from Cursor; they do not
record it, and nothing here can.

Built by [Altrace](https://github.com/altrace-dev-role). The proxy whose store
this reads is a separate, closed product; `rashomon` is useful without it and
reports honestly when it is absent.

## Commands

- `rashomon watch` — install the `PreToolUse`, `PostToolUse` and
  `PostToolUseFailure` recorders and the
  `SessionStart` / `SessionEnd` liveness probe. The only command that writes to
  your configuration. It prints the exact `detach --install <id>` line that
  undoes it without a store.
- `rashomon detach` — remove them, leaving everything else in the file as it was
  found.
- `rashomon detach --install <id>` — remove the entries carrying one install id,
  opening no store.
- `rashomon detach --all` — remove every entry whose command line carries an
  rashomon install marker, whatever its id, for when the store is gone and the id
  with it.
- `rashomon status` — say what is installed here: the store and its install id,
  each entry as present, absent or unreadable, the install ids of anything else
  sharing the file, and whether hooks are disabled and by which layer. It
  reads; it writes nothing and creates no store — not even the store it is
  reporting on, because a command whose whole answer may be "nothing is
  installed here" must not be the command that installs something.
- `rashomon report [--session S] [--json] [--redact] [--proxy-store PATH]` —
  render declarations, executions and coverage as text for a terminal, or as
  JSON for a consumer. What is null in the JSON reads as `unknown` in the text
  — or `not read` for the counts of a transcript that could not be read — and
  never as `0`. The accounting equation is rendered once per transcript the
  run's declarations named, because a nested `claude -p` writes its own
  transcript under the parent's session id.

  The destinations section is rendered whenever a proxy store can be read.
  With no flag, `report` looks for `~/.altrace/observe/causal.db`, which is
  where the observe profile puts it; `--proxy-store PATH` points somewhere
  else. There is no flag to turn the join off — remove the store, or point the
  flag at a path that does not exist.

  `--redact` drops the agent's summary entirely and replaces each hostname with
  an HMAC under THIS INSTALL'S OWN KEY plus the host's last label, stable
  within a report and across reports from the same machine, so a host can be
  followed across sections and between runs.

  What the key buys: the same host digests DIFFERENTLY on another machine, so a
  recipient who does not have your key cannot test a guess. It was previously
  an unkeyed hash, which made it a dictionary lookup — the space of hostnames
  is small and guessable, and a reader holding a candidate simply hashed it and
  compared.

  What it does not buy: eight hex characters is 32 bits, so two hosts can
  collide; the last label is kept in clear on purpose, because `.internal` and
  `.amazonaws.com` say something true about where traffic went without naming
  anyone; and anyone who can read this install's store can compute these. The
  redacted report states all of that in its own header, because the report is
  what gets shared and this file is not.

  `report` creates no store — a location that has recorded nothing renders "no
  sessions recorded" rather than minting an install identity to say so — but it
  is not read-only either: when a proxy store is readable it WRITES the project
  baseline, `baseline/<project>.json` under the store root, holding the project
  path and the hostnames seen for it. That file is what "new for this project"
  compares against, and it lives outside `runs/` so that evicting runs cannot
  make every host novel again.
- `rashomon forget --host H` — evict every call that named host `H`, its rows
  from the view, and its entry in the project baseline. It does not delete from
  the proxy's own store: that database is hash-chained and opened read-only
  here, so the gap record carries a keyed digest of the host and the report
  keeps the destination suppressed rather than claiming the row is gone.
- `rashomon forget --since T` — evict records recorded at or after `T` (an RFC 3339
  time, or a duration such as `24h` meaning that long ago), leaving a coverage
  gap record behind.
- `rashomon forget --before T` — the retention counterpart: evict records recorded
  before `T`, leaving the same gap record. The two are opposite open ends of one
  window and one code path; naming both is refused rather than resolved.
  Forgetting what was never recorded is a satisfied request, not an error.
- `rashomon env [--port N]` — print the proxy variables to export, for use with
  `eval`. HTTPS only: plain HTTP is not observed in this release, so no
  variable for it is printed.
- `rashomon run [--proxy-status PATH] -- <cmd...>` — run a command with those
  variables set, if and only if an observe-mode proxy is running
  (`--proxy-status` overrides where that is checked), and report on the session it
  produced. It checks first and launches anyway without the variables when the
  check fails, saying why in one line: you asked to run your command, and a
  wrapper that declined because a status file was missing would be worse than
  one that runs without recording. The exception is that nothing is recording
  at all, where it refuses and tells you to run `watch` first. The child's exit
  code is returned unchanged and the report goes to stderr, so the child's
  stdout stays pipeable.
- `rashomon version` — print the version.

Invoked by Claude Code, never by hand: `rashomon hook` for `PreToolUse`,
`rashomon post` for `PostToolUse`, and `rashomon probe start|end` for
`SessionStart` and `SessionEnd`. Each carries `--install <id>` in the installed
command line. That id is how `watch` and `detach` tell their entries from
another install's, and the hook paths read it too: an entry naming an install
other than the one this environment resolves stands down — exit 0, one fixed
line on stderr, and nothing written to the store, not even a run directory for
a session it has never seen. An entry naming no install records exactly as it
always has. `watch` names the other installs whose entries share the file,
because those entries fire in this environment and record nothing in it.

The store is at `$RASHOMON_HOME`, else `$XDG_STATE_HOME/rashomon`, else
`~/.local/state/rashomon`. `RASHOMON_STORE_CAP_BYTES` bounds it (default 512 MiB;
the oldest runs not written to within an hour are evicted, each leaving a gap
record). `CLAUDE_CONFIG_DIR` is honoured exactly as Claude Code honours it.

## Installing

    go install github.com/altrace-dev-role/rashomon/cmd/rashomon@main

Or clone and build:

    git clone https://github.com/altrace-dev-role/rashomon
    cd rashomon && go build ./cmd/rashomon

`@main` rather than `@latest` because there are no released versions yet and
`@latest` has nothing to resolve to. Once the first `v*` tag exists, `@latest`
is the one to use and will resolve to it.

A Homebrew formula follows the first release.

INSTALL FROM A STABLE LOCATION. `watch` writes the running executable's
ABSOLUTE PATH into `~/.claude/settings.json`, and Claude Code executes that
path on every tool call for as long as the entry is installed. Whatever the
binary was when you ran `watch` is what fires afterwards, so a binary that
moves, is deleted, or is rebuilt somewhere else leaves an entry naming a file
that no longer exists — and every tool call then fires a hook that cannot
start.

`watch` refuses the worst case outright: the binary `go run` compiles lives in
a `go-build` temporary directory that is gone when the process exits, so
installing from it would be installing a path guaranteed to be dead within
seconds. The cases it cannot refuse are yours to avoid — build into a
directory you keep, or `go install` it so it lands in your GOBIN, and re-run
`watch` after any move. The other commands are unaffected: they do not care where the
binary lives.

## Claude Code integration in this repository

This repository carries its own Claude Code surface, and none of it bends the
rule above about settings files: the binary still writes only to
`~/.claude/settings.json`, and only when you run `watch`. The repository
installs no hook of its own — there is no committed `.claude/settings.json` —
so nothing here runs in your sessions until you run `watch` yourself or add
the optional state line below.

- `scripts/claude-rashomon` (POSIX) and `scripts/claude-rashomon.ps1`
  (Windows) start a recorded session in one word: check `claude` exists, run
  `watch`, then start `claude` with arguments passed through. An invocation
  that starts no session installs nothing: `--help`, `--version`, and the
  management subcommands (`doctor`, `mcp`, `update`, and the rest listed in
  the script) pass straight through without `watch`.
- `scripts/rashomon-sessionstart.sh` is an optional `SessionStart` hook that
  puts one line of recorder state into a session's context. This repository
  does not install it; its header shows the entry to add to your own
  `~/.claude/settings.json` if you want the line. It runs `status`, creates
  no store, always exits 0, tells present, absent and unreadable-or-unknown
  entries apart, and says `unknown` for anything it cannot establish. On
  Windows it needs Git Bash, which Claude Code uses for hooks when present.
- `skills/rashomon*` are the slash commands: `/rashomon` starts recording
  (the only one that may install anything), `/rashomon-forget` erases
  records, and `/rashomon-report`, `/rashomon-status` and `/rashomon-stop`
  only read or detach.

  THEY ARE ASSETS YOU INSTALL, not project settings of this repository, and
  they are deliberately not under `.claude/` here. A skill in this
  repository's `.claude/skills` would load only for someone who has THIS
  repository open — which is the one place the commands are least useful.
  You want them where you actually work, so copy the ones you want:

      cp -r skills/rashomon* ~/.claude/skills/        # every project
      cp -r skills/rashomon* /path/to/project/.claude/skills/   # one project

  The same reasoning as the optional `SessionStart` hook above: this
  repository ships the asset and says how to install it, and installs
  nothing itself.
- `.cursor/commands/rashomon*.md` give Cursor the same slash commands for
  OPERATING the tool — status, report, detach, and arming `watch` for the
  machine's Claude Code sessions. They do not extend the scope stated at the
  top of this file: Cursor's own agent loop is not recorded, every command
  says so, and recording Cursor would be a new adapter in the binary, not a
  script.

## The constraint that shapes everything

Exit code 2 from a `PreToolUse` hook **blocks the tool call**. Claude Code's hook
documentation is explicit that this is not advisory: a JSON `permissionDecision`
of `allow` cannot override it.

In Go, exit code 2 is what a panic does. Measured with compiled binaries:

| Failure | Exit code | Effect on the agent |
| --- | --- | --- |
| `panic("boom")` | 2 | tool call blocked |
| nil-map write | 2 | tool call blocked |
| nil-pointer dereference | 2 | tool call blocked |
| index out of range | 2 | tool call blocked |
| send on a closed channel | 2 | tool call blocked |
| panic in a spawned goroutine | 2 | tool call blocked |
| `os.Exit(1)` / `log.Fatal` | 1 | none |

`recover()` in `main` does not catch a panic raised in another goroutine.

So the only Go failure that turns an observability tool into an enforcer is an
unrecovered panic — and the first unexpected payload is enough to cause one. The
user would see their tool calls being blocked and would reasonably conclude that
Claude Code was broken, not that this program was. Every goroutine this program
starts carries its own deferred recover, or it starts none.

## Installation and restore

The target is `~/.claude/settings.json`, named in code and never inferred.
Nothing is ever written to a settings file committed to a repository.

The installed entry uses `"matcher": "*"` and `"timeout": 5`. The timeout field
is in seconds; the documented default is 600.

The matcher is not a tuning knob. A `Bash` matcher produces no declaration for
`Edit`, `Write`, `WebFetch`, or any `mcp__*` call, while the report goes on
claiming full coverage — a confident number that is wrong, which is worse than no
number at all. This mistake is common enough in real configurations that it is
worth naming as the default failure mode rather than a hypothetical one.

Install identity is a stable install id, not a session id. `watch` runs before
any Claude Code session exists, so `session_id` is unavailable at install time.
`detach` removes our entry when no other install claims it, and removes only its
own claim otherwise.

`detach` preserves concurrent edits. Claude Code writes to this file during
normal use — every "always allow" the user accepts appends to `permissions.allow`
— so a `detach` that aborted on any change to the file would make the tool
uninstallable within minutes of first use. Abort is reserved for a change inside
our own entry's region, and when it aborts it says so.

Writes are atomic: temp file, `fsync`, then `rename(2)` within the same
directory.

`watch` refuses when hooks are disabled, and the precedence matters in both
directions. A managed `disableAllHooks: true` beats a project-level `false`, so
`watch` refuses. A user-level `true` does not beat a project-level `false`,
because in that arrangement hooks do run, so `watch` must not refuse. Getting
this right means implementing Claude Code's settings precedence including the
managed path, which is more work than a single file read.

Recovering when the binary or the store is gone. The installed command line is
an absolute path to the binary. Delete the binary and Claude Code errors on
every tool call, and `detach` — which is that binary — cannot run: install
`rashomon` again, anywhere, and run the line `watch` printed at install time,
`rashomon detach --install <id>`. An entry is recognised by the marker in its
command line and never by the path, which is what lets a binary at a new path
remove the old install's entries. Delete the store and the install id goes with
it; `rashomon detach --all` is for that case, and removes every entry carrying an
rashomon install marker whatever its id. Neither form opens the store, so neither
can create one. A plain `rashomon detach` does read the id from the store, and on
a machine that has none it names the two forms above rather than creating a
store to answer its own question. Both refuse, naming the field, when an entry
they would remove has been edited, and both leave every foreign entry byte for
byte as it was found.

## What is captured

The `PreToolUse` payload carries twelve fields: `session_id`, `prompt_id`
(absent until first input), `transcript_path`, `cwd`, `scratchpad_dir`,
`permission_mode`, `hook_event_name`, `tool_name`, `tool_input`, `tool_use_id`,
and — inside a subagent call only — `agent_id` and `agent_type`.

Persisted: `tool_use_id`, `session_id`, `prompt_id`, `agent_id`,
`transcript_path`, `permission_mode`, `tool_name`, and the derived shape of
`tool_input`.

Nothing else from inside `tool_input` is persisted. The exception is
hostnames: `hosts` and `ssh_hosts` carry the hosts a call NAMED, extracted
from inside `tool_input` and stored in clear, because the whole destinations
comparison is "what did it say it would reach" against "what did the wire
see", and a digest cannot be joined against a proxy's rows. Argument values,
command strings, prompts and responses are not persisted.

The derived shape is `program`, `verb_class`, `argc` and `digest`; the record
that carries it carries `schema_version`. A command that will not tokenize records `argc: null`, never
`0` — zero is a count, and in that case we do not have one. `digest` is an HMAC
under a per-install random key, so the same command digests differently on two
installs and the store cannot be run as a dictionary attack against known command
strings. The key file is mode `0600`.

The digest covers the command line alone for a shell tool, and the whole
canonical `tool_input` for every other tool. Claude Code attaches a free-text
`description` to a `Bash` call and the wording differs from call to call, so a
digest over the whole input would give the same `git status` a fresh value on
every call and group nothing. A `Write` call has no command line, and there the
whole input is all there is to tell two of them apart.

A command line is split as a shell would split it, without expanding anything.
The token an unterminated quote interrupts is not among the tokens returned: it
was never completed, and emitting it would carry bytes from inside the quoted
string out into `program`.

The `PostToolUse` payload carries the same session fields plus `tool_name`,
`tool_use_id`, `tool_input` and `tool_response`. The execution record persists
`tool_use_id`, `session_id` and `tool_name` beside its own `seq`,
`recorded_at_unix_ms` and `schema_version`, and — since schema 2 — how the call
ENDED: `outcome`, `exit_code`, `is_interrupt`, `duration_ms`, and
`executed_digest`.

That last one is why the post path reads `tool_input` at all. It derives the
same shape digest the declaration derived, under the same per-install key, so
the two can be compared: a call whose executed digest differs from its declared
one is reported as having executed differently from what was declared. The
input itself is not persisted — only the digest of it survives.

`tool_response` is tool output: the file a `Read` returned, the bytes a command
printed. The payload struct has no field for it, so `encoding/json` discards it
and it is never a value in this process. The guarantee is structural rather
than a matter of remembering to redact, and the record has no field whose width
it could move: a 20-byte response and a 20-KB one serialize to the same number
of bytes. `tool_input` has no field there either — the post handler derives no
shape, and the declaration it answers already carries the one derived at
`PreToolUse`.

## Terminal records

Every entry is followed by a terminal record on every exit path the program
controls: clean return, error return, and `SIGTERM` — which is what a hook
timeout cancellation delivers, and which is catchable.

`SIGKILL` is not catchable. That is the definition of the uncontrolled path: an
entry with no terminal record. It is not papered over. The run's coverage record
carries `state: "unverified"` and `reason: "unterminated_entry"`.

When the instrumentation itself fails, the coverage record carries `state` and
`reason` and no count field of any kind. A run that could not measure itself does
not get to report a number.

## Declarations, executions, and three lists that are not the same claim

`report` renders three lists about executions, and reading any of them as
another is the mistake this section exists to prevent.

`declarations.without_execution` names the declared `tool_use_id`s with no
execution record, each with the `permission_mode` it was declared in. It is not
a list of denials. It holds the denied, the failed, and the calls whose
`PostToolUse` invocation recorded nothing, together; the mode is carried so
that a consumer can exclude the modes in which nothing is ever denied, which is
as close to a verdict as this store can honestly get.

`executed_but_unrecorded` names the ids the transcript holds a `tool_result`
for and this store holds no execution record for. That is a coverage failure —
the tool finished and the recorder did not fire or did not land — and it adds
`execution_mismatch` to the run's coverage reasons.

`declared_without_result` names the declared ids the transcript holds no
`tool_result` for. That is not a coverage failure. It is a denial, a tool
error, or a transcript that has not caught up, and it is never to be rendered
as a count of denials.

Both transcript lists are `null` rather than empty when the transcript could
not be read, under the same rule as every count beside them: "no results" and
"could not look" are different facts.

## Destinations: what the wire saw, and what no transcript can tell you

Everything above is the session's own account of itself: what it declared, what
ran, what the recorder could vouch for. `--proxy-store PATH` adds the other
side. Altrace's proxy, in observe mode, writes a hash-chained row for every
CONNECT it admits or refuses; pointing `report` at that database joins the two
records and prints what the comparison shows.

The join is the point, and one line of it cannot be produced any other way:

    reached but never named: files.pythonhosted.org, pypi.org

`pip download requests` names no hostname anywhere in its command line. The
transcript cannot tell you where it went, the hook log cannot, and neither can
the agent — it does not know. The proxy saw both hosts. The report's other
lines are the symmetric cases: `declared but not observed`, for a host a call
named that never appeared on the wire (denied, cached, failed before
connecting, or simply not seen), and `executed differently from declared`, for
a call whose recorded execution does not match the shape it declared.

WHAT IT REFUSES TO CLAIM matters as much as what it shows.

- `client plane` is listed separately, not as a finding. Claude Code's own
  model traffic transits the same proxy, so `api.anthropic.com` appears with no
  declaring tool call in every single session — and an agent's own request to
  that host is indistinguishable from the client's. Reporting it as a finding
  would make the central line fire falsely every time, and a reader who saw it
  be wrong once would discount it when it was right.
- `proxy on path` has two reachable values and a third reserved. `unknown`
  is a real answer, and covers two cases: the store could not be read, or the
  store was readable and held no rows inside this session's window, so whether
  the proxy was on the path cannot be determined. Collapsing that into `false`
  would assert an absence that was never measured.
- `inherited` counts rows from outside the session's window and excludes them
  from every other number, so another session's traffic through the same proxy
  cannot enter this one's totals.
- Tool-family coverage is DERIVED FROM THE JOIN, never probed. Four statuses,
  because each says something different: `transit measured` (it declared hosts
  and at least one was observed — proof for this session, not a claim about the
  program in general), `declared hosts not observed` (it named hosts and none
  appeared, which does not distinguish "does not honour the variables" from
  "the calls did not run" from "the response was cached"), `exercised, no hosts
  declared` (`go build`, `git status` — nothing to join on), and `unknown` (the
  store could not be read). The last one is the row that matters most: without
  it every family would read as unobserved when the store was simply missing,
  which blames the families for the reader's own blindness.

### What this cannot observe, whatever the session did

Printed on every report, including a completely healthy one, because these are
properties of the instrument rather than of the session. A reader told only
what WAS observed will read the rest as an absence of traffic rather than an
absence of observation.

- Node's built-in `fetch`. Measured, not assumed: on v22.19.0 it made a request
  with no CONNECT reaching the proxy, with and without `NODE_USE_ENV_PROXY`.
- `ssh`, and git over ssh — not CONNECT, so outside what this proxy sees.
- DNS resolution: a name is resolved before any proxy is consulted.
- Raw sockets: no proxy variable applies.
- Plain HTTP: not observed in this release, since no `HTTP_PROXY` is exported.

The proxy's database is opened READ-ONLY and never written to. It belongs to a
different program, its records are hash-chained, and deleting a row would break
the chain it exists to provide — which is why `forget --host` suppresses a
destination from this report's view rather than claiming to have removed it
from there.

## The store

Directory `0700`, files `0600`, one JSON object per line, `schema_version` on
every record. The two modes are a Unix guarantee. Windows cannot express them —
`CreateDirectory` ignores the mode argument and `Chmod` toggles only the
read-only attribute — so there the store is protected by the ACL its parent
directory hands down, and by nothing this program set. Same store, weaker claim,
stated rather than quietly dropped.

`forget` and size-cap eviction each write a coverage gap record. Records leave
the store only with a marker saying they did.

[`docs/store-schema.json`](docs/store-schema.json) is that contract in a form a
machine can check: a JSON Schema (draft 2020-12) with one definition per record
type, every key required, nullable keys typed as such, closed vocabularies as
enums, and `additionalProperties: false` so a key nobody declared is a
validation failure rather than a surprise. It cannot drift from the tests:
`TestStoreSchemaMatchesTheAllowlists` holds each definition's property set equal
to the key allowlist H-13 enforces on the records themselves, and
`TestStoreSchemaReasonsAreTheCodeReasons` holds every reason code in the
implementation to appearing in an enum there.

## Coverage is decided at run time

The resolved-config state — our entry present, probe fresh — is written into the
run's own coverage record at start and at end. `report` reads that record. It
never re-reads today's configuration to judge a past run.

The alternative is a trap worth stating explicitly. If a run rendered unverified
because our entry is absent *at report time*, then `detach` would retroactively
invalidate every run ever captured, and the one action documented as safe would
destroy everything the user had collected.

`watch` installs a liveness probe alongside the recorder. Its limit belongs in
the documentation rather than in a footnote: the probe detects hook-system death.
It cannot detect a recorder that runs and drops records. H-10 below is what
catches that, and the probe should not be credited with more than it does.

VERIFIED COVERAGE IS SCOPED TO A PERSISTENT INSTALL. The probe resolves the
settings file Claude Code itself resolves, and asks whether OUR entry — our
matcher, our timeout, our install id — is in it. So coverage reads `verified`
only for a session whose recorder was installed in that file by `watch`. A
session started with `claude --settings <some other file>`, or under a
`CLAUDE_CONFIG_DIR` that differs from the one the probe resolves, records its
declarations perfectly well and still reports `hook_entry_absent`: the probe
honestly cannot confirm an entry that is not in the file it reads. That is the
right failure direction — it under-claims — but it means a temporary or
side-loaded install cannot produce a verified report, by construction.

## Constraints

- No content is ever written to the store: prompts, responses, argument
  values, command strings, tool
  outputs. Identifiers (`tool_use_id`, `session_id`, `prompt_id`, `agent_id`,
  `transcript_path`, `permission_mode`) are not content and are persisted
  verbatim; without them the store has no join key.
- Never render zero when we mean unknown.
- No model in any path.
- No account, no telemetry, no phone-home. `rashomon env` prints `127.0.0.1`
  only; `rashomon run` exports the listen address the proxy's own status file
  names, and does not today check that it is loopback.
- Installing the package writes nothing. Only an explicit `watch` touches the
  configuration.

## Acceptance

Two lists, and the split is deliberate.

Headless items (**H-n**) are automated, need no Claude Code process, and must be
shown to fail before they pass. An H-item that cannot be made to fail by breaking
the implementation is not a test, and reporting it as a spec defect is the
expected response rather than a courtesy.

Live items (**L-n**) need a real Claude Code session. There are three, they are
manual, and they are signed off with pasted terminal output.

### Headless

**H-1 — no fault reaches the agent as a 2.** Table-driven, exec'ing the compiled
binary: not a function under test, and not `go run`, which masks the child's exit
code as 1. The injected faults panic rather than return an error — nil-map write,
nil-pointer dereference, index out of range on a truncated payload, send on a
closed channel, and a panic inside a spawned goroutine. `ExitCode() == 0` for
every case. The suite also runs a fixture binary that panics without recovery and
asserts the harness reports 2; without that control, the test cannot distinguish
"robust" from "never observed an exit code".

**H-2 — the healthy path.** Valid payload, writable store: exit 0, exactly one
entry and one terminal record. Without this, H-1 is passed by a shell script that
does nothing but `exit 0`.

**H-3 — the installed entry, read back in full.** After `watch`: the entry is
under `hooks.PreToolUse` and not a sibling event, `type == "command"`,
`matcher == "*"`, `timeout == 5`, and the command path resolves to a file that
exists and is executable. The `PostToolUse` entry is read back the same way.
Installed from a copy of the binary in a directory whose name contains a space,
the command line still splits with that path as its first field and still
records a declaration when run through `sh -c`.

**H-4 — foreign hooks survive.** Seed the target with a foreign `PreToolUse`
entry under a *different* matcher, which is the arrangement that actually occurs.
After `watch` it is byte-identical and still present; after `detach`, likewise.
This catches an installer that assigns where it should append.

**H-5 — first run on a clean machine.** With `~/.claude/settings.json` absent,
and separately empty, and separately present without a `hooks` key: `watch`
succeeds and produces a valid config. A rule of "parse before writing, abort on
failure", read literally, means refusing to install on any machine that has never
run Claude Code.

**H-6 — idempotent install, one owner concept.** `watch` twice installs exactly
one entry of ours. `detach` removes our entry when no other install claims it,
and removes only its own claim otherwise. Both paths asserted. Both installs'
entries then fire against one store, and only ours records; the other's leaves
no run directory at all, and `watch` says it is there.

**H-7 — detach preserves concurrent edits.** Install, append an entry to
`permissions.allow` simulating an accepted "always allow", `detach`, then assert
both halves: the appended entry survives and our entry is gone. A `detach` that
aborts fails this test.

**H-8 — atomic write, with the window forced.** A deterministic fault injected
between write and rename, not a randomized kill against a sub-millisecond write.
After every iteration the config equals either the exact pre-state or the exact
fully-installed post-state and nothing else, and the loop is asserted to have
reached both — otherwise it never exercised the window. `fsync` is required and
is review-only: a process kill cannot verify it, because the page cache survives.

**H-9 — refuse when hooks are disabled, in both directions.** Managed
`disableAllHooks: true` over a project-level `false`: `watch` refuses. User-level
`true` over a project-level `false`: hooks do run, so `watch` must not refuse. The
second is the false-positive direction and is the one that matters.

**H-10 — the accounting equation.** Set equality, not per-record shape: the
recorded `tool_use_id` set equals the distinct-id set parsed from
`transcript_path`, unioned with `<session>/subagents/agent-*.jsonl`. An assertion
of the form "one entry per recorded `tool_use_id`" quantifies over the ids the
handler recorded, and is therefore passed by a handler that records one call in
fifty. Driven by a fixture transcript and a scripted sequence of payloads; no
live session required.

**H-11 — the uncontrolled path.** A `SIGKILL`ed handler leaves exactly one entry
with no terminal record, and the run's coverage record carries
`state: "unverified"`, `reason: "unterminated_entry"`.

**H-12 — instrumentation failure writes no count.** On the `SIGTERM` path and the
internal-error path, the coverage record's key set equals an explicit allowlist
containing `state` and `reason` and containing no count field of any kind. The
assertion is on the key set. Asserting that one named key is absent is vacuously
true whenever the code never produces that key, which is exactly when the
assertion is least useful.

**H-13 — content cannot be present, asserted by shape.** The record's key set
equals the allowlist, and the serialized record's byte length is identical for a
20-byte command and a 20-KB command. A canary search then runs over the store,
created files, stdout, and stderr as a second line of defence — but only as that:
absence of a literal proves nothing against an encoding. Hook stdout and stderr
reach Claude Code's debug log, so a traceback carrying the payload would write
command strings to disk.

**H-14 — shape fields degrade honestly.** `program`, `verb_class`, `argc`,
`digest`, and `schema_version` are present. A command that will not tokenize
records `argc: null`, never `0`. Two separate installs produce different digests
for the same command, and the key file is mode `0600`. The same command under
two different `description` values digests identically, and two `Write` calls
differing only in `content` digest differently.

**H-15 — the store contract.** Directory `0700`, files `0600`, one JSON object
per line, `schema_version` on every record. `forget` and size-cap eviction
each write a coverage gap record, asserted after each. Never a silent
deletion.

**H-16 — concurrency, with the window actually opened.** 64 concurrent handlers,
then the same run again with records padded past whatever buffer the
implementation uses (> 4 KiB). All 64 parse, ids distinct and complete. Then the
lock is deleted and the test is confirmed to go red: a concurrency test that
cannot be made to fail is not testing anything. One tension has to be resolved
here and stated in the implementation — returning 0 promptly under lock
contention pulls against landing all 64 records. The wait is bounded well under
the 5-second hook timeout, and a terminal record is written on give-up.

**H-17 — no phone-home.** A full capture cycle with outbound network denied
behaves identically, and the handler opens zero non-loopback sockets.

**H-18 — nothing installs silently.** Installing the package into a clean `HOME`
leaves `~/.claude/settings.json` absent or unchanged.

**H-19 — detach does not rewrite history.** `watch`, one tool call, `detach`,
`report`: the completed run still renders its true coverage.

**H-20 — the execution record.** One record per `PostToolUse` invocation,
carrying ids and the tool name and nothing else: its key set equals an explicit
allowlist, and its serialized width is identical for a 20-byte and a 20-KB
`tool_response`. A canary nested inside a 20-KB response reaches neither the
store, nor stdout, nor stderr, nor `report`. A transcript `tool_result` with no
execution record renders `execution_mismatch`; a declaration with no
`tool_result` renders nothing that reads as a denial. The five panicking faults
at both post-path injection points exit 0 with no traceback, and an entry
belonging to another install writes nothing at all.

### Live

**L-1 — real tools, real ids.** After `watch`, a session containing at least a
`Bash`, a `Write`, and an `mcp__*` call. For each of the three, a record exists
with the matching `tool_name`, and the recorded id set equals the
transcript-derived set. The count is deliberately not asserted: with
`matcher: "*"` the hook fires on every tool, so a real session produces many more
than three, and a test demanding exactly three would push the implementer toward
narrowing the matcher — the precise bug the matcher rule exists to prevent.

**L-2 — coexistence.** A foreign `PreToolUse` hook still fires during a watched
session; its sentinel appears alongside our record for the same `tool_use_id`.

**L-3 — mid-session removal.** Remove our entry from the configuration while a
session is running. The run renders coverage unverified, not an empty declaration
list.

## Handing back a result

Every H-item is reported with the command that ran it and that command's output.
Every L-item is reported with pasted terminal output and a note that it was
manual. An H-item that could not be made to fail by breaking the implementation
is reported as a spec defect.

## Building and testing

    go build ./cmd/rashomon
    go test ./...
    python3 test/mutation/sweep.py

The acceptance suite compiles the binary with `-tags rashomonfault` and execs it.
That tag adds the fault injection points H-1, H-8, H-11, H-12 and H-20 need,
and one kind, `fail=N`, that returns an error instead of panicking because it
exercises a retry a panic would never let run; a released binary carries no
injection path at all, because an environment variable that made the recorder
abandon a run would attack the one guarantee this program exists to provide.

`sweep.py` is the other half of the contract: it applies deliberate breaks and
confirms the corresponding tests go red. An item it reports as undetected is a
spec defect and should be treated as one.

Its coverage is not uniform, and the boundary is worth stating: the breaks
cover the items documented in this section, plus the settings and tokenizer
parsers. The later items — the destinations join, the launcher, the redaction
and baseline work — are covered by their own tests and by construction-truth
assertions, not yet by a mutation. The sweep now refuses to run at all unless
the suite is green first, and reports a mutation whose judging test SKIPPED as
"not judged" rather than as a pass.

Three parsers are also tested directly, beside the acceptance suite: the command
line tokenizer, the settings document parser's refusals and its byte-identity
round trip, and `shellQuote`. Each is an input-to-output function whose failures
reach the acceptance suite only as an install that quietly does nothing, and
`sweep.py` carries a break for each.

`test/live/l-items.sh` runs the three live items against a real `claude`
session on the machine it is run on. It writes to the real
`~/.claude/settings.json` — that is the test — and removes what it wrote.

## Status

Complete against the specification above. Every headless item is green and has
been shown to fail under a named break; the three live items were run against
real Claude Code sessions rather than by hand. The `PostToolUse` follow-on the
specification deferred is in: `watch` installs it with the same matcher and
timeout, each completed call leaves an execution record, and `report` renders
the three lists described above. Its headless item is **H-20**, which
continues the numbering rather than amending an item above.

`watch` installs FIVE entries in total: `PreToolUse`, `PostToolUse`,
`PostToolUseFailure`, `SessionStart` and `SessionEnd`. The failure event is
subscribed because without it a call that failed left an execution record
claiming it succeeded — the outcome was not unknown, it was wrong.

**Decisions the specification asked to have stated.**

- Goroutines: one, and it is not on the recorder's path. `safe.Go` had no
  production caller when this was written; `rashomon run` now uses it for the
  signal relay that forwards a `SIGTERM` sent to the wrapper on to the child,
  so a supervisor or a timeout cannot orphan the command you asked to run. It
  is cancelled and waited for before the call returns, so it cannot outlive the
  launch or leave a handler installed for the report. The hook, post and probe
  paths still start none: a test asserts `internal/hook` cannot reach
  `internal/launch`, so the recorder cannot acquire a goroutine — or the
  `os/exec` that package needs — by accident.
- H-16's tension: the append lock is waited on for at most two seconds against
  the five-second hook timeout. On give-up the terminal record goes to
  `spill.ndjson`, which is written without waiting, and carries the
  `tool_use_id` of the declaration that was dropped. A handler that has already
  spent the budget does not spend it again.
- H-16's "delete the lock": on Linux and macOS a single `write(2)` under
  `O_APPEND` is serialised by the kernel, so with one write per record the lock
  protected nothing and removing it could not make the test go red. Records now
  carry a per-run `seq` allocated inside the critical section, which gives the
  file a total order and gives the lock an invariant. The lock is on the data
  file's inode rather than a separate file, so the check is done by removing
  the acquisition, not by deleting a file.
- "At start and at end": `watch` installs `SessionStart` and `SessionEnd` hooks
  alongside the recorder. They are the liveness probe, and their coverage
  records are the start and end of the run, written by the run itself. The
  end-phase record is where an unterminated entry is recorded, at run time.
- Byte identity for foreign entries: the settings file is held as ordered raw
  members and only the `hooks → event → array` levels this program edits are
  re-encoded. Everything else, including every foreign matcher group, is
  written back as the bytes it was read as.
- `detach` and `watch` refuse to touch an entry of ours whose matcher, hook
  count, type or timeout has changed. The executable path is not protected:
  refreshing a moved binary is what `watch` is for.
- Fault injection is a build tag, not an environment variable, for the reason
  given under building and testing.
- Standing down: two installs' entries can sit in one `settings.json` — the
  arrangement H-6 models, and the one `detach` preserves by removing only its
  own claim — and Claude Code runs both with one environment. Both therefore
  resolve the same store, and every tool call would be recorded twice. An entry
  that does not belong to the store it resolves writes nothing at all rather
  than a record saying it declined: a run directory for a session this install
  never saw is itself a false trace. The stderr line is a constant, because
  hook stderr is written to Claude Code's debug log.
- The post phase's coverage record resolves the `PostToolUse` entry, not the
  recorder's. A configuration carrying one and not the other is exactly the
  arrangement in which reading the wrong one lies.
- An execution that cannot take the append lock goes to `spill.ndjson` with a
  null `seq`, as a terminal does. It is the only record its `tool_use_id` gets,
  and dropping one would read afterwards as a call that never ran.

**Where the specification was found wanting.**

- H-13's width assertion is not achievable as written. `argc` is a count that
  legitimately varies with the command, and its decimal width changes the
  record's width, so a 20 KB command with spaces cannot serialise to the same
  length as a 20-byte one. The invariant that holds — and is tested — is that
  width does not move when argument *bytes* grow with the shape held constant.
- §0's inventory of exit-2 causes is incomplete. Runtime fatal errors also exit
  2 and `recover()` cannot catch them: concurrent map access, stack exhaustion,
  out of memory, all-goroutine deadlock. An unbounded read of `tool_input` is
  therefore a path to blocking the user's tool call with no panic involved.
  stdin is capped at 8 MiB for that reason, and the barrier against this class
  is structural, not `recover`.
- H-16's red-check, as above, does not work against an implementation that
  writes each record in one call.
- H-14's digest stability is not achievable over the whole `tool_input`. Claude
  Code attaches a free-text `description` to a `Bash` call, so the same command
  arrives with different bytes on every call and the digest groups nothing. A
  shell tool digests its command line alone; the fixture that made the
  whole-input digest look stable carried no description.

**What the live runs showed.** `claude -p` fires `SessionStart`, `PreToolUse`
and `SessionEnd` hooks. Claude Code applies a change to `~/.claude/settings.json`
inside a running session in both directions: the recorder started firing in the
session that wrote the file, and stopped firing in a session it was detached
from, which is what L-3 relies on. A nested `claude -p` inherits the parent
session's id from the environment unless `--session-id` is given. It also
writes its own transcript, so one run directory holds declarations against more
than one `transcript_path`. H-10's equation is therefore computed per transcript
and rendered as `transcripts`, an array sorted by path: checked against a single
transcript, the same records read as a mismatch in both directions and a run
that had captured every call rendered unverified for nothing. Declarations
naming no transcript join no group and are counted in
`declarations.without_transcript`.

**Windows.** The append lock is `LockFileEx` with
`LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY` over the maximum byte
range, polled on the same budget and backoff as the `flock` path — a lock
covers a range rather than a file, and a range fixed at acquisition would leave
every appended byte outside it. Record files are opened `O_RDWR` rather than
`O_WRONLY` for one Windows reason: an `O_APPEND` handle is granted
`FILE_APPEND_DATA` and neither `FILE_READ_DATA` nor `FILE_WRITE_DATA`, and
`LockFileEx` refuses a handle holding neither, so every append would have
failed to lock. Four differences from Unix survive and none is hidden: `0700`
and `0600` are inexpressible, as above; byte-range locks are mandatory rather
than advisory, so while a handler holds the append lock a reader of that range
is refused rather than served a partial line, and `report` can fail where on
Unix it would read; a file another process has open cannot be deleted, so
size-cap eviction can fail against a live run, and because the gap record is
written before the removal a failed removal leaves a gap naming records that
are still there, returned as an error rather than swallowed; and `SIGTERM` is
never delivered, so a hook timeout there is the uncontrolled path — no terminal
record, and the run reads `unverified` with `unterminated_entry`. None of this
was run on Windows. The module builds and vets under `GOOS=windows` for amd64,
386 and arm64, and `internal/store/lock_test.go` states the lock contract the
Windows code has to meet, with three breaks in `sweep.py`. Two headless items
would not pass there as written: H-15 asserts the two modes, and H-17's halves
skip themselves without `unshare` and `strace`.

**Known limits.** The probe detects hook-system death and nothing subtler;
H-10 is what catches a recorder that runs and drops records. `forget` rewriting
`spill.ndjson` can race a spill write that waited out its fifty-millisecond
patience; the loss is one terminal, which reads as unverified and never as
verified. `watch` refuses to install a `go run` binary, and the command line it
installs is quoted for a POSIX shell, which is a second thing to fix before a
Windows install is usable. Locking is implemented for Unix and Windows; on any
other platform the store refuses to open rather than corrupt itself. A failed
settings read is retried three times a few milliseconds apart before
`hook_entry` resolves to `unknown`, so another tool's non-atomic rewrite of
`settings.json` no longer taints a run, and a rewrite that outlasts the retry
still reads as unverified rather than as present. A foreign install's entry
firing where this environment's store does not yet exist still creates the
empty store to learn that the id does not match; an `--install` given with no
store present cannot be ours, and standing down before opening would avoid even
that trace.

## License

Apache 2.0. See [LICENSE](LICENSE).
