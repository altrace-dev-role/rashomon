#!/bin/bash
# Paired read of the peak-RSS entry rows (test/perf, TestRES28b) of two trees
# on one runner. The base tree is checked out at ta and the head tree at tb,
# two paths of equal length under the workspace. The legs run in the order
# A B B A A B B A: the A legs and the B legs have the same mean position in
# time, so a drift of the runner that is linear over the job falls on both
# trees alike and the comparison is between the trees, not between runners.
#
#   abba.sh host        print and check the toolchain, the host and both trees
#   abba.sh leg X N     leg N on tree X (A or B): the RSS step's go test,
#                       filtered to TestRES28b_EveryEntryStaysWithinItsRow
#   abba.sh verdict     parse the eight legs and apply the rule below
#   abba.sh binsize X   bytes, sha256 and section table of tree X's binary,
#                       built with the flags of the harness (test/perf/rss_test.go)
#
# Rule, fixed before the run. The harness prints each reading in MiB at two
# decimals, and the readings move in steps of 64 KiB (0.0625 MiB). Per row,
# mA are the medians (5th of 9 sorted) of the four A legs, mB those of the
# four B legs.
#   band     [min(mA) - 0.094, max(mA) + 0.094]: one and a half steps, so one
#            step above the base reads inside, two steps read outside, and the
#            two-decimal print never decides a reading on the edge.
#   RUNNER   all four mB inside the band
#   COST     all four mB above it; its size is mean(mB) - mean(mA)
#   LOWER    all four mB below it
#   SPLIT    any other case
#   INVALID  a leg without the row's nine readings; a leg not run on go1.27.4
#            with GOTOOLCHAIN=local; or mA spanning more than three steps,
#            read at three and a half steps (0.219 MiB) for the same reason
#            as the band.
# The row the job is read by is probe.start; probe.end and hook.bash_short
# get a verdict by the same rule; the other five rows are printed beside them.
# A paired reading is compared with the other paired readings only, never
# with a CI row (another checkout path), and a difference of one step or less
# is not resolved by this design.
set -u
export GOTOOLCHAIN=local
W=${GITHUB_WORKSPACE:-$PWD}
O=${PAIRED_OUT:-${RUNNER_TEMP:-/tmp}/paired}
mkdir -p "$O"
GOWANT=go1.27.4
LEGS="1-A 2-B 3-B 4-A 5-A 6-B 7-B 8-A"
RULE_ROWS="probe.start probe.end hook.bash_short"

tree() {
	case "$1" in
	A) echo "$W/ta" ;;
	B) echo "$W/tb" ;;
	*) echo "abba.sh: tree '$1' is not A or B" >&2; exit 2 ;;
	esac
}

case "${1:-}" in
host)
	ok=1
	gv=$(go env GOVERSION)
	tc=$(go env GOTOOLCHAIN)
	echo "go version: $(go version)"
	echo "go env: GOVERSION=$gv GOTOOLCHAIN=$tc GOOS=$(go env GOOS) GOARCH=$(go env GOARCH)"
	echo "host: $(uname -srm); page size $(getconf PAGESIZE); $(nproc) CPUs; $(grep -m1 'model name' /proc/cpuinfo | sed 's/.*: //')"
	echo "runner image: ${ImageOS:-unknown} ${ImageVersion:-unknown}"
	echo "THP: $(cat /sys/kernel/mm/transparent_hugepage/enabled 2>/dev/null || echo absent)"
	[ "$gv" = "$GOWANT" ] || { echo "INVALID: GOVERSION $gv, want $GOWANT"; ok=0; }
	[ "$tc" = local ] || { echo "INVALID: GOTOOLCHAIN $tc, want local"; ok=0; }
	dA=$(cd "$(tree A)" && pwd -P) || { echo "INVALID: tree A missing"; ok=0; dA=""; }
	dB=$(cd "$(tree B)" && pwd -P) || { echo "INVALID: tree B missing"; ok=0; dB=""; }
	gA=$(git -C "$dA" rev-parse HEAD 2>/dev/null)
	gB=$(git -C "$dB" rev-parse HEAD 2>/dev/null)
	echo "tree A: $dA (${#dA} bytes) at $gA"
	echo "tree B: $dB (${#dB} bytes) at $gB"
	[ "$gA" = "${BASE_SHA:-}" ] || { echo "INVALID: tree A at '$gA', want '${BASE_SHA:-}'"; ok=0; }
	[ "$gB" = "${HEAD_SHA:-}" ] || { echo "INVALID: tree B at '$gB', want '${HEAD_SHA:-}'"; ok=0; }
	[ "${#dA}" = "${#dB}" ] || { echo "INVALID: the two tree paths differ in length"; ok=0; }
	if [ $ok = 1 ]; then touch "$O/host.ok"; echo "host check: pass"; else echo "host check: FAIL"; exit 1; fi
	;;
leg)
	X=$2
	N=$3
	d=$(tree "$X")
	f=$O/leg-$N-$X.log
	[ -f "$O/host.ok" ] || { echo "abba.sh: the host check did not pass" >&2; exit 2; }
	echo "LEG $N $X start $(date -u +%H:%M:%S) goversion $(go env GOVERSION) gotoolchain $(go env GOTOOLCHAIN)" > "$f"
	# The RSS step's command with one test selected. RASHOMON_PERF_REQUIRE_ALL
	# is left unset: it acts only after the tests ran (Settle, rss_test.go)
	# and, with one test selected, would fail every leg for the gates it did
	# not select; a reading is the same either way. A row over its budget
	# fails the test (rc 1); the readings decide, so the leg goes on.
	(cd "$d" && go test -tags perf ./test/perf -run '^TestRES28b_EveryEntryStaysWithinItsRow$' -count=1 -v) >> "$f" 2>&1
	rc=$?
	echo "LEG $N $X end $(date -u +%H:%M:%S) rc=$rc" >> "$f"
	cat "$f"
	;;
