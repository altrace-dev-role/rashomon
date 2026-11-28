#!/bin/sh
# memsample.sh ROOT OUT SECONDS: the run's memory series (T2; the director's 2026-11-11 14:2x ask: peak memory per run
# beside its disk). Every SECONDS while ROOT lives, appends one tab-separated row to OUT:
#   epoch  tree_rss_mb  tree_procs  swap_used_mb  compressor_mb  free_gib
# tree_* sum ROOT's descendants (the run's claude, its go builds and hooks) minus this sampler's own subtree; RSS
# undercounts pages already compressed or swapped, so the machine's swap and compressor columns carry the pressure the
# run adds, read as deltas from the first row. free_gib is the data volume's, because swap files live on it. run.sh
# starts it in the background with ROOT=$$ (the session stays in the foreground: B188, a backgrounded child ignores
# SIGINT) and stops it at the end; it also stops by itself when ROOT is gone.
set -u
ROOT=${1:?ROOT}; OUT=${2:?OUT}; S=${3:?SECONDS}
SELF=$$
PAGE=$(sysctl -n hw.pagesize)
printf 'epoch\ttree_rss_mb\ttree_procs\tswap_used_mb\tcompressor_mb\tfree_gib\n' > "$OUT"
while kill -0 "$ROOT" 2> /dev/null; do
  tree=$(ps -axo pid=,ppid=,rss= | awk -v root="$ROOT" -v self="$SELF" '
    { par[$1] = $2; rss[$1] = $3; pids[NR] = $1 }
    END {
      n = 0; kb = 0
      for (i = 1; i <= NR; i++) {
        p = pids[i]; q = p; inside = 0; mine = 0
        for (k = 0; k < 64 && q > 1; k++) { if (q == self) { mine = 1; break } if (q == root) { inside = 1; break } q = par[q] }
        if (inside && !mine && p != root) { n++; kb += rss[p] }
      }
      printf "%d\t%d", kb / 1024, n
    }')
  swap=$(sysctl -n vm.swapusage | awk '{ for (i = 1; i <= NF; i++) if ($i == "used") { v = $(i + 2); sub("M", "", v); print int(v) } }')
  comp=$(vm_stat | awk -v pg="$PAGE" '/occupied by compressor/ { gsub("\\.", "", $NF); print int($NF * pg / 1048576) }')
  free=$(df -k /System/Volumes/Data | awk 'NR == 2 { printf "%.2f", $4 / 1048576 }')
  printf '%s\t%s\t%s\t%s\t%s\n' "$(date +%s)" "$tree" "$swap" "$comp" "$free" >> "$OUT"
  sleep "$S"
done
