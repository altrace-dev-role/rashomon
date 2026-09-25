# rashomon

**A coding agent writes its own account of what it did. `rashomon` writes a second one, from outside.**

*For **Claude Code**, on macOS and Linux. Alpha: no tagged release yet — build from
source below. No other agent harness is recorded.*

When you hand a coding agent your machine, it works for twenty or forty minutes
and then tells you what happened — from the same context, with the same
incentive to look successful. `rashomon` records the session independently: what
the agent *declared* it would run and what *actually ran*.

Three accounts of one session: what it declared, what ran, and what it says it
did — reconciled, with the disagreements shown. The name is the point.

## What a disagreement looks like

A session whose test run failed while a subagent looked through the code, and
whose closing message says the tests pass. This excerpt is `rashomon report`
exactly as it renders that session:

      the agent's account:
        "Done. I refactored ParseConfig and all tests pass."
      subagents: 1
        agent-a41f (Explore): 3 declarations, 3 executions, 1 Bash
        (these calls do not appear in the main transcript)
      failed calls: 1
        the final message contains none of these 43 words: fail, failed, failing, error, errors, couldn't, could not, unable, not able, didn't, did not, blocked and 31 more of 43 (--json lists them all)

The agent's account is its own words, quoted. The `subagents` and
`failed calls` lines come from the hooks, not from anything the agent wrote:
the session's `go test ./...` ended with exit code 1, and the subagent's three
calls never appear in the main transcript at all. The report says which
acknowledgement words are *absent* from the summary; it does not say why.

To see the same disagreement in your own session: run `rashomon watch`, break
one test in a project you have open, and start `claude`. Ask it to have a
subagent look through the code for something, then to run the test suite and
finish with a one-line summary. Whatever that summary says, `rashomon report`
quotes it above the `subagents` and `failed calls` lines — and if the summary
never mentions the failure, the `failed calls` line lists the words it does not
use.

## What a report tells you

- **What subagents did** that the main transcript never shows — per-agent call
  and command counts.
- **Which tool calls failed**, and whether the agent's closing summary mentions
  failure at all. It reports the words that are *absent*; it never characterises
  intent.
- **What executed differently from what was declared** — a hook or wrapper that
  rewrote a command before it ran.
- **What could not be seen**, on every report, including a healthy one.

## Install

**As a Claude Code plugin** (macOS and Linux):

    claude plugin marketplace add altrace-dev-role/rashomon
    claude plugin install rashomon@rashomon
    claude plugin enable rashomon@rashomon

Installing does not start recording; enabling does. Start a new Claude Code
session after enabling. `/plugin` shows it and turns it off again, and
`/rashomon:status` / `/rashomon:report` work inside Claude Code. The plugin
installs the release's `rashomon-plugin.zip`, which carries a prebuilt recorder
per platform, because a plugin cannot compile Go at install time — so it needs
a tagged release to exist. To try the plugin from a checkout instead:

    scripts/build-plugin.sh
    claude --plugin-dir ./plugin

If you already ran `watch`, run `rashomon detach` first. With both installed,
every call is recorded twice; `rashomon status` names the overlap and the
report marks the session `duplicate_declarations`.

