#!/bin/sh
# batch.sh: the T1c batch (design B, pins.txt). The channel smoke (smoke.sh), then n=10 per arm in the order G, R, U
# repeated ten times, serial through the one logged-in configuration directory, each run checked (channel.py) and
# judged (judge/judge.sh) as it ends, rows in RK_RUNS/t1c-rows.txt. After every run: the run's cost against the per-run
# cap (T1_BUDGET_USD) and the judged t1c-* spend against T1_BATCH_CAP_USD; once every arm has three judged runs, the
# pooled mean of those nine against T1_N3_MEAN_CEILING_USD (read once, kept in RK_RUNS/t1c-n3-read.txt); any of them
# over stops the batch. Runs nothing without RK_T1C_GO=yes, which only the owner's word sets.
# A harness failure stops the batch: a run with no result, transcript or hook log; a run whose channel was closed
# (B448: a steer row suppressed by no_mirror or unverified_channel, or for G no additionalContext from the prompt hook);
# a judge that refused (exit 3: a tool missing, B449) or wrote no row. PATH gets RK_GO_TOOLCHAIN_BIN first, so the
# judges' go is the toolchain's (B449: the pilot's driver PATH held no go), and every tool is checked before the first
# run. A run that the API refused is void and stops the batch before the channel check and the judge (B453: t1c-g1 ended
# at turn 1 with is_error true under HTTP 429 seven_day and was judged as a row): is_error true, num_turns under 2, or a
# limit text in the result. The configuration directory's account (oauthAccount email and accountUuid) is logged at
# start beside the meter, because the meter file is the main configuration's account and the run's may differ (B454);
# the batch refuses a configuration directory whose accountUuid is not the meter's (RK_METER_ACCOUNT_JSON, default
# $HOME/.claude.json, the main configuration's account file).
# Required environment, none defaulted here so a batch is reproducible from its launch line: RK_CONFIG_DIR,
# RK_RUNS, RK_GOCACHE, RK_BIN_R, RK_BIN_G, RK_PGRAPH, RK_PGRAPH_G, RK_KNOWLEDGE, RK_GO_TOOLCHAIN_BIN, RK_GOSLOT,
# RK_FIXTURES, RK_BASELINE_LOG; RK_METER_FILE (optional, the usage meter json to log at start and end). The run.sh
# overrides RK_PROMPT_FILE, RK_MODEL, RK_MAX_TURNS and RK_BUDGET_USD are cleared, so every run uses the frozen prompt,
# model and caps. RK_T1C_SKIP_SMOKE=yes skips the smoke when one already passed on this harness build.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
H=$HERE/..
[ "${RK_T1C_GO:-}" = yes ] || { echo "batch.sh: RK_T1C_GO is not yes; nothing runs without the owner's word" >&2; exit 2; }
for v in RK_CONFIG_DIR RK_RUNS RK_GOCACHE RK_BIN_R RK_BIN_G RK_PGRAPH RK_PGRAPH_G RK_KNOWLEDGE RK_GO_TOOLCHAIN_BIN RK_GOSLOT RK_FIXTURES RK_BASELINE_LOG; do
  eval "val=\${$v:-}"; [ -n "$val" ] || { echo "batch.sh: $v unset" >&2; exit 2; }
