#!/bin/sh
# P-TRU-1: does one hook output carrying both additionalContext and
# systemMessage deliver each half where steer needs it, on this machine's
# Claude Code: additionalContext to the agent and systemMessage to the user's
# terminal. internal/steer/client.go's floor table opens an event at a version
# only on a pass here, and closes a version that fails (an exclusion).
#
#   test/live/p-tru-1.sh [EVENT ...]    EVENT is PostToolUse, PostToolUseFailure or
#                                       SessionStart; the first two by default
#
# One claude -p session per event, on haiku. A hook on Bash prints the one
# JSON object with nonce A as additionalContext and nonce B as systemMessage;
# the prompt has the model run `true` (PostToolUse) or `false`
# (PostToolUseFailure). An event passes when A is in a hook_additional_context
# attachment for the event in the session's transcript and B is in a
# system/informational event on the stream, which is what the terminal shows.
# test/live/hc_shape.py reads both and prints yes or no, never a value (H-115).
#
# SessionStart is judged per source, each with its own nonce pair, hook
# (matcher) and log, since Claude Code gives the two halves differently by
# source on one version and steer's floor is keyed by event and source: one
# session started with a new id (startup), then claude -p --resume of it
# asking again (resume), sending /compact (compact) and sending /clear (clear,
# whose attachments land in the new session's transcript, read beside the
# first). /compact here is a manual compact; an automatic one is not driven.
#
# Exit 0 when every event passed, 1 when any failed, 3 when none failed but
# the hook never ran on one (no login, or the model did not run the command:
# the run says nothing of the channel), 2 when it refused to start. Each
# event's verdict and the claude version go to a log under $PTRU_LOGS,
# /tmp/rk-probe-g by default; nothing a session sent is in it.
#
# A pass clears a shadow by hand, not by this script (CHANNEL-EVIDENCE1). When
# a Stop's read of the transcript did not show a note rashomon delivered,
# rashomon shadows that event until a probe passes or Claude Code is newer
# than the version that shadowed (a call on a newer version clears it itself).
# After exit 0, remove the event's marker from rashomon's store root:
#   rm -f "${RASHOMON_HOME:-${XDG_STATE_HOME:-$HOME/.local/state}/rashomon}/channel-evidence-PostToolUse"
# (channel-evidence-PostToolUseFailure for that event). This script touches
# nothing of rashomon's store, so the removal stays a person's step.
#
# Claude Code runs under $CLAUDE_CONFIG_DIR, $HOME/.claude-probe by default: a
# configuration of its own, logged in once by a person, which each log names,
# so the evidence says whose login answered. Every session runs with
# --setting-sources project in an empty project directory, so no settings
# file but the probe's own --settings is read; the script still refuses a
# settings.json in that directory that declares hooks or cannot be read, so a
# user-level hook can never answer for the probe's. It writes nothing in the
# configuration: every session it starts loses its transcript and session-env
# directory on exit (a /clear's new session with it), and TMPDIR points into
# its scratch while it runs.
set -eu
CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude-probe}
export CLAUDE_CONFIG_DIR="$CFG"
LOGS=${PTRU_LOGS:-/tmp/rk-probe-g}
EVENTS=${*:-PostToolUse PostToolUseFailure}
for ev in $EVENTS; do
  case $ev in
    PostToolUse | PostToolUseFailure | SessionStart) ;;
    *) echo "refusing: $ev is not an event this probe drives" >&2; exit 2 ;;
  esac
done
if [ -e "$CFG/settings.json" ] && ! python3 -I -c 'import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if isinstance(d, dict) and "hooks" not in d else 1)' "$CFG/settings.json" 2> /dev/null; then
  echo "refusing: $CFG/settings.json declares hooks or cannot be read; its hooks would fire beside the probe's" >&2; exit 2
fi
if ! command -v claude > /dev/null 2>&1; then
  echo "refusing: no claude on PATH" >&2; exit 2
