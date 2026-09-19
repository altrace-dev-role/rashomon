---
name: rashomon
description: 'Start recording what this Claude Code session asks to run. Use when the user says "rashomon", "start rashomon", "claude rashomon", "watch this session", "record my session", asks to turn the recorder on, or wants an easier way to launch recorded sessions. For reports use rashomon-report; to stop use rashomon-stop; for install state use rashomon-status.'
---

Install the rashomon recorder so every tool call from now on is recorded as a
declaration (identifiers and shape only — never content).

1. Resolve the binary, in this order. Set `$RASHOMON` to the first that works:
   - `command -v rashomon`
   - `~/.local/bin/rashomon` if present and executable
   - Neither: build it from this repository if the cwd is the rashomon repo
     (`go build -o ~/.local/bin/rashomon ./cmd/rashomon`), otherwise
     `go install github.com/altrace-dev-role/rashomon/cmd/rashomon@latest`
     (binary lands in `$(go env GOPATH)/bin`).
   Never install from a `go run` path — `watch` refuses temporary build
   directories because the installed hook entry records the absolute path.

2. Run `$RASHOMON watch`. It writes five hook entries (PreToolUse,
   PostToolUse, PostToolUseFailure, SessionStart, SessionEnd) into
   `~/.claude/settings.json` and prints an install id plus the exact
   `detach --install <id>` line that undoes it. Quote that undo line back to
   the user verbatim.

3. Run `$RASHOMON status` and show the result, so the user sees the five
   entries as present and where the store lives.

4. Tell the user, briefly:
   - Recording covers tool calls from this point on. A session the recorder
     joined mid-way reports its coverage as `unverified` (reason
     `probe_absent`) — that is honest accounting, not a failure. Sessions
     started after this install report verified coverage.
   - `/rashomon-report` renders what was recorded; `/rashomon-stop` removes
     the hooks and keeps the store.

5. Offer the one-word launcher once, if it is not already on their PATH.
   From the rashomon repo:

       ln -sf "$(pwd)/scripts/claude-rashomon" ~/.local/bin/claude-rashomon

   From then on `claude-rashomon` in any directory runs `watch` and starts
   `claude` in one step, and because watch runs before the session starts,
   those sessions report verified coverage. Extra arguments pass through to
   `claude` unchanged.

Do not edit `~/.claude/settings.json` by hand and do not install the hook
entries yourself — `watch` is the only writer, and it preserves foreign
entries byte for byte.
