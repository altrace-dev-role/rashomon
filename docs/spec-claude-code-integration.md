# Claude Code integration: plugin install, recording state, and the post-turn digest

Status: proposed, revision 3. Sign-off is per part, and Part 5 additionally
depends on a constraints amendment this document asks for by name. Parts 1
through 4 are one release and are useful without Part 5.

Revision history. Revision 3 corrects one of revision 2's own claims, adds
H-100 for the read-path race, and requires the summary text to arrive on
stdin and never be echoed. Revision 2 takes an architecture, security and test review
of revision 1, which found six things wrong with it. The digest grouped by
`(transcript, prompt)` while claiming to include subagent calls, and those
contradict -- subagent declarations carry their own transcript path, so the
paired key drops a median 51% of the calls in the 22% of sessions that use
them, silently. `SilentFailures` reads the last message of the whole
transcript, so a turn-scoped digest would have judged one turn's failures
against another's text, and raced the flush at `Stop` besides. The read path
takes no lock, which never mattered for `report` and does for a digest running
at `Stop`. `WithoutExecution` and `ReasonUnterminatedEntry` are the same
"not yet is not missing" mistake as `run_not_closed`, which revision 1 caught
once and then missed twice. `hook_entry` was going to collapse plugin presence
into the existing boolean, which would have destroyed what `verified` means.
And `pause` wrote a coverage record but no gap record, leaving the paused span
on-disk identical to one that was never recorded. Revision 2 also adds the
injection requirements to Part 5 and a sanitisation requirement to Part 4, and
drops PATH resolution from the hook path.

Two claims were checked and rejected. A review said `README.md` already
states a side-loaded install "cannot produce a verified report, by
construction"; no such text exists on the merge target, so H-71 is not a
reversal of a published property. And revision 2 itself asserted that a
backgrounded shell in flight at `Stop` shows up as `ReasonUnterminatedEntry`;
it does not -- `read.go:184` and `post.go:201-203` make `Unterminated` a fact
about the hook process, not the tool, and the in-flight case is
`WithoutExecution` alone. The implementer caught that one, which is the right
direction for a correction to travel.

Scope of change. A new `plugin/` tree; `internal/install` and
`internal/settings` for ownership; `cmd/rashomon` for two new commands and a
sixth hook entry; a new `internal/digest`; `internal/report` for the shared
projection and the renderer's vocabulary test. No new dependency, no new
witness, no other harness. Nothing is added to what the recorder writes: every
number this document renders is already in the store.

Numbering. H-70 is the highest item on the merge target, so items here are
provisional from H-71 and are grepped against the merge target
(`H-\(7[1-9]\|[89][0-9]\|100\)`) before they become the contract.

## Why

Three failures, each found by using the tool rather than by reading it.

**You must install before the session you want recorded.** `watch` edits
`~/.claude/settings.json`, and Claude Code reads that file at startup. A user
who installs after starting gets a report whose every line reads `unverified`.
That is the most likely first run and the worst one, and no amount of report
quality fixes it, because the records do not exist.

**There is no way to stop recording short of removing the entries.** `detach`
is uninstall. There is no pause, and nothing on screen says whether the
recorder is on. `status` answers it, but `status` is a command, and the whole
of the third failure is that nobody runs commands.

**Nobody reads the report.** It renders well now. It is still a thing you must
remember to run, in a terminal, after the moment you cared about. The five
skills in `skills/` were the answer to that and they are install-by-copy
assets, which is the first failure wearing different clothes: a command nobody
copies is a command nobody has.

The three share one cause. rashomon is a CLI that Claude Code happens to
invoke, when it could be something Claude Code carries.

## What exists, as it is

