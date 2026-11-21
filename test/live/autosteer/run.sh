#!/bin/sh
# run.sh ARM NAME: one claude -p session of slice T1 in a fresh scratch worktree of the pinned tree.
#   ARM  U (unaided) | R (runtime: RK_BIN_R, RK_PGRAPH) | G (AutoSteer: RK_BIN_G, RK_PGRAPH_G, RK_KNOWLEDGE)
#   NAME a new run name; the run directory is $RK_RUNS/NAME (default /tmp/rk-autosteer/runs/NAME)
# Environment: RK_CONFIG_DIR (required; logged in; no CLAUDE.md), RK_REPO (default: this repository),
# RK_RUNS, RK_GOCACHE (shared by concurrent runs), RK_MAX_TURNS and RK_BUDGET_USD (defaults in the slice's pins.txt),
# RK_GO_TOOLCHAIN_BIN (R, G: a go >= 1.26 bin dir for pgraph sync, which pins GOTOOLCHAIN=local).
# RK_SLICE (default t1) names the slice directory whose pins.txt, prompt.txt and graph.json the run uses (t1b: the
# corrected harness's re-run of T1). RK_PROMPT_FILE and RK_MODEL replace the slice's prompt and model for the channel
# smoke only (t1b/smoke.sh); a batch clears them, and the run directory records the slice, prompt and model used.
# A run is launched from a terminal (addendum 11, 13a). With CLAUDECODE set, a launch from inside a Claude Code
# session, run.sh refuses before the run directory: such a session adds names no env -u list is known to cover.
# The marker variables (CLAUDECODE, CLAUDE_CODE_ENTRYPOINT, AI_AGENT) are still cleared for every child (watch --yes
# refuses otherwise). run.sh also refuses when RK_RUNS or any ancestor holds CLAUDE.md, CLAUDE.local.md,
# .claude/CLAUDE.md or .claude/rules (13b): every arm's session works below RK_RUNS and would load it as memory.
# T2 knobs, each unset for T1, T1b and T1c, whose claude command line is therefore unchanged: T1_PERMISSION_MODE
# (default acceptEdits), T1_AGENT_GOTOOLCHAIN (the session's GOTOOLCHAIN, default go1.27.3), RK_CLAUDE_BIN (an explicit
# CLI path; when set, its --version must equal RK_CLAUDE_VERSION read with dots for dashes, the spelling hook.sh puts in
# AI_AGENT, or the run is refused before its directory exists) and T1_MEM_SAMPLE_S (when set, memsample.sh samples this
# script's process tree every that many seconds into mem.tsv). The run directory's knobs file records all four.
# Product inputs go to the arms the slice's T2_PRODUCT_INPUT_ARMS names (unset for T1, T1b and T1c: G alone, so their
# runs are unchanged; T2: R and G, each through its own binary). Those arms get graph.json (at T2_GRAPH_CONFIG_SHA256
# when the slice pins it), `graph index` and the knowledge import, and two T2 inputs, each unset for T1, T1b and T1c:
# RK_SEED (the seeded rows file, at T1_KNOWLEDGE_SEED_SHA256, copied to the tree's .rashomon/knowledge/t2-seed.json
# before `graph index`) and RK_MANAGED (a directory holding rashomon/spec.json at T1_MANAGED_SPEC_SHA256, read in place:
# the file and its directory root-owned and neither group- nor world-writable, by adminok below, or the run is refused
# before its tree (amendment A6); the session and the report run with RASHOMON_MANAGED_SETTINGS_PATH naming
# RK_MANAGED/managed-settings.json, which must not exist, so the settings reader takes it as a layer that sets
# nothing). Every arm's
# session runs with that variable unset first, so no arm sees one from the launching shell. The knobs file records
# seed and managed-spec; index-sums.txt records the sha256 of every file under the tree's .rashomon/ after the index.
set -u
ARM=${1:?ARM}; NAME=${2:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
SLICE=${RK_SLICE:-t1}
[ -f "$HERE/$SLICE/pins.txt" ] || { echo "run.sh: no slice $SLICE under $HERE" >&2; exit 2; }
. "$HERE/$SLICE/pins.txt"
RK_REPO=${RK_REPO:-$(cd "$HERE/../../.." && pwd)}
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
# One path form (addendum 8): RK_RUNS by its physical path, so the session's PWD, its getcwd and every path the agent
# names agree. Under a symlinked root (/tmp is /private/tmp) the product reads a write named by the logical path as
# outside the project: no overlay, no unit brief, no counted write (t1c-r6, 28 of 28 Edit writes;
# internal/hook/overlaywrite.go:307-313, internal/verify/classify.go:449 at 20a64198).
mkdir -p "$RK_RUNS" && RK_RUNS=$(cd "$RK_RUNS" && pwd -P) || { echo "run.sh: RK_RUNS [$RK_RUNS] cannot be made or resolved" >&2; exit 2; }
RK_GOCACHE=${RK_GOCACHE:-/tmp/rk-autosteer/gocache}
RK_MAX_TURNS=${RK_MAX_TURNS:-$T1_MAX_TURNS}
RK_BUDGET_USD=${RK_BUDGET_USD:-$T1_BUDGET_USD}
RK_MODEL=${RK_MODEL:-$T1_MODEL}
RK_PROMPT_FILE=${RK_PROMPT_FILE:-$HERE/$SLICE/prompt.txt}
case "$ARM" in U|R|G) ;; *) echo "run.sh: ARM is U, R or G" >&2; exit 2;; esac
[ -n "${RK_CONFIG_DIR:-}" ] || { echo "run.sh: RK_CONFIG_DIR unset" >&2; exit 2; }
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { echo "run.sh: $RK_CONFIG_DIR holds a CLAUDE.md; no arm loads instructions" >&2; exit 2; }
# Addendum 11, 13a and 13b (the header): no launch from a Claude Code session, no memory file above the run tree.
[ -z "${CLAUDECODE+x}" ] || { echo "run.sh: CLAUDECODE is set, so this is a launch from inside a Claude Code session; launch from a terminal" >&2; exit 2; }
anc=$RK_RUNS
while :; do
  for mf in CLAUDE.md CLAUDE.local.md .claude/CLAUDE.md .claude/rules; do
    [ -e "${anc%/}/$mf" ] && { echo "run.sh: ${anc%/}/$mf exists in RK_RUNS or an ancestor; every arm would load it as memory" >&2; exit 2; }
  done
  up=$(dirname "$anc"); [ "$up" = "$anc" ] && break; anc=$up
