#!/bin/sh
# smoke.sh NAME: T2's smoke (r3 section 6, in addendum 8's order), launched by batch.sh after its preflight with
# RK_T2_SMOKE_ONLY=yes (the six index pins stay TBD until it passes; the batch then launches with RK_T2_SKIP_SMOKE=yes).
# Every free check runs before the first paid run: keys.txt, S-2, the J3 base, then one setup-only run per input arm (R,
# then G; RK_SETUP_ONLY=yes: run.sh stops before the session, so no meter gate) whose index and import output S-3 reads
# against its pin, whose index-sums.txt gives the S-6 pins, and in whose tree S-7's TestJ3 compiles the seed; their trees
# are then removed and free space read again. Then one paid run per arm through run.sh in the order G, R, U, each behind
# a meter gate (two readings, 20 s apart): G, the launch's first run, at T2_METER_LAUNCH_PCT, R and U at
# T2_METER_STOP_PCT (pins.txt), so G's assertions (S-1, S-1b, S-4, S-5) fail before R and U are spent. Every run, setup
# only included, gets its own APFS clone RK_RUNS/NAME.gocache of RK_GOCACHE_BASE (cp -c -R), measured into its knobs and
# removed by its exact path after it, the removal verified. Each paid run is checked (result.json, transcript,
# hooks.log, t1c/channel.py), its tools.txt written, judged by judge/judge.sh into RK_RUNS/t2-smoke-rows.txt, and the
# five index-sums entries batch.sh's INDEX check reads must equal its setup-only run's. Exit 0 only when S-1 to S-8 hold;
# exit 3 on S-3 (the line goes to the director); exit 1 on any other failure. The last line is the verdict, S1 in or
# out, the points per run, the six index pins and the count of pinned S-3 lines not printed.
set -u
NAME=${1:?NAME}
M0=
HERE=$(cd "$(dirname "$0")" && pwd); H=$HERE/..
. "$HERE/pins.txt"; . "$HERE/keys.txt"
fail() { echo "SMOKE FAIL $NAME: $*"; exit 1; }
. "$HERE/s3pin.sh"
if grep -n '=TBD' "$HERE/keys.txt"; then fail "keys.txt holds the TBD values above"; fi
for v in T2_METER_LAUNCH_PCT T2_METER_STOP_PCT; do eval "x=\${$v:-}"; case "$x" in ''|*[!0-9]*) fail "$v is [$x] in pins.txt, not a whole number" ;; esac; done
for v in T2_METER_MAX_AGE_S T2_METER_HOLD_MAX_S T2_METER_5H_PCT T2_METER_5H_LAUNCH_PCT; do eval "x=\${$v:-}"; case "$x" in ''|*[!0-9]*) fail "$v is [$x] in pins.txt, not a whole number" ;; esac; done
case "${T2_METER_5H_MULT:-}" in ''|*[!0-9.]*|.*|*.|*.*.*) fail "T2_METER_5H_MULT is [${T2_METER_5H_MULT:-}] in pins.txt, not a decimal number" ;; esac
for v in RK_RUNS RK_BIN_R RK_BIN_G RK_SEED RK_METER_FILE RK_GOSLOT RK_J3_BASE RK_J3_TREE RK_GOCACHE_BASE; do
  eval "x=\${$v:-}"; [ -n "$x" ] || fail "$v unset"
