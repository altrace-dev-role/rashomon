#!/bin/sh
# judge.sh RUNDIR: T2's judges in order (r3 section 8): J3 (j3.sh run), J4 (j4.sh), J5 (j5.sh, with A8), J2 (j2.py),
# section 7's flags (../flags.py), then row.py's row in RUNDIR/judge/row.txt, appended to RK_RUNS/RK_T2_ROWS (default
# t2-rows.txt). Exit 3 with no row when a tool or a judge file is missing, keys.txt holds a TBD, or flags.py refuses;
# exit 3 after the row when the primary cannot be read (J4, or a scored truth: dectruth.out, and s1cross.out when
# pins.txt's T2_S1_LIVE is yes; addendum 3 3g, addendum 4 D10; a J3 failure leaves the primary judged, addendum 5
# 4a). The arm binary never judges.
set -u
D=${1:?RUNDIR}; D=${D%/}
HERE=$(cd "$(dirname "$0")" && pwd)
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
for tool in go git python3; do
  command -v "$tool" > /dev/null 2>&1 || { echo "judge.sh: $tool is not on PATH; no judge runs and no row is written" >&2; exit 3; }
done
for f in j3.sh j3_test.go j3_conflicts_test.go j4.sh j5.sh j2.py s1cross.sh dectruth.py row.py ../flags.py ../keys.txt ../pins.txt ../../t1c/judge/a8.py; do
  [ -f "$HERE/$f" ] || { echo "judge.sh: $HERE/$f is missing; no row" >&2; exit 3; }
done
if grep -n '=TBD' "$HERE/../keys.txt" >&2; then echo "judge.sh: keys.txt holds the TBD values above; no row" >&2; exit 3; fi
arm=$(cat "$D/arm" 2> /dev/null) || { echo "judge.sh: $D/arm unreadable; no row" >&2; exit 3; }
sh "$HERE/j3.sh" run "$D" > /dev/null || echo "j3 failed"
sh "$HERE/s1cross.sh" "$D/tree" > "$D/judge/s1cross.out" 2> "$D/judge/s1cross.err" || echo "s1cross failed"
( . "$HERE/../pins.txt"; python3 -I -B "$HERE/dectruth.py" "$D" "$RK_SEED" "$T1_TREE_SHA" ) \
  > "$D/judge/dectruth.out" 2> "$D/judge/dectruth.err" || echo "dectruth failed"
sh "$HERE/j4.sh" "$D" > /dev/null || echo "j4 failed"
sh "$HERE/j5.sh" "$D" > /dev/null || echo "j5 failed"
python3 -B "$HERE/j2.py" "$D" > /dev/null || echo "j2 failed"
python3 -B "$HERE/../flags.py" "$D" "$arm" || exit 3
python3 -B "$HERE/row.py" "$D" "$RK_RUNS"