done
CLI=${RK_CLAUDE_BIN:-claude}
if [ -n "${RK_CLAUDE_BIN:-}" ]; then
  CV=$("$CLI" --version 2> /dev/null | awk '{ print $1 }')
  [ -n "${RK_CLAUDE_VERSION:-}" ] && [ "$CV" = "$(echo "$RK_CLAUDE_VERSION" | tr - .)" ] || { echo "run.sh: $CLI is version [$CV], RK_CLAUDE_VERSION [${RK_CLAUDE_VERSION:-unset}]" >&2; exit 2; }
fi
D=$RK_RUNS/$NAME
[ -e "$D" ] && { echo "run.sh: $D exists; pick a new NAME" >&2; exit 2; }
mkdir -p "$D" "$RK_GOCACHE"
MS=
if [ -n "${T1_MEM_SAMPLE_S:-}" ]; then sh "$HERE/memsample.sh" "$$" "$D/mem.tsv" "$T1_MEM_SAMPLE_S" & MS=$!; fi
echo "$ARM" > "$D/arm"
echo "$SLICE" > "$D/slice"; echo "$RK_MODEL" > "$D/model"
printf 'cli %s\nclaude-version %s\npermission-mode %s\nagent-gotoolchain %s\nmem-sample-s %s\n' "$CLI" "${RK_CLAUDE_VERSION:-unset}" \
  "${T1_PERMISSION_MODE:-acceptEdits}" "${T1_AGENT_GOTOOLCHAIN:-go1.27.3}" "${T1_MEM_SAMPLE_S:-off}" > "$D/knobs"