**As a settings install** (Windows is not usable yet; see [Status](#status)):

    go install github.com/altrace-dev-role/rashomon/cmd/rashomon@main

This writes to `$(go env GOPATH)/bin` — make sure that is on your `PATH`. Or
clone and build:

    git clone https://github.com/altrace-dev-role/rashomon
    cd rashomon && go build ./cmd/rashomon

`@main` rather than `@latest`: there are no tagged releases yet, so `@latest`
has nothing to resolve to.

> **Install to a location that will not move.** `watch` writes the running
> binary's absolute path into your Claude Code settings, and Claude Code
> executes that path on **every tool call**. If you built inside a clone you
> later delete, every tool call fires a hook that cannot start. Build into a
> directory you keep — or `go install` it — and re-run `watch` after any move.
> (`watch` refuses a `go run` binary outright: that one lives in a temp
> directory that is gone seconds later.)
>
> **Why a broken hook matters more than usual:** a `PreToolUse` hook that exits
> non-zero in the wrong way can *block* the tool call, and the user sees Claude
> Code failing rather than this program. Every failure path here is engineered
> to exit 0 for that reason — see
> [`docs/design-notes.md`](docs/design-notes.md).

## Quickstart

```sh
rashomon watch                      # install the recorders (the only command that installs anything)
claude                              # work normally
rashomon report                     # read the sessions back
```

`watch` adds eight entries to your Claude Code settings — `PreToolUse`,
`PostToolUse`, `PostToolUseFailure`, `SessionStart`, `SessionEnd`, `Stop`,
`StopFailure`, `UserPromptSubmit`. The first three match `*` (every tool);
the rest carry no matcher, since Claude Code documents none for the session,
turn-end and prompt events. All eight run a 5-second timeout except the three
recap entries, which get 10: finding a turn's boundaries means reading a whole
run, not one call.
`rashomon detach` removes them again, leaving every other entry byte-for-byte
as found.

`Stop` and `StopFailure` drive an exception-only recap: after a turn, it
prints at most one line — and only when there is something worth looking
at (a recorded failure, a declaration without recorded execution, coverage
that did not verify, or a truncated/unknown projection) — with a pointer to
`rashomon report --session <id>` for the detail. A clean turn prints
nothing at all; `rashomon status` says whether a turn has actually been
evaluated, so silence never gets read as proof the turn was clean.

Saying **No** at a permission prompt interrupts the turn, and Claude Code
fires no `Stop` after an interrupt. `UserPromptSubmit` catches that case:
when you send your next prompt it checks the turn that just ended, and if no
recap reached it and it has something to show, prints the line then, marked
`previous turn`.

## What is recorded, and what never is

**Recorded** — identifiers, shapes and hostnames. `tool_use_id`, `session_id`,
`prompt_id`, `agent_id`, `agent_type`, `transcript_path`, `cwd`,
`permission_mode`, `tool_name`; the call's `program`, `verb_class`, argument
*count* and a keyed digest of its shape; how it ended (`outcome`, `exit_code`,
`is_interrupt`, `duration_ms`); and a `file_label` classifying a touched path
into categories such as `ssh_key`, `env_file`, `cloud_config` or `certificate`.

Note that `cwd` and `transcript_path` are filesystem paths and carry directory
names. [`docs/store-schema.json`](docs/store-schema.json) is the exhaustive and
authoritative field list — every key required, `additionalProperties: false`.

**Hostnames are stored in clear**, because the report has to name them:
`report --chain` lists the hosts each call named beside that call, and a
digest cannot be rendered back into a name. They are extracted from
`WebFetch.url` and from any URL appearing in a `Bash` command line — *whether
or not the call reached it*. Only those two tools are read.

**Never recorded:** prompts, responses, argument values, command strings, file
contents, tool output. Not redacted — *structurally absent*. The payload struct
has no field for tool output, so the JSON decoder discards it and it is never a
value in the process at all.

Those rules govern the **store**. The **report** additionally reads the agent's
final message from Claude Code's own transcript at render time — that is
content, it is never written to the store, never transmitted, and `--redact`
drops it entirely rather than partially cleaning prose that may name anything.

The shape digest is an HMAC under a random per-install key, so the same command
digests differently on two machines and the store cannot be run as a dictionary
attack against known commands. Store directory `0700`, files `0600`.

### Where the store lives, and how to remove it

`$RASHOMON_HOME`, else `$XDG_STATE_HOME/rashomon`, else
`~/.local/state/rashomon`. It is bounded by `RASHOMON_STORE_CAP_BYTES` (default
512 MiB); past the cap the oldest runs are evicted, each leaving a gap record so
the deletion is visible. **`detach` removes the hooks, not the records** — to
remove everything, delete that directory.

### Redaction, and what it does not hide

`report --redact` digests hostnames under this install's key. It is for sharing
a report, and its limits are real: the digest is 32 bits, so two hosts can
collide; **the last label is kept in clear on purpose**, so `.internal` and
`.amazonaws.com` survive; and anyone holding this install's store can recompute
every digest. The redacted report repeats all of this in its own header.

`forget --host <h>` removes every call that named that host from this store,
and leaves a gap record saying something was removed — keyed by a digest of the
host, not its name.

## What it cannot see

**Network destinations are not observed in this alpha.** The text report says
so in one line:

    destinations: not observed in this alpha

It is printed for every session, including a completely healthy one, because it
is a property of the instrument and not of the session. A reader told only what
*was* observed will read the rest as an absence of traffic rather than an
absence of observation. `--json` states the same fact in its own form: the
`destinations` object carries `"observed": false` and the reason
`no_proxy_store`, so its empty host lists read as *not watched*, not as *none*.
`--chain` lists the hosts each call *named*; it does not say whether the call
reached them.

## The two rules it will not break

**Never print "nothing happened" where the truth is "not watching."** Every
count that could not be measured renders as unknown, never as zero. A run that
could not measure itself does not get to report a number.

**Never print "not watching" where the truth is "nothing happened."** Every
degradation line has a healthy twin, so a clean run and a blind run do not read
the same.

One consequence worth expecting: coverage reads `verified` only for a session
whose recorder `watch` installed in the settings file Claude Code itself
resolves. A session started with `claude --settings <other file>`, or under a
different `CLAUDE_CONFIG_DIR`, records perfectly well and still reports
`hook_entry_absent` — the probe under-claims rather than confirming an entry it
cannot read.

The reasoning behind all of this, and the acceptance criteria that hold it, is
in [`docs/design-notes.md`](docs/design-notes.md).

## Commands

| Command | What it does |
| --- | --- |
| `rashomon watch` | Install the recorders and the liveness probe. **The only command that installs anything.** |
| `rashomon report [--session S] [--json] [--redact] [--chain]` | Render **every** recorded session, or one named with `--session`. `--chain` adds the causal view: which prompt produced which calls |
| `rashomon status` | Say what is installed here. Reads only; creates nothing |
| `rashomon detach` | Remove the recorders, leaving every other entry byte-for-byte as found |
| `rashomon forget --since T \| --before T \| --host H` | Erase records, leaving a gap record saying so |
| `rashomon version` | Print the version |

`rashomon <command> --help` for the full flag set.

`detach` has two recovery forms for when things are gone: `detach --install <id>`
(printed by `watch` at install time) removes one install's entries without
opening a store, and `detach --all` removes every entry carrying a `rashomon`
marker, for when the store is gone and the id with it. Both refuse, naming the
field, if an entry of ours was hand-edited.

## Editor integration

This repository installs nothing in your sessions. There is no committed
`.claude/settings.json`; nothing here runs until you run `watch` yourself.

Slash commands ship as **assets you copy** from a clone of this repository
(`go install` leaves none), deliberately not under `.claude/` here — a skill in
this repository would load only for someone who has *this* repository open,
which is the one place it is least useful:

    cp -r skills/rashomon* "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/skills/"     # every project
    cp -r skills/rashomon* /path/to/project/.claude/skills/                  # one project

`scripts/claude-rashomon` starts a recorded session in one word.
`scripts/rashomon-sessionstart.sh` is an optional `SessionStart` hook that puts
one line of recorder state into a session's context; its header shows the entry
to add. `.cursor/commands/` gives Cursor commands for *operating* the tool —
status, report, stop and arming `watch`, but not `forget`. Cursor's own agent
loop is not recorded, and every command says so.

## Scope and threat model

**Claude Code only.** The hooks, the store and the report describe Claude Code
sessions. No other agent harness is recorded.

**`rashomon` installs no certificate and decrypts nothing.**

**No account. No telemetry. No phone-home. No model in any path.**

**What it defends against, stated plainly:** this release defends against the
agent's *narration* — a summary that omits, compresses or misremembers. It does
not defend against an agent that *tampers*. The store, the digest key and
`forget` are all reachable by any shell command the agent makes. Treat the
record as an honest second account, not as tamper-proof evidence.

**A declaration is not an execution.** `PreToolUse` fires before the permission
prompt resolves, so the store holds records for calls you went on to deny. That
is deliberate — intent is worth keeping whether or not it was granted — and
every record carries the `permission_mode` it was declared in so a consumer can
exclude what was never allowed to run.

## Building from source

    go build ./cmd/rashomon
    go test ./...

## Status

Alpha. No tagged releases, no signed binaries, no Homebrew formula. macOS and
Linux are the supported targets.

**Windows is not usable yet.** `watch` quotes the installed command line for a
POSIX shell, so the entry it writes will not run there. The code paths build and
are covered by tests; the install path is a known gap, and
`scripts/claude-rashomon.ps1` lands with it.

## Contributing

Issues and pull requests are welcome. `main` is protected: changes land through
a pull request with CI green. Security issues should go through
[private vulnerability reporting](https://github.com/altrace-dev-role/rashomon/security/advisories/new)
rather than a public issue — see [SECURITY.md](SECURITY.md).

## License

Apache 2.0. See [LICENSE](LICENSE).

---

Built by [Altrace](https://github.com/altrace-dev-role).
