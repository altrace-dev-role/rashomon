#!/bin/sh
# run.sh ARM NAME: one claude -p session of slice T1 in a fresh scratch worktree of the pinned tree.
#   ARM  U (unaided) | R (runtime: RK_BIN_R, RK_PGRAPH) | G (AutoSteer: RK_BIN_G, RK_PGRAPH_G, RK_KNOWLEDGE)
#   NAME a new run name; the run directory is $RK_RUNS/NAME (default /tmp/rk-autosteer/runs/NAME)
# Environment: RK_CONFIG_DIR (required; logged in; no CLAUDE.md), RK_REPO (default: this repository),
# RK_RUNS, RK_GOCACHE (shared by concurrent runs), RK_MAX_TURNS and RK_BUDGET_USD (defaults in t1/pins.txt),
# RK_GO_TOOLCHAIN_BIN (R, G: a go >= 1.26 bin dir for pgraph sync, which pins GOTOOLCHAIN=local).
# The Claude Code marker variables (CLAUDECODE, CLAUDE_CODE_ENTRYPOINT, AI_AGENT) are cleared for every child, so a
# run launched from inside a Claude Code session behaves as one launched from a terminal; watch --yes refuses otherwise.
set -u
ARM=${1:?ARM}; NAME=${2:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/t1/pins.txt"
RK_REPO=${RK_REPO:-$(cd "$HERE/../../.." && pwd)}
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
RK_GOCACHE=${RK_GOCACHE:-/tmp/rk-autosteer/gocache}
RK_MAX_TURNS=${RK_MAX_TURNS:-$T1_MAX_TURNS}
RK_BUDGET_USD=${RK_BUDGET_USD:-$T1_BUDGET_USD}
case "$ARM" in U|R|G) ;; *) echo "run.sh: ARM is U, R or G" >&2; exit 2;; esac
[ -n "${RK_CONFIG_DIR:-}" ] || { echo "run.sh: RK_CONFIG_DIR unset" >&2; exit 2; }
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { echo "run.sh: $RK_CONFIG_DIR holds a CLAUDE.md; no arm loads instructions" >&2; exit 2; }
D=$RK_RUNS/$NAME
[ -e "$D" ] && { echo "run.sh: $D exists; pick a new NAME" >&2; exit 2; }
mkdir -p "$D" "$RK_GOCACHE"
echo "$ARM" > "$D/arm"
git -C "$RK_REPO" worktree add -q --detach "$D/tree" "$T1_TREE_SHA" || exit 1
[ "$(git -C "$D/tree" rev-parse HEAD)" = "$T1_TREE_SHA" ] || { echo "run.sh: tree is not at $T1_TREE_SHA" >&2; exit 1; }
git -C "$D/tree" rev-parse HEAD > "$D/tree-sha"
SID=$(uuidgen | tr 'A-Z' 'a-z'); echo "$SID" > "$D/session-id"
PROMPT=$(cat "$HERE/t1/prompt.txt")
printf '%s' "$PROMPT" > "$D/prompt.txt"
BIN=
case "$ARM" in R) BIN=${RK_BIN_R:?RK_BIN_R};; G) BIN=${RK_BIN_G:?RK_BIN_G};; esac
if [ -n "$BIN" ]; then
  export RASHOMON_HOME="$D/store" RK_BIN="$BIN" RK_RUN_DIR="$D"
  mkdir -p "$RASHOMON_HOME"
  sh "$HERE/wrap.sh" install || exit 1
  trap 'sh "$HERE/wrap.sh" restore' EXIT
fi
# R and G both sync the platform graph before the session, as the shipped plugin does at SessionStart (hooks.json runs
# pgraph sync -hook; rashomon watch installs no such entry). R syncs with the pinned pgraph (RK_PGRAPH, T1_PGRAPH_SHA);
# G with the arm's own pgraph (RK_PGRAPH_G, T1_G_PGRAPH_SHA), whose sidecars its binary reads.
if [ "$ARM" = R ] || [ "$ARM" = G ]; then
  PG=${RK_PGRAPH:?RK_PGRAPH}; [ "$ARM" = G ] && PG=${RK_PGRAPH_G:-$RK_PGRAPH}
  ( PATH=${RK_GO_TOOLCHAIN_BIN:-/nonexistent}:$PATH "$PG" sync "$D/tree" ) > "$D/pgraph-sync.txt" 2>&1 || { echo "run.sh: pgraph sync failed" >&2; exit 1; }
fi
if [ "$ARM" = G ]; then
  mkdir -p "$D/tree/.rashomon"; cp "$HERE/t1/graph.json" "$D/tree/.rashomon/graph.json"
  "$BIN" graph index "$D/tree" > "$D/index.txt" 2>&1 || { echo "run.sh: graph index failed" >&2; exit 1; }
  if [ -n "${RK_KNOWLEDGE:-}" ]; then
    # Every knowledge file must be at a pinned digest (T1_KNOWLEDGE_*_SHA256), so the rows arm G imports are the frozen ones.
    for k in $RK_KNOWLEDGE; do
      sum=$(shasum -a 256 "$k" | cut -d' ' -f1)
      [ "$sum" = "$T1_KNOWLEDGE_LEDGER_SHA256" ] || [ "$sum" = "$T1_KNOWLEDGE_NOTES_SHA256" ] || { echo "run.sh: $k is at digest $sum, not a pinned one" >&2; exit 1; }
    done
    (cd "$D/tree" && "$BIN" graph knowledge import $RK_KNOWLEDGE) > "$D/knowledge.txt" 2>&1 || { echo "run.sh: knowledge import failed" >&2; exit 1; }
  fi
fi
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/started"
( cd "$D/tree" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT -u CLAUDE_PROJECT_DIR CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" RASHOMON_HOLDOUT=off GOTOOLCHAIN=go1.27.3 GOCACHE="$RK_GOCACHE" \
    claude -p "$PROMPT" --model "$T1_MODEL" --session-id "$SID" \
    --allowedTools "$T1_ALLOWED_TOOLS" --permission-mode acceptEdits \
    --max-turns "$RK_MAX_TURNS" --max-budget-usd "$RK_BUDGET_USD" --output-format json \
    < /dev/null > "$D/result.json" 2> "$D/stderr.txt" )
echo "$?" > "$D/exit"
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/ended"
for f in "$RK_CONFIG_DIR"/projects/*/"$SID".jsonl; do [ -f "$f" ] && cp "$f" "$D/transcript.jsonl"; done
git -C "$D/tree" status --porcelain | grep -v -E '^\?\? \.(rashomon|pgraph)/' > "$D/status.txt"
git -C "$D/tree" diff > "$D/diff.patch"
git -C "$D/tree" ls-files --others --exclude-standard | grep -v -E '^\.(rashomon|pgraph)/' > "$D/new-files.txt"
[ -n "$BIN" ] && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT "$BIN" report --session "$SID" > "$D/report.txt" 2>&1
echo "$NAME ($ARM): exit $(cat "$D/exit") $(cat "$D/started") to $(cat "$D/ended"); transcript $( [ -f "$D/transcript.jsonl" ] && wc -l < "$D/transcript.jsonl" | tr -d ' ' || echo none) lines; changed $(wc -l < "$D/status.txt" | tr -d ' ') paths"