INPUTS=no
case " ${T2_PRODUCT_INPUT_ARMS:-G} " in *" $ARM "*) [ "$ARM" = U ] || INPUTS=yes ;; esac
# adminok.sh: the text run.sh gains at the re-cut (pre-run amendment t2-prereg-amend-s1-s3-2026-11-12.txt, part A).
# adminok FILE DIR: discover.go:251-255 (adminOwned, a125cc06) in find(1). FILE a regular file and DIR a directory,
# each owned by uid 0 and neither group- nor world-writable. find without -H or -L does not follow a symlink named
# on its command line, as OSFS.Owner lstats (TestVER41b_OwnerIsLstats), so a symlink at either fails.
adminok() {
  [ -n "$(find "$1" -prune -user 0 -type f ! -perm -020 ! -perm -002 -print 2> /dev/null)" ] &&
    [ -n "$(find "$2" -prune -user 0 -type d ! -perm -020 ! -perm -002 -print 2> /dev/null)" ]
}
MANAGED=
if [ "$INPUTS" = yes ] && [ -n "${RK_MANAGED:-}" ]; then
  case "$RK_MANAGED" in *[[:space:]]*) echo "run.sh: RK_MANAGED holds a space; the managed path is passed unquoted through env" >&2; exit 2;; esac
  sum=$(shasum -a 256 "$RK_MANAGED/rashomon/spec.json" | cut -d' ' -f1)
  [ "$sum" = "${T1_MANAGED_SPEC_SHA256:-}" ] || { echo "run.sh: $RK_MANAGED/rashomon/spec.json is at digest $sum, not T1_MANAGED_SPEC_SHA256" >&2; exit 1; }
  adminok "$RK_MANAGED/rashomon/spec.json" "$RK_MANAGED/rashomon" || { echo "run.sh: $RK_MANAGED/rashomon/spec.json or its directory is not admin-owned (discover.go:251-255); G's binary would ignore it" >&2; exit 2; }
  [ -e "$RK_MANAGED/managed-settings.json" ] && { echo "run.sh: $RK_MANAGED/managed-settings.json exists; the managed settings layer must set nothing" >&2; exit 2; }
  MANAGED=$RK_MANAGED/managed-settings.json
fi
SEEDSUM=off
if [ "$INPUTS" = yes ] && [ -n "${RK_SEED:-}" ]; then
  SEEDSUM=$(shasum -a 256 "$RK_SEED" | cut -d' ' -f1)
  [ "$SEEDSUM" = "${T1_KNOWLEDGE_SEED_SHA256:-}" ] || { echo "run.sh: $RK_SEED is at digest $SEEDSUM, not T1_KNOWLEDGE_SEED_SHA256" >&2; exit 1; }
