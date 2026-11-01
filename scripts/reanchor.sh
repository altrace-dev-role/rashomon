#!/bin/sh
# usage: scripts/reanchor.sh RUN_ID [--no-hygiene] [reanchor args...]
#
# Downloads one CI run's bench artifacts and Perf gates job logs with gh
# (read-only) into a temp dir and runs go run ./scripts/reanchor over it. The
# Go tool reads a directory and starts nothing itself (H-17); every network
# and child-process step is here. Run from the module root; it edits
# test/perf/budgets.json (do not pass --budgets). REPO overrides the
# repository. Pass --platform, the row selection and any other flag after
# RUN_ID, e.g.
#
#   scripts/reanchor.sh 41166204201 --platform darwin/arm64 --rows RunPost/edit_314k
#
# After a write it runs go generate ./internal/budget, then (unless
# --no-hygiene) go run ./test/tools/reviewsync, which drops budgets.json's
# stale entries from test/tools/hygiene/reviewed.txt and adds the new
# findings inside the text the re-anchor composed. If reviewsync refuses (a
# finding elsewhere in budgets.json), budgets.json is put back as it was and
# the exit status is 2. --report writes nothing and runs neither.
set -eu

[ $# -ge 1 ] || { echo "usage: $0 RUN_ID [--no-hygiene] [reanchor args...]" >&2; exit 2; }
run=$1
shift
case $run in *[!0-9]*) echo "reanchor.sh: RUN_ID must be numeric, not $run" >&2; exit 2 ;; esac
repo=${REPO:-altrace-dev-role/rashomon}
budgets=test/perf/budgets.json

# Split the wrapper's own flags from the Go tool's, keeping the rest in order.
report=0
hygiene=1
n=$#
while [ "$n" -gt 0 ]; do
	a=$1
	shift
	n=$((n - 1))
	case $a in
	--report) report=1 ;;
	--no-hygiene) hygiene=0; continue ;;
	--budgets|--budgets=*) echo "reanchor.sh: edits $budgets only; run the Go tool directly for another file" >&2; exit 2 ;;
	esac
	set -- "$@" "$a"
done

dir=$(mktemp -d "${TMPDIR:-/tmp}/reanchor-XXXXXX")
trap 'rm -rf "$dir"' EXIT

gh run download "$run" --repo "$repo" -D "$dir"

# One line a Perf gates job: "Perf gates (<os>) <job id> <conclusion>".
jobs=$(gh run view "$run" --repo "$repo" --json jobs \
	--jq '.jobs[] | select(.name | startswith("Perf gates (")) | "\(.name) \(.databaseId) \(.conclusion)"')
[ -n "$jobs" ] || { echo "reanchor.sh: run $run has no Perf gates job" >&2; exit 2; }
echo "$jobs" | while read -r _ _ name id conc; do
	os=${name#\(}
	os=${os%\)}
	# The log carries terminal colour codes, which gh refuses to emit unless
	# told to; they are stripped so the Go tool reads plain text.
	gh api --allow-escape-sequences "repos/$repo/actions/jobs/$id/logs" |
		sed "s/$(printf '\033')\\[[0-9;]*[A-Za-z]//g" > "$dir/perf-$os.log"
	echo "$id $conc" > "$dir/perf-$os.job"
done

if [ "$report" = 1 ]; then
	exec go run ./scripts/reanchor --run "$run" --dir "$dir" "$@"
fi

cp "$budgets" "$dir/budgets.orig"
go run ./scripts/reanchor --run "$run" --dir "$dir" --neutral "$dir/neutral.json" "$@"
if cmp -s "$budgets" "$dir/budgets.orig"; then
	exit 0
fi
if [ "$hygiene" = 1 ]; then
	if ! go run ./test/tools/reviewsync -neutral "$dir/neutral.json" "$budgets"; then
		cp "$dir/budgets.orig" "$budgets"
		echo "reanchor.sh: budgets.json restored; nothing was kept" >&2
		exit 2
	fi
fi
go generate ./internal/budget
