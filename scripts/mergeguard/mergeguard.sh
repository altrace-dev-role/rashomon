#!/bin/sh
# mergeguard.sh [COMMIT [REMOTE]]: the report-only merge guard (6.4, v0).
# It reads COMMIT's Rashomon-Receipt trailer (HEAD by default), fetches
# refs/notes/rashomon from REMOTE when one is named, reads COMMIT's note, and
# has the planner (the command in MERGEGUARD, else mergeguard on the PATH)
# turn the two into a rerun script, which it runs: one line a gate, then the
# counts. In CI, COMMIT is the pull request's head, where the receipt hooks
# wrote the trailer and the note; the gates rerun on the checked-out tree.
# It never fails the build in v0: every path exits 0.
commit=${1:-HEAD}
remote=${2:-}
none() {
	printf '%s\n' "mergeguard: $1" 'mergeguard: 0 agree, 0 differ, 0 not rerun'
	exit 0
}
root=$(git rev-parse --show-toplevel) || none "not in a git work tree"
value=$(git log -1 --format='%(trailers:key=Rashomon-Receipt,valueonly)' "$commit") || none "no commit $commit"
[ -n "$value" ] || none "$commit carries no Rashomon-Receipt trailer"
if [ -n "$remote" ]; then
	git fetch -q "$remote" '+refs/notes/rashomon:refs/notes/rashomon' ||
		printf '%s\n' "mergeguard: fetching refs/notes/rashomon from $remote failed"
fi
tmp=$(mktemp -d) || none "no temporary directory"
trap 'rm -rf "$tmp"' EXIT
git notes --ref rashomon show "$commit" >"$tmp/note" 2>/dev/null || none "$commit has no rashomon note"
# MERGEGUARD may be a command with arguments (go run ./scripts/mergeguard),
# so it is split into words on purpose.
# shellcheck disable=SC2086
${MERGEGUARD:-mergeguard} -trailer "$value" -root "$root" <"$tmp/note" >"$tmp/rerun.sh" ||
	none "the planner did not run"
sh "$tmp/rerun.sh"
exit 0
