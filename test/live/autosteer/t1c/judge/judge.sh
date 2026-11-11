#!/bin/sh
# judge.sh RUNDIR: every T1c judge in order (A1, A8 from the transcript and result; A2, A3 to A5, A6, A7 on the final
# tree through the slot; A3' to A5' from a3.sh's hook outputs), then one evidence row (section 7.5) written by row.py,
# appended to RK_RUNS/t1c-rows.txt and printed.
# T1c is slice T1 re-registered (pins.txt deltas 1 to 6). The judges are T1b's bytes except: a3.sh prepares the
# dispatch fixture with the cli overlay (delta 4); a3v2.py reads A3', A4', A5' (delta 3); row.py writes the row with the
# primary A1a all-four (delta 2) and exits 3 when the primary cannot be read, so the batch stops on it.
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
python3 "$HERE/a3v2.py" run "$D" > /dev/null || echo "a3v2 failed"
sh "$HERE/a6.sh" "$D" > /dev/null || echo "a6 failed"
sh "$HERE/a7.sh" "$D" > /dev/null || echo "a7 failed"
python3 "$HERE/row.py" "$D" "$RK_RUNS"