verdict)
	bad=0
	[ -f "$O/host.ok" ] || { echo "INVALID: the host check did not pass"; bad=1; }
	: > "$O/all.rows"
	for l in $LEGS; do
		f=$O/leg-$l.log
		if [ ! -f "$f" ]; then echo "INVALID: leg $l has no log"; bad=1; continue; fi
		if ! awk -v want="$GOWANT" 'NR == 1 { ok = ($1 == "LEG" && $7 == want && $9 == "local") } END { exit !ok }' "$f"; then
			echo "INVALID: leg $l header: $(head -1 "$f")"
			bad=1
		fi
		# One line per row: leg, row id, the count of readings, their median.
		awk -v leg="$l" '
			/=== RUN/ {
				id = ""
				for (i = 1; i <= NF; i++) if ($i ~ /^TestRES28b_EveryEntryStaysWithinItsRow\//) { id = $i; sub(/.*\//, "", id) }
				next
			}
			/nine peak RSS readings in MiB: \[/ && id != "" {
				s = $0; sub(/.*\[/, "", s); sub(/\].*/, "", s); n = split(s, v, " ")
				for (i = 2; i <= n; i++) { x = v[i]; j = i - 1; while (j >= 1 && v[j] + 0 > x + 0) { v[j + 1] = v[j]; j-- } v[j + 1] = x }
				printf "%s %s %d %s\n", leg, id, n, (n == 9 ? v[5] : "NA")
				id = ""
			}
		' "$f" >> "$O/all.rows"
	done
	awk -v rule="$RULE_ROWS" -v bad=$bad '
		BEGIN { step = 0.0625; wide = 0.094; spanmax = 3.5 * step; split("1-A 2-B 3-B 4-A 5-A 6-B 7-B 8-A", L, " ") }
		{ if (!($2 in seen)) { seen[$2] = 1; ids[++nid] = $2 } cnt[$2, $1] = $3; med[$2, $1] = $4 }
		END {
			printf "%-16s", "medians (MiB)"; for (k = 1; k <= 8; k++) printf " %6s", L[k]; printf "\n"
			for (r = 1; r <= nid; r++) {
				id = ids[r]; printf "%-16s", id
				for (k = 1; k <= 8; k++) printf " %6s", ((id, L[k]) in med ? med[id, L[k]] : "-")
				printf "\n"
			}
			nr = split(rule, R, " "); code = 0
			for (r = 1; r <= nr; r++) {
				id = R[r]; why = (bad ? "a leg or the host is invalid" : ""); amin = 1e9; amax = -1e9; sa = 0; sb = 0; as = ""; bs = ""
				for (k = 1; k <= 8; k++) {
					key = id SUBSEP L[k]
					if (!(key in cnt) || cnt[key] != 9) { if (why == "") why = "leg " L[k] " lacks nine readings"; continue }
					x = med[key] + 0
					if (L[k] ~ /A$/) { sa += x; as = as " " med[key]; if (x < amin) amin = x; if (x > amax) amax = x }
					else { sb += x; bs = bs " " med[key] }
				}
				if (why == "" && amax - amin > spanmax) why = sprintf("the A medians span %.3f MiB, more than three steps", amax - amin)
				if (why != "") { v = "INVALID (" why ")"; if (r == 1) code = 4 }
				else {
					lo = amin - wide; hi = amax + wide; inside = 0; above = 0; below = 0
					for (k = 1; k <= 8; k++) if (L[k] ~ /B$/) { x = med[id, L[k]] + 0; if (x > hi) above++; else if (x < lo) below++; else inside++ }
					d = sb / 4 - sa / 4
					if (inside == 4) v = "RUNNER"
					else if (above == 4) v = sprintf("COST %+.3f MiB", d)
					else if (below == 4) v = "LOWER"
					else v = "SPLIT"
					v = v sprintf(" | A medians%s, band [%.3f, %.3f] | B medians%s | mean B - mean A %+.3f MiB", as, lo, hi, bs, d)
				}
				printf "VERDICT %s%s: %s\n", id, (r == 1 ? " (the row the job is read by)" : ""), v
			}
			exit code
		}
	' "$O/all.rows"
	;;
binsize)
	X=$2
	d=$(tree "$X")
	b=$O/rashomon-$X
	# The harness's build (rss_test.go buildAndRun): -trimpath, -ldflags "-s -w",
	# CGO_ENABLED=0, in the tree's root, so the bytes match what the legs ran.
	if ! (cd "$d" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$b" ./cmd/rashomon); then
		echo "BIN $X build failed"
		exit 0
	fi
	echo "BIN $X bytes $(stat -c %s "$b") sha256 $(sha256sum "$b" | cut -d' ' -f1)"
	readelf -S -W "$b" | sed "s/^/SECT $X /"
	;;
*)
	echo "usage: abba.sh host | leg A|B N | verdict | binsize A|B" >&2
	exit 2
	;;
esac