done
[ ! -d "$RK_GOCACHE_BASE" ] || { gbp=$(cd -- "$RK_GOCACHE_BASE" && pwd -P); [ "$RK_GOCACHE_BASE" = "$gbp" ]; } || fail "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not its physical path [$gbp]; refused, not followed"
[ -d "$RK_GOCACHE_BASE" ] || fail "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not a directory"
# RK_REPO as run.sh defaults it (run.sh:35); the smoke uses it only to remove its setup-only worktrees.
RK_REPO=${RK_REPO:-$(cd "$H/../../.." && pwd)}
# One path form (addendum 8): RK_RUNS by its physical path, as run.sh resolves it, so the smoke's paths are the runs'.
mkdir -p "$RK_RUNS" && RK_RUNS=$(cd "$RK_RUNS" && pwd -P) || fail "RK_RUNS [$RK_RUNS] cannot be made or resolved"
# The meter gate (the re-cut: q4-answer.txt items 1-4; the director 14:13 PST 11-14 items 3a, 3b, 3d): meter.py gate
# reads two readings 20 s apart and HOLDs (re-read every 30 s up to T2_METER_HOLD_MAX_S; over the five-hour rule, one
# wait to resets_at + 60 s first) before it stops; no run starts on a HOLD. Its ledger RK_RUNS/t2-meter.txt, shared
# with batch.sh, holds each run's GO pair, its end and its five-hour delta, so D starts at this smoke's G run (3d).
# mgate LABEL RUN [options]: GO sets P1 P2 (weekly) and MF1 MF2 (five-hour); anything else fails the smoke.
MLEDGER=$RK_RUNS/t2-meter.txt
mgate() {
  mgl=$1; mgn=$2; shift 2
  mg=$(python3 -I -B "$HERE/meter.py" gate --file "$RK_METER_FILE" --ledger "$MLEDGER" --name "$mgn" --max-age "$T2_METER_MAX_AGE_S" --hold-max "$T2_METER_HOLD_MAX_S" --five-line "$T2_METER_5H_PCT" --mult "$T2_METER_5H_MULT" --five-launch "$T2_METER_5H_LAUNCH_PCT" "$@"); mgrc=$?
  echo "meter $mgl: ${mg:-no output} (rc $mgrc)"
  [ "$mgrc" = 0 ] || fail "meter $mgl: no GO from meter.py gate (rc $mgrc; 3 is a HOLD past T2_METER_HOLD_MAX_S, 2 a pins or ledger error); no run started"
  set -- $mg
  { [ "$#" = 7 ] && [ "$1" = GO ]; } || fail "meter $mgl: meter.py gate printed [$mg]"
  P1=$2; P2=$3; MF1=$4; MF2=$5
}
gate() { if [ -z "${M0:-}" ]; then lim=$T2_METER_LAUNCH_PCT; key=T2_METER_LAUNCH_PCT; else lim=$T2_METER_STOP_PCT; key=T2_METER_STOP_PCT; fi; mgate "$1" "$2" --weekly "$lim" --weekly-key "$key"; }
freeg() { df -k "$RK_RUNS" | awk 'NR==2 { printf "%.3f", $4 / 1048576 }'; }
# usedk: the Used KiB (df -k column 3) of the volume holding RK_RUNS (/System/Volumes/Data for /private/tmp). S-8's
# per-run drop reads Used, not Avail: Avail is shared with swap and system snapshots (addendum 8, 4d).
usedk() { df -k "$RK_RUNS" | awk 'NR==2 { print $3 }'; }
dfmnt() { df -k "$RK_RUNS" | awk 'NR==2 { print $NF }'; }
ovs() {
  { find "$1" \( -path "$1/judge" -o -path "$1/tree" \) -prune -o -type f -name "$T2K_OVERLAY_GLOB" -print
    find "$1/tree/.rashomon" "$1/tree/.pgraph" -type f -name "$T2K_OVERLAY_GLOB" 2> /dev/null; } | wc -l | tr -d ' '
}
# Go build cache per run (addendum 8): GCBK the base's KiB; GCMAX the largest growth of a paid run's clone over it.
GCBK=$(du -sk "$RK_GOCACHE_BASE" | cut -f1); GC=; GCD=; GCMAX=0
# gclone RUN: the run's clone RK_RUNS/RUN.gocache, refused when the path exists (GC cleared, so nothing of another's is
# removed). A failed copy keeps GC, so the EXIT trap removes the partial clone.
gclone() {
  GC=$RK_RUNS/$1.gocache; GCD=$RK_RUNS/$1
  if [ -e "$GC" ]; then GC=; fail "the gocache clone $RK_RUNS/$1.gocache exists before its run"; fi
  cp -c -R "$RK_GOCACHE_BASE" "$GC" || fail "the gocache clone $GC failed"
}
# gcdrop: as batch.sh's; gk is the clone's KiB at its drop, empty when there was no clone to measure.
gcdrop() { gk=; [ -n "${GC:-}" ] || return 0; case "$GC" in "$RK_RUNS"/*.gocache) ;; *) echo "HARNESS: the gocache clone path [$GC] is not RK_RUNS/NAME.gocache; not removed"; GC=; return 1 ;; esac; if [ -d "$GC" ]; then gk=$(du -sk "$GC" | cut -f1); [ -d "${GCD:-}" ] && printf 'gocache-clone-kib %s\ngocache-base-kib %s\n' "$gk" "$GCBK" >> "$GCD/knobs"; rm -rf "$GC"; [ ! -e "$GC" ] || { echo "HARNESS: the gocache clone $GC is still present after its drop"; GC=; return 1; }; echo "  gocache clone $GC removed ($gk KiB; base $GCBK KiB)"; fi; GC=; }
trap gcdrop EXIT
# idx5 SUMS: the five index-sums.txt entries batch.sh's INDEX check reads (batch.sh:283-284), "sum path" each.
idx5() { for f in ./graph.json ./knowledge/t2-seed.json ./graph.jsonl ./knowledge/approach-notes.json ./knowledge/failure-ledger.json; do awk -v f="$f" '$2 == f { print $1 " " $2 }' "$1" 2> /dev/null; done; }
# icmp RUNDIR COPY LABEL: the paid run's five entries must equal its setup-only run's, or the pins read from the setup
# only run would refuse every batch run; other differing lines are shown, not gated.
icmp() {
  a5=$(idx5 "$1/index-sums.txt"); b5=$(idx5 "$2")
  if [ "$(printf '%s\n' "$a5" | grep -c .)" != 5 ] || [ "$a5" != "$b5" ]; then
    printf '%s\n' "paid $1:" "$a5" "setup only $2:" "$b5" | sed 's/^/  /'
    fail "S-6 $3: the five index-sums entries batch.sh's INDEX check reads differ from the setup-only run's"
  fi
  echo "== S-6 $3: the five INDEX entries equal the setup-only run's; other lines differing $(diff "$1/index-sums.txt" "$2" | grep -c '^[<>]') (not gated)"
}
FREE0=$(freeg)
echo "== smoke $NAME; free $FREE0 GiB; G $T1_G_BIN_SHA; R $T1_R_BIN_SHA; RK_MANAGED ${RK_MANAGED:-unset}; gocache base $RK_GOCACHE_BASE $GCBK KiB"
if [ -z "${RK_MANAGED:-}" ]; then echo "== S-4 RK_MANAGED unset: S1 out (r3 section 4)"; S1=out; fi
mkdir -p "$RK_RUNS/$NAME.home"
S2=$(cd "$RK_J3_TREE" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT BIN="$RK_BIN_G" RASHOMON_HOME="$RK_RUNS/$NAME.home" T1_G_BIN_SHA="$T1_G_BIN_SHA" T2_HERE="$HERE" sh -c "$T2K_UNENFORCED_CMD" 2>&1); rc=$?
echo "== S-2 UnenforcedRuleKinds at T1_G_BIN_SHA (rc=$rc):"; printf '%s\n' "$S2" | sed 's/^/  /'
{ [ "$rc" -eq 0 ] && [ -n "$S2" ]; } || fail "S-2: the command failed or printed nothing"
# S-2 (addendum 11 section 9, ruled 10:50 PST 11-14): Boundary is not listed, and the values are exactly
# [decision reuse] on one names line (compared sorted, a duplicate kept, so a repeat or a missing kind fails).
if printf '%s\n' "$S2" | grep -q -i -w -E 'boundary'; then fail "S-2: Boundary is listed unenforced"; fi
S2N=$(printf '%s\n' "$S2" | grep -c '^UnenforcedRuleKinds names '); S2V=$(printf '%s\n' "$S2" | sed -n 's/^UnenforcedRuleKinds names \[[^]]*\] values \[\([^]]*\)\]$/\1/p')
{ [ "$S2N" = 1 ] && [ "$(printf '%s\n' $S2V | sort | tr '\n' ' ')" = "decision reuse " ]; } || fail "S-2: UnenforcedRuleKinds values [$S2V] on $S2N names lines; want exactly [decision reuse] on one"
if [ -e "$RK_J3_BASE" ]; then
  [ "$(git -C "$RK_J3_BASE" rev-parse HEAD 2> /dev/null)" = "$T1_TREE_SHA" ] || fail "J3 base $RK_J3_BASE is not at T1_TREE_SHA"
else
  sh "$HERE/judge/j3.sh" root "$RK_J3_BASE" || fail "J3 base build failed ($RK_J3_BASE.build.txt)"
fi
[ -s "$RK_J3_BASE/.rashomon/graph.jsonl" ] || fail "J3 base has no .rashomon/graph.jsonl"
echo "== J3 base $RK_J3_BASE at $T1_TREE_SHA"
# The setup-only runs: without run.sh's RK_SETUP_ONLY stop, a setup-only run would be a paid session with no gate.
grep -q 'RK_SETUP_ONLY:-}" = yes' "$H/run.sh" || fail "run.sh has no RK_SETUP_ONLY stop; a setup-only run would start a session"
pre() {
  a=$1; n=$NAME-$(echo "$a" | tr GR gr)pre; d=$RK_RUNS/$n
  gclone "$n"
  RK_SETUP_ONLY=yes GOCACHE=$GC RK_GOCACHE=$GC sh "$H/run.sh" "$a" "$n" > "$RK_RUNS/$n.launch.txt" 2>&1; rc=$?
  echo "== $n ($a, setup only) rc=$rc: $(tail -1 "$RK_RUNS/$n.launch.txt")"
  gcdrop || fail "GOCACHE DROP FAILED $n"
  [ "$rc" -eq 0 ] || fail "run.sh $n (setup only) rc=$rc"
  grep -q -x 'setup-only yes' "$d/knobs" 2> /dev/null || fail "$n: knobs holds no setup-only yes line"
}
pre R
pre G
RP=$RK_RUNS/$NAME-rpre; GP=$RK_RUNS/$NAME-gpre
s3pin "$RP" "$HERE/$T2_S3_PIN_R" rpre
s3pin "$GP" "$HERE/$T2_S3_PIN_G" gpre
PINS=
for a in R G; do
  d=$RK_RUNS/$NAME-$(echo "$a" | tr RG rg)pre
  for x in "INDEX_GRAPH_JSONL ./graph.jsonl" "KNOWLEDGE_NOTES_JSON ./knowledge/approach-notes.json" "KNOWLEDGE_LEDGER_JSON ./knowledge/failure-ledger.json"; do
    v=$(awk -v f="${x#* }" '$2 == f { print $1 }' "$d/index-sums.txt" 2> /dev/null)
    [ -n "$v" ] || { sed 's/^/  /' "$d/index-sums.txt"; fail "S-6: $a's index-sums.txt has no ${x#* } (its list is above)"; }
    PINS="$PINS T2_${a}_${x% *}_SHA256=$v"
  done
done
echo "== S-6 pins (setup-only runs):$PINS"
echo "== S-7 cleaner (batch.sh preflight, ruling C11):"; grep '^  cleaner' "$RK_RUNS/t2-batch.log" 2> /dev/null | tail -3
[ "$(grep '^  cleaner rc=' "$RK_RUNS/t2-batch.log" 2> /dev/null | tail -1)" = "  cleaner rc=0" ] || fail "S-7: no cleaner rc=0 in $RK_RUNS/t2-batch.log"
JOB="export GOTOOLCHAIN=$T1_JUDGE_GOTOOLCHAIN; go test -tags t2conflicts ./test/live/autosteer/t2/judge -run '^TestJ3_SeedRulesCompile\$' -count=1 -v"
(
  unset RASHOMON_T2_SPEC
  export RASHOMON_T2_SEED="$RK_SEED" RASHOMON_T2_J3_BASE="$GP/tree"
  if [ -n "${RK_MANAGED:-}" ]; then export RASHOMON_T2_SPEC="$RK_MANAGED/rashomon/spec.json"; fi
  "$RK_GOSLOT" "s7-$NAME" "$RK_J3_TREE" sh -c "$JOB"
) > "$RK_RUNS/$NAME.s7.txt" 2>&1
grep -q '^--- PASS: TestJ3_SeedRulesCompile' "$RK_RUNS/$NAME.s7.txt" || fail "S-7: TestJ3_SeedRulesCompile did not pass in $GP/tree ($RK_RUNS/$NAME.s7.txt)"
echo "== S-7 TestJ3_SeedRulesCompile passes in $GP/tree"
for d in "$RP" "$GP"; do
  for f in index-sums.txt index.txt knowledge.txt knobs; do cp "$d/$f" "$d.$f" || fail "copying $d/$f beside its run"; done
  git -C "$RK_REPO" worktree remove --force "$d/tree" || fail "git worktree remove $d/tree"
  case "$d" in "$RK_RUNS/$NAME"-[rg]pre) rm -rf "$d" ;; *) fail "refusing to remove [$d]" ;; esac
  [ ! -e "$d" ] || fail "the setup-only run $d is still present after its removal"
done
USED0=$(usedk)
FREE0=$(freeg)
echo "== setup-only runs removed (index-sums, index, knowledge and knobs kept beside each); free $FREE0 GiB before the first paid run; used $USED0 KiB on $(dfmnt)"
case "$USED0" in ''|*[!0-9]*) fail "S-8: df -k Used on RK_RUNS unreadable [$USED0] before the first paid run" ;; esac
one() {
  a=$1; n=$NAME-$(echo "$a" | tr GRU gru); d=$RK_RUNS/$n
  gate "before $n" "$n"
  if [ -z "${M0:-}" ]; then M0=$P1; [ "$P2" -lt "$M0" ] && M0=$P2; fi
  gclone "$n"
  GOCACHE=$GC  RK_GOCACHE=$GC sh "$H/run.sh" "$a" "$n" > "$RK_RUNS/$n.launch.txt" 2>&1; rc=$?
  python3 -I -B "$HERE/meter.py" end "$MLEDGER" "$n" || fail "meter end $n not written to $MLEDGER (run.sh rc=$rc)"
  echo "== $n ($a) rc=$rc: $(tail -1 "$RK_RUNS/$n.launch.txt")"
  [ -d "$d" ] && printf 'gocache-base %s\ngocache-clone %s\n' "$RK_GOCACHE_BASE" "$GC" >> "$d/knobs"
  [ "$rc" -eq 0 ] || fail "run.sh $n rc=$rc"
  python3 -I -B - "$d" "$a" <<'PY' || fail "sanity $n"
import json, os, sys
D, arm = sys.argv[1], sys.argv[2]
try:
    with open(os.path.join(D, "transcript.jsonl"), errors="replace") as f:
        lines = f.read().splitlines()
except OSError:
    print("  sanity: no transcript")
    sys.exit(1)
tools = {}
for line in lines:
    try:
        m = json.loads(line).get("message")
    except (ValueError, AttributeError):
        continue
    for b in (m.get("content") if isinstance(m, dict) and isinstance(m.get("content"), list) else []):
        if isinstance(b, dict) and b.get("type") == "tool_use":
            tools[str(b.get("name"))] = tools.get(str(b.get("name")), 0) + 1
with open(os.path.join(D, "tools.txt"), "w") as f:
    f.write(" ".join("%s=%d" % (k, tools[k]) for k in sorted(tools)) + "\n")
try:
    with open(os.path.join(D, "result.json")) as f:
        r = json.load(f)
except (OSError, ValueError) as e:
    print("  sanity: no readable result.json (%s)" % type(e).__name__)
    sys.exit(1)
print("  result: type %s subtype %s is_error %s turns %s cost %s" % (r.get("type"), r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd")))
print("  tools: %s" % " ".join("%s=%d" % (k, tools[k]) for k in sorted(tools)))
if arm in ("R", "G") and not os.path.exists(os.path.join(D, "hooks.log")):
    print("  sanity: no hooks.log")
    sys.exit(1)
cap = r.get("subtype") in ("error_max_turns", "error_max_budget_usd")
if cap:
    with open(os.path.join(D, "flags"), "a") as f:
        f.write("cap_hit subtype %s, turns %s, cost %s\n" % (r.get("subtype"), r.get("num_turns"), r.get("total_cost_usd")))
sys.exit(0 if r.get("type") == "result" and (cap or not r.get("is_error")) else 1)
PY
  if [ "$a" != U ]; then python3 -B "$H/t1c/channel.py" "$d" "$a" || fail "channel closed $n (B448)"; fi
  RK_T2_ROWS=t2-smoke-rows.txt sh "$HERE/judge/judge.sh" "$d" > "$RK_RUNS/$n.judge.txt" 2>&1; jrc=$?
  { [ "$jrc" -ne 3 ] && [ -f "$d/judge/row.txt" ]; } || fail "judge $n rc=$jrc ($RK_RUNS/$n.judge.txt)"
  echo "  row: $(cat "$d/judge/row.txt")"
  gcdrop || fail "GOCACHE DROP FAILED $n: its clone is not removed, so S-8's peak would count two"
  [ -n "$gk" ] || fail "the gocache clone of $n was not there to measure"
  gg=$((gk - GCBK)); [ "$gg" -gt "$GCMAX" ] && GCMAX=$gg
  echo "  gocache growth $n: $gg KiB over the base (largest so far $GCMAX KiB)"
}
one G
G=$RK_RUNS/$NAME-g
[ -s "$G/status.txt" ] || fail "S-1: G made no edit (status.txt empty); its store cannot be read after an edit"
OV=$(ovs "$G"); UB=$(cat "$G"/$T2K_STEER_LOG 2> /dev/null | grep -c -F "$T2K_UNITBRIEF_PREFIX"); UB=${UB:-0}
echo "== S-1 G's store after an edit: overlay files $OV; unit brief lines $UB in $T2K_STEER_LOG"
{ [ "$OV" -ge 1 ] && [ "$UB" -ge 1 ]; } || fail "S-1"
python3 -I -B "$HERE/s1b.py" check G "$G" "$T2K_RECORDS" || fail "S-1b: G's writes are not read as in its project (the line above); the batch would measure nothing (addendum 8)"
if [ -n "${RK_MANAGED:-}" ]; then
  S4=$(grep -r -h -F --exclude-dir=tree --exclude-dir=judge "$T2K_S1_LOADED" "$G" 2> /dev/null | head -3 | cut -c1-300)
  echo "== S-4 S1 loaded-rule line from G's binary:"; printf '%s\n' "${S4:-(none)}" | sed 's/^/  /'
  [ -n "$S4" ] || fail "S-4: no S1 loaded-rule line, so S1 is out (r3 section 4): relaunch the smoke without RK_MANAGED"
  S1=in
fi
CS=$(grep -c -F "$T2K_CONSENT_SCOPE" "$G/consent.txt" 2> /dev/null); CS=${CS:-0}
SL=absent; for f in "$G"/$T2K_SESSION_LOG; do [ -e "$f" ] && SL=present; done
echo "== S-5 G: session scope lines in consent.txt $CS; session log $SL; overlay files $OV"
{ [ "$CS" -ge 1 ] && [ "$SL" = present ] && [ "$OV" -ge 1 ]; } || fail "S-5 (G)"
# V1 (addendum 11 section 9, ruled 10:50 PST 11-14): after consent, G's store reads record scope v5 with the
# session-content marker set, read from the store itself (consentscope.py: the fold and both markers).
V1=$(python3 -I -B "$HERE/consentscope.py" "$G/store" 2>&1); v1rc=$?
echo "== V1 G: $V1"
[ "$v1rc" = 0 ] || fail "V1: G's store does not read record scope v5 with the session-content marker set (rc $v1rc)"
icmp "$G" "$GP.index-sums.txt" G
one R
R=$RK_RUNS/$NAME-r
echo "== V1 R (read, not gated; R records v5 without the marker by design): $(python3 -I -B "$HERE/consentscope.py" "$R/store" 2>&1)"
s3pin "$R" "$HERE/$T2_S3_PIN_R" R
S3=$(cat "$R/hooks.log" "$R/report.txt" "$R/stderr.txt" 2> /dev/null | grep -i -E 'error|refus|invalid|cannot' | grep -i -E 'spec|managed|seed|knowledge|rule' | head -10 | cut -c1-300)
if [ -n "$S3" ]; then
  echo "== S-3 R's binary on the spec and the seed: error lines in hooks.log, report.txt or stderr.txt:"; printf '%s\n' "$S3" | sed 's/^/  /'
  echo "SMOKE STOP $NAME: S-3 R printed an error line on the spec or the seed; the line goes to the director, no rule is stripped (r3 S-3)"
  exit 3
fi
echo "== S-3 R's binary on the spec and the seed: no error line in hooks.log, report.txt or stderr.txt"
icmp "$R" "$RP.index-sums.txt" R
one U
U=$RK_RUNS/$NAME-u
for x in "$R" "$U"; do
  nc=$(grep -c '^NO-CONSENT' "$x/flags" 2> /dev/null); nc=${nc:-0}
  echo "== S-5 $(basename "$x"): NO-CONSENT flags $nc"
  [ "$nc" -eq 0 ] || fail "S-5: $(basename "$x") holds an overlay file with no session consent"
done
mgate "after the last run" "$NAME-after" --no-five; M1=$P1; [ "$P2" -gt "$M1" ] && M1=$P2
PTS=$(awk -v a="$M0" -v b="$M1" 'BEGIN { printf "%.2f", (b - a) / 3 }')
echo "== S-6 meter $M0 (the lower reading before the first run) to $M1 (the higher after the last): $((M1 - M0)) points over 3 runs, $PTS per run"
MAXK=0
for d in "$G" "$R" "$U"; do
  k=$(du -sk "$d" | cut -f1); echo "== S-8 $(basename "$d") $k KiB"
  [ "$k" -gt "$MAXK" ] && MAXK=$k
done
F1=$(freeg)
U1=$(usedk)
case "$U1" in ''|*[!0-9]*) fail "S-8: df -k Used on RK_RUNS unreadable [$U1] after the last paid run" ;; esac
NR=$(( ${RK_T2_N_MAX:-$T1_N_PER_ARM} * 3 ))
# S-8 at the peak (addendum 8, 5b): the batch holds one clone at a time (each drop verified), so the largest clone
# growth counts once, not once per run. The per-run drop is the Used growth from FREE0 to F1 (4d); the gates read Avail.
S8=$(awk -v k="$MAXK" -v n="$NR" -v g="$GCMAX" -v f0="$FREE0" -v f1="$F1" -v u0="$USED0" -v u1="$U1" 'BEGIN { u = (u1 - u0) / 3; per = (u > k) ? u : k; need = (per * n + g) / 1048576; printf "%.0f %.3f %s %.3f %.3f %.0f %.0f %.0f", per, need, (f1 - need >= 4.0) ? "yes" : "no", (per + g) * n / 1048576, 4.0 + need + 1.0, u1 - u0, (f0 - f1) * 1048576, u }')
echo "== S-8 FREE0 to F1 on $(dfmnt): Used growth $(echo "$S8" | cut -d' ' -f6) KiB, over 3 $(echo "$S8" | cut -d' ' -f8) KiB (read for per); Avail drop $(echo "$S8" | cut -d' ' -f7) KiB (printed, not read: Avail is shared with swap and snapshots, addendum 8 4d)"
echo "== S-8 per run $(echo "$S8" | cut -d' ' -f1) KiB (the larger of the largest run dir and the Used growth over 3; section 12 estimates 85 MB); the largest clone growth $GCMAX KiB, held once at the peak; $NR runs need $(echo "$S8" | cut -d' ' -f2) GiB; free $F1 GiB; the 4.0 GiB floor holds after the batch: $(echo "$S8" | cut -d' ' -f3)"
echo "== S-8 superseded reading, (per run + clone growth) x $NR runs: $(echo "$S8" | cut -d' ' -f4) GiB (printed, not gated)"
echo "== S-8 G6 implied 4.0 + $(echo "$S8" | cut -d' ' -f2) + 1.0 = $(echo "$S8" | cut -d' ' -f5) GiB against the sealed 7.052 (the measured growth for amend-gocache's 0.252 estimate; for ruling, addendum 8)"
[ "$(echo "$S8" | cut -d' ' -f3)" = yes ] || fail "S-8: the batch would cross the 4.0 GiB floor"
MISS=0
for x in "$RP" "$GP" "$R"; do m=$(LC_ALL=C comm -23 "$x.s3pin" "$x.s3got" | wc -l | tr -d ' '); MISS=$((MISS + m)); done
echo "smoke $NAME pass: S-1 to S-8 hold; S1 $S1; meter $M0 to $M1 ($PTS points per run); free $FREE0 to $F1 GiB; pins$PINS; S-3 MISSING $MISS"