`watch` renders five hook entries into `~/.claude/settings.json`:
`PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `SessionStart`,
`SessionEnd` (`internal/install/install.go:47-53`). Each command is
`<exe> <subcommand> --install <32 hex>` (`install.go:123-125`), with
`matcher: "*"` on the three tool events only and `timeout: 5`
(`install.go:69-71`).

The install id is the spine of everything. `Present` decides whether an entry
is ours, `Owner` reads the id back out of a command line, `intact` refuses to
touch an entry a user has edited, and `standsDown` makes a foreign id record
nothing (`install.go:172`, `:321-332`, `:366-394`, `main.go:226`). Two
installs can share one settings file because of it.

Coverage is resolved per invocation by reading that same file.
`internal/hook/coverage.go:30-44` calls `install.Present(doc, st.InstallID(),
event)` and resolves `present`, `absent`, or -- on a lasting read failure --
`unknown`. The comment is explicit that a narrowed entry is `absent`, "because
that is what it is: a config under which most tool calls produce no record."

`report.Session` (`internal/report/report.go:175`) already carries every
signal a post-turn summary needs: `Coverage`, `Declarations` (including
`WithoutExecution`, `ByTool`, `ByLabel`), `SilentFailures`, `Subagents`,
`Destinations`. `ByProgram` and `ByVerbClass` are not on the merge target --
they arrive with #20, and Part 4 uses them only once it has merged. It is reachable only through `report.Build`,
which reads the proxy database, globs subagent transcripts, and writes
`baseline/<project>.json` (`destinations.go:632`). `report` is therefore not a
read-only command today.

Declarations already carry `prompt_id`, and `chains.go:169` already groups by
`(transcript, prompt)`. Turn-scoped grouping is existing machinery.

## Measurements taken

Taken on a real store on macOS, 3 runs, 560 KiB, largest session 242
declarations.

1. **A session's report JSON is 63,439 B, and 93% of it is one field.**
   `transcripts` alone is 58,899 B. The fields a post-turn summary needs --
   `coverage`, `declarations`, `silent_failures`, `subagents`, `destinations`,
   `account` -- total **3,436 B**, about 860 tokens. Size is not an argument
   against a post-turn summary, and the earlier claim in planning that a
   report costs 41,000 tokens was an artefact of that one field.
2. **About 1,811 bytes per recorded call.** A 500-call session costs roughly
   906 KB. The 512 MiB cap is reached at about 298,000 calls. Command length
   is irrelevant: a 5,000-character command costs 4 bytes more than `ls`.
3. **One `PreToolUse` invocation costs 42 ms end to end, and process start is
   58% of it.** Measured over 30 sequential invocations against a fresh store,
   shell overhead subtracted: `rashomon hook` 42 ms, `rashomon version`
   (process start and exit, no store work) 24.3 ms. rashomon's own work --
   read, derive, append, fsync, resolve coverage, append, fsync -- is therefore
   about 18 ms, and each invocation writes two records plus one coverage
   record.

   This bounds what optimisation can achieve: most of the cost is a 10.9 MB Go
   binary starting, and the SQLite driver is linked into it whether or not a
   hook path uses it. It also settles Part 2's cost question -- a `stat` before
   the store opens is a rounding error against 24 ms of startup -- and it says
   the 50 ms budget is already 84% spent, so nothing further belongs on the
   per-call path. Part 4's entry is on `Stop`, once per turn, and not on it.

Owed before Part 3 is approved, each with a budget:

4. The digest's wall time and byte size on the largest session available,
   budget 50 ms and the ceiling Part 3 sets.
5. Whether a `/config` plugin-option change reaches a running session's next
   hook invocation, or needs `/reload-plugins`. This is undocumented. Part 2
   no longer depends on it, having chosen a state file over an environment
   variable, but Part 1 wants it for the `/config` surface.

## Constraints: an amendment this document asks for

`README.md` publishes five constraints. This work contradicts three of them,
and the honest course is to say so here rather than let a diff discover it.

```
Current:
  - No model in any path.
  - No account, no telemetry, no phone-home.
  - Installing the package writes nothing. Only an explicit `watch` touches
    the configuration.

Proposed:
  - No model on the recording path. The recap may call a model, and only
    when the user has turned that on.
  - No telemetry. Nothing leaves the machine except a recap the user
    enabled, and the README says what it contains.
  - Installation does not start recording without enablement.
