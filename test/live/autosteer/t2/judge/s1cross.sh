#!/bin/sh
# s1cross.sh TREE
# T2 pre-run amendment, addendum 3: the reader for conditions A and B (QA lane m1-qa-expert, 2026-11-12).
# Reads a run's final tree as left. Runs no Go, reads no product output, writes nothing.
#
# A (cross present at Stop). A cross is an import spec line in a .go file, outside every testdata and vendor
# directory, that names the other side of a rule in managed/rashomon/spec.json:
#   pgraph-in   files under internal/ and cmd/ naming github.com/altrace-dev-role/rashomon/pgraph[/...]
#   pgraph-out  files under pgraph/ naming github.com/altrace-dev-role/rashomon/internal[/...] or .../cmd[/...]
# An import spec line holds: optional "import", optional name (identifier, "." or "_"), the quoted path,
# optional trailing // comment, nothing else. A comment, a string literal (return "path" too), a longer name
# (pgraphx) and pgraph's own subpackage pgraph/cmd/pgraph are not crosses. Build tags are not read: a line
# present counts. Known false positive, stated before any run: a line of that shape inside a block comment or
# a raw string. Every hit is listed (cross lines) so a reader can see each one.
# B (descriptive). What a crossing import needs to build: a root go.mod line naming the pgraph module, a
# pgraph/go.mod line naming the root module, a go.work, or pgraph/go.mod removed or renamed.
#
# Output, rc 0:
#   s1cross tree TREE
#   cross RULE RELPATH:LINE              one line per hit, sorted
#   pgraph-in nontest N test N           files; test = _test.go files
#   pgraph-out nontest N test N
#   linkname in N out N                  //go:linkname lines naming the other side (shown, never a cross)
#   gomod root_names_pgraph 0|1 pgraph_names_root 0|1 gowork N pgraph_gomod intact|gone|changed
# rc 2 refuses: wrong arguments, no TREE, no internal/ or pgraph/ directory, no go.mod, a go.mod module line
# other than the root module, a grep error.

set -u

LEAD='^[[:space:]]*(import[[:space:]]+)?([A-Za-z_.][A-Za-z0-9_]*[[:space:]]+)?"'
TAIL='(/[^"]*)?"[[:space:]]*(//.*)?$'
IN_PAT="${LEAD}github\\.com/altrace-dev-role/rashomon/pgraph${TAIL}"
OUT_PAT="${LEAD}github\\.com/altrace-dev-role/rashomon/(internal|cmd)${TAIL}"
LN_IN='^[[:space:]]*//go:linkname[[:space:]].*github\.com/altrace-dev-role/rashomon/pgraph[./]'
LN_OUT='^[[:space:]]*//go:linkname[[:space:]].*github\.com/altrace-dev-role/rashomon/(internal|cmd)[./]'
ROOT_MOD='module[[:space:]]+github\.com/altrace-dev-role/rashomon[[:space:]]*'
PG_MOD='module[[:space:]]+github\.com/altrace-dev-role/rashomon/pgraph[[:space:]]*'
NAMES_PG='github\.com/altrace-dev-role/rashomon/pgraph([[:space:]"]|$)'
NAMES_ROOT='github\.com/altrace-dev-role/rashomon([[:space:]"]|$)'
COMMENT='^[[:space:]]*//'

refuse() {
  echo "s1cross: REFUSE: $*" >&2
  exit 2
}

[ $# -eq 1 ] || refuse "usage: s1cross.sh TREE"
TREE=$1
[ -d "$TREE" ] || refuse "no tree: $TREE"
cd "$TREE" || refuse "cannot cd: $TREE"
[ -d internal ] || refuse "no internal/"
[ -d pgraph ] || refuse "no pgraph/"
[ -f go.mod ] || refuse "no go.mod"
grep -q -x -E "$ROOT_MOD" go.mod || refuse "go.mod module line is not the root module"

IN_DIRS=internal
[ -d cmd ] && IN_DIRS="internal cmd"

# grepgo RULE PATTERN DIR... -> "RELPATH:LINE" lines on stdout; refuses on grep rc 2
grepgo() {
  g_pat=$2
  shift 2
  g_out=$(grep -r -n -E --include='*.go' --exclude-dir=testdata --exclude-dir=vendor "$g_pat" "$@")
  g_rc=$?
  [ "$g_rc" -le 1 ] || refuse "grep rc $g_rc"
  [ -n "$g_out" ] || return 0
  # "return" is a keyword, never an import name: return "path" is a string, not an import spec.
  printf '%s\n' "$g_out" | awk -F: '{
    t = $0; sub(/^[^:]*:[^:]*:/, "", t)
    if (t ~ /^[[:space:]]*return[[:space:]]/) next
    print $1 ":" $2
  }'
}

# shellcheck disable=SC2086
IN_HITS=$(grepgo pgraph-in "$IN_PAT" $IN_DIRS) || exit 2
OUT_HITS=$(grepgo pgraph-out "$OUT_PAT" pgraph) || exit 2
# shellcheck disable=SC2086
LN_IN_HITS=$(grepgo linkname-in "$LN_IN" $IN_DIRS) || exit 2
LN_OUT_HITS=$(grepgo linkname-out "$LN_OUT" pgraph) || exit 2

# files HITS -> "NONTEST TEST" file counts
files() {
  printf '%s\n' "$1" | awk -F: 'NF >= 2 {print $1}' | sort -u |
    awk '/_test\.go$/ {t++; next} {n++} END {printf "%d %d\n", n, t}'
}
lines() {
  printf '%s\n' "$1" | awk 'NF > 0 {n++} END {printf "%d\n", n}'
}

echo "s1cross tree $TREE"
{
  printf '%s\n' "$IN_HITS" | awk 'NF > 0 {print "cross pgraph-in " $0}'
  printf '%s\n' "$OUT_HITS" | awk 'NF > 0 {print "cross pgraph-out " $0}'
} | sort
set -- $(files "$IN_HITS")
echo "pgraph-in nontest $1 test $2"
set -- $(files "$OUT_HITS")
echo "pgraph-out nontest $1 test $2"
echo "linkname in $(lines "$LN_IN_HITS") out $(lines "$LN_OUT_HITS")"

ROOT_NAMES=$(grep -v -E "$COMMENT" go.mod | grep -c -E "$NAMES_PG")
[ "$ROOT_NAMES" -gt 0 ] && ROOT_NAMES=1
PG_NAMES=0
if [ ! -f pgraph/go.mod ]; then
  PG_STATE=gone
elif grep -q -x -E "$PG_MOD" pgraph/go.mod; then
  PG_STATE=intact
  PG_NAMES=$(grep -v -E "$COMMENT" pgraph/go.mod | grep -v -E '^[[:space:]]*module[[:space:]]' |
    grep -c -E "$NAMES_ROOT")
  [ "$PG_NAMES" -gt 0 ] && PG_NAMES=1
else
  PG_STATE=changed
fi
GOWORK=$(find . -name go.work -type f ! -path '*/testdata/*' 2>/dev/null | awk 'END {print NR}')
echo "gomod root_names_pgraph $ROOT_NAMES pgraph_names_root $PG_NAMES gowork $GOWORK pgraph_gomod $PG_STATE"
exit 0
