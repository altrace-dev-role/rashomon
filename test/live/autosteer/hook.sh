#!/bin/sh
# hook.sh VERB...: the harness's hook entry. Runs RK_BIN with the arguments watch wrote. With RK_RUN_DIR set,
# appends each event's verb, exit code and stdout to RK_RUN_DIR/hooks.log, the elapsed milliseconds to
# hook-ms.log (perl's clock; the two perl starts cost the session a few ms each and are outside the interval),
# then prints the stdout for Claude Code.
# Harness identity: rashomon's steer step reads CLAUDE_CODE_ENTRYPOINT and AI_AGENT ("claude-code_<v>_harness") to
# name the client and its version; a note with no readable version is built and not sent. What Claude Code set for
# the hook command is recorded once per run in hook-env.txt, before anything here changes it.
# Claude Code 2.1.280 sets CLAUDE_CODE_ENTRYPOINT=sdk-cli for the hooks of a `claude -p` session (every T1 pilot run's
# hook-env.txt), and rashomon reads sdk-cli as an SDK session, whose notes it suppresses by design (no person at the
# prompt: no_mirror). The harness stands in for a terminal session, so cli is exported always, never only when unset
# (B448: the pilot's hooks all ran as sdk-cli and no note reached any agent). AI_AGENT is exported from
# RK_CLAUDE_VERSION (default 2-1-280) only when Claude Code leaves it unset: the value it sets is the one read.
if [ -n "${RK_RUN_DIR:-}" ] && [ ! -f "$RK_RUN_DIR/hook-env.txt" ]; then
  { echo "CLAUDECODE=${CLAUDECODE:-<unset>}"; echo "CLAUDE_CODE_ENTRYPOINT=${CLAUDE_CODE_ENTRYPOINT:-<unset>}"; echo "AI_AGENT=${AI_AGENT:-<unset>}"; echo "CLAUDE_PROJECT_DIR=${CLAUDE_PROJECT_DIR:-<unset>}"; } > "$RK_RUN_DIR/hook-env.txt"
fi
export CLAUDE_CODE_ENTRYPOINT=cli
[ -n "${AI_AGENT:-}" ] || export AI_AGENT="claude-code_${RK_CLAUDE_VERSION:-2-1-280}_harness"
if [ -n "${RK_RUN_DIR:-}" ]; then
  t0=$(perl -MTime::HiRes=time -e 'print int(time*1000)')
  tmp=$(mktemp); "$RK_BIN" "$@" > "$tmp" 2>> "$RK_RUN_DIR/hook-stderr.txt"; rc=$?
  t1=$(perl -MTime::HiRes=time -e 'print int(time*1000)')
  echo "$1 ${2:-} $((t1 - t0)) rc=$rc" >> "$RK_RUN_DIR/hook-ms.log"
  { echo "== $(date -u +%H:%M:%SZ) $* rc=$rc"; cat "$tmp"; echo; } >> "$RK_RUN_DIR/hooks.log"
  cat "$tmp"; rm -f "$tmp"; exit $rc
fi
exec "$RK_BIN" "$@"