```

The third changes whatever else is decided, because enabling a plugin writes
`enabledPlugins` to the settings file. `defaultEnabled: false` keeps the
promise that matters -- nothing records until a person says so -- but it does
not keep the literal words, and it does not fully guarantee them either: an
existing `enabledPlugins` entry at any scope, or a dependency requirement,
overrides the manifest default.

The first two are a repositioning of what rashomon is, not a feature flag.
**Parts 1 through 4 need only the third.** Part 5 needs the first two, and is
not built until they are signed off.

## Non-goals

- No new witness. Nothing here observes what the agent's processes do. The
  evidence remains what the hooks record and what the proxy saw.
- **No new content in the store.** Every number rendered here already exists
  in a record. The digest is a projection, not a collection.
- No verdict from rashomon. A count is a fact; "on task" is not, and rashomon
  cannot reach it -- the store keeps a `prompt_id` and never prompt text
  (`store/record.go:73`).
- No enforcement from the five recorder entries. They record; they never
  block; exit code 2 stays impossible for them. Part 5 asks a separate
  question about its own entry and answers it there.
- **Silence is not a claim.** An exception-only notification is a policy about
  when to speak, never evidence that there was nothing to say.
- No defence against the user. A user-level recorder cannot stop the person
  running it from turning it off, and pretending otherwise would be the kind
  of unearned assurance this tool exists to avoid. What it must do instead is
  leave the change visible in the record.
- Claude Code only.

## Part 1: the plugin, and ownership without a shared secret

### The problem underneath

A plugin ships hooks in `hooks/hooks.json`, and they are active whenever the
plugin is enabled. The manifest is one artefact shipped to every user, so it
cannot carry a per-machine install id -- and the install id is what `Present`,
`Owner`, `intact` and `standsDown` are built on.

The consequence is not cosmetic. With a plugin-only install, `Resolve` reads
`~/.claude/settings.json`, finds no entry of ours, and records `hook_entry:
absent` on every coverage record. Every session reads unverified. Part 1 would
cause the exact failure Part 1 exists to remove.

### Resolution: ownership becomes a question about origin, not about a secret

`hook_entry` is answering "is a recorder of ours configured to run here". The
install id was a way to answer it, not the meaning of it. So the resolution
adds a second place to look and a second way to be ours:

- **A settings entry is ours if it carries our install id.** Unchanged.
- **A plugin entry is ours if an enabled plugin named `rashomon` declares a
  hook whose command resolves to our binary.** There is no secret because
  there is no ambiguity to resolve: the plugin directory is the evidence.

`install.Present` grows an origin-aware sibling rather than being widened in
place, because the existing function has a precise meaning that `detach` and
`intact` depend on and must keep. Coverage calls the new one; `detach` and
`intact` keep calling the old one, since a plugin's entries are not theirs to
remove.

**The record names the origin; it does not collapse to a boolean.**
`hook_entry` gains `present_settings` and `present_plugin` beside the existing
`absent` and `unknown`. This is the difference between a defensible fix and a
hollow one: `entryEvent` (`coverage.go:84-89`) deliberately checks the
*PostToolUse* entry during the post phase so a run cannot claim its executions
were covered by an entry that records none, and any fix shaped "I am running,
therefore I am installed" destroys the only thing `verified` means. Two
sources of truth are fine when the record says which one answered, and fatal
when they merge. `status` becomes an origin table for the same reason.

**Owed, and it bounds the whole part:** can a hook confirm the plugin is
*enabled for this session*, or only that it is present on disk? If only
present, plugin origin tops out at `unknown` rather than `present_plugin`, and
H-71 becomes unreachable as written. The answer goes in the code and in
Measurements before Part 1 is approved.

`standsDown` is unchanged in shape. A plugin invocation names no `--install`,
which already records normally (`main.go:226-233`).

### Duplicate prevention

A user with both a plugin and a `watch` install records every call twice, and
`standsDown` does not help, because it stands down on an id *mismatch* and the
plugin entry carries no id. Dedup on `(session_id, tool_use_id)` is wrong:
`PostToolUse` and `PostToolUseFailure` legitimately share that identity, and
`chains.go:215-217` exists because a single id can carry two execution records.

Resolution: **`watch` refuses when an enabled rashomon plugin already provides
the entries**, and says so with the command that removes the other one. The
refusal is the same shape as the existing `disableAllHooks` refusal
(`main.go:418`), which is the precedent for "installing here would produce
something that does not work".

`status` reports both origins separately, so a user can see which one is live.

### Migration

An existing `watch` user who enables the plugin hits that refusal only on the
next `watch`; their settings entries keep working in the meantime. `status`
names the overlap and prints `rashomon detach` as the resolution. Nothing is
removed automatically: this tool does not edit a user's settings file except
when asked.

### What the plugin carries

`.claude-plugin/plugin.json` with `defaultEnabled: false`; `hooks/hooks.json`
with the five recorder entries plus Part 4's; `bin/` with the binary; and
`skills/` with the five that exist today, renamed so they namespace well --
`/rashomon:report`, `:status`, `:stop`, `:forget`, and `:watch` for the one
currently called `rashomon`. Their cross-references are updated with them.

Hook commands use exec form (`args`), which takes no shell and passes each
element verbatim, and they name the **plugin-local binary absolutely** via
`${CLAUDE_PLUGIN_ROOT}`. They do not resolve `rashomon` on `PATH`: a
PATH-resolved shim invoked on every tool call is a hijack primitive on the
hottest path in the product. `status` prints the resolved absolute path so a
user can see which binary is running. The ported skills lose their
three-path preamble for the same reason. That sidesteps the Windows defect in `shellQuote` for the
plugin path; it does not fix `shellQuote`, which `watch` still uses, and that
bug stays open.

`bin/` costs one distribution channel: a plugin carrying it cannot be
distributed through claude.ai organization settings. Marketplace and git
distribution are unaffected, and Part 1 names the git URL as the channel.

### Acceptance

**H-71 -- a plugin-only install reads verified.** Enable the plugin with no
settings entry present, run a session, render the report. Coverage is
`verified`. Break: revert coverage to the settings-only `Present` and H-71
fails with `hook_entry: absent` on every record -- the failure this part
exists to prevent.

**H-72 -- `detach` does not touch plugin entries.** With both origins present,
`detach` removes the settings entries and leaves the plugin's. Break: point
`detach` at the origin-aware lookup and it reports success having removed
nothing it owns.

**H-73 -- `watch` refuses under a live plugin.** With the plugin enabled,
`watch` exits non-zero, names the plugin, and writes nothing. Break: drop the
refusal and the same session records every call twice, which H-74 then
catches.

**H-74 -- one call, one declaration.** With both origins installed and the
refusal bypassed in a fixture, exactly one declaration per tool call. Break:
remove the guard and the count doubles.

**H-75 -- `status` names both origins.** Break: collapse them into one line
and a user with a stale settings entry cannot tell which is recording.

**H-76 -- the skills resolve the plugin binary first.** Break: leave the
three-path preamble and a machine with an old `~/go/bin/rashomon` runs the
wrong binary from the plugin's own command.

**H-77 -- the manifest ships disabled.** `defaultEnabled` is `false` and a
fresh install records nothing until enabled. Break: set it `true` and
installation starts recording, contradicting the amended third constraint.

### Effort

Days. The ownership change is the work; the manifest and the skills are hours.

## Part 2: recording state

### Threat model first

The question "can recording be turned off" has a different answer per actor,
and the mechanism follows from that rather than from an intuition about files
versus environment variables.

| Actor | Can change state? | Should they? |
| --- | --- | --- |
| The person at the keyboard | Yes, every way | Yes -- it is their machine |
| The audited agent, via `Bash` | Yes: it can write files, edit settings, and set variables in its own children | No, but nothing at this layer can stop it |
| Another process as the same user | Yes | No, same |
| Anything not running as this user | No | -- |

A file is therefore *not* inherently safer than an environment variable, and
the planning draft was wrong to argue it was. `internal/fault/fault.go:5-8`
says an environment variable must not be able to make the recorder abandon a
run, and that reasoning holds for a *silent* abandonment -- which is the real
property. The rule this part adopts:

> Recording state may be changed by anyone who can run as this user. Every
> change must leave evidence in the store that a reader of the report sees.

That converts an unwinnable access-control problem into a recordkeeping one,
which is the kind of problem this tool is already good at.

### Mechanism

`rashomon pause` and `rashomon resume`, plus state in `status`.

State lives in a file under the store root, checked **before** the store is
opened -- `store.Open` creates the store, and a paused machine that has never
recorded must not acquire an install identity by being asked whether it is
paused. The precedent is `storeInstalled()` (`main.go:1290`), which stats the
marker for exactly this reason (the comment above it).

The check goes at the top of `cmdHook`, `cmdPost` and `cmdProbe`, after
`WatchSignals()` and before `openForHook`.

An environment variable is **not** the mechanism, for a reason narrower than
the planning draft's: a `CLAUDE_PLUGIN_OPTION_*` value is per-process and
invisible afterwards, so a session recorded under it leaves no evidence of why
it stopped. The `/config` row remains valuable as a *surface* and Part 2 may
mirror it, but the file is the state.

### Pausing leaves a record, except where it cannot

A paused session writes a coverage record carrying a new reason,
`recording_paused`, so a reader sees a deliberate gap rather than an
unexplained one.

**A coverage record alone is not enough, and revision 1 stopped a step
short.** Between `pause` and `resume` the hooks do not run at all, so nothing
writes anything and the paused span is on-disk identical to a span where the
recorder was never installed -- zero rendering as clean, the failure this
codebase refuses everywhere else. So `pause` and `resume` each write a **gap
record** from the command itself, using the machinery in
`internal/store/gaps.go` that already carries a reason and a from/to window
and is already how eviction leaves its trace. The report renders that span as
a named unknown. The two records answer different questions: the coverage
record says "this invocation stood down", the gap record says "this window has
no records, and here is why".

This conflicts with checking before the store is opened, and the conflict is
resolved rather than hidden: **where a store already exists, the paused record
is written; where none exists, the machine stays silent and records nothing,
because creating a store to say "not recording" is worse than saying nothing.**
`status` covers that case -- it reports paused state without a store.

### Reaching plugin-owned hooks

`pause` is a state file the hook reads, so it reaches every origin, including
plugin entries a settings edit could never remove. This is the reason the state
file beats removing entries, and it is what stops Part 1 from making the second
failure worse.

`/plugin disable` remains available and is coarser: it takes the skills and
`bin/` with it. `status` reports that case as absent rather than paused,
because it is.

### Acceptance

**H-78 -- pause takes effect on the next call, in the same session.** Break:
cache the state for the process lifetime and a pause mid-session does nothing
until restart, which is the behaviour users will assume is a bug.

**H-79 -- pause reaches plugin-owned entries.** Break: implement pause as a
settings edit and a plugin-only install cannot be paused at all.

**H-80 -- a paused session with a store writes `recording_paused`.** Break:
return early before the coverage write and the gap renders identically to a
crash.

**H-81 -- a paused machine with no store creates nothing.** Assert no store
root after a full paused session. Break: move the check after `openForHook`
and every paused tool call mints key material.

**H-82 -- `status` distinguishes paused, absent and unknown.** Break: collapse
paused into absent and the user cannot tell "I turned it off" from "it was
never installed".

### Effort

Days, most of it in the schema change and its test.

## Part 3: the turn digest

`rashomon digest [--session S] [--prompt P]`, a small JSON projection of what
one turn recorded.

### Scope, and why it is a turn

A `Stop` hook fires per turn. A session-scoped digest rendered at `Stop` would
raise a finding from an early turn against an unrelated later one. The digest
is therefore scoped to one `prompt_id`, reusing the `(transcript, prompt)`
grouping at `chains.go:169`.

**Grouped by `prompt_id` alone, not by `(transcript, prompt)`.** Revision 1
said both, and they contradict: a subagent's declarations carry their **own**
`TranscriptPath` (`internal/store/record.go:76`), so the paired key puts them
in a different bucket and drops them silently. Measured in `account.go:63-68`:
22% of sessions use subagents, and in those they make a median 51% of all
calls -- so the paired key would render a confident half-count as a clean
number. Subagent calls are attributed to the parent turn, because a subagent's
`PreToolUse` carries the parent's `prompt_id`
(`spec-chain-and-scope.md:129`), and the digest names the subagent count
separately so a reader can see the composition.

### Reading, and only reading, and not echoing

Assembled from `st.ReadRun` (`read.go:83`) -- three NDJSON files in one
directory -- plus `buildSilentFailures` (`account.go:235`) and the counting
and coverage loops at `report.go:425-435` and `:447-470`, which are extracted
so both callers share one implementation rather than drifting.

**The digest does not open the transcript.** `buildAccount` (`account.go:144`)
reads the last assistant message of the *whole* transcript, which is
cross-turn by construction: scope the failures to a turn and they are judged
against a later turn's text. At `Stop` it is also racy, because the final
message may not be flushed, which would report `FinalMessageAvailable: false`
on a healthy turn. The summary text comes from the `Stop` payload's
`last_assistant_message` instead, passed in **on stdin, not in argv** -- that
text runs to kilobytes and argv is readable by any process that can run `ps`.

The digest **never echoes it back**. It is attacker-influenceable: a poisoned
repository or page reaches the transcript and the summary quotes it. Absent
words, counts and booleans derived from the message may be rendered; the
message may not. `buildSilentFailures`' counting half is pure record work and
is already turn-safe.

**The read path takes no lock, and this is new exposure.** `eachLine`
(`read.go:161`) scans without one; writers take `lockFile`
(`store.go:250`, `:295`). `report` ran after the fact and never raced. The
digest runs at `Stop`, concurrent with in-flight `PostToolUse` writes from
backgrounded shells and subagents, so it can read a torn tail line and bump
`Skipped`. A torn tail must surface as an explicit unknown, never as a
smaller clean count.

It opens the store with `OpenExisting`, never `Open`. It does not read the
proxy database, does not glob subagent transcripts, and **does not write
`baseline/`** -- which `report` does today, and which is why `report` could not
simply be reused.

### Coverage at Stop is not coverage at SessionEnd

`report.go:467` adds `run_not_closed` whenever `EndRecorded` is false. At
`Stop` the session is still open, so projecting session coverage would mark
every healthy turn as missing coverage.

The digest therefore carries **turn coverage**, whose clean state is reachable
mid-session: the probe fired at start, the entry was present for the calls in
this turn, and no gap intersects it. `run_not_closed` is not among its reasons.
Session coverage is unchanged and stays where it is.

The in-flight case is `WithoutExecution` (`report.go:409-411`): a call still
running at `Stop` is "declared, not yet executed", and rendering it as missing
would mark a healthy turn unverified.

`ReasonUnterminatedEntry` is **not** the same thing, and revision 2 said it
was. `read.go:184` defines `Unterminated` as "the signature of a handler that
was killed between the two" -- the hook *process*, not the tool -- and
`post.go:201-203` is explicit that "an execution record is not an entry that
something later closes, it is the close". A backgrounded shell still running
therefore surfaces as `WithoutExecution`, not as `Unterminated`. The gate is
applied to both regardless, for the same reason `run_not_closed` is
suppressed: while the run is open, neither drives a coverage *verdict*. Both
lists stay visible; only the trigger is gated, and both behave exactly as
`report` does today once `EndRecorded` is true.

### Never zero for unknown

A turn with no recorded declarations is **not** rendered as zero calls. It is
`unknown`, with a reason, because "the agent used no tools" and "nothing was
recorded" are different facts with the same shape on disk. The digest
distinguishes them using turn coverage: recorded-and-empty is zero,
not-recorded is unknown.

The same rule covers paused recording, eviction, a truncated digest, and an
unreadable final message. Each yields a named unknown; none yields silence and
none yields a pass.

### Size

One stated ceiling of **8 KiB** for the whole document, not a per-field cap.
`WithoutExecution`, `Unterminated` and `Dropped` grow with the turn;
`ByTool` and `ByLabel` grow with distinct values and are not fixed either
(and `ByProgram` too, once #20 lands).
Truncation applies to all five in a fixed order, and every truncated field
carries its omitted count. A truncated digest sets a flag that Part 4 renders,
because a summary computed from a partial record must say so.

### Turns that do not end in Stop

`Stop` does not fire on user interrupt, and an API error routes to
`StopFailure`. Both are turns worth a digest, so Part 4 subscribes to
`StopFailure` as well, and the digest carries how the turn ended. A turn ended
by interrupt produces no notification and is visible in `status`.

### Acceptance

**H-83 -- the digest is turn-scoped.** Two turns, a failure in the first only;
the second digest is empty of it. Break: scope to the session and the second
turn inherits the first's finding.

**H-84 -- an open run is not a coverage gap.** Break: project session coverage
and `run_not_closed` appears on every healthy turn.

**H-85 -- zero is not unknown.** A turn with tools recorded and none used
renders zero; a turn with no records renders unknown. Break: collapse them and
a session with a dead recorder reads as a session with a well-behaved agent.

**H-86 -- the digest writes nothing.** Snapshot the store and `baseline/`
before and after. Break: build it on `report.Build` and `baseline/` changes.

**H-87 -- the digest opens no store.** Run against a machine with none;
assert none is created. Break: use `Open` and asking for a digest installs an
identity.

**H-88 -- the ceiling holds with a correct omitted count.** Break: cap the
fields independently and a turn wide in five dimensions exceeds the total
while every field is within its own cap.

**H-89 -- subagent calls land in the parent turn and are counted separately.**
Break: key on `(transcript, prompt)` instead of `prompt_id` alone, and the
digest silently undercounts by the subagent's share.

**H-100 -- a concurrent writer does not produce a smaller clean count.** Run
the digest against a run directory while a writer appends, and a torn tail
must surface as an explicit unknown. Break: drop the lock and let `Skipped`
pass unreported, and a turn reads clean because the evidence of its
messiness was half-written when it was read. Numbered above Part 5's range
because Part 3's block was already allocated, and numbered at all because an
unnumbered regression test is one nobody defends.

### Effort

Days. The extraction of the shared loops is most of it.

## Part 4: the exception line

A sixth hook entry, on `Stop` and `StopFailure`, printing one line when there
is something to look at.

### When it speaks

Coverage not verified; a recorded failure; a declaration without recorded
execution; a destination new for this project; a truncated or unknown digest.
A turn with none of those prints nothing.

The reasoning is that a line which is identical on almost every turn is
trained out within a week, and a notification nobody reads is the third
failure again.

### Silence is a policy, not a claim

Silence means no notification. It does not mean the turn was clean: it can
equally mean the plugin was disabled, the hook did not run, or the turn ended
without `Stop`.

So **`status` carries what silence cannot**: whether the last turn was
evaluated, when the last successful evaluation was, and the recap's own health.
"Evaluated, no findings" and "not evaluated" are different lines there. This is
the sentence that keeps exception-only from becoming silence-reads-as-zero, and
H-92 is its test.

`statusLine` is not used. It is a singleton, and claiming it would clobber
whatever the user already has.

### What the line says

```
※ rashomon: 1 recorded failure. 2 declarations without recorded execution.
            → /rashomon:report --session a3f2
