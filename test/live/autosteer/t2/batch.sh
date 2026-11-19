#!/bin/sh
# batch.sh: the T2 batch (pins.txt). The smoke (smoke.sh), then n=T1_N_PER_ARM replicates (to 10 under RK_T2_N_MAX=10,
# r3 section 5), each one run per arm, the
# arm order rotated by replicate (ruling C9: U R G, then R G U, then G U R), one run at a time, serial through the one
# configuration directory pins.txt names, each run checked (t1c/channel.py, unchanged) and judged (judge/judge.sh) as
# it ends, rows in RK_RUNS/t2-rows.txt. Runs nothing without RK_T2_GO=yes, set at launch only when G2 to G4 are in the
# tip and the meter reads T2_METER_LAUNCH_PCT or under (pins.txt; r3 section 1 G1; the owner's "stop waiting", 16:38:36 item 1). The whole
# batch runs under caffeinate -i (the director, 23:34:47Z item 6: the Mac slept mid-run on 10-28).
# Carried from T1c (t1c/batch.sh): a harness failure stops the batch (no transcript or hook log; a closed channel, B448;
# a judge that refused or wrote no row); every tool is checked before the first run; the account is logged beside the
# meter (B454) and must equal the meter's.
# T2 deltas (pins.txt header): pins.txt holding any TBD is refused; the CLI is the pinned path at the pinned version,
# exported as RK_CLAUDE_BIN and RK_CLAUDE_VERSION for run.sh and hook.sh; the configuration directory is pins.txt's
# T1_CONFIG_DIR, never the environment's; `claude auth status` on it must read loggedIn true; a /tmp configuration
# directory has its oldest file time read, backups/ exempt (ruling C11), and the 00:00 cleaner margin quoted, and the
# batch refuses when the next 00:00 pass would delete a file (macOS clears /tmp files untouched for 72 h at 00:00 local);
# prompt.txt must hash to T2_PROMPT_SHA256.
# Run outcomes (ruling C8), read by the sanity step from result.json, stderr.txt, status.txt and the transcript:
#   pass     judged and counted.
#   cap_hit  result subtype error_max_turns or error_max_budget_usd (the pinned CLI's two cap subtypes; num_turns is not
#            the cap's count: T1c success runs reported 153 and 159 under a 150 cap), or a cost over T1_BUDGET_USD:
#            counted in n, judged on its final tree as left, flagged in RUN/flags, never replaced. T1c's batch.sh
#            stopped on t1c-g15's cap hit (t1c-batch.log 382); this one does not.
#   void     the run made no edit (status.txt empty, no Edit, Write, MultiEdit or NotebookEdit call) and died on an
#            infrastructure cause (no result, not logged in, a limit text, an API error, fewer than 2 turns), the cause
#            quoted into RUN/void; rerun once under NAMEx; both recorded; a second void stops the batch.
#   harness  anything else (an error result after an edit, no transcript, no hooks.log for R or G): the batch stops.
# Before every run (the re-cut: q4-answer.txt items 1-4; the director 14:13 PST 11-14 items 3a, 3b, 3d) meter.py's
# gate reads RK_METER_FILE twice, 20 s apart. A replicate's first run also needs both weekly readings at or under
# T2_METER_LAUNCH_PCT (a launch's first replicate) or T2_METER_STOP_PCT (later ones); every run needs the five-hour
# rule (pins.txt; smoke.sh gates its own runs the same way). A HOLD re-reads every 30 s, bounded by
# T2_METER_HOLD_MAX_S, then stops clean with no run voided. Before every replicate after a launch's first, the
# launch's own spend is read against the 3c cap (the director 14:23 PST item 1). After every run the t2-* spend is
# read against T1_BATCH_CAP_USD; over it, the batch stops. On any stop, and at the end, the count line names the
# complete replicates:
# only those count, and the row says n=k. The count line has j2.py's pair line beside it (R5 on G beside U and R over
# the complete replicates; addendum 6 D15). A run whose judge/j2.json reads a setup failure is a harness failure: the
# batch stops and the run is not counted (D15).
# Required environment, none defaulted so a batch is reproducible from its launch line: RK_RUNS, RK_GOCACHE_BASE, RK_BIN_R,
# RK_BIN_G, RK_PGRAPH, RK_PGRAPH_G, RK_KNOWLEDGE, RK_SEED (the seeded rows file), RK_GO_TOOLCHAIN_BIN, RK_GOSLOT,
# RK_METER_FILE. The run.sh overrides RK_PROMPT_FILE, RK_MODEL, RK_MAX_TURNS and RK_BUDGET_USD are cleared.
# RK_T2_SKIP_SMOKE=yes skips the smoke when one already passed on this harness build.
# RK_T2_SMOKE_ONLY=yes runs the preflight (the six T2_[RG]_ index and knowledge pins alone may read TBD) and the smoke,
# then exits; the smoke's S-6 line fills those pins and the batch launches with RK_T2_SKIP_SMOKE=yes. keys.txt holding
# any TBD is refused in both modes. RK_J3_BASE and RK_J3_TREE are judge/j3.sh's (the smoke builds RK_J3_BASE if absent).
# RK_T2_N_MAX=10 registers the extension (r3 section 5) and needs RK_T2_EXT_CAP_USD, the director's cap on the spend of
# replicates 4 to 10 (T1_BATCH_CAP_USD stays on replicates 1 to 3). Before replicate 4 the log holds M0 (the lower
# reading before replicate 1), M3 (the higher reading now), M3 + (7/3)(M3 - M0) in integers (3 x M3 + 7 x (M3 - M0)
# against 3 x T2_METER_EXT_PCT) and the decision; over T2_METER_EXT_PCT (93 in pins.txt: the owner's word of
# 2026-11-12 (ceiling 95), changed before any data) the batch ends at n=T1_N_PER_ARM as a normal end.
# RK_MANAGED (optional): the managed spec directory, S1 (ruling A3), for the arms T2_PRODUCT_INPUT_ARMS names; unset
# when the smoke dropped S1. The launching environment must not set RASHOMON_MANAGED_SETTINGS_PATH, and after every run
# the run's knobs file must read seed off and managed-spec off for U (and any arm outside T2_PRODUCT_INPUT_ARMS), and
# the pinned seed and its own managed path (or off without RK_MANAGED) for the input arms, whose index-sums.txt must
# hold graph.json, the seed copy and the arm's own graph.jsonl and knowledge outputs at their pins (the director,
# 15:50:40 PST item 7). The R guard (item 6): G minus R is what the re-cut registers G minus R to be.
# Every run's tool-name counts go to RUN/tools.txt and beside its row (item 4: an arm that shells out is visible).
# Go build cache (amend-gocache): RK_GOCACHE_BASE is the frozen base, checked once before the smoke against
# T2_GOCACHE_BASE_MANIFEST_SHA256 (cachemanifest.py). Each run gets its own APFS clone RK_RUNS/NAME.gocache (cp -c -R)
# as run.sh's RK_GOCACHE; its knobs file records the base, the clone and the clone's du -sk beside the base's, and the
# clone is removed by that exact path after the run's judges, at a void, or on exit. The base is never used in place.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
H=$HERE/..
[ "${RK_T2_GO:-}" = yes ] || { echo "batch.sh: RK_T2_GO is not yes; set it only when G2 to G4 are in the tip and the meter reads T2_METER_LAUNCH_PCT (pins.txt) or under (r3 section 1)" >&2; exit 2; }
if [ -z "${RK_T2_CAFFEINATED:-}" ]; then
  export RK_T2_CAFFEINATED=1
  exec caffeinate -i sh "$HERE/batch.sh" "$@"
