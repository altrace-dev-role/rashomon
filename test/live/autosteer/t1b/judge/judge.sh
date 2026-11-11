#!/bin/sh
# judge.sh RUNDIR: every T1b judge in order (A1, A8 from the transcript and result; A2, A3 to A5, A6, A7 on the final
# tree through the slot), then one evidence row (section 7.5) appended to RK_RUNS/t1b-rows.txt and printed.
# T1b is slice T1 under the corrected harness (B448 to B450). The judges are T1's bytes except: this script runs no
# judge and writes no row unless go, gofmt, python3, perl and git are on PATH, exit 3 (B449: the pilot's batch PATH
# held no go, every Go-backed judge exited 127 and the rows read them as unparsed and red); a6.sh judges gofmt by
# exit status and output together (B450); the rows file is the slice's own.
# Environment: see a2.sh, a3.sh, a6.sh, a7.sh; RK_RUNS (default /tmp/rk-autosteer/runs).
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
for tool in go gofmt python3 perl git; do
  command -v "$tool" > /dev/null 2>&1 || { echo "judge.sh: $tool is not on PATH; no judge runs and no row is written (B449)" >&2; exit 3; }
done
python3 "$HERE/a1.py" "$D" > /dev/null || echo "a1 failed"
python3 "$HERE/a8.py" "$D" > /dev/null || echo "a8 failed"
sh "$HERE/a2.sh" "$D" > /dev/null || echo "a2 failed"
sh "$HERE/a3.sh" "$D" > /dev/null || echo "a3 failed"
sh "$HERE/a6.sh" "$D" > /dev/null || echo "a6 failed"
sh "$HERE/a7.sh" "$D" > /dev/null || echo "a7 failed"
python3 - "$D" "$RK_RUNS" <<'PY'
import json, os, sys
D, runs = sys.argv[1:3]
def j(name):
    p = os.path.join(D, "judge", name)
    return json.load(open(p)) if os.path.exists(p) else {}
a1, a2, a3, a6, a7, a8 = (j(n) for n in ("a1.json", "a2.json", "a3.json", "a6.json", "a7.json", "a8.json"))
arm = open(os.path.join(D, "arm")).read().strip() if os.path.exists(os.path.join(D, "arm")) else "?"
sha = open(os.path.join(D, "tree-sha")).read().strip() if os.path.exists(os.path.join(D, "tree-sha")) else ""
a1a = a1.get("a1a", {}); opened = sum(1 for v in a1a.values() if v == "opened"); searched = sum(1 for v in a1a.values() if v == "searched")
a1c = a1.get("a1c") or {}
row = " | ".join([arm, os.path.basename(D), sha[:8],
                  "A1a %d/%d opened %d searched" % (opened, len(a1a), searched), "A1b %s" % a1.get("a1b"),
                  "A1c %s" % ("n/a" if not a1c else "unit=%s reuse=%s consumer=%s knowledge=%s" % (a1c.get("unit"), a1c.get("reuse"), a1c.get("consumer"), a1c.get("knowledge"))),
                  "A2 %s" % a2.get("a2"), "A3 %s" % a3.get("a3"), "A4 %s" % a3.get("a4"), "A5 %s" % a3.get("a5"),
                  "A6 %s" % a6.get("a6"), "A7 %s%s" % ("green" if a7.get("a7_green") else "red", " unbacked-done" if (a7.get("stop") or {}).get("unbacked_done") else ""),
                  "cost %s" % a8.get("cost_usd"), "turns %s" % a8.get("turns"), "calls %s" % a8.get("tool_calls")])
primary = bool(a2.get("a2")) and a3.get("a3") == "both" and a3.get("a4") == "match"
row += " | primary %s" % ("pass" if primary else "fail")
open(os.path.join(D, "judge", "row.txt"), "w").write(row + "\n")
os.makedirs(runs, exist_ok=True)
open(os.path.join(runs, "t1b-rows.txt"), "a").write(row + "\n")
print(row)
PY
