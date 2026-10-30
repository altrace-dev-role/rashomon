#!/bin/sh
# P-TRU-1: does one hook output carrying both additionalContext and
# systemMessage deliver each half where steer needs it, on this machine's
# Claude Code: additionalContext to the agent and systemMessage to the user's
# terminal. internal/steer/client.go's floor table opens an event at a version
# only on a pass here, and closes a version that fails (an exclusion).
#
#   test/live/p-tru-1.sh [EVENT ...]    EVENT is PostToolUse or PostToolUseFailure; both by default
#
# One claude -p session per event, on haiku. A hook on Bash prints the one
# JSON object with nonce A as additionalContext and nonce B as systemMessage;
# the prompt has the model run `true` (PostToolUse) or `false`
# (PostToolUseFailure). An event passes when A is in a hook_additional_context
# attachment for the event in the session's transcript and B is in a
# system/informational event on the stream, which is what the terminal shows.
# test/live/hc_shape.py reads both and prints yes or no, never a value (H-115).
#
# Exit 0 when every event passed, 1 when any failed, 3 when none failed but
# the hook never ran on one (no login, or the model did not run the command:
# the run says nothing of the channel), 2 when it refused to start. Each
# event's verdict and the claude version go to a log under $PTRU_LOGS,
# /tmp/rk-probe-g by default; nothing a session sent is in it.
#
# Claude Code runs under $CLAUDE_CONFIG_DIR, $HOME/.claude-probe by default: a
# configuration of its own, logged in once by a person, so no hook of the
# user's fires beside this one. The script refuses when that directory has a
# settings.json, as hc-items.sh does, since --settings adds to it. Every
# session it starts loses its transcript and session-env directory on exit,
# and TMPDIR points into its scratch while it runs.
set -eu
CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude-probe}
export CLAUDE_CONFIG_DIR="$CFG"
LOGS=${PTRU_LOGS:-/tmp/rk-probe-g}
EVENTS=${*:-PostToolUse PostToolUseFailure}
for ev in $EVENTS; do
  case $ev in
    PostToolUse | PostToolUseFailure) ;;
    *) echo "refusing: $ev is not an event this probe drives" >&2; exit 2 ;;
  esac
done
if [ -e "$CFG/settings.json" ]; then
  echo "refusing: $CFG/settings.json exists; its hooks would fire beside the probe's" >&2; exit 2
fi
if ! command -v claude > /dev/null 2>&1; then
  echo "refusing: no claude on PATH" >&2; exit 2
fi
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/rashomon-ptru.XXXXXX")
SIDS=
# As hc-items.sh: each session's transcript and session-env directory, then the
# scratch. HUP is trapped with INT and TERM so dash runs the EXIT trap on it.
cleanup() {
  set +e
  for s in $SIDS; do
    for f in "$CFG"/projects/*/"$s".jsonl; do
      [ -e "$f" ] || continue
      rm -rf "$f" "${f%.jsonl}"; rmdir "$(dirname "$f")" 2> /dev/null
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

VERSION=$(claude --version 2> /dev/null | head -n 1)
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
for m in /etc/claude-code/managed-settings.json "/Library/Application Support/ClaudeCode/managed-settings.json"; do
  if [ -e "$m" ]; then echo "managed settings present ($m): its hooks fire beside the probe's"; fi
done
worst=0
for ev in $EVENTS; do
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
    --session-id "$S" --settings "$D/settings.json" --allowedTools "Bash($cmd)" --model haiku --max-turns 3 \
    --output-format stream-json --verbose < /dev/null > "$D/stream.ndjson" 2> /dev/null || RC=$?
  cd "$T"
  rc=0
  {
    echo "P-TRU-1 $ev"
    echo "claude --version: $VERSION"
    echo "run at: $STAMP"
    echo "claude exit $RC"
    $SHAPE ptru1 "$D/stream.ndjson" "$(transcript "$S")" "$ev" "$A" "$B" "$D/fired" || rc=$?
    echo "exit $rc"
  } > "$log"
  cat "$log"
  echo "log: $log"
  case $rc in
    1) worst=1 ;;
    3) [ "$worst" = 1 ] || worst=3 ;;
  esac
done
exit "$worst"