```

It states records. Not "1 call failed and the summary did not mention it" --
the failure-word check is `strings.Contains` over 43 broad substrings
including `issue`, `problem` and `bug` (`account.go:113-136`), so it cannot
establish that a specific failure went unacknowledged, and it is deliberately
biased against firing. Where it does fire, the line says the summary contains
none of the words and the report lists them, as it does today.

Not "2 declared, never executed" either: `report.go:115` is explicit that
`Unexecuted` "is not a denial", and holds the denied, the failed and the
unrecorded together. "Without recorded execution" keeps absent evidence
distinct from proof that something did not happen.

The call to action is the slash command, because the reader is inside Claude
Code and a terminal is a context switch. That is what lets the line stay short:
it carries the signal, and one keystroke carries the detail.

### Mechanism, and the failure rule

Plain stdout on a hook goes to the debug log, so the human channel is
`systemMessage` in JSON on stdout, capped at 10,000 characters. Exit stays 0.

The entry's `timeout` is **10 seconds**, not the `install.Timeout` of 5 that
the recorder entries use and not the platform default of 600. The digest's
budget is 50 ms; ten seconds is a wide margin that still cannot hold a turn
open meaningfully.

`guarded()` exits 1 and writes to stderr, which a hook renders as a visible
`hook error`. That is right for a command and wrong for this: under
exception-only, a broken recap would be the only thing on screen, every turn.
**The recap exits 0 and says nothing on any internal failure**, and records its
own failure where `status` reports it. This is the one place the recorder's
"fail quietly" discipline and the user-facing surface agree.

### Acceptance

**H-90 -- a clean turn prints nothing.** Break: print a terse line anyway and
the surface is ignorable within a week, by the argument this part rests on.

**H-91 -- a turn with a finding prints once, naming it.** Break: render from
session scope and a later clean turn repeats an earlier finding.

**H-92 -- `status` distinguishes "evaluated, no findings" from "not
evaluated".** Break: report only the last finding and silence becomes
indistinguishable from a recap that never ran -- exactly the collapse this
part's policy would otherwise cause.

**H-93 -- the recap never produces a hook error.** Force an internal panic and
a malformed store; the hook exits 0 and prints nothing both times. Break:
wrap it in `guarded()` and a corrupt store puts an error on screen after every
turn.

**H-94 -- the renderer states no verdict.** `internal/report/vocabulary_test.go`
is extended to the recap package with the vocabulary widened from intent words
to verdict words. Break: render "stayed on task" and the test fails. The test
checks printed words and not semantic correctness, which is a bound on what it
proves and is written into it.

### Effort

Days.

## Part 5: a model-phrased reading

**Not built until the first two constraints above are signed off.** It is a
separate proposal that happens to live in this document, and it inherits no
approval from Parts 1 through 4.

### What it can actually claim

`Stop` carries `last_assistant_message`, which is the final response and not
the request. A model given the digest and that message can check **consistency
between what was said and what was recorded**. It cannot judge whether the work
matched the task, because nothing in this path carries the task. The planning
premise that a model "knows the conversation, so it can say whether the agent
stayed on task" does not survive: either the scope narrows to consistency, or
bounded task context is supplied with the disclosure that goes with it.

Part 5 takes the narrow scope. The wider one is a later question.

### Three unresolved questions it carries

- **Authentication.** `ANTHROPIC_API_KEY` is usually absent, because setting it
  overrides a subscription. The OAuth token is not reachable by a subprocess in
  any documented way. A native `type: "prompt"` hook avoids the question
  entirely but returns only `{ok, reason, impossible}` and cannot print a line;
  on `Stop`, `ok: false` hands its text to the audited agent as its next
  instruction, which is the thing this whole design exists to avoid.
- **Blocking.** Display and non-blocking are mutually exclusive: an
  `async: true` hook's `systemMessage` is explicitly not shown to the user. A
  user-visible model line means the user waits, which contradicts the original
  goal of a background summary. Part 5 owes a deadline and a defined fallback
  to Part 4's line.
- **Whether it may cause more agent work.** `additionalContext` on `Stop`
  avoids exit 2 and the error notice, but still continues the conversation. It
  does not preserve "the hook path records; it never blocks". Part 5 must
  answer this outright rather than let the mechanism choice answer it.

Also carried: cost and retention.

### Injection, which is not deferrable to implementation

A poisoned repository, page or MCP result reaches the transcript, the reader
reads it, and its text prints in rashomon's voice -- and, if the finding
channel continues the turn, re-enters the audited agent's context. The audit
tool becomes the laundering channel. Requirements, which are part of the
contract and not implementation detail:

- `type: "prompt"` only. **`type: "agent"` is forbidden** -- a reader with
  Read, Grep and Glob is a reader that injected text can aim at the filesystem.
- Model output is untrusted data: attributed, control characters stripped,
  length capped, and **every claim citing an evidence id present in the digest
  or dropped**.
- Inform-only by default.

**The digest's JSON encoding is not a defence for Part 4.** `encoding/json`
escapes control bytes structurally, so a digest carrying a hostile program
name is safe *as a document*. A consumer that decodes it and prints the value
with `%s` restores the bytes exactly. The guarantee is at the boundary, not
end to end, so Part 4's renderer sanitises on output regardless of what the
digest did on input.

**A second carrier exists today and is not Part 5's fault.** `shape.program`
is `path.Base()` of the first non-meta token of a command line -- unbounded,
attacker-influenceable text -- and the report renders such values with `%s`,
unquoted (`internal/report/text.go:108-115`). A binary named with CR or ANSI
bytes can therefore overwrite a rendered line, including forging a clean one.
This lands the moment `ByProgram` renders (#20), so **sanitising tool-derived
values -- capped, non-graphic runes rejected, quoted on render -- is a Part 4
requirement asserted as a value-level check in H-13's family**, not a Part 5
one.

### Acceptance

**H-95 -- the facts render with the reading, never instead of it.** Break:
render the model's prose alone and the only checkable half is gone.

**H-96 -- every model claim cites a digest field.** Break: allow free prose and
the line can assert something no record supports.

**H-97 -- a timeout falls back to Part 4's line exactly once.** Break: allow a
late response and the same turn is reported twice.

**H-98 -- prompt injection cannot widen the reader.** Instructions embedded in
a transcript and in tool output must not make it read files, run commands, or
spawn an agent. Break: grant it tools and the fixture exfiltrates.

**H-99 -- the recorder still reaches no network.** Whatever holds the model
call is a named sanctioned source in the shape of H-26, the recorder cannot
reach it, and no third source hides behind it. Break: import it from
`internal/hook` and H-99 fails.

### Effort

Unknown until the three questions are answered. Not estimated here.

## Schema

One new coverage reason, `recording_paused` (Part 2), added to the reason
vocabulary in `internal/store/record.go` and to `docs/store-schema.json`. It is
a reason and not a record type, so it needs no version bump.

The digest is a new document and not a store record. It is versioned
independently, starts at 1, and is not written to the store.

## Decisions taken

1. **Ownership becomes origin-aware rather than id-only.** The install id
   stays for settings entries; a plugin's entries are ours because the plugin
   directory says so.
2. **The plugin ships disabled.** `defaultEnabled: false`.
3. **`watch` refuses under a live plugin** rather than deduplicating records.
4. **Pause is a state file, not an environment variable and not a settings
   edit.** The file reaches plugin entries; the variable leaves no evidence;
   the settings edit cannot reach a plugin at all.
5. **The recap is exception-only**, and `status` carries what silence cannot.
6. **No `statusLine`.**
7. **The line states records**, not inferences about what the summary
   acknowledged.
8. **The recap never errors on screen.**
9. **The digest is turn-scoped, and subagent calls belong to the parent turn**,
   because that is what `prompt_id` says.
10. **Part 5 is separate** and inherits no approval from Parts 1-4.

## Ownership and order

Part 1, then Part 2 -- Part 2 is a ship blocker for Part 1, because a plugin
that cannot be turned off makes the second failure worse than it is today.
Part 3 is independent of both and can be built in parallel. Part 4 depends on
Part 3 for its numbers and on Part 1 for the slash command it points at.
Part 5 after a separate sign-off.

`internal/install`, `internal/settings`, `cmd/rashomon` for Parts 1 and 2;
`internal/digest` and `internal/report` for Parts 3 and 4.

## Sign-off

Parts 1 through 4 are proposed together and need the third constraint amended.
Part 5 needs the first and second amended, and is not started before that
happens.

Each part lands as its own pull request, reported the way every H-item is: the
command that ran it and its output, and the break that made it fail first.

Approval, and the constraints amendment, are requested by comment on the pull
request carrying this document: #21,
<https://github.com/altrace-dev-role/rashomon/pull/21>. The number and the link
are written out because a squash merge leaves this file in `main` with no other
pointer to that thread, and the comment is where the approval lives.
