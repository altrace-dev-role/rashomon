# altrace-attest

`attest` records what a Claude Code session *asked* to run, and the evidence that
the recorder was running while it did. It captures identifiers and the derived
shape of each tool call. It never captures content.

A declaration is not an execution. Claude Code's `PreToolUse` hook fires before
the permission flow resolves, so the store contains records for calls the user
went on to deny. That is deliberate: a declaration is the agent's stated intent,
and intent is worth keeping whether or not it was granted. Every record carries
`permission_mode` so that a later comparison can exclude what was never allowed
to run. Closing the loop properly — matching declarations against what actually
executed — needs `PostToolUse` and a comparison step. Neither is in scope here.

The store exists to be joined against. Every record carries `tool_use_id`,
`session_id`, `prompt_id`, and, inside a subagent call, `agent_id`. Without those
keys the capture is a pile of anonymous shapes and nothing downstream can
integrate it.

Claude Code only. No other agent harness is in scope.

## Commands

- `attest watch` — install the hook entry and the liveness probe. The only
  command that writes to your configuration.
- `attest detach` — remove our entry, leaving everything else in the file as it
  was found.
- `attest report` — render a run's declarations and its coverage state.
- `attest forget --since <when>` — evict records, leaving a coverage gap marker
  behind.

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

## What is captured

The `PreToolUse` payload carries twelve fields: `session_id`, `prompt_id`
(absent until first input), `transcript_path`, `cwd`, `scratchpad_dir`,
`permission_mode`, `hook_event_name`, `tool_name`, `tool_input`, `tool_use_id`,
and — inside a subagent call only — `agent_id` and `agent_type`.

Persisted: `tool_use_id`, `session_id`, `prompt_id`, `agent_id`,
`transcript_path`, `permission_mode`, `tool_name`, and the derived shape of
`tool_input`.

Nothing from inside `tool_input` is persisted.

The derived shape is `program`, `verb_class`, `argc`, `digest`, and
`schema_version`. A command that will not tokenize records `argc: null`, never
`0` — zero is a count, and in that case we do not have one. `digest` is an HMAC
under a per-install random key, so the same command digests differently on two
installs and the store cannot be run as a dictionary attack against known command
strings. The key file is mode `0600`.

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

## The store

Directory `0700`, files `0600`, one JSON object per line, `schema_version` on
every record.

`forget --since` and size-cap eviction each write a coverage gap record. Records
leave the store only with a marker saying they did.

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

## Constraints

- No content, ever: prompts, responses, argument values, command strings, tool
  outputs. Identifiers (`tool_use_id`, `session_id`, `prompt_id`, `agent_id`,
  `transcript_path`, `permission_mode`) are not content and are persisted
  verbatim; without them the store has no join key.
- Never render zero when we mean unknown.
- No model in any path.
- `127.0.0.1` only. No account, no telemetry, no phone-home.
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
exists and is executable.

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
and removes only its own claim otherwise. Both paths asserted.

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
for the same command, and the key file is mode `0600`.

**H-15 — the store contract.** Directory `0700`, files `0600`, one JSON object
per line, `schema_version` on every record. `forget --since` and size-cap
eviction each write a coverage gap record, asserted after each. Never a silent
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

    go build ./cmd/attest
    go test ./...

The acceptance suite compiles the binary with `-tags attestfault` and execs it.
That tag adds the fault injection points H-1 needs; a released binary carries no
injection path at all, because an environment variable that made the recorder
abandon a run would attack the one guarantee this program exists to provide.

## Status

Piece two, capture and delivery accounting, is implemented. Piece one, install
and restore, is not: `watch`, `detach`, `report` and `forget` are declared and
refuse rather than half-work.

Green, and each shown to fail when the implementation is broken rather than
merely observed to pass:

- **H-1**, including the control fixture and a guard against the suite going
  vacuous if the binary is built without fault injection.
- **H-2**, plus the absent-versus-empty distinction and the subagent join keys.
- **H-12**, internal-error path only.
- **H-13**, all three: closed key sets asserted over nested paths, width
  independent of input size, and the canary sweep behind them.
- **H-14**, including basename-only programs and per-install digests.
- **H-15**, permissions, framing and schema version. Also session ids that
  cannot escape the store, which the acceptance list does not name and should.
- **H-17**, structurally: the binary's transitive imports contain no `net`,
  `net/http`, `crypto/tls` or `os/exec`, so there is no code path to a socket.

Not yet covered: **H-3** through **H-9** and **H-19**, which are the install and
restore surface; **H-10**, which needs transcript parsing; **H-11** and the
SIGTERM half of **H-12**; **H-16**; the gap records **H-15** requires of `forget`
and size-cap eviction; and all three live items.

## License

Apache 2.0. See [LICENSE](LICENSE).
