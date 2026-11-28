#!/bin/sh
# j1live.sh NAME: J1-LIVE (pre-registration r3 section 8): one interactive session on G's binary, in plan mode, driven
# through tmux; the first plan rejected once, the second accepted; the plan line read on the POST channel only (the
# director 2026-11-11 16:40:01 PST). n=1, a demonstration, cap 0.50 USD.
# run.sh G NAME runs unchanged, so G's setup is the batch's: the worktree at T1_TREE_SHA, wrap.sh install (watch --yes,
# consent --yes, hook.sh), pgraph sync with RK_PGRAPH_G, graph.json, the seed, the managed spec when RK_MANAGED is set,
# graph index, the knowledge import and the report. Only the CLI differs: RK_CLAUDE_BIN is claude-tmux.sh, which
# answers --version from the pinned CLI and hands run.sh's arguments to drive.py. Then read.py writes RUN/j1-read.txt.
# Refuses: without RK_T2_J1_GO=yes; with T1_G_BIN_SHA or T1_G_PGRAPH_SHA TBD; when RK_BIN_G or RK_PGRAPH_G does not
# embed its pin unmodified (go version -m: vcs.revision, vcs.modified=false); when either meter reading (two, 20 s
# apart) is over T2_METER_LAUNCH_PCT (a launch: pins.txt); when the configuration directory's account is not the meter's (B454) or auth status
# is not loggedIn true; when its settings.json holds hooks (a run is in flight), a t2 batch runs, or another J1-LIVE
# session runs; when tmux (RK_TMUX, default /opt/homebrew/bin/tmux) is missing. The six index pins may be TBD:
# J1-LIVE runs after the re-cut and before the smoke (r3 QA-ADD 1), because the interactive session writes onboarding
# and folder-trust state into the one configuration directory; its .claude.json key names, hasCompletedOnboarding,
# theme and project count are logged before and after (no account field's value).
# Required environment, as batch.sh's: RK_RUNS, RK_GOCACHE_BASE, RK_BIN_G, RK_PGRAPH, RK_PGRAPH_G, RK_KNOWLEDGE,
# RK_SEED, RK_GO_TOOLCHAIN_BIN, RK_METER_FILE; RK_MANAGED optional. The launch log is RK_RUNS/NAME.j1live.txt, copied
# to RUN/j1/j1live.txt at the end. The run gets its own APFS clone RK_RUNS/NAME.gocache of RK_GOCACHE_BASE, removed
# by its exact path after the run (addendum 5 5c), as batch.sh's one_run.
set -u
NAME=${1:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
T2=$(cd "$HERE/.." && pwd)
H=$(cd "$T2/.." && pwd)
no() { echo "j1live.sh: $*" >&2; exit 2; }
[ "${RK_T2_J1_GO:-}" = yes ] || no "RK_T2_J1_GO is not yes"
. "$T2/pins.txt"
{ [ "$T1_G_BIN_SHA" != TBD ] && [ "$T1_G_PGRAPH_SHA" != TBD ]; } || no "T1_G_BIN_SHA or T1_G_PGRAPH_SHA is TBD; re-cut first"
[ -z "${RASHOMON_MANAGED_SETTINGS_PATH+x}" ] || no "RASHOMON_MANAGED_SETTINGS_PATH is set in the launching environment; only run.sh sets it"
for v in RK_RUNS RK_GOCACHE_BASE RK_BIN_G RK_PGRAPH RK_PGRAPH_G RK_KNOWLEDGE RK_SEED RK_GO_TOOLCHAIN_BIN RK_METER_FILE; do
  eval "val=\${$v:-}"; [ -n "$val" ] || no "$v unset"
done
# One path form (addendum 8): RK_RUNS by its physical path, as run.sh resolves it, so D and the log are the run's.
mkdir -p "$RK_RUNS" && RK_RUNS=$(cd "$RK_RUNS" && pwd -P) || no "RK_RUNS [$RK_RUNS] cannot be made or resolved"
if [ -n "${RK_CONFIG_DIR:-}" ] && [ "$(cd -- "$RK_CONFIG_DIR" 2> /dev/null && pwd -P)" != "$T1_CONFIG_DIR" ]; then no "RK_CONFIG_DIR is $RK_CONFIG_DIR; pins.txt names $T1_CONFIG_DIR"; fi
C=$T1_CONFIG_DIR
D=$RK_RUNS/$NAME
LOG=$RK_RUNS/$NAME.j1live.txt
[ -e "$D" ] || [ -e "$LOG" ] && no "$D or $LOG exists; pick a new NAME"
GC=$RK_RUNS/$NAME.gocache
[ -e "$GC" ] && no "$GC exists; pick a new NAME"
[ -d "$RK_GOCACHE_BASE" ] || no "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not a directory"
export PATH="$RK_GO_TOOLCHAIN_BIN:$PATH"
pin() { go version -m "$1" 2> /dev/null | awk -v s="$2" '$2 == "vcs.revision=" s { r = 1 } $2 == "vcs.modified=false" { m = 1 } END { exit !(r && m) }'; }
pin "$RK_BIN_G" "$T1_G_BIN_SHA" || no "$RK_BIN_G does not embed vcs.revision=$T1_G_BIN_SHA unmodified"
pin "$RK_PGRAPH_G" "$T1_G_PGRAPH_SHA" || no "$RK_PGRAPH_G does not embed vcs.revision=$T1_G_PGRAPH_SHA unmodified"
TMUXB=${RK_TMUX:-/opt/homebrew/bin/tmux}
[ -x "$TMUXB" ] || no "no tmux at $TMUXB"
[ -x "$T1_CLAUDE_BIN" ] || no "$T1_CLAUDE_BIN is not executable"
CV=$("$T1_CLAUDE_BIN" --version 2> /dev/null | awk '{ print $1 }')
[ "$CV" = "$(echo "$T1_CLAUDE_VERSION" | tr - .)" ] || no "$T1_CLAUDE_BIN is version [$CV], pinned $T1_CLAUDE_VERSION"
acct() { python3 -I -B -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("accountUuid"))' "$1" 2>&1; }
[ "$(acct "$C/.claude.json")" = "$(acct "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")" ] || no "the configuration's account is not the meter's (B454)"
AUTH=$(env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR="$C" "$T1_CLAUDE_BIN" auth status 2>&1)
printf '%s\n' "$AUTH" | python3 -I -B -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("loggedIn") is True else 1)' 2> /dev/null || no "auth status on $C is not loggedIn true"
python3 -I -B -c 'import json,os,sys; p=sys.argv[1]; sys.exit(1 if os.path.exists(p) and json.load(open(p)).get("hooks") else 0)' "$C/settings.json" || no "$C/settings.json holds hooks; a run is in flight"
pgrep -f "autosteer/t2/batch.sh" > /dev/null && no "a t2 batch is running"
pgrep -f "t2/j1live/drive.py" > /dev/null && no "another J1-LIVE session is running"
# The meter gate (the re-cut: q4-answer.txt items 1-4; the director 14:13 PST 11-14 3a, 3d): t2/meter.py at the launch
# line; a HOLD re-reads every 30 s up to T2_METER_HOLD_MAX_S. Its ledger is this launch's own RK_RUNS/NAME.meter.txt,
# so no D: the five-hour line is T2_METER_5H_LAUNCH_PCT. m1 and m2 print weekly/five-hour.
meter() { python3 -I -B "$T2/meter.py" read "$RK_METER_FILE" 2>&1; }
mg=$(python3 -I -B "$T2/meter.py" gate --file "$RK_METER_FILE" --ledger "$RK_RUNS/$NAME.meter.txt" --name "$NAME" --weekly "${T2_METER_LAUNCH_PCT:-}" --weekly-key T2_METER_LAUNCH_PCT --max-age "${T2_METER_MAX_AGE_S:-}" --hold-max "${T2_METER_HOLD_MAX_S:-}" --five-line "${T2_METER_5H_PCT:-}" --mult "${T2_METER_5H_MULT:-}" --five-launch "${T2_METER_5H_LAUNCH_PCT:-}"); mrc=$?
[ "$mrc" = 0 ] || no "METER STOP (meter.py gate rc $mrc): ${mg:-the lines above}"
m1=$(echo "$mg" | awk '{ print $2 "/" $4 }'); m2=$(echo "$mg" | awk '{ print $3 "/" $5 }')
say() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LOG"; }
cfgflags() {
  python3 -I -B - "$C/.claude.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("  keys %s" % " ".join(sorted(d)))
print("  hasCompletedOnboarding %s; theme %s; projects %d" % (d.get("hasCompletedOnboarding"), d.get("theme"), len(d.get("projects") or {})))
PY
}
say "=== J1-LIVE $NAME; meter $m1 then $m2 (launch line $T2_METER_LAUNCH_PCT); claude $CV at $T1_CLAUDE_BIN; G $T1_G_BIN_SHA; G pgraph $T1_G_PGRAPH_SHA; tmux $("$TMUXB" -V); config $C; harness $(git -C "$H" rev-parse --short HEAD 2> /dev/null)"
say "config .claude.json before:"; cfgflags 2>&1 | tee -a "$LOG"
# Go build cache (addendum 5 5c; the director 14:15 PST (i) and 15:29 PST item 4): this run's own APFS clone of
# RK_GOCACHE_BASE, exported as GOCACHE as well as RK_GOCACHE, so run.sh's setup steps write to the clone and not to
# the launching shell's cache. Dropped by gcdrop (batch.sh's, byte for byte) before the end line, and on EXIT, INT
# and TERM (j2probe.sh's traps). The traps are set only after the path is checked absent here, so the drop never
# removes a path this run did not make; the early check above refuses before anything is written.
# Base check (addendum 11, 7a; the director 16:32 PST (b)): batch.sh's four lines (:261-264) with two declared
# substitutions only, "$HERE/cachemanifest.py" to "$T2/cachemanifest.py" and "batch refused" to "j1live refused";
# t2-skel/a11-basecheck-equal.py holds the three copies equal. Then the device preflight (16:32 PST (b)): RK_RUNS
# and the base on one APFS device, where cp -c -R can only clone (cp -c otherwise falls back to a byte copy, B684).
# Addendum 11 7a, ruled 11-13 10:18 PST item 12: the base must be byte-equal to its own physical path (pwd -P).
[ ! -d "$RK_GOCACHE_BASE" ] || { gbp=$(cd -- "$RK_GOCACHE_BASE" && pwd -P); [ "$RK_GOCACHE_BASE" = "$gbp" ]; } || { say "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not its physical path [$gbp]; refused, not followed"; exit 2; }
[ -d "$RK_GOCACHE_BASE" ] || { say "RK_GOCACHE_BASE $RK_GOCACHE_BASE is not a directory"; exit 2; }
gm=$RK_RUNS/t2-gocache-base-$(date -u +%Y%m%dT%H%M%SZ)-$$.manifest
gml=$(python3 -I -B "$T2/cachemanifest.py" "$RK_GOCACHE_BASE" "$gm" 2>&1); say "gocache base $RK_GOCACHE_BASE: $gml ($gm)"
[ "$(printf '%s\n' "$gml" | sed -n 's/.* manifest_sha256 \([0-9a-f]*\)$/\1/p')" = "$T2_GOCACHE_BASE_MANIFEST_SHA256" ] || { say "the gocache base manifest is not T2_GOCACHE_BASE_MANIFEST_SHA256 (amend-gocache); j1live refused"; exit 2; }
devr=$(stat -f %d "$RK_RUNS"); devb=$(stat -f %d "$RK_GOCACHE_BASE/."); devm=$(df -P "$RK_RUNS" | awk 'NR==2 { print $NF }')
[ -n "$devr" ] && [ "$devr" = "$devb" ] && mount | grep -q -F " on $devm (apfs," || { say "RK_RUNS $RK_RUNS (device $devr, mounted on $devm) and RK_GOCACHE_BASE $RK_GOCACHE_BASE (device $devb) are not one APFS device: cp -c -R would byte-copy (B684); j1live refused"; exit 2; }
GCD=$D; GCBK=$(du -sk "$RK_GOCACHE_BASE" | cut -f1)
[ -e "$GC" ] && { say "HARNESS: $GC exists before its clone; not touched"; exit 2; }
gcdrop() { [ -n "${GC:-}" ] || return 0; case "$GC" in "$RK_RUNS"/*.gocache) ;; *) say "HARNESS: the gocache clone path [$GC] is not RK_RUNS/NAME.gocache; not removed"; GC=; return 1 ;; esac; if [ -d "$GC" ]; then gk=$(du -sk "$GC" | cut -f1); [ -d "${GCD:-}" ] && printf 'gocache-clone-kib %s\ngocache-base-kib %s\n' "$gk" "${GCBK:-unread}" >> "$GCD/knobs"; rm -rf "$GC"; [ ! -e "$GC" ] || { say "HARNESS: the gocache clone $GC is still present after its drop"; GC=; return 1; }; say "gocache clone $GC removed ($gk KiB; base ${GCBK:-unread} KiB)"; fi; GC=; }
trap gcdrop EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
cp -c -R "$RK_GOCACHE_BASE" "$GC" || { say "HARNESS: the gocache clone $GC failed"; exit 1; }
export RK_GOCACHE="$GC" GOCACHE="$GC"
say "gocache clone $GC of $RK_GOCACHE_BASE ($GCBK KiB); GOCACHE and RK_GOCACHE exported"
export RK_SLICE=t2 RK_CONFIG_DIR="$C" RK_CLAUDE_BIN="$HERE/claude-tmux.sh" RK_CLAUDE_VERSION="$T1_CLAUDE_VERSION" \
  J1_REAL_CLAUDE="$T1_CLAUDE_BIN" J1_TMUX="$TMUXB" RK_PROMPT_FILE="$HERE/prompt.txt"
unset RK_MODEL RK_MAX_TURNS RK_BUDGET_USD
say "run.sh G $NAME (RK_CLAUDE_BIN $RK_CLAUDE_BIN; prompt $(shasum -a 256 "$RK_PROMPT_FILE" | cut -c1-16); RK_MANAGED ${RK_MANAGED:-unset})"
caffeinate -i sh "$H/run.sh" G "$NAME" 2>&1 | tee -a "$LOG"
[ -d "$D" ] && printf 'gocache-base %s\ngocache-clone %s\n' "$RK_GOCACHE_BASE" "$GC" >> "$D/knobs"
[ -d "$D" ] || { say "run.sh made no run directory"; exit 1; }
say "drive.py exit $(cat "$D/exit" 2> /dev/null || echo none)"
python3 -I -B "$HERE/read.py" "$D" > "$D/j1-read.txt" 2>&1
say "read.py rc=$? into $D/j1-read.txt:"
grep -E '^(REJECTED_EXIT_LINE|PLAN_POST_LINE_FOR_ACCEPTED_EXIT|RECORD_ORDER_ACCEPTED|PLAN_PRE_LINE|cost:)' "$D/j1-read.txt" | tee -a "$LOG"
say "config .claude.json after:"; cfgflags 2>&1 | tee -a "$LOG"
gcdrop; grc=$?
say "=== J1-LIVE $NAME end; meter $(meter)"
mkdir -p "$D/j1" && cp "$LOG" "$D/j1/j1live.txt"
crc=$?; [ "$grc" = 0 ] || { echo "j1live.sh: GOCACHE DROP FAILED for $RK_RUNS/$NAME.gocache (see $LOG); a harness failure" >&2; exit 1; }; exit "$crc"