fi
case $CFG in
  "$HOME"/*) SHOWN="~/${CFG#"$HOME"/}" ;;
  *) SHOWN=$CFG ;;
esac
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/rashomon-ptru.XXXXXX")
SIDS=
# As hc-items.sh: each session's transcript and session-env directory, then the
# scratch. Every transcript in a session's project directory goes with it: the
# directory is this run's own (its scratch project), and a /clear's new session
# writes there under an id the script did not choose. HUP is trapped with INT
# and TERM so dash runs the EXIT trap on it.
cleanup() {
  set +e
  for s in $SIDS; do
    for f in "$CFG"/projects/*/"$s".jsonl; do
      [ -e "$f" ] || continue
      d=$(dirname "$f")
      for g in "$d"/*.jsonl; do
        [ -e "$g" ] || continue
        rm -rf "$g" "${g%.jsonl}"; rmdir "$CFG/session-env/$(basename "$g" .jsonl)" 2> /dev/null
      done
      rmdir "$d/memory" "$d" 2> /dev/null
    done
    rmdir "$CFG/session-env/$s" 2> /dev/null
  done
  rm -rf "$T"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
case $T in *[!A-Za-z0-9/._-]*) echo "refusing: the scratch path needs quoting this script does not do" >&2; exit 2 ;; esac
mkdir "$T/tmp"
export TMPDIR="$T/tmp"
cp "$ROOT/test/live/hc_shape.py" "$T/shape.py"
SHAPE="python3 -I $T/shape.py"
mkdir -p "$LOGS"

uuid() { python3 -I -c 'import uuid; print(uuid.uuid4())'; }
token() { python3 -I -c 'import secrets; print(secrets.token_hex(4))'; }
TO=
if command -v timeout > /dev/null 2>&1; then TO="timeout 600"; fi
transcript() {
  for f in "$CFG"/projects/*/"$1".jsonl; do
    if [ -f "$f" ]; then echo "$f"; return 0; fi
  done
  echo -
}
# beside is another session's transcript in session $1's project directory:
# the one a /clear started.
beside() {
  f=$(transcript "$1")
  if [ "$f" != - ]; then
    for g in "$(dirname "$f")"/*.jsonl; do
      if [ -f "$g" ] && [ "$g" != "$f" ]; then echo "$g"; return 0; fi
    done
  fi
  echo -
}
note() {
  case $1 in
    1) worst=1 ;;
    3) [ "$worst" = 1 ] || worst=3 ;;
  esac
}
# session_start drives SessionStart's four sources in one session, each with
# its own hook (matcher), nonce pair and log.
session_start() {
  D=$T/SessionStart; mkdir -p "$D/proj"
  groups=
  for src in startup resume compact clear; do
    X=$(token)
    echo "PTRU1-CTX-$X" > "$D/$src.a"; echo "PTRU1-SM-$X" > "$D/$src.b"
    cat > "$D/emit-$src.sh" << SH
#!/bin/sh
cat > /dev/null
: > "$D/fired-$src"
echo '{"systemMessage": "PTRU1-SM-$X", "hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": "PTRU1-CTX-$X"}}'
SH
    groups="$groups${groups:+, }{\"matcher\": \"$src\", \"hooks\": [{\"type\": \"command\", \"command\": \"sh $D/emit-$src.sh\", \"timeout\": 20}]}"
  done
  echo "{\"hooks\": {\"SessionStart\": [$groups]}}" > "$D/settings.json"
  S=$(uuid); SIDS="$SIDS $S"
  ask="Reply with every token that begins with PTRU1- and appears anywhere in your context, one per line, and nothing else."
  cd "$D/proj"
  ss_run startup "$ask" --session-id "$S"
  ss_run resume --resume "$S" "$ask"
  ss_run compact --resume "$S" /compact
  ss_run clear --resume "$S" /clear
  cd "$T"
  for src in startup resume compact clear; do
    log=$LOGS/p-tru-1-SessionStart-$src-$STAMP.log
    tr=$(transcript "$S")
    if [ "$src" = clear ]; then tr=$(beside "$S"); fi
    rc=0
    {
      echo "P-TRU-1 SessionStart $src"
      echo "claude --version: $VERSION"
      echo "config dir: $SHOWN"
      echo "run at: $STAMP"
      echo "claude exit $(cat "$D/$src.rc")"
      $SHAPE ptru1 "$D/$src.ndjson" "$tr" SessionStart "$(cat "$D/$src.a")" "$(cat "$D/$src.b")" "$D/fired-$src" || rc=$?
      echo "exit $rc"
    } > "$log"
    cat "$log"
    echo "log: $log"
    note "$rc"
  done
}
# ss_run SOURCE ARG...: one SessionStart session's call, its stream and exit
# code in $D.
ss_run() {
  src=$1; shift
  rc=0
  $TO claude -p "$@" --settings "$D/settings.json" --setting-sources project --model haiku --max-turns 2 \
    --output-format stream-json --verbose < /dev/null > "$D/$src.ndjson" 2> /dev/null || rc=$?
  echo "$rc" > "$D/$src.rc"
}

VERSION=$(claude --version 2> /dev/null | head -n 1)
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
for m in /etc/claude-code/managed-settings.json "/Library/Application Support/ClaudeCode/managed-settings.json"; do
  if [ -e "$m" ]; then echo "managed settings present ($m): its hooks fire beside the probe's"; fi
done
worst=0
for ev in $EVENTS; do
  if [ "$ev" = SessionStart ]; then
    session_start
    continue
  fi
  D=$T/$ev; mkdir -p "$D/proj"
  log=$LOGS/p-tru-1-$ev-$STAMP.log
  cmd=true
  if [ "$ev" = PostToolUseFailure ]; then cmd=false; fi
  X=$(token); A=PTRU1-CTX-$X; B=PTRU1-SM-$X
  cat > "$D/emit.sh" << SH
#!/bin/sh
cat > /dev/null
: > "$D/fired"
echo '{"systemMessage": "$B", "hookSpecificOutput": {"hookEventName": "$ev", "additionalContext": "$A"}}'
SH
  cat > "$D/settings.json" << JSON
{"hooks": {"$ev": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "sh $D/emit.sh", "timeout": 20}]}]}}
JSON
  S=$(uuid); SIDS="$SIDS $S"
  cd "$D/proj"
  RC=0
  $TO claude -p "Run the Bash command $cmd once. Then reply with every token that begins with PTRU1- and appears anywhere in your context, one per line, and nothing else." \
    --session-id "$S" --settings "$D/settings.json" --setting-sources project --allowedTools "Bash($cmd)" --model haiku --max-turns 3 \
    --output-format stream-json --verbose < /dev/null > "$D/stream.ndjson" 2> /dev/null || RC=$?
  cd "$T"
  rc=0
  {
    echo "P-TRU-1 $ev"
    echo "claude --version: $VERSION"
    echo "config dir: $SHOWN"
    echo "run at: $STAMP"
    echo "claude exit $RC"
    $SHAPE ptru1 "$D/stream.ndjson" "$(transcript "$S")" "$ev" "$A" "$B" "$D/fired" || rc=$?
    echo "exit $rc"
  } > "$log"
  cat "$log"
  echo "log: $log"
  note "$rc"
done
exit "$worst"