fi
if [ "${RK_T2_SMOKE_ONLY:-}" = yes ]; then
  [ "${RK_T2_SKIP_SMOKE:-}" != yes ] || { echo "batch.sh: RK_T2_SMOKE_ONLY and RK_T2_SKIP_SMOKE are both yes" >&2; exit 2; }
  TBD=$(grep -n '=TBD' "$HERE/pins.txt" | grep -v -E '^[0-9]+:T2_[RG]_(INDEX_GRAPH_JSONL|KNOWLEDGE_NOTES_JSON|KNOWLEDGE_LEDGER_JSON)_SHA256=TBD$')
else
  TBD=$(grep -n '=TBD' "$HERE/pins.txt")
fi
[ -z "$TBD" ] || { printf '%s\n' "$TBD" >&2; echo "batch.sh: pins.txt holds the TBD values above; pre-register first" >&2; exit 2; }
if grep -n '=TBD' "$HERE/keys.txt" >&2; then echo "batch.sh: keys.txt holds the TBD values above; fill it at the re-cut" >&2; exit 2; fi
[ -z "${RASHOMON_MANAGED_SETTINGS_PATH+x}" ] || { echo "batch.sh: RASHOMON_MANAGED_SETTINGS_PATH is set in the launching environment; only run.sh sets it, for the input arms" >&2; exit 2; }
for v in RK_RUNS RK_GOCACHE_BASE RK_BIN_R RK_BIN_G RK_PGRAPH RK_PGRAPH_G RK_KNOWLEDGE RK_SEED RK_GO_TOOLCHAIN_BIN RK_GOSLOT RK_METER_FILE RK_J3_BASE RK_J3_TREE; do
  eval "val=\${$v:-}"; [ -n "$val" ] || { echo "batch.sh: $v unset" >&2; exit 2; }
done
# One path form (addendum 8): RK_RUNS by its physical path, as run.sh resolves it, so the batch's paths are the runs'.
mkdir -p "$RK_RUNS" && RK_RUNS=$(cd "$RK_RUNS" && pwd -P) || { echo "batch.sh: RK_RUNS [$RK_RUNS] cannot be made or resolved" >&2; exit 2; }
export RK_SLICE=t2
. "$HERE/pins.txt"
for v in T2_METER_LAUNCH_PCT T2_METER_STOP_PCT T2_METER_EXT_PCT T2_METER_MAX_AGE_S T2_METER_HOLD_MAX_S T2_METER_5H_PCT T2_METER_5H_LAUNCH_PCT; do
  eval "val=\${$v:-}"; case "$val" in ''|*[!0-9]*) echo "batch.sh: $v is [$val] in pins.txt, not a whole number" >&2; exit 2 ;; esac
done
for v in T2_METER_5H_MULT T2_USD_PER_WEEKLY_PT; do
  eval "val=\${$v:-}"; case "$val" in ''|*[!0-9.]*|.*|*.|*.*.*) echo "batch.sh: $v is [$val] in pins.txt, not a decimal number" >&2; exit 2 ;; esac
done
[ "$T2_METER_LAUNCH_PCT" -le "$T2_METER_STOP_PCT" ] || { echo "batch.sh: T2_METER_LAUNCH_PCT $T2_METER_LAUNCH_PCT is over T2_METER_STOP_PCT $T2_METER_STOP_PCT" >&2; exit 2; }
# addendum 5 5a, in both modes: T2_S1_LIVE=yes scores a boundary share for S1 (b)'s rules, which load only through
# the managed path, so yes needs RK_MANAGED. Any value but yes or no is refused here too (registered beside 5a):
# row.py:190 reads it as S1 live unread and leaves every row UNJUDGED, so it is caught before any paid run.
case "${T2_S1_LIVE:-}" in
  yes) [ -n "${RK_MANAGED:-}" ] || { echo "batch.sh: T2_S1_LIVE is yes and RK_MANAGED is unset; S1 (b) loads" \
         "through the managed path (addendum 5 5a)" >&2; exit 2; } ;;
  no) [ -z "${RK_MANAGED:-}" ] || { echo "batch.sh: T2_S1_LIVE is no and RK_MANAGED is set; addendum 5 D17 pairs no" \
         "with S1 (a), RK_MANAGED unset (addendum 9)" >&2; exit 2; } ;;
  *) echo "batch.sh: T2_S1_LIVE is [${T2_S1_LIVE:-}] in pins.txt, not yes or no" >&2; exit 2 ;;
esac
if [ -n "${RK_CONFIG_DIR:-}" ] && [ "$(cd -- "$RK_CONFIG_DIR" 2> /dev/null && pwd -P)" != "$T1_CONFIG_DIR" ]; then
  echo "batch.sh: RK_CONFIG_DIR is $RK_CONFIG_DIR; pins.txt names $T1_CONFIG_DIR, one path for the whole batch" >&2; exit 2
