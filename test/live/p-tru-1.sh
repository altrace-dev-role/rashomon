#!/bin/sh
# P-TRU-1, the per-release canary of internal/steer/client.go's verified list
# (mirrorVerifiedOn): on the Claude Code installed here, does a PostToolUse and
# a PostToolUseFailure hook's hookSpecificOutput.additionalContext reach the
# model, and its systemMessage reach the user?
#
# Run it by hand, at the keyboard, in a terminal (not through a Claude Code
# `!` prompt: step 2 prints a command to run while it waits), against a
# scratch config directory, never ~/.claude. Log in there once, then run the
# probe:
#
#   CLAUDE_CONFIG_DIR=$HOME/.claude-ptru claude          # /login, then /exit
#   CLAUDE_CONFIG_DIR=$HOME/.claude-ptru test/live/p-tru-1.sh /path/to/artifacts
#
# It refuses to run when CLAUDE_CONFIG_DIR is unset or is ~/.claude, when that
# directory's settings.json holds hooks or enabled plugins, when the machine
# has a managed-settings.json (their hooks would fire beside the probe's), or
# when the artifact directory already holds an artifact. It installs nothing:
# no rashomon hook, no consent, no write under ~/.claude. The probe hook is
# this shell script's own, handed to each session with --settings; it reads
# nothing from its payload and prints a token per event carrying that run's
# nonce.
#
# Two sessions, the same frozen prompt: run `true` (PostToolUse) and `false`
# (PostToolUseFailure) with the Bash tool and repeat every PTRU- line shown.
#   1. claude -p (entrypoint sdk-cli), run here: the control. Its artifact is
#      sdk-cli.json; when its PostToolUse context is not delivered and echoed,
#      nothing is proved and the script exits 2 before step 2.
#   2. An interactive session (entrypoint cli), the surface users run: the
#      script prints one command to run in another terminal, waits until that
#      session ends (PTRU_WAIT seconds, default 900), and writes cli.json.
# test/live/p_tru_1.py reads each transcript and writes its artifact: per event
# fired, delivered (the context token in what the client rendered for the
# model), echoed (the model repeated it), mirror (the systemMessage recorded
# for the user); the version, the entrypoint, the scratch settings' key names,
# and the transcript's path and sha256.
#
# The transcripts stay where Claude Code wrote them, under the scratch config
# directory, so each artifact's sha256 can be checked; the rest of the scratch
# is removed on exit.
set -eu
DIR=${1:-}
if [ -z "$DIR" ]; then
  echo "usage: CLAUDE_CONFIG_DIR=<scratch> $0 <artifact directory>" >&2; exit 1
fi
CFG=${CLAUDE_CONFIG_DIR:-}
if [ -z "$CFG" ]; then
  echo "refusing: set CLAUDE_CONFIG_DIR to a scratch config directory you have logged in under" >&2; exit 1
fi
if [ ! -d "$CFG" ]; then
  echo "refusing: $CFG is not a directory; log in there first" >&2; exit 1
fi
CFG=$(cd "$CFG" && pwd)
if [ "$(cd "$CFG" && pwd -P)" = "$(cd "$HOME" && pwd -P)/.claude" ]; then
  echo "refusing: CLAUDE_CONFIG_DIR is ~/.claude; use a scratch directory" >&2; exit 1
fi
for m in "/Library/Application Support/ClaudeCode/managed-settings.json" /etc/claude-code/managed-settings.json; do
  if [ -e "$m" ]; then
    echo "refusing: $m exists; managed settings reach every session, and their hooks would fire beside the probe's" >&2; exit 1
  fi
done
for tool in claude python3; do
  if ! command -v "$tool" > /dev/null 2>&1; then
    echo "refusing: no $tool on PATH" >&2; exit 1
  fi
done
HERE=$(cd "$(dirname "$0")" && pwd)
KEYS=$(python3 -I "$HERE/p_tru_1.py" settings "$CFG/settings.json")
for a in sdk-cli.json cli.json; do
  if [ -e "$DIR/$a" ]; then
    echo "refusing: $DIR/$a exists" >&2; exit 1
  fi
