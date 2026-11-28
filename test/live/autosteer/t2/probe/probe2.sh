#!/bin/sh
# probe2.sh NAME: T2 probe 2 (the director, 2026-11-11 15:50:40 PST item 3), the channel question the engine lane's
# hook half turns on. One headless `claude -p` session in acceptEdits, in plan-probe.sh's scratch one-file Go
# repository, with a context hook (hookctx.py) loaded through --settings for this session only, so no configuration
# directory's settings.json is edited and no rashomon binary is involved. On the edit, PreToolUse returns
# hookSpecificOutput.additionalContext "probe-a-<nonce A>" and PostToolUse "probe-b-<nonce B>". probe2-read.py prints
# PRE_CONTEXT_REACHES_MODEL and POST_CONTEXT_REACHES_MODEL from the transcript's attachment records.
# Deltas from plan-probe.sh: acceptEdits for plan; two nonces for one; the hook emits on Edit, Write and MultiEdit (not
# ExitPlanMode); caps 6 turns and 0.20 USD (the director's cap) for 10 and 0.50; RK_T2_PROBE2_GO for RK_T2_PROBE_GO.
# Environment as plan-probe.sh's: RK_CONFIG_DIR (logged in, no CLAUDE.md, no hooks in settings.json, accountUuid equal
# to RK_METER_ACCOUNT_JSON's, default ~/.claude.json: B454), RK_CLAUDE_BIN (an explicit CLI path), RK_RUNS (default
# /tmp/rk-autosteer/runs), RK_MODEL (default claude-sonnet-5), RK_METER_FILE (optional, logged at start and end).
set -u
NAME=${1:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "${RK_T2_PROBE2_GO:-}" = yes ] || { echo "probe2.sh: RK_T2_PROBE2_GO is not yes; nothing runs without the director's word" >&2; exit 2; }
[ -n "${RK_CONFIG_DIR:-}" ] || { echo "probe2.sh: RK_CONFIG_DIR unset" >&2; exit 2; }
[ -x "${RK_CLAUDE_BIN:-}" ] || { echo "probe2.sh: RK_CLAUDE_BIN is not an executable path" >&2; exit 2; }
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
RK_MODEL=${RK_MODEL:-claude-sonnet-5}
D=$RK_RUNS/$NAME
[ -e "$D" ] && { echo "probe2.sh: $D exists; pick a new NAME" >&2; exit 2; }
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { echo "probe2.sh: $RK_CONFIG_DIR holds a CLAUDE.md" >&2; exit 2; }
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(1 if d.get("hooks") else 0)' "$RK_CONFIG_DIR/settings.json" \
  || { echo "probe2.sh: $RK_CONFIG_DIR/settings.json holds hooks; the probe's must be the only ones" >&2; exit 2; }
acct() { python3 -I -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("accountUuid"))' "$1"; }
[ "$(acct "$RK_CONFIG_DIR/.claude.json")" = "$(acct "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")" ] \
  || { echo "probe2.sh: config account is not the meter's (B454)" >&2; exit 2; }
meter() { if [ -n "${RK_METER_FILE:-}" ] && [ -f "$RK_METER_FILE" ]; then cat "$RK_METER_FILE"; else echo "no meter file"; fi; }

mkdir -p "$D/tree"
env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT "$RK_CLAUDE_BIN" --version > "$D/claude-version" 2>&1
echo "$RK_MODEL" > "$D/model"
meter > "$D/meter-start"
printf 'package calc\n\n// Add returns a plus b.\nfunc Add(a, b int) int { return a + b }\n' > "$D/tree/calc.go"
printf 'module example.com/calc\n\ngo 1.25\n' > "$D/tree/go.mod"
git -C "$D/tree" init -q && git -C "$D/tree" add -A && \
  git -C "$D/tree" -c user.name=probe -c user.email=probe@localhost commit -q -m base || { echo "probe2.sh: scratch repo failed" >&2; exit 1; }
NA=$(uuidgen | tr 'A-Z' 'a-z' | cut -c1-8); echo "$NA" > "$D/nonce-a"
NB=$(uuidgen | tr 'A-Z' 'a-z' | cut -c1-8); echo "$NB" > "$D/nonce-b"
[ "$NA" != "$NB" ] || { echo "probe2.sh: the two nonces are equal" >&2; exit 1; }
SID=$(uuidgen | tr 'A-Z' 'a-z'); echo "$SID" > "$D/session-id"
python3 -I - "$HERE/hookctx.py" > "$D/settings.json" <<'PY'
import json, sys
h = sys.argv[1]
def entry(phase):
    return [{"matcher": "Edit|Write|MultiEdit", "hooks": [{"type": "command", "command": "python3 -I '%s' %s" % (h, phase)}]}]
print(json.dumps({"hooks": {"PreToolUse": entry("pre"), "PostToolUse": entry("post")}}, indent=2))
PY
cp "$HERE/probe2-prompt.txt" "$D/prompt.txt"
PROMPT=$(cat "$HERE/probe2-prompt.txt")
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/started"
( cd "$D/tree" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT -u CLAUDE_PROJECT_DIR \
    CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" PROBE_DIR="$D" PROBE_NONCE_A="$NA" PROBE_NONCE_B="$NB" \
    "$RK_CLAUDE_BIN" -p "$PROMPT" --model "$RK_MODEL" --session-id "$SID" --settings "$D/settings.json" \
    --allowedTools "Read,Edit,Write" --permission-mode acceptEdits \
    --max-turns 6 --max-budget-usd 0.20 --output-format json \
    < /dev/null > "$D/result.json" 2> "$D/stderr.txt" )
echo "$?" > "$D/exit"
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/ended"
meter > "$D/meter-end"
for f in "$RK_CONFIG_DIR"/projects/*/"$SID".jsonl; do [ -f "$f" ] && cp "$f" "$D/transcript.jsonl"; done
git -C "$D/tree" diff > "$D/diff.patch"
git -C "$D/tree" status --porcelain > "$D/status.txt"
python3 -I "$HERE/probe2-read.py" "$D"