fi
export RK_CONFIG_DIR="$T1_CONFIG_DIR" RK_CLAUDE_BIN="$T1_CLAUDE_BIN" RK_CLAUDE_VERSION="$T1_CLAUDE_VERSION"
export PATH="$RK_GO_TOOLCHAIN_BIN:$PATH"
unset RK_PROMPT_FILE RK_MODEL RK_MAX_TURNS RK_BUDGET_USD
for tool in go gofmt python3 perl git uuidgen caffeinate; do command -v "$tool" > /dev/null 2>&1 || { echo "batch.sh: $tool is not on PATH" >&2; exit 2; }; done
[ -x "$RK_CLAUDE_BIN" ] || { echo "batch.sh: $RK_CLAUDE_BIN is not executable" >&2; exit 2; }
CV=$("$RK_CLAUDE_BIN" --version 2> /dev/null | awk '{ print $1 }')
[ "$CV" = "$(echo "$RK_CLAUDE_VERSION" | tr - .)" ] || { echo "batch.sh: $RK_CLAUDE_BIN is version [$CV], pinned $RK_CLAUDE_VERSION" >&2; exit 2; }
for f in "$HERE/smoke.sh" "$HERE/judge/judge.sh" "$HERE/judge/j3.sh" "$HERE/judge/j4.sh" "$HERE/judge/j5.sh" "$HERE/judge/j2.py" \
    "$HERE/judge/row.py" "$HERE/flags.py" "$HERE/keys.txt" "$HERE/cachemanifest.py" "$HERE/s3pin.sh" "$H/t1c/judge/a8.py" "$H/t1c/channel.py" "$HERE/prompt.txt" "$HERE/meter.py" "$HERE/consentscope.py" "$RK_METER_FILE"; do
  [ -f "$f" ] || { echo "batch.sh: missing $f" >&2; exit 2; }
