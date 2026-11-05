#!/bin/sh
# batch.sh: the T1b batch. The channel smoke (smoke.sh), then n=3 per arm in the order U, R, G, U, R, G, U, R, G, serial
# through the one logged-in configuration directory, each run checked (channel.py) and judged (judge/judge.sh) as it
# ends, rows in RK_RUNS/t1b-rows.txt, then the per-arm cost means against the n=10 gate (T1_N3_MEAN_CEILING_USD,
# ruling 12). Runs nothing without RK_T1B_GO=yes, which only the owner's word sets.
# A harness failure stops the batch: a run with no result, transcript or hook log; a run whose channel was closed
# (B448: a steer row suppressed by no_mirror or unverified_channel, or for G no additionalContext from the prompt hook);
# a judge that refused (exit 3: a tool missing, B449) or wrote no row. PATH gets RK_GO_TOOLCHAIN_BIN first, so the
# judges' go is the toolchain's (B449: the pilot's driver PATH held no go), and every tool is checked before the first
# run. Required environment, none defaulted here so a batch is reproducible from its launch line: RK_CONFIG_DIR,
# RK_RUNS, RK_GOCACHE, RK_BIN_R, RK_BIN_G, RK_PGRAPH, RK_PGRAPH_G, RK_KNOWLEDGE, RK_GO_TOOLCHAIN_BIN, RK_GOSLOT,
# RK_FIXTURES, RK_BASELINE_LOG; RK_METER_FILE (optional, the usage meter json to log at start and end). The run.sh
# overrides RK_PROMPT_FILE, RK_MODEL, RK_MAX_TURNS and RK_BUDGET_USD are cleared, so every run uses the frozen prompt,
# model and caps. RK_T1B_SKIP_SMOKE=yes skips the smoke when one already passed on this harness build.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
H=$HERE/..
[ "${RK_T1B_GO:-}" = yes ] || { echo "batch.sh: RK_T1B_GO is not yes; nothing runs without the owner's word" >&2; exit 2; }
for v in RK_CONFIG_DIR RK_RUNS RK_GOCACHE RK_BIN_R RK_BIN_G RK_PGRAPH RK_PGRAPH_G RK_KNOWLEDGE RK_GO_TOOLCHAIN_BIN RK_GOSLOT RK_FIXTURES RK_BASELINE_LOG; do
  eval "val=\${$v:-}"; [ -n "$val" ] || { echo "batch.sh: $v unset" >&2; exit 2; }
done
export PATH="$RK_GO_TOOLCHAIN_BIN:$PATH"
unset RK_PROMPT_FILE RK_MODEL RK_MAX_TURNS RK_BUDGET_USD
for tool in claude go gofmt python3 perl git uuidgen; do command -v "$tool" > /dev/null 2>&1 || { echo "batch.sh: $tool is not on PATH" >&2; exit 2; }; done
export RK_SLICE=t1b
. "$HERE/pins.txt"
LOG=$RK_RUNS/t1b-batch.log
mkdir -p "$RK_RUNS"
say() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LOG"; }
meter() { if [ -n "${RK_METER_FILE:-}" ] && [ -f "$RK_METER_FILE" ]; then python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("ts"), d.get("rate_limits"))' "$RK_METER_FILE" 2>&1; else echo "no meter file"; fi; }
say "=== t1b batch start; $(go version); claude $(claude --version 2>&1 | head -1); meter $(meter); config $RK_CONFIG_DIR; harness $(git -C "$H" rev-parse --short HEAD 2>/dev/null)"
for f in "$RK_BIN_R" "$RK_BIN_G" "$RK_PGRAPH" "$RK_PGRAPH_G" "$RK_BASELINE_LOG"; do [ -e "$f" ] || { say "missing $f"; exit 2; }; done
[ "$(shasum -a 256 "$RK_BASELINE_LOG" | cut -d' ' -f1)" = "$T1_BASELINE_LOG_SHA256" ] || { say "baseline log digest moved"; exit 2; }
[ -f "$RK_CONFIG_DIR/CLAUDE.md" ] && { say "$RK_CONFIG_DIR holds a CLAUDE.md"; exit 2; }
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(1 if d.get("hooks") else 0)' "$RK_CONFIG_DIR/settings.json" || { say "$RK_CONFIG_DIR/settings.json already holds hooks; arm U would not be unaided"; exit 2; }
ORDER="U:t1b-u1 R:t1b-r1 G:t1b-g1 U:t1b-u2 R:t1b-r2 G:t1b-g2 U:t1b-u3 R:t1b-r3 G:t1b-g3"
for spec in $ORDER; do [ -e "$RK_RUNS/${spec#*:}" ] && { say "$RK_RUNS/${spec#*:} exists"; exit 2; }; done
if [ "${RK_T1B_SKIP_SMOKE:-}" != yes ]; then
  smoke=t1b-smoke-$(date -u +%H%M%S); say "smoke $smoke"
  sh "$HERE/smoke.sh" "$smoke" > "$RK_RUNS/$smoke.out" 2>&1; rc=$?; cat "$RK_RUNS/$smoke.out" >> "$LOG"
  [ "$rc" -eq 0 ] || { say "SMOKE FAIL rc=$rc; batch stopped"; exit 3; }
  say "smoke passed: $(tail -1 "$RK_RUNS/$smoke.out")"
fi
for spec in $ORDER; do
  arm=${spec%%:*}; name=${spec#*:}; D=$RK_RUNS/$name
  say "start $name ($arm)"
  sh "$H/run.sh" "$arm" "$name" > "$RK_RUNS/$name.launch.txt" 2>&1; rc=$?
  say "end $name rc=$rc: $(tail -1 "$RK_RUNS/$name.launch.txt")"
  python3 - "$D" "$arm" <<'PY' >> "$LOG" 2>&1 || { say "SANITY FAIL $name; batch stopped"; exit 3; }
import json, os, sys
D, arm = sys.argv[1:3]
r = json.load(open(os.path.join(D, "result.json")))
assert r.get("type") == "result", r
print("  result: subtype %s is_error %s turns %s cost %s duration_ms %s" % (r.get("subtype"), r.get("is_error"), r.get("num_turns"), r.get("total_cost_usd"), r.get("duration_ms")))
assert "Not logged in" not in str(r.get("result")), "not logged in"
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
done
say "rows ($RK_RUNS/t1b-rows.txt):"; cat "$RK_RUNS/t1b-rows.txt" | tee -a "$LOG"
python3 - "$RK_RUNS" "$T1_N3_MEAN_CEILING_USD" <<'PY' 2>&1 | tee -a "$LOG"
import json, os, sys
runs, ceiling = sys.argv[1], float(sys.argv[2])
by = {}
for n in sorted(os.listdir(runs)):
    p = os.path.join(runs, n, "judge", "a8.json")
    if n.startswith("t1b-") and os.path.exists(p):
        arm = open(os.path.join(runs, n, "arm")).read().strip()
        by.setdefault(arm, []).append(json.load(open(p)).get("cost_usd") or 0.0)
for arm in sorted(by):
    c = by[arm]; print("arm %s n=%d mean cost %.3f USD (%s); n=10 gate at %.2f: %s" % (arm, len(c), sum(c)/len(c), ", ".join("%.3f" % x for x in c), ceiling, "open" if sum(c)/len(c) <= ceiling else "CLOSED (owner's word)"))
PY
say "=== t1b batch done; meter $(meter)"
