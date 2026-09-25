# Contributing

This program records what a Claude Code session declared, what ran, and what
the wire saw, and says in fixed vocabulary what it does not know. Two
properties carry everything else, and a change that weakens either is a change
that should not land, however much else it improves.

**It never records content.** No prompt, response, file content, tool output or
argument value reaches a record or a file in the store. The exceptions are the
hostnames a call names and the program name of a command. What may be stored is
an identifier Claude Code assigned, a value from a closed vocabulary, a count, a
keyed digest, a canonical hostname, a program name, or a path the payload
carries as metadata (`cwd`, `transcript_path`). The one piece of content that
reaches stdout is the report's quote of the agent's final message, read from
Claude Code's transcript at render time and never stored.

**It never claims what it did not measure.** A count it could not take renders
as `unknown` or `not read`, never as `0`. An absence renders as an absence,
never as a negative finding.

## Getting set up

    git clone https://github.com/altrace-dev-role/rashomon
    cd rashomon
    go build ./cmd/rashomon
    go test ./...

Go 1.25 or later (`go.mod` says 1.25.0). The one non-standard dependency is a
pure-Go SQLite driver, linked for a proxy-store reader the alpha leaves dormant
(#28).

Run the tool against a scratch store rather than your own while you work:

    export RASHOMON_HOME=$(mktemp -d)
    export CLAUDE_CONFIG_DIR=$(mktemp -d)

Both are read everywhere the real paths are, so nothing you do while
developing touches `~/.claude/settings.json` or a store you care about.

## Before you open a pull request

    go build ./...
    go vet ./...
    go vet -tags rashomonfault ./...
    gofmt -l .
    go test ./... -count=1
    python3 test/mutation/sweep.py

The sweep is the part people skip, and it is the part that matters. It applies
a deliberate break to the implementation and confirms the item that claims to
cover it goes red. An item it reports as `undetected` is a specification
defect, not a test that needs adjusting.

**Run the whole sweep, not the entries you added.** It anchors each break on an
exact snippet of source, so moving a line or changing a signature elsewhere can
orphan an anchor that has nothing to do with your change. An orphaned anchor is
reported as `ANCHOR MISSING` and scores its item undetected — the sweep says
so, but only if you ran it. This has caught real regressions and has also been
missed by running a filtered subset.

## Adding behaviour

Acceptance items are numbered `H-n` and live in `test/acceptance/hNN_*_test.go`,
one file per item, with a comment at the top saying what the item is for and
what risk it holds down. An item is not finished until:

1. it fails under a named break, and you have seen it fail;
2. that break is registered in `test/mutation/sweep.py`;
3. the sweep reports it detected.

Negative items need positive twins. "No path reached the store" passes
trivially against a function that computes nothing, so something in the same
item has to prove the path was computed at all.

Say **why** in comments, not what. Most of the comments in this codebase record
a decision and the failure that motivated it, often with the measurement that
settled it. That is the house style, and a comment that only restates the code
is noise.

## Where things are

| package | what it owns |
| --- | --- |
| `internal/shape` | the only code allowed to look inside `tool_input` |
| `internal/hook` | the PreToolUse, PostToolUse and probe entry points |
| `internal/store` | the append-only record store and its locking |
| `internal/settings` | reading and editing Claude Code settings files |
| `internal/report` | reconciling the three records and rendering them |
| `internal/wire` | reading the proxy's store |
| `internal/install` | the hook entries, and what counts as one of ours |

`internal/shape` deserves a note. It is the single file anyone has to audit to
believe the no-content guarantee, which is why host extraction and file
labelling live there rather than closer to their callers. Adding a second
reader of `tool_input` anywhere else doubles the cost of that audit, and a
duplicate reader inside the package is the same problem wearing a different
hat.

## Known limitations

Stated so nobody rediscovers them as bugs:

- **Windows installs do not work.** The hook command line is quoted for a POSIX
  shell, so a Windows executable path comes out wrapped in apostrophes that
  `cmd.exe` does not honour. `internal/install/windows_quoting_test.go` pins the
  current behaviour; fixing the quoting means deleting that test and the caveat
  in the README with it.
- **Two H-17 halves are Linux-only.** They gate on `unshare` and `strace`, so
  they skip on macOS. CI installs both on the Linux runner and treats a skip as
  a failure, which is where that coverage actually happens.
- **`TestRead_AgainstARealProxyStore` is opt-in** behind `RASHOMON_REAL_STORE`
  and has never run against a store the proxy itself wrote.

## Reporting a security issue

See `SECURITY.md`. Please do not open a public issue for anything that would
let a record carry content it should not.
