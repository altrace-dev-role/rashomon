#!/bin/sh
# plan-probe.sh NAME: the T2 plan probe (W-M2.6's "probe at task open"; the director's AC-M2.1/J1 requirement). One
# headless `claude -p` session in plan mode, in a scratch one-file Go repository, with ExitPlanMode in the allowed tools
# and a logging hook (hooklog.py) loaded through --settings for this session only, so no configuration directory's
# settings.json is edited and no rashomon binary is involved. It settles, before any T2 run is priced:
#   (i)   PreToolUse fires on ExitPlanMode under -p;
#   (ii)  the session leaves plan mode and lands the edit (if not, every plan-mode T2 run is void);
#   (iii) the PreToolUse additionalContext line reaches the model (a hook_additional_context attachment in the
#         transcript), and (iv) the PostToolUse one, W-M2.6's fallback channel.
# probe-read.py prints the four verdicts. Caps: 10 turns, 0.50 USD. Runs nothing without RK_T2_PROBE_GO=yes, which only
# the director's word sets. Environment: RK_CONFIG_DIR (logged in, no CLAUDE.md, no hooks in settings.json, accountUuid
# equal to RK_METER_ACCOUNT_JSON's, default ~/.claude.json: B454), RK_CLAUDE_BIN (an explicit CLI path: the version is
# a pin, and the PATH symlink moves), RK_RUNS (default /tmp/rk-autosteer/runs), RK_MODEL (default T2's, claude-sonnet-5),
# RK_METER_FILE (optional, logged at start and end).
set -u
NAME=${1:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "${RK_T2_PROBE_GO:-}" = yes ] || { echo "plan-probe.sh: RK_T2_PROBE_GO is not yes; nothing runs without the director's word" >&2; exit 2; }
[ -n "${RK_CONFIG_DIR:-}" ] || { echo "plan-probe.sh: RK_CONFIG_DIR unset" >&2; exit 2; }
[ -x "${RK_CLAUDE_BIN:-}" ] || { echo "plan-probe.sh: RK_CLAUDE_BIN is not an executable path" >&2; exit 2; }
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
RK_MODEL=${RK_MODEL:-claude-sonnet-5}
D=$RK_RUNS/$NAME
[ -e "$D" ] && { echo "plan-probe.sh: $D exists; pick a new NAME" >&2; exit 2; }
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { echo "plan-probe.sh: $RK_CONFIG_DIR holds a CLAUDE.md" >&2; exit 2; }
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(1 if d.get("hooks") else 0)' "$RK_CONFIG_DIR/settings.json" \
  || { echo "plan-probe.sh: $RK_CONFIG_DIR/settings.json holds hooks; the probe's must be the only ones" >&2; exit 2; }
acct() { python3 -I -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("accountUuid"))' "$1"; }
[ "$(acct "$RK_CONFIG_DIR/.claude.json")" = "$(acct "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")" ] \
  || { echo "plan-probe.sh: config account is not the meter's (B454)" >&2; exit 2; }
meter() { if [ -n "${RK_METER_FILE:-}" ] && [ -f "$RK_METER_FILE" ]; then cat "$RK_METER_FILE"; else echo "no meter file"; fi; }

mkdir -p "$D/tree"
env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT "$RK_CLAUDE_BIN" --version > "$D/claude-version" 2>&1
echo "$RK_MODEL" > "$D/model"
meter > "$D/meter-start"
printf 'package calc\n\n// Add returns a plus b.\nfunc Add(a, b int) int { return a + b }\n' > "$D/tree/calc.go"
printf 'module example.com/calc\n\ngo 1.25\n' > "$D/tree/go.mod"
git -C "$D/tree" init -q && git -C "$D/tree" add -A && \
  git -C "$D/tree" -c user.name=probe -c user.email=probe@localhost commit -q -m base || { echo "plan-probe.sh: scratch repo failed" >&2; exit 1; }
NONCE=$(uuidgen | tr 'A-Z' 'a-z' | cut -c1-8); echo "$NONCE" > "$D/nonce"
SID=$(uuidgen | tr 'A-Z' 'a-z'); echo "$SID" > "$D/session-id"
python3 -I - "$HERE/hooklog.py" > "$D/settings.json" <<'PY'
import json, sys
h = sys.argv[1]
def entry(phase):
    return [{"matcher": "ExitPlanMode|Edit|Write", "hooks": [{"type": "command", "command": "python3 -I '%s' %s" % (h, phase)}]}]
print(json.dumps({"hooks": {"PreToolUse": entry("pre"), "PostToolUse": entry("post")}}, indent=2))
PY
cp "$HERE/probe-prompt.txt" "$D/prompt.txt"
PROMPT=$(cat "$HERE/probe-prompt.txt")
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/started"
( cd "$D/tree" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT -u CLAUDE_PROJECT_DIR \
    CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" PROBE_DIR="$D" PROBE_NONCE="$NONCE" \
    "$RK_CLAUDE_BIN" -p "$PROMPT" --model "$RK_MODEL" --session-id "$SID" --settings "$D/settings.json" \
    --allowedTools "Read,Edit,Write,ExitPlanMode" --permission-mode plan \
    --max-turns 10 --max-budget-usd 0.50 --output-format json \
    < /dev/null > "$D/result.json" 2> "$D/stderr.txt" )
echo "$?" > "$D/exit"
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/ended"
meter > "$D/meter-end"
for f in "$RK_CONFIG_DIR"/projects/*/"$SID".jsonl; do [ -f "$f" ] && cp "$f" "$D/transcript.jsonl"; done
git -C "$D/tree" diff > "$D/diff.patch"
git -C "$D/tree" status --porcelain > "$D/status.txt"
python3 -I "$HERE/probe-read.py" "$D"