done
[ "$(shasum -a 256 "$HERE/prompt.txt" | cut -d' ' -f1)" = "$T2_PROMPT_SHA256" ] || { echo "batch.sh: prompt.txt digest is not T2_PROMPT_SHA256" >&2; exit 2; }
LOG=$RK_RUNS/t2-batch.log
mkdir -p "$RK_RUNS"
say() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LOG"; }
meter() { python3 -I -B -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("ts"), d.get("rate_limits"))' "$RK_METER_FILE" 2>&1; }
# mgate RUN [--weekly N --weekly-key K]: meter.py's gate before RUN (q4-answer.txt items 1-4; the director 14:13 PST
# 3a, 3d). Two readings 20 s apart; a HOLD re-reads every 30 s up to T2_METER_HOLD_MAX_S (over the five-hour rule,
# one wait to resets_at + 60 s first). GO sets MW1 MW2 (weekly) and MF1 MF2 (five-hour); a HOLD past the bound, or a
# pins or ledger error, stops the batch with no run voided. The ledger RK_RUNS/t2-meter.txt is the smoke's too: each
# GO pair, each run's end and its five-hour delta, so D starts at the smoke's G run (3d).
MLEDGER=$RK_RUNS/t2-meter.txt
mgate() {
  mgn=$1; shift
  mg=$(python3 -I -B "$HERE/meter.py" gate --file "$RK_METER_FILE" --ledger "$MLEDGER" --name "$mgn" --max-age "$T2_METER_MAX_AGE_S" --hold-max "$T2_METER_HOLD_MAX_S" --five-line "$T2_METER_5H_PCT" --mult "$T2_METER_5H_MULT" --five-launch "$T2_METER_5H_LAUNCH_PCT" "$@" 2>> "$LOG"); mgrc=$?
  say "meter gate before $mgn: ${mg:-no output} (rc $mgrc)"
  case "$mgrc" in
    0) set -- $mg; { [ "$#" = 7 ] && [ "$1" = GO ]; } || stop "METER GATE before $mgn printed [$mg]; batch stopped"; MW1=$2; MW2=$3; MF1=$4; MF2=$5 ;;
    3) stop "METER STOP before $mgn: $mg; no run voided (3a); batch stopped" ;;
    *) stop "METER GATE ERROR before $mgn (rc $mgrc; the log lines above); batch stopped" ;;
  esac
}
account() { python3 -I -B -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("emailAddress"), a.get("accountUuid"))' "${1:-$RK_CONFIG_DIR/.claude.json}" 2>&1; }
# count: the complete replicates (every arm's counted run, NAME or its rerun NAMEx when NAME is void, has a judged row).
count() {
  python3 -I -B - "$RK_RUNS" "$NREP" "$HERE/judge/j2.py" <<'PY' 2>&1 | tee -a "$LOG"
import json, os, subprocess, sys
runs, n, j2 = sys.argv[1], int(sys.argv[2]), sys.argv[3]
def j2setup(name):
    try:
        return str(json.load(open(os.path.join(runs, name, "judge", "j2.json"))).get("setup", ""))
    except (OSError, ValueError, AttributeError):
        return ""
def j2unread(name):
    # arm G only (the director 10:13 PST item 4): a missing or unparsable j2.json, or one reading "unread", is a setup failure
    try:
        return str(json.load(open(os.path.join(runs, name, "judge", "j2.json"))).get("unitbrief_delivered", "")).startswith("unread")
    except (OSError, ValueError, AttributeError):
        return True
complete, partial, groups = [], [], {}
for i in range(1, n + 1):
    got = []
    for a in "gru":
        name = "t2-%s%d" % (a, i)
        if os.path.exists(os.path.join(runs, name, "void")):
            name += "x"
        row = os.path.exists(os.path.join(runs, name, "judge", "row.txt"))
        got.append((name, row and not j2setup(name).startswith("setup failure") and not (a == "g" and j2unread(name)), row))
    groups[i] = [nm for nm, _, _ in got]
    if all(ok for _, ok, _ in got):
        complete.append(i)
    elif any(os.path.exists(os.path.join(runs, nm)) for nm, _, _ in got):
        partial.append("%d (%s)" % (i, ", ".join("%s %s" % (nm, "judged" if ok else "judged, J2 setup failure" if row else "not judged")
                                                for nm, ok, row in got)))
print("  count: complete replicates %d of %d %s: n=%d per arm; recorded and not counted: %s"
      % (len(complete), n, complete, len(complete), "; ".join(partial) or "none"))
# addendum 5 4a (:112-113): a J3 failure stays in the primary and is counted per arm over the complete replicates, by
# row.txt's own column (row.py prints "J3 detector unread" exactly when J3 is not read).
j3u, j3bad = {a: [] for a in "gru"}, []
for rep in complete:
    for a, nm in zip("gru", groups[rep]):
        try:
            rcols = open(os.path.join(runs, nm, "judge", "row.txt")).read().rstrip("\n").split(" | ")
        except (OSError, ValueError):
            j3bad.append(nm)
            continue
        if "J3 detector unread" in rcols:
            j3u[a].append(nm)
print("  J3 detector unread (addendum 5 4a) per arm over complete replicates: %s; runs %s%s"
      % (" ".join("%s=%d" % (a, len(j3u[a])) for a in "gru"), " ".join(j3u["g"] + j3u["r"] + j3u["u"]) or "none",
         "; row.txt unread: " + " ".join(j3bad) if j3bad else ""))
args = []
for i in complete:
    args += (["--"] if args else []) + [os.path.join(runs, nm) for nm in groups[i]]
if not args:
    print("  J2 pair (addendum 6 D15): no complete replicate")
else:
    p = subprocess.run([sys.executable, "-I", j2, "pair"] + args, capture_output=True, text=True)
    try:
        res = json.loads(p.stdout.strip().splitlines()[-1]) if p.returncode == 0 else None
    except (ValueError, IndexError):
        res = None
    if res is None:
        print("  J2 pair (addendum 6 D15) over replicates %s: rc=%d, unread: %s" % (complete, p.returncode, " ".join((p.stderr or p.stdout).split())[-300:]))
    else:
        print("  J2 pair (addendum 6 D15) over replicates %s: %s; notes %s" % (complete, res.get("line"), json.dumps(res.get("notes"), sort_keys=True)))
PY
}
stop() { say "$*"; count; say "=== t2 batch stopped; meter $(meter)"; exit 3; }
# j2setup RUN: a judge/j2.json reading "setup failure" stops the batch as a harness failure (addendum 6 D15; line 27).
# For arm G (RUN/arm), a j2.json that is missing, unparsable or reads "unread" (j2.py rc 3) is a setup failure too and
# stops it (the director 10:13 PST item 4, registered before any data; draft addendum 5).
j2setup() { j2s=$(python3 -I -B -c 'import json,sys; d=json.load(open(sys.argv[1])); s=str(d.get("setup", "")); u=str(d.get("unitbrief_delivered", "")); print(s if s.startswith("setup failure") or not u.startswith("unread") else u)' "$1/judge/j2.json" 2> /dev/null); case "$j2s" in "setup failure"*) stop "J2 SETUP FAILURE ${1##*/}: $j2s; a harness failure, not counted (addendum 6 D15); batch stopped" ;; esac; if [ "$(cat "$1/arm" 2> /dev/null)" = G ]; then case "$j2s" in ""|unread*) stop "J2 UNREAD ${1##*/} (G): ${j2s:-no readable j2.json}; a setup failure (the director 10:13 PST item 4), not counted; batch stopped" ;; esac; fi; }
# gcdrop: the current run's Go cache clone (one_run) is measured into its knobs and removed by its exact path, which
# must be RK_RUNS/NAME.gocache; the drop is verified (addendum 8, 5b: S-8's peak counts one clone, so a clone left on
# disk is a harness failure). Returns 1, GC cleared so the EXIT trap does not measure it twice, on either refusal.
gcdrop() { [ -n "${GC:-}" ] || return 0; case "$GC" in "$RK_RUNS"/*.gocache) ;; *) say "HARNESS: the gocache clone path [$GC] is not RK_RUNS/NAME.gocache; not removed"; GC=; return 1 ;; esac; if [ -d "$GC" ]; then gk=$(du -sk "$GC" | cut -f1); [ -d "${GCD:-}" ] && printf 'gocache-clone-kib %s\ngocache-base-kib %s\n' "$gk" "${GCBK:-unread}" >> "$GCD/knobs"; rm -rf "$GC"; [ ! -e "$GC" ] || { say "HARNESS: the gocache clone $GC is still present after its drop"; GC=; return 1; }; say "gocache clone $GC removed ($gk KiB; base ${GCBK:-unread} KiB)"; fi; GC=; }
trap gcdrop EXIT
say "=== t2 batch start; $(go version); claude $CV at $RK_CLAUDE_BIN; meter $(meter) (the main configuration's account; B454); config $RK_CONFIG_DIR account $(account); harness $(git -C "$H" rev-parse --short HEAD 2>/dev/null); caffeinate -i (pid $$)"
[ "$(account)" = "$(account "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")" ] || { say "config account $(account) is not the meter's $(account "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}") (B454)"; exit 2; }
AUTH=$(env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR="$RK_CONFIG_DIR" "$RK_CLAUDE_BIN" auth status 2>&1)
printf '%s\n' "$AUTH" | python3 -B -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("loggedIn") is True else 1)' 2> /dev/null || { say "auth status on $RK_CONFIG_DIR is not loggedIn true: $(printf '%s' "$AUTH" | tr '\n' ' ' | cut -c1-200)"; exit 2; }
say "auth status on $RK_CONFIG_DIR: loggedIn true"
case "$RK_CONFIG_DIR" in
/tmp/*|/private/tmp/*)
  python3 -B - "$RK_CONFIG_DIR" <<'PY' 2>&1 | tee -a "$LOG"; [ "$(tail -1 "$LOG" | sed -n 's/^  cleaner rc=//p')" = 0 ] || { say "the next 00:00 cleaner pass would delete a configuration file; batch refused"; exit 2; }
import datetime, os, sys
root = sys.argv[1]
exempt = os.path.join(root, "backups")
oldest, path, skipped = None, None, 0
for d, _, names in os.walk(root):
    if d == exempt or d.startswith(exempt + os.sep):
        skipped += len(names)
        continue
    for n in names:
        p = os.path.join(d, n)
        try:
            st = os.lstat(p)
        except OSError:
            continue
        touched = max(st.st_atime, st.st_mtime, st.st_ctime)
        if oldest is None or touched < oldest:
            oldest, path = touched, p
print("  cleaner: backups/ exempt (ruling C11), %d files not read" % skipped)
now = datetime.datetime.now().astimezone()
if oldest is None:
    print("  cleaner: no files under %s" % root); print("  cleaner rc=1"); sys.exit(0)
t = datetime.datetime.fromtimestamp(oldest).astimezone()
first = (now + datetime.timedelta(days=1)).replace(hour=0, minute=0, second=0, microsecond=0)
for k in range(2):
    m = first + datetime.timedelta(days=k)
    margin = (t + datetime.timedelta(hours=72)) - m
    print("  cleaner: oldest touch %s (%s); the 00:00 pass at %s leaves a margin of %.1f h" % (t.isoformat(timespec="seconds"), path, m.isoformat(timespec="minutes"), margin.total_seconds() / 3600))
print("  cleaner rc=%d" % (0 if (t + datetime.timedelta(hours=72)) > first else 1))
PY
  ;;
esac
for f in "$RK_BIN_R" "$RK_BIN_G" "$RK_PGRAPH" "$RK_PGRAPH_G" "$RK_SEED"; do [ -e "$f" ] || { say "missing $f"; exit 2; }; done
[ "$(shasum -a 256 "$RK_SEED" | cut -d' ' -f1)" = "$T1_KNOWLEDGE_SEED_SHA256" ] || { say "seed file digest is not T1_KNOWLEDGE_SEED_SHA256"; exit 2; }
if [ -n "${RK_MANAGED:-}" ]; then
  [ "$(shasum -a 256 "$RK_MANAGED/rashomon/spec.json" | cut -d' ' -f1)" = "$T1_MANAGED_SPEC_SHA256" ] || { say "managed spec digest is not T1_MANAGED_SPEC_SHA256"; exit 2; }
  say "S1 in: the managed spec $RK_MANAGED/rashomon/spec.json at T1_MANAGED_SPEC_SHA256 for arms $T2_PRODUCT_INPUT_ARMS"
else
  say "S1 out: no RK_MANAGED; arms $T2_PRODUCT_INPUT_ARMS run on the seed alone"
fi
for pair in "$RK_BIN_G:$T1_G_BIN_SHA" "$RK_PGRAPH_G:$T1_G_PGRAPH_SHA" "$RK_BIN_R:$T1_R_BIN_SHA" "$RK_PGRAPH:$T1_PGRAPH_SHA"; do
  bin=${pair%%:*}; want=${pair#*:}
  go version -m "$bin" 2>/dev/null | grep -q "vcs.revision=$want" || { say "$bin: embedded vcs.revision is not the pinned $want"; exit 2; }
done
# (h), the director 14:15 PST on addendum 3 D5, extended to R's two binaries 15:06 PST (addendum 9): all four come
# from a clean checkout of their pinned revision, so go version -m shows vcs.modified=false (D5's literal: a binary
# with no vcs.modified line is refused too). D5 is G's build rule; the R half is addendum 9's.
for bin in "$RK_BIN_G" "$RK_PGRAPH_G" "$RK_BIN_R" "$RK_PGRAPH"; do
  go version -m "$bin" 2>/dev/null | grep -q -x -E '[[:space:]]*build[[:space:]]+vcs\.modified=false' ||
    { say "$bin: go version -m does not show vcs.modified=false, so not a clean checkout (addendum 3 D5)"; exit 2; }
done
REPO=$(cd "$H/../../.." && pwd)
mb=$(git -C "$REPO" merge-base "$T1_G_BIN_SHA" "$T1_R_BIN_SHA" 2> /dev/null)
[ "$mb" = "$T1_R_BIN_SHA" ] || { say "merge-base of G $T1_G_BIN_SHA and R $T1_R_BIN_SHA is [$mb], not R: G minus R would not be what the re-cut registers G minus R to be (item 6 (i))"; exit 2; }
# (ii) R on the first-parent chain of M, a pinned main sha, never a moving ref; (iii) G's main commits after R = L.
case "$T1_G_MAIN_L" in none) l_want= ;; *) l_want=$(printf '%s\n' $T1_G_MAIN_L | sort); [ -n "$l_want" ] && ! printf '%s\n' "$l_want" | grep -q -v -x -E '[0-9a-f]{40}' || { say "T1_G_MAIN_L [$T1_G_MAIN_L] is neither none nor full shas (item 6 (iii))"; exit 2; } ;; esac
case "$T1_MAIN_M_SHA" in *[!0-9a-f]*) false ;; *) [ ${#T1_MAIN_M_SHA} = 40 ] ;; esac && [ "$(git -C "$REPO" rev-list --first-parent "$T1_MAIN_M_SHA" 2> /dev/null | grep -c -x "$T1_R_BIN_SHA")" = 1 ] || { say "R $T1_R_BIN_SHA is not on the first-parent chain of M [$T1_MAIN_M_SHA], a pinned main sha, never a moving ref: G minus R would not be what the re-cut registers G minus R to be (item 6 (ii))"; exit 2; }
l_fp=$(git -C "$REPO" rev-list --first-parent "$T1_R_BIN_SHA..$T1_MAIN_M_SHA" 2> /dev/null) && l_got=$(for l_c in $l_fp; do git -C "$REPO" merge-base --is-ancestor "$l_c" "$T1_G_BIN_SHA" 2> /dev/null; case $? in (0) echo "$l_c" ;; (1) ;; (*) echo "error:$l_c" ;; esac; done | sort) && [ "$l_got" = "$l_want" ] || { say "G holds main commits [$(echo $l_got)] after R, not T1_G_MAIN_L [$T1_G_MAIN_L]: G minus R would not be what the re-cut registers G minus R to be (item 6 (iii))"; exit 2; }; say "R guard (item 6): (i) R $T1_R_BIN_SHA is an ancestor of G; (ii) R is on M $T1_MAIN_M_SHA's first-parent chain; (iii) G's main commits after R are T1_G_MAIN_L [$T1_G_MAIN_L]"
[ "$(git -C "$RK_J3_TREE" rev-parse HEAD 2> /dev/null)" = "$T1_G_BIN_SHA" ] || { say "RK_J3_TREE $RK_J3_TREE is not at T1_G_BIN_SHA"; exit 2; }
for f in j3_test.go j3_conflicts_test.go; do
  cmp -s "$HERE/judge/$f" "$RK_J3_TREE/test/live/autosteer/t2/judge/$f" || { say "RK_J3_TREE's $f is not judge/$f byte for byte"; exit 2; }
done
# Addendum 11 7a, ruled 11-13 10:18 PST item 12: the base must be byte-equal to its own physical path (pwd -P).
[ ! -d "$RK_GOCACHE_BASE" ] || { gbp=$(cd -- "$RK_GOCACHE_BASE" && pwd -P); [ "$RK_GOCACHE_BASE" = "$gbp" ]; } || { say "RK_GOCACHE_BASE [$RK_GOCACHE_BASE] is not its physical path [$gbp]; refused, not followed"; exit 2; }
[ -d "$RK_GOCACHE_BASE" ] || { say "RK_GOCACHE_BASE $RK_GOCACHE_BASE is not a directory"; exit 2; }
gm=$RK_RUNS/t2-gocache-base-$(date -u +%Y%m%dT%H%M%SZ)-$$.manifest
gml=$(python3 -I -B "$HERE/cachemanifest.py" "$RK_GOCACHE_BASE" "$gm" 2>&1); say "gocache base $RK_GOCACHE_BASE: $gml ($gm)"
[ "$(printf '%s\n' "$gml" | sed -n 's/.* manifest_sha256 \([0-9a-f]*\)$/\1/p')" = "$T2_GOCACHE_BASE_MANIFEST_SHA256" ] || { say "the gocache base manifest is not T2_GOCACHE_BASE_MANIFEST_SHA256 (amend-gocache); batch refused"; exit 2; }
devr=$(stat -f %d "$RK_RUNS"); devb=$(stat -f %d "$RK_GOCACHE_BASE/."); devm=$(df -P "$RK_RUNS" | awk 'NR==2 { print $NF }')
[ -n "$devr" ] && [ "$devr" = "$devb" ] && mount | grep -q -F " on $devm (apfs," || { say "RK_RUNS $RK_RUNS (device $devr, mounted on $devm) and RK_GOCACHE_BASE $RK_GOCACHE_BASE (device $devb) are not one APFS device: cp -c -R would byte-copy (B684); batch refused"; exit 2; }
GCBK=$(du -sk "$RK_GOCACHE_BASE" | cut -f1)
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { say "$RK_CONFIG_DIR holds a CLAUDE.md"; exit 2; }
python3 -B -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(1 if d.get("hooks") else 0)' "$RK_CONFIG_DIR/settings.json" || { say "$RK_CONFIG_DIR/settings.json already holds hooks; arm U would not be unaided"; exit 2; }
# RK_T2_ORDER names the ARM:NAME pairs of one launch; the default is ruling C9's rotation over T1_N_PER_ARM replicates,
# so every stopping point between replicates holds equal n per arm. A later launch over the remaining names appends to
# the same log and rows file.
NMAX=${RK_T2_N_MAX:-$T1_N_PER_ARM}
case "$NMAX" in
  "$T1_N_PER_ARM") ;;
  10) awk -v c="${RK_T2_EXT_CAP_USD:-}" 'BEGIN { exit !(c ~ /^[0-9]+(\.[0-9]+)?$/ && c + 0 > 0) }' || { say "RK_T2_N_MAX=10 needs RK_T2_EXT_CAP_USD, the director's cap (r3 section 5)"; exit 2; } ;;
  *) say "RK_T2_N_MAX is [$NMAX]: $T1_N_PER_ARM or 10 only (r3 section 5)"; exit 2 ;;
esac
DEF=; i=1
while [ "$i" -le "$NMAX" ]; do
  case $(( (i - 1) % 3 )) in 0) arms="U R G" ;; 1) arms="R G U" ;; *) arms="G U R" ;; esac
  for a in $arms; do DEF="$DEF $a:t2-$(echo "$a" | tr GRU gru)$i"; done
  i=$((i + 1))
done
ORDER=${RK_T2_ORDER:-$DEF}
NREP=$(for s in $ORDER; do echo "${s#*:}"; done | sed 's/^t2-[gru]//; s/x$//' | sort -n | tail -1)
[ "${NREP:-0}" -ge "$T1_N_PER_ARM" ] 2> /dev/null || NREP=$T1_N_PER_ARM
say "order:$ORDER (n max $NMAX; extension cap ${RK_T2_EXT_CAP_USD:-none})"
for spec in $ORDER; do for nm in "${spec#*:}" "${spec#*:}x"; do [ -e "$RK_RUNS/$nm" ] && { say "$RK_RUNS/$nm exists"; exit 2; }; done; done
if [ "${RK_T2_SKIP_SMOKE:-}" != yes ]; then
  smoke=t2-smoke-$(date -u +%H%M%S); say "smoke $smoke; meter before $(meter)"
  sh "$HERE/smoke.sh" "$smoke" > "$RK_RUNS/$smoke.out" 2>&1; rc=$?; cat "$RK_RUNS/$smoke.out" >> "$LOG"
  [ "$rc" -eq 0 ] || { say "SMOKE FAIL rc=$rc; batch stopped; meter after $(meter)"; exit 3; }
  say "smoke passed: $(tail -1 "$RK_RUNS/$smoke.out"); meter after $(meter)"
  if [ "${RK_T2_SMOKE_ONLY:-}" = yes ]; then say "smoke only: the S-6 pins go into pins.txt, then the batch launches with RK_T2_SKIP_SMOKE=yes"; exit 0; fi
fi
# one_run ARM NAME: run.sh, the knobs check, then the sanity verdict; returns 0 pass, 10 cap_hit, 20 void, 3 harness.
one_run() {
  arm=$1; name=$2; D=$RK_RUNS/$name
  say "start $name ($arm)"
  GC=$RK_RUNS/$name.gocache; GCD=$D
  [ -e "$GC" ] && { say "HARNESS: $GC exists before its run"; GC=; return 3; }
  cp -c -R "$RK_GOCACHE_BASE" "$GC" || { say "HARNESS: the gocache clone $GC failed"; return 3; }
  GOCACHE=$GC RK_GOCACHE=$GC sh "$H/run.sh" "$arm" "$name" > "$RK_RUNS/$name.launch.txt" 2>&1; rc=$?
  python3 -I -B "$HERE/meter.py" end "$MLEDGER" "$name" 2>> "$LOG" || { say "HARNESS: meter end $name not written to $MLEDGER (run.sh rc=$rc)"; return 3; }
  say "end $name rc=$rc: $(tail -1 "$RK_RUNS/$name.launch.txt")"
  [ -d "$D" ] && printf 'gocache-base %s\ngocache-clone %s\n' "$RK_GOCACHE_BASE" "$GC" >> "$D/knobs"
  ks=$(sed -n 's/^seed //p' "$D/knobs" 2> /dev/null); km=$(sed -n 's/^managed-spec //p' "$D/knobs" 2> /dev/null)
  inp=no; case " $T2_PRODUCT_INPUT_ARMS " in *" $arm "*) [ "$arm" = U ] || inp=yes ;; esac
  wk=off; wm=off
  if [ "$inp" = yes ]; then wk=$T1_KNOWLEDGE_SEED_SHA256; [ -n "${RK_MANAGED:-}" ] && wm=$RK_MANAGED/managed-settings.json; fi
  { [ "$ks" = "$wk" ] && [ "$km" = "$wm" ]; } || { say "KNOBS FAIL $name ($arm): seed [$ks] managed-spec [$km], want [$wk] [$wm]"; return 3; }
  if [ "$inp" = yes ]; then
    case "$arm" in
      R) wj=$T2_R_INDEX_GRAPH_JSONL_SHA256; wn=$T2_R_KNOWLEDGE_NOTES_JSON_SHA256; wl=$T2_R_KNOWLEDGE_LEDGER_JSON_SHA256 ;;
      *) wj=$T2_G_INDEX_GRAPH_JSONL_SHA256; wn=$T2_G_KNOWLEDGE_NOTES_JSON_SHA256; wl=$T2_G_KNOWLEDGE_LEDGER_JSON_SHA256 ;;
    esac
    for want in "./graph.json $T2_GRAPH_CONFIG_SHA256" "./knowledge/t2-seed.json $T1_KNOWLEDGE_SEED_SHA256" \
        "./graph.jsonl $wj" "./knowledge/approach-notes.json $wn" "./knowledge/failure-ledger.json $wl"; do
      f=${want% *}; w=${want#* }
      got=$(awk -v f="$f" '$2 == f { print $1 }' "$D/index-sums.txt" 2> /dev/null)
      [ "$got" = "$w" ] || { say "INDEX FAIL $name ($arm): $f at [$got], pinned [$w]"; return 3; }
    done
  fi
  python3 -I -B - "$D" "$arm" "$T1_BUDGET_USD" <<'PY' >> "$LOG" 2>&1
import json, os, sys
D, arm, per_run = sys.argv[1], sys.argv[2], float(sys.argv[3])
def read(name):
    try:
        return open(os.path.join(D, name), errors="replace").read()
    except OSError:
        return None
def edited():
    status = (read("status.txt") or "").strip()
    calls, tools = 0, {}
    for line in (read("transcript.jsonl") or "").splitlines():
        try:
            m = json.loads(line).get("message")
        except ValueError:
            continue
        for b in (m.get("content") if isinstance(m, dict) and isinstance(m.get("content"), list) else []):
            if isinstance(b, dict) and b.get("type") == "tool_use":
                tools[str(b.get("name"))] = tools.get(str(b.get("name")), 0) + 1
                if b.get("name") in ("Edit", "Write", "MultiEdit", "NotebookEdit"):
                    calls += 1
    with open(os.path.join(D, "tools.txt"), "w") as f:
        f.write(" ".join("%s=%d" % (k, tools[k]) for k in sorted(tools)) + "\n")
    return bool(status) or calls > 0, "status.txt %d paths, edit calls %d" % (len(status.splitlines()) if status else 0, calls)
def verdict(kind, why, rc):
    print("  sanity %s: %s" % (kind, why))
    if kind in ("cap_hit", "void"):
        with open(os.path.join(D, "flags" if kind == "cap_hit" else "void"), "a") as f:
            f.write("%s %s\n" % (kind, why))
    sys.exit(rc)
did, how = edited()
err = " ".join((read("stderr.txt") or "").split())[-200:]
try:
    r = json.load(open(os.path.join(D, "result.json")))
except (OSError, ValueError) as e:
    r = None
    infra = "no readable result.json (%s); stderr tail [%s]" % (type(e).__name__, err)
if r is not None:
    print("  result: subtype %s is_error %s turns %s cost %s duration_ms %s api_error_status %s"
          % (r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd"), r.get("duration_ms"), r.get("api_error_status")))
    text = str(r.get("result"))
    low = text.lower()
    infra = None
    if r.get("type") != "result":
        infra = "result.json type %s" % r.get("type")
    elif "not logged in" in low:
        infra = "not logged in: %s" % text[:160]
    elif any(w in low for w in ("hit your weekly limit", "hit your 5-hour limit", "rate_limit", "rate limit")):
        infra = "a limit text: %s" % text[:160]
    elif r.get("api_error_status"):
        infra = "api_error_status %s: %s" % (r.get("api_error_status"), text[:160])
    elif (r.get("num_turns") or 0) < 2:
        infra = "num_turns %s: the API refused the run; %s" % (r.get("num_turns"), text[:160])
if infra is not None:
    if did:
        verdict("harness", "%s, after an edit (%s): not void, the batch stops" % (infra, how), 3)
    verdict("void", "%s; no edit (%s)" % (infra, how), 20)
if not os.path.exists(os.path.join(D, "transcript.jsonl")):
    verdict("harness", "no transcript", 3)
if arm in ("R", "G") and not os.path.exists(os.path.join(D, "hooks.log")):
    verdict("harness", "no hooks.log", 3)
cost = r.get("total_cost_usd")
if r.get("subtype") in ("error_max_turns", "error_max_budget_usd"):
    verdict("cap_hit", "subtype %s, turns %s, cost %s (%s)" % (r.get("subtype"), r.get("num_turns"), cost, how), 10)
if isinstance(cost, (int, float)) and cost > per_run:
    verdict("cap_hit", "cost %.4f over the per-run cap %.2f, subtype %s (%s)" % (cost, per_run, r.get("subtype"), how), 10)
if r.get("is_error"):
    verdict("harness", "is_error true, subtype %s: %s" % (r.get("subtype"), str(r.get("result"))[:200]), 3)
verdict("pass", "subtype %s (%s)" % (r.get("subtype"), how), 0)
PY
  src1=$?
  # V1 (addendum 11 section 9, ruled 10:50 PST 11-14): a G run's store, after its consent, reads record scope v5 with
  # the session-content marker set (consentscope.py: the fold and both markers). A G run that does not is a harness
  # failure: the batch stops; never void, never rerun. R records v5 without the marker by design and is not read.
  if [ "$arm" = G ] && { [ "$src1" = 0 ] || [ "$src1" = 10 ]; }; then
    v1=$(python3 -I -B "$HERE/consentscope.py" "$D/store" 2>&1); v1rc=$?
    say "V1 $name: $v1"
    [ "$v1rc" = 0 ] || { say "HARNESS: V1 $name: G's store does not read record scope v5 with the session-content marker set (rc $v1rc)"; return 3; }
  fi
  return "$src1"
}
# judge_run ARM NAME: the channel check (R and G), the judge, the row, then the batch cap.
judge_run() {
  arm=$1; name=$2; D=$RK_RUNS/$name
  if [ "$arm" != U ]; then
    python3 -B "$H/t1c/channel.py" "$D" "$arm" >> "$LOG" 2>&1 || stop "CHANNEL CLOSED $name (B448); batch stopped"
  fi
  say "judge $name"
  sh "$HERE/judge/judge.sh" "$D" >> "$LOG" 2>&1; jrc=$?
  { [ "$jrc" -ne 3 ] && [ -f "$D/judge/row.txt" ]; } || stop "JUDGE FAIL $name rc=$jrc; batch stopped"
  say "row: $(cat "$D/judge/row.txt")$( [ -f "$D/flags" ] && printf ' [flags: %s]' "$(tr '\n' ' ' < "$D/flags")") [tools: $(cat "$D/tools.txt" 2> /dev/null)]"
  j2setup "$D"
  python3 -I -B - "$RK_RUNS" "$name" "$T1_BATCH_CAP_USD" "${RK_T2_EXT_CAP_USD:-0}" "$T1_N_PER_ARM" <<'PY' 2>&1 | tee -a "$LOG"; mrc=$(tail -1 "$LOG" | sed -n 's/^  money rc=//p')
import json, os, re, sys
runs, name, cap, ext, n = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4]), int(sys.argv[5])
def cost(x):
    try:
        return float(json.load(open(os.path.join(runs, x, "result.json"))).get("total_cost_usd"))
    except (OSError, ValueError, TypeError):
        return None
def rep(x):
    m = re.fullmatch(r"t2-[gru]([0-9]+)x?", x)
    return int(m.group(1)) if m else None
mine = rep(name) is not None and rep(name) > n
lim, scope = (ext, "replicates %d to 10, RK_T2_EXT_CAP_USD" % (n + 1)) if mine else (cap, "replicates 1 to %d, T1_BATCH_CAP_USD" % n)
spent = {x: cost(x) for x in os.listdir(runs) if rep(x) is not None and (rep(x) > n) == mine}
if spent.get(name) is None:
    print("  run %s: no total_cost_usd in result.json; the batch cap cannot be read" % name)
    print("  money rc=5"); sys.exit(0)
total = sum(v for v in spent.values() if v is not None)
print("  run %s cost %.2f USD; spend so far %.2f USD of the %.2f cap (%s) over %d runs (void runs included)"
      % (name, spent[name], total, lim, scope, sum(v is not None for v in spent.values())))
print("  money rc=%d" % (1 if total > lim else 0))
PY
  case "$mrc" in
    0) ;;
    1) stop "BATCH CAP: t2-* spend passed its cap (T1_BATCH_CAP_USD, or RK_T2_EXT_CAP_USD from replicate 4); batch stopped (pre-registered, t2/pins.txt; r3 section 5)" ;;
    *) stop "MONEY READ FAILED after $name (rc '$mrc'); batch stopped" ;;
  esac
}
# The meter gate runs before every run: with the weekly line at a replicate's first run, without it for the rest and
# for a rerun, since the five-hour rule (3d) is per run. 3c (the director 14:23 PST item 1): the cap is computed once,
# at this launch's first GO, from W0, the higher weekly reading of that pair; before every later replicate the spend of
# this launch's own runs (RAN, void runs included) is read against it, and at or over it the batch stops.
rep_prev=; RAN=; CAP3C=
for spec in $ORDER; do
  arm=${spec%%:*}; name=${spec#*:}
  rep=$(echo "$name" | sed 's/^t2-[gru]//')
  if [ "$rep" != "$rep_prev" ]; then
    if [ -n "$RAN" ]; then
      sp=$(python3 -I -B "$HERE/meter.py" spend "$RK_RUNS" "$CAP3C" $RAN 2>&1); sprc=$?
      say "3c before replicate $rep: $sp"
      case "$sprc" in
        0) ;;
        1) stop "3C CAP: this launch's spend reached the 3c cap $CAP3C USD before replicate $rep; batch stopped (the director 14:23 PST item 1)" ;;
        *) stop "3C SPEND READ FAILED before replicate $rep (rc $sprc); batch stopped" ;;
      esac
    fi
    if [ -z "$rep_prev" ]; then lim=$T2_METER_LAUNCH_PCT; key=T2_METER_LAUNCH_PCT; else lim=$T2_METER_STOP_PCT; key=T2_METER_STOP_PCT; fi
    mgate "$name" --weekly "$lim" --weekly-key "$key"
    m1=$MW1; m2=$MW2
    say "replicate $rep: meter weekly $m1 then $m2, five-hour $MF1 then $MF2; runs at $lim or under ($key)"
    if [ -z "$CAP3C" ]; then
      W0=$m1; [ "$m2" -gt "$W0" ] && W0=$m2
      c3=$(python3 -I -B "$HERE/meter.py" cap "$T2_METER_STOP_PCT" "$W0" "$T2_USD_PER_WEEKLY_PT" 2>&1) || stop "3C CAP not computed from W0 $W0: $c3; batch stopped"
      CAP3C=${c3%% *}
      say "3c cap $c3 (W0 $W0, the higher weekly reading of this launch's first gated pair; T1_BATCH_CAP_USD and RK_T2_EXT_CAP_USD stand, the lower binds)"
    fi
    if [ "$rep" = 1 ]; then M0=$m1; [ "$m2" -lt "$M0" ] && M0=$m2; say "extension M0 $M0"; fi
    if [ "$rep" = $((T1_N_PER_ARM + 1)) ]; then
      M0=$(sed -n 's/^[^ ]* extension M0 \([0-9][0-9]*\)$/\1/p' "$LOG" | tail -1)
      [ -n "$M0" ] || { say "extension declined: no extension M0 line in $LOG; the batch ends at n=$T1_N_PER_ARM"; break; }
      M3=$m1; [ "$m2" -gt "$M3" ] && M3=$m2
      P3=$((3 * M3 + 7 * (M3 - M0)))
      say "extension before replicate $rep: M0 $M0 (the lower reading before replicate 1), M3 $M3 (the higher reading now); M3 + (7/3) x (M3 - M0) = $M3 + (7/3) x $((M3 - M0)) = $(awk -v p="$P3" 'BEGIN { printf "%.2f", p / 3 }') (3 x $M3 + 7 x $((M3 - M0)) = $P3 against $((3 * T2_METER_EXT_PCT))); $T2_METER_EXT_PCT or under runs (T2_METER_EXT_PCT; r3 section 5 as amended)"
      [ "$P3" -le $((3 * T2_METER_EXT_PCT)) ] || { say "extension declined: the projection is over $T2_METER_EXT_PCT (T2_METER_EXT_PCT); the batch ends at n=$T1_N_PER_ARM"; break; }
      say "extension runs: replicates $rep to $NMAX"
    fi
    rep_prev=$rep
  else
    mgate "$name"
  fi
  try=$name
  while :; do
    RAN="$RAN $try"
    one_run "$arm" "$try"; src=$?
    case "$src" in
      0|10) judge_run "$arm" "$try"; gcdrop || stop "GOCACHE DROP FAILED $try: its clone is not removed, so S-8's peak would count two; batch stopped (addendum 8, 5b)"; break ;;
      20) gcdrop || stop "GOCACHE DROP FAILED $try: its clone is not removed, so S-8's peak would count two; batch stopped (addendum 8, 5b)"
          [ "$try" = "$name" ] || stop "VOID TWICE: $name and $try both void; batch stopped"
          say "VOID $name: $(cat "$RK_RUNS/$name/void" 2> /dev/null | tr '\n' ' ')rerun once as ${name}x (ruling C8)"
          try=${name}x
          mgate "$try" ;;
      *) stop "SANITY FAIL $try (harness; never rerun its name); batch stopped" ;;
    esac
  done
done
say "rows ($RK_RUNS/t2-rows.txt):"; cat "$RK_RUNS/t2-rows.txt" | tee -a "$LOG"
count
say "=== t2 batch done; meter $(meter)"