done
mkdir -p "$DIR"
WAIT=${PTRU_WAIT:-900}
CLAUDE_BIN=$(command -v claude)
T=$(mktemp -d "${TMPDIR:-/tmp}/ptru.XXXXXX")
trap 'rm -rf "$T"' EXIT
echo "claude --version: $("$CLAUDE_BIN" --version)"

cat > "$T/probe.sh" <<'SH'
#!/bin/sh
cat > /dev/null
printf '{"systemMessage": "PTRU-MIRROR-%s-%s", "hookSpecificOutput": {"hookEventName": "%s", "additionalContext": "PTRU-CONTEXT-%s-%s"}}\n' "$1" "$2" "$1" "$1" "$2"
SH
chmod +x "$T/probe.sh"
cat > "$T/prompt.txt" <<'TXT'
This session checks hook delivery. Use the Bash tool to run exactly: true
Then use the Bash tool to run exactly: false
Then reply with every line beginning PTRU- that you were shown after either call, verbatim, one per line, and nothing else.
TXT
mkdir "$T/work"

# session NAME: a nonce, a session id and a settings file whose probe prints
# that nonce, as $T/NAME.nonce, $T/NAME.sid and $T/NAME.json.
session() {
  python3 -I -c 'import secrets; print(secrets.token_hex(8))' > "$T/$1.nonce"
  python3 -I -c 'import uuid; print(uuid.uuid4())' > "$T/$1.sid"
  n=$(cat "$T/$1.nonce")
  cat > "$T/$1.json" <<JSON
{"hooks": {
  "PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "$T/probe.sh PostToolUse $n", "timeout": 10}]}],
  "PostToolUseFailure": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "$T/probe.sh PostToolUseFailure $n", "timeout": 10}]}]
}}
JSON
}

# report NAME [--stream FILE]: the artifact for session NAME's transcript.
report() {
  name=$1
  shift
  tr=$(find "$CFG/projects" -name "$(cat "$T/$name.sid").jsonl" -type f 2>/dev/null | head -n 1)
  if [ -z "$tr" ]; then
    echo "no transcript for session $(cat "$T/$name.sid") under $CFG/projects" >&2; return 1
  fi
  python3 -I "$HERE/p_tru_1.py" report "$@" --transcript "$tr" --nonce "$(cat "$T/$name.nonce")" \
    --settings-keys "$KEYS" --out "$DIR/$name.json"
}

echo "step 1 of 2, claude -p (sdk-cli): the control"
session sdk-cli
rc=0
(cd "$T/work" && CLAUDE_CONFIG_DIR="$CFG" "$CLAUDE_BIN" -p "$(cat "$T/prompt.txt")" --session-id "$(cat "$T/sdk-cli.sid")" \
  --settings "$T/sdk-cli.json" --allowedTools "Bash(true),Bash(false)" --max-turns 6 \
  --output-format stream-json --verbose < /dev/null > "$T/stream.ndjson") || rc=$?
echo "claude -p: exit $rc"
rc=0
report sdk-cli --stream "$T/stream.ndjson" || rc=$?
if [ "$rc" -ne 0 ]; then
  echo "step 1 proved nothing (exit $rc); no interactive step" >&2; exit "$rc"
fi

session cli
cat > "$T/run.sh" <<SH
#!/bin/sh
cd "$T/work" || exit 1
rc=0
CLAUDE_CONFIG_DIR="$CFG" "$CLAUDE_BIN" --session-id "$(cat "$T/cli.sid")" --settings "$T/cli.json" \\
  --allowedTools "Bash(true),Bash(false)" "\$(cat "$T/prompt.txt")" || rc=\$?
echo "\$rc" > "$T/ended"
SH
cat <<EOF

step 2 of 2, the cli surface. In another terminal, run:

  sh $T/run.sh

It starts Claude Code with the frozen prompt below as its first message.
Accept the folder-trust question if it is asked, type nothing else, and once
the reply is printed type /exit.

$(sed 's/^/  | /' "$T/prompt.txt")

Waiting up to $WAIT s for that session to end.
EOF
waited=0
while [ ! -e "$T/ended" ]; do
  if [ "$waited" -ge "$WAIT" ]; then
    echo "no interactive session ended within $WAIT s; step 2 not run" >&2; exit 1
  fi
  sleep 1
  waited=$((waited + 1))
done
echo "interactive claude: exit $(cat "$T/ended")"
report cli
