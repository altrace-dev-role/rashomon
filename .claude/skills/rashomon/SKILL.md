---
name: rashomon
description: 'Start recording what this Claude Code session asks to run. Use when the user says "rashomon", "start rashomon", "claude rashomon", "watch this session", "record my session", asks to turn the recorder on, or wants an easier way to launch recorded sessions. For reports use rashomon-report; to stop use rashomon-stop; for install state use rashomon-status.'
---

Install the rashomon recorder so every tool call from now on is recorded as a
declaration (identifiers and shape only — never content).

1. Resolve the binary, in this order. Set `$RASHOMON` to the first absolute
   path that works:
   - `~/.local/bin/rashomon` if present and executable
   - `~/go/bin/rashomon` if present and executable (where `go install` puts it)
   - `/opt/homebrew/bin/rashomon` or `/usr/local/bin/rashomon` (brew installs;
     checked explicitly because a GUI-launched app's PATH may not carry them)
   - `command -v rashomon`, kept only when it returns an absolute path
   - None found: if `command -v go` also fails, stop and point the user at the
     README's install options instead of building. With Go present, build from
     this repository when the cwd is the rashomon repo
     (`go build -o ~/.local/bin/rashomon ./cmd/rashomon`; on Windows,
     PowerShell does not expand `~` for go — use
     `go install ./cmd/rashomon`, which lands `rashomon.exe` in
     `"$(go env GOPATH)\bin"`). Outside the repo, ask the user where their
     rashomon checkout is rather than fetching an unpinned module from the
     network.
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
     started after this install are eligible for verified coverage, because
     the probe is in place from their first moment; the report is the proof.
   - `/rashomon-report` renders what was recorded; `/rashomon-stop` removes
     the hooks and keeps the store.

5. Offer the one-word launcher once, if it is not already on their PATH.
   From the rashomon repo, on macOS or Linux:

       mkdir -p ~/.local/bin
       ln -sf "$(git rev-parse --show-toplevel)/scripts/claude-rashomon" ~/.local/bin/claude-rashomon
       case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) echo 'add ~/.local/bin to your PATH' ;; esac

   macOS does not ship `~/.local/bin` or put it on PATH — the mkdir and the
   PATH check matter there. On Windows, the launcher is
   `scripts\claude-rashomon.ps1` (run it with `powershell -File`, or put
   `scripts\` on PATH); the POSIX launcher and the SessionStart state line
   need Git Bash, which Claude Code uses for hooks on Windows when present.
   From then on `claude-rashomon` in any directory runs `watch` and starts
   `claude` in one step, with the liveness probe in place from the session's
   first moment — no mid-session `probe_absent` downgrade. Extra arguments
   pass through to `claude` unchanged.

Do not edit `~/.claude/settings.json` by hand and do not install the hook
entries yourself — `watch` is the only writer, and it preserves foreign
entries byte for byte.