done
export PATH="$RK_GO_TOOLCHAIN_BIN:$PATH"
unset RK_PROMPT_FILE RK_MODEL RK_MAX_TURNS RK_BUDGET_USD
for tool in claude go gofmt python3 perl git uuidgen; do command -v "$tool" > /dev/null 2>&1 || { echo "batch.sh: $tool is not on PATH" >&2; exit 2; }; done
export RK_SLICE=t1c
. "$HERE/pins.txt"
LOG=$RK_RUNS/t1c-batch.log
mkdir -p "$RK_RUNS"
say() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LOG"; }
meter() { if [ -n "${RK_METER_FILE:-}" ] && [ -f "$RK_METER_FILE" ]; then python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("ts"), d.get("rate_limits"))' "$RK_METER_FILE" 2>&1; else echo "no meter file"; fi; }
account() { python3 -c 'import json,sys; a=(json.load(open(sys.argv[1])).get("oauthAccount") or {}); print(a.get("emailAddress"), a.get("accountUuid"))' "${1:-$RK_CONFIG_DIR/.claude.json}" 2>&1; }
say "=== t1c batch start; $(go version); claude $(claude --version 2>&1 | head -1); meter $(meter) (the main configuration's account; see B454); config $RK_CONFIG_DIR account $(account); harness $(git -C "$H" rev-parse --short HEAD 2>/dev/null)"
[ "$(account)" = "$(account "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}")" ] || { say "config account $(account) is not the meter's $(account "${RK_METER_ACCOUNT_JSON:-$HOME/.claude.json}") (B454)"; exit 2; }
for f in "$RK_BIN_R" "$RK_BIN_G" "$RK_PGRAPH" "$RK_PGRAPH_G" "$RK_BASELINE_LOG"; do [ -e "$f" ] || { say "missing $f"; exit 2; }; done
[ "$(shasum -a 256 "$RK_BASELINE_LOG" | cut -d' ' -f1)" = "$T1_BASELINE_LOG_SHA256" ] || { say "baseline log digest moved"; exit 2; }
for pair in "$RK_BIN_G:$T1_G_BIN_SHA" "$RK_PGRAPH_G:$T1_G_PGRAPH_SHA" "$RK_BIN_R:$T1_R_BIN_SHA" "$RK_PGRAPH:$T1_PGRAPH_SHA"; do
  bin=${pair%%:*}; want=${pair#*:}
  go version -m "$bin" 2>/dev/null | grep -q "vcs.revision=$want" || { say "$bin: embedded vcs.revision is not the pinned $want (the T1b G binary carried 158c7a29 under a 1938fbc3 pin)"; exit 2; }
done
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { say "$RK_CONFIG_DIR holds a CLAUDE.md"; exit 2; }
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(1 if d.get("hooks") else 0)' "$RK_CONFIG_DIR/settings.json" || { say "$RK_CONFIG_DIR/settings.json already holds hooks; arm U would not be unaided"; exit 2; }
# RK_T1C_ORDER names the ARM:NAME pairs of one launch; the default is the whole design, G R U interleaved ten times so
# every stopping point holds equal n per arm (G first: the arm that waited on the freeze). A later launch over the
# remaining names, after a stop is resolved, appends to the same log and rows file; the caps read every judged t1c-* run.
ORDER=${RK_T1C_ORDER:-"G:t1c-g1 R:t1c-r1 U:t1c-u1 G:t1c-g2 R:t1c-r2 U:t1c-u2 G:t1c-g3 R:t1c-r3 U:t1c-u3 G:t1c-g4 R:t1c-r4 U:t1c-u4 G:t1c-g5 R:t1c-r5 U:t1c-u5 G:t1c-g6 R:t1c-r6 U:t1c-u6 G:t1c-g7 R:t1c-r7 U:t1c-u7 G:t1c-g8 R:t1c-r8 U:t1c-u8 G:t1c-g9 R:t1c-r9 U:t1c-u9 G:t1c-g10 R:t1c-r10 U:t1c-u10"}
for spec in $ORDER; do [ -e "$RK_RUNS/${spec#*:}" ] && { say "$RK_RUNS/${spec#*:} exists"; exit 2; }; done
if [ "${RK_T1C_SKIP_SMOKE:-}" != yes ]; then
  smoke=t1c-smoke-$(date -u +%H%M%S); say "smoke $smoke"
  sh "$HERE/smoke.sh" "$smoke" > "$RK_RUNS/$smoke.out" 2>&1; rc=$?; cat "$RK_RUNS/$smoke.out" >> "$LOG"
  [ "$rc" -eq 0 ] || { say "SMOKE FAIL rc=$rc; batch stopped"; exit 3; }
  say "smoke passed: $(tail -1 "$RK_RUNS/$smoke.out")"
fi
for spec in $ORDER; do
  arm=${spec%%:*}; name=${spec#*:}; D=$RK_RUNS/$name
  say "start $name ($arm)"
  sh "$H/run.sh" "$arm" "$name" > "$RK_RUNS/$name.launch.txt" 2>&1; rc=$?
  say "end $name rc=$rc: $(tail -1 "$RK_RUNS/$name.launch.txt")"
  python3 - "$D" "$arm" <<'PY' >> "$LOG" 2>&1 || { say "SANITY FAIL $name (the run is void; never rerun its name); batch stopped"; exit 3; }
import json, os, sys
D, arm = sys.argv[1:3]
r = json.load(open(os.path.join(D, "result.json")))
assert r.get("type") == "result", r
print("  result: subtype %s is_error %s turns %s cost %s duration_ms %s" % (r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd"), r.get("duration_ms")))
assert "Not logged in" not in str(r.get("result")), "not logged in"
assert not r.get("is_error"), "is_error true: %s" % str(r.get("result"))[:200]
assert (r.get("num_turns") or 0) >= 2, "num_turns %s: the API refused the run" % r.get("num_turns")
assert not any(w in str(r.get("result")).lower() for w in ("hit your weekly limit", "hit your 5-hour limit", "rate_limit", "rate limit")), "a limit text in the result: %s" % str(r.get("result"))[:200]
assert os.path.exists(os.path.join(D, "transcript.jsonl")), "no transcript"
if arm in ("R", "G"):
    assert os.path.exists(os.path.join(D, "hooks.log")), "no hooks.log"
    assert "recap pending" in open(os.path.join(D, "hooks.log")).read(), "no prompt hook in hooks.log"
PY
  if [ "$arm" != U ]; then
    python3 "$HERE/channel.py" "$D" "$arm" >> "$LOG" 2>&1 || { say "CHANNEL CLOSED $name (B448); batch stopped"; exit 3; }
  fi
  say "judge $name"
  sh "$HERE/judge/judge.sh" "$D" >> "$LOG" 2>&1; jrc=$?
  { [ "$jrc" -ne 3 ] && [ -f "$D/judge/row.txt" ]; } || { say "JUDGE FAIL $name rc=$jrc; batch stopped"; exit 3; }
  say "row: $(cat "$D/judge/row.txt")"
  python3 - "$RK_RUNS" "$name" "$T1_BUDGET_USD" "$T1_BATCH_CAP_USD" "$T1_N3_MEAN_CEILING_USD" <<'PY' 2>&1 | tee -a "$LOG"; mrc=$(tail -1 "$LOG" | sed -n 's/^  money rc=//p')
import json, os, re, sys
runs, name, per_run, cap, ceiling = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4]), float(sys.argv[5])
def cost(n):
    p = os.path.join(runs, n, "judge", "a8.json")
    return (json.load(open(p)).get("cost_usd") or 0.0) if os.path.exists(p) else None
judged = {n: cost(n) for n in os.listdir(runs) if re.fullmatch(r"t1c-[gru][0-9]+", n) and cost(n) is not None}
def done(rc):
    print("  money rc=%d" % rc)
    sys.exit(0)
if judged.get(name) is None:
    print("  run %s: no judged cost (judge/a8.json); the caps cannot be read" % name); done(5)
print("  run %s cost %.2f USD of the %.2f per-run cap" % (name, judged[name], per_run))
if judged[name] > per_run:
    done(4)
spent = sum(judged.values())
print("  batch spend so far %.2f USD of the %.2f cap over %d judged runs" % (spent, cap, len(judged)))
if spent > cap:
    done(1)
marker = os.path.join(runs, "t1c-n3-read.txt")
# the first three judged runs of each arm by name number, so a void run's replacement name (never a rerun) still counts
first3 = {a: [judged[n] for n in sorted((n for n in judged if n[4] == a), key=lambda n: int(n[5:]))[:3]] for a in "gru"}
if not os.path.exists(marker) and all(len(c) == 3 for c in first3.values()):
    pooled = [v for c in first3.values() for v in c]
    mean = sum(pooled) / len(pooled)
    line = "n=3 ceiling read once: pooled mean %.3f USD over 9 runs against %.2f (185/30 = 6.17); arm means %s" % (
        mean, ceiling, ", ".join("%s %.3f" % (a.upper(), sum(c) / 3) for a, c in sorted(first3.items())))
    open(marker, "w").write(line + "\n")
    print("  " + line)
    if mean > ceiling:
        done(2)
done(0)
PY
  case "$mrc" in
    0) ;;
    1) say "BATCH CAP: judged t1c-* spend passed T1_BATCH_CAP_USD; batch stopped (pre-registered, t1c/pins.txt)"; exit 3 ;;
    2) say "N3 CEILING: the pooled n=3 mean passed T1_N3_MEAN_CEILING_USD; batch stopped before the tenth run (pins.txt delta 6)"; exit 3 ;;
    4) say "PER-RUN CAP: $name cost more than T1_BUDGET_USD; the run's cap did not hold; batch stopped"; exit 3 ;;
    *) say "MONEY READ FAILED after $name (rc '$mrc'); batch stopped"; exit 3 ;;
  esac