fi
printf 'seed %s\nmanaged-spec %s\n' "$SEEDSUM" "${MANAGED:-off}" >> "$D/knobs"
git -C "$RK_REPO" worktree add -q --detach "$D/tree" "$T1_TREE_SHA" || exit 1
[ "$(git -C "$D/tree" rev-parse HEAD)" = "$T1_TREE_SHA" ] || { echo "run.sh: tree is not at $T1_TREE_SHA" >&2; exit 1; }
git -C "$D/tree" rev-parse HEAD > "$D/tree-sha"
LP=$(cd "$D/tree" && echo "$PWD"); PP=$(cd "$D/tree" && pwd -P)
{ [ -n "$LP" ] && [ "$LP" = "$PP" ]; } || { echo "run.sh: in $D/tree the session's PWD is [$LP] and the physical path [$PP]; the product would read its writes as outside the project (addendum 8)" >&2; exit 2; }
SID=$(uuidgen | tr 'A-Z' 'a-z'); echo "$SID" > "$D/session-id"
PROMPT=$(cat "$RK_PROMPT_FILE")
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
if [ "$INPUTS" = yes ]; then
  if [ -n "${T2_GRAPH_CONFIG_SHA256:-}" ]; then
    sum=$(shasum -a 256 "$HERE/$SLICE/graph.json" | cut -d' ' -f1)
    [ "$sum" = "$T2_GRAPH_CONFIG_SHA256" ] || { echo "run.sh: $HERE/$SLICE/graph.json is at digest $sum, not T2_GRAPH_CONFIG_SHA256" >&2; exit 1; }
  fi
  mkdir -p "$D/tree/.rashomon"; cp "$HERE/$SLICE/graph.json" "$D/tree/.rashomon/graph.json"
  if [ "$SEEDSUM" != off ]; then
    mkdir -p "$D/tree/.rashomon/knowledge"; cp "$RK_SEED" "$D/tree/.rashomon/knowledge/t2-seed.json"
  fi
  "$BIN" graph index "$D/tree" > "$D/index.txt" 2>&1 || { echo "run.sh: graph index failed" >&2; exit 1; }
  if [ -n "${RK_KNOWLEDGE:-}" ]; then
    # Every knowledge file must be at a pinned digest (T1_KNOWLEDGE_*_SHA256), so the rows arm G imports are the frozen ones.
    for k in $RK_KNOWLEDGE; do
      sum=$(shasum -a 256 "$k" | cut -d' ' -f1)
      [ "$sum" = "$T1_KNOWLEDGE_LEDGER_SHA256" ] || [ "$sum" = "$T1_KNOWLEDGE_NOTES_SHA256" ] || { echo "run.sh: $k is at digest $sum, not a pinned one" >&2; exit 1; }
    done
    (cd "$D/tree" && "$BIN" graph knowledge import $RK_KNOWLEDGE) > "$D/knowledge.txt" 2>&1 || { echo "run.sh: knowledge import failed" >&2; exit 1; }
  fi
  ( cd "$D/tree/.rashomon" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do shasum -a 256 "$f"; done ) > "$D/index-sums.txt"
fi
if [ "${RK_SETUP_ONLY:-}" = yes ]; then printf 'setup-only yes\n' >> "$D/knobs"; echo "$NAME ($ARM): setup only, no session"; exit 0; fi
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/started"
( cd "$D/tree" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT -u CLAUDE_PROJECT_DIR -u RASHOMON_MANAGED_SETTINGS_PATH -u RASHOMON_STEER_ARM ${MANAGED:+RASHOMON_MANAGED_SETTINGS_PATH=$MANAGED} CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" RASHOMON_HOLDOUT=off GOTOOLCHAIN="${T1_AGENT_GOTOOLCHAIN:-go1.27.3}" GOCACHE="$RK_GOCACHE" \
    "$CLI" -p "$PROMPT" --model "$RK_MODEL" --session-id "$SID" \
    --allowedTools "$T1_ALLOWED_TOOLS" --permission-mode "${T1_PERMISSION_MODE:-acceptEdits}" \
    --max-turns "$RK_MAX_TURNS" --max-budget-usd "$RK_BUDGET_USD" --output-format json \
    < /dev/null > "$D/result.json" 2> "$D/stderr.txt" )
echo "$?" > "$D/exit"
date -u +%Y-%m-%dT%H:%M:%SZ > "$D/ended"
[ -n "$MS" ] && { kill "$MS"; wait "$MS"; } 2> /dev/null
for f in "$RK_CONFIG_DIR"/projects/*/"$SID".jsonl; do [ -f "$f" ] && cp "$f" "$D/transcript.jsonl"; done
git -C "$D/tree" status --porcelain | grep -v -E '^\?\? \.(rashomon|pgraph)/' > "$D/status.txt"
git -C "$D/tree" diff > "$D/diff.patch"
git -C "$D/tree" ls-files --others --exclude-standard | grep -v -E '^\.(rashomon|pgraph)/' > "$D/new-files.txt"
[ -n "$BIN" ] && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT -u RASHOMON_MANAGED_SETTINGS_PATH ${MANAGED:+RASHOMON_MANAGED_SETTINGS_PATH=$MANAGED} "$BIN" report --session "$SID" > "$D/report.txt" 2>&1
echo "$NAME ($ARM): exit $(cat "$D/exit") $(cat "$D/started") to $(cat "$D/ended"); transcript $( [ -f "$D/transcript.jsonl" ] && wc -l < "$D/transcript.jsonl" | tr -d ' ' || echo none) lines; changed $(wc -l < "$D/status.txt" | tr -d ' ') paths"