done
say "rows ($RK_RUNS/t1c-rows.txt):"; cat "$RK_RUNS/t1c-rows.txt" | tee -a "$LOG"
python3 - "$RK_RUNS" "$T1_N3_MEAN_CEILING_USD" <<'PY' 2>&1 | tee -a "$LOG"
import json, os, re, sys
runs, ceiling = sys.argv[1], float(sys.argv[2])
by = {}
for n in sorted(os.listdir(runs)):
    p = os.path.join(runs, n, "judge", "a8.json")
    if re.fullmatch(r"t1c-[gru][0-9]+", n) and os.path.exists(p):
        arm = open(os.path.join(runs, n, "arm")).read().strip()
        by.setdefault(arm, []).append(json.load(open(p)).get("cost_usd") or 0.0)
for arm in sorted(by):
    c = by[arm]; print("arm %s n=%d mean cost %.3f USD (%s); the n=3 ceiling %.2f was read once (t1c-n3-read.txt)" % (arm, len(c), sum(c)/len(c), ", ".join("%.3f" % x for x in c), ceiling))
# the gated primary per arm, then the pre-registered sensitivity line (pins.txt delta 2): the same read over the runs
# with an edit, so a pass by a run that never edited is visible beside it
rows = {}
for n in sorted(os.listdir(runs)):
    p = os.path.join(runs, n, "judge", "row.txt")
    if re.fullmatch(r"t1c-[gru][0-9]+", n) and os.path.exists(p):
        f = [x.strip() for x in open(p).read().strip().split(" | ")]
        rows.setdefault(f[0], []).append((f[-1].endswith(" pass"), "first_edit none" not in f))
for arm in sorted(rows):
    r = rows[arm]; e = [ok for ok, edited in r if edited]
    print("arm %s primary A1a all-four %d/%d pass; over the runs with an edit %d/%d (sensitivity, ungated)" % (arm, sum(ok for ok, _ in r), len(r), sum(e), len(e)))
PY
say "=== t1c batch done; meter $(meter)"
