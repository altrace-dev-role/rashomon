package session_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/session"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// The second line in its three forms, written out here rather than read
// from the renderer, so a change of a word turns a test red.
const (
	oneUnit   = "a unit rashomon named this session; no write under it seen since."
	twoUnits  = "units rashomon named this session; no write under either seen since."
	manyUnits = "units rashomon named this session; no write under any seen since."
)

// indent is the Stop's continuation indent the lines print under: recap's
// reference mark, one column, then " rashomon: ".
const indent = 12

// lineColumns is recap's line budget.
const lineColumns = 80

// row is a requirement of pred on arg in status, last stated after seq last.
func row(pred, arg string, status session.ReqStatus, last int64) session.ReqState {
	return session.ReqState{Pred: pred, Arg: arg, Status: status, StatedAt: 1, LastStatedAt: last}
}

// unit is an opened_unit requirement.
func unit(arg string, status session.ReqStatus, last int64) session.ReqState {
	return row(steer.PredOpenedUnit, arg, status, last)
}

func wantLines(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("lines:\n got %q\nwant %q", got, want)
	}
}

// soleRow is the two lines of one open row on arg.
func soleRow(arg string) []string {
	return []string{"open: '" + arg + "'", oneUnit}
}

// TestOpenReqs_NoRowNoLine: with nothing listed the renderer returns nil,
// not an empty list, so a reader prints no line.
func TestOpenReqs_NoRowNoLine(t *testing.T) {
	for _, reqs := range [][]session.ReqState{nil, {}} {
		if got := session.OpenReqsLines(reqs); got != nil {
			t.Fatalf("OpenReqsLines(%v) = %q, want nil", reqs, got)
		}
	}
	t.Logf("0 rows: %q", session.OpenReqsLines(nil))
}

// TestOpenReqs_RowCountForms: the two lines for 1, 2, 3 and 5 open rows,
// newest first. The log holds the lines each count prints.
func TestOpenReqs_RowCountForms(t *testing.T) {
	all := []session.ReqState{
		unit("internal/recap", session.ReqOpen, 9),
		unit("internal/verify", session.ReqOpen, 8),
		unit("cmd/rashomon", session.ReqOpen, 7),
		unit("internal/steer", session.ReqOpen, 6),
		unit("internal/intent", session.ReqOpen, 5),
	}
	for _, tc := range []struct {
		rows int
		want []string
	}{
		{1, []string{"open: 'internal/recap'", oneUnit}},
		{2, []string{"open: 'internal/recap' and 'internal/verify'", twoUnits}},
		{3, []string{"open: 'internal/recap', 'internal/verify' and 1 more", manyUnits}},
		{5, []string{"open: 'internal/recap', 'internal/verify' and 3 more", manyUnits}},
	} {
		t.Run(fmt.Sprintf("%d_rows", tc.rows), func(t *testing.T) {
			got := session.OpenReqsLines(all[:tc.rows])
			t.Logf("%d rows: %q", tc.rows, got)
			wantLines(t, got, tc.want)
		})
	}
}

// TestOpenReqs_NewestFirst: within a group the newest LastStatedAt comes
// first, whatever the input order, and the input is left as it was.
func TestOpenReqs_NewestFirst(t *testing.T) {
	reqs := []session.ReqState{
		unit("internal/a", session.ReqOpen, 5),
		unit("internal/b", session.ReqOpen, 9),
		unit("internal/c", session.ReqOpen, 7),
	}
	before := slices.Clone(reqs)
	wantLines(t, session.OpenReqsLines(reqs), []string{"open: 'internal/b', 'internal/c' and 1 more", manyUnits})
	if !slices.Equal(reqs, before) {
		t.Fatalf("the input changed:\n got %+v\nwant %+v", reqs, before)
	}
}

// TestOpenReqs_TiesByPath: rows stated after the same seq come in path
// order.
func TestOpenReqs_TiesByPath(t *testing.T) {
	reqs := []session.ReqState{
		unit("internal/c", session.ReqOpen, 4),
		unit("internal/a", session.ReqOpen, 4),
		unit("internal/b", session.ReqOpen, 4),
	}
	wantLines(t, session.OpenReqsLines(reqs), []string{"open: 'internal/a', 'internal/b' and 1 more", manyUnits})
}

// TestOpenReqs_SupersededAfterOpen: a superseded row is listed, after every
// open row even when it was stated later, with the mark after its name.
func TestOpenReqs_SupersededAfterOpen(t *testing.T) {
	reqs := []session.ReqState{
		unit("internal/z", session.ReqSuperseded, 9),
		unit("internal/a", session.ReqOpen, 1),
	}
	wantLines(t, session.OpenReqsLines(reqs),
		[]string{"open: 'internal/a' and 'internal/z' (at an earlier prompt)", twoUnits})
}

// TestOpenReqs_MoreCountsBothGroups: N counts the rows of both groups that
// the first line does not name.
func TestOpenReqs_MoreCountsBothGroups(t *testing.T) {
	reqs := []session.ReqState{
		unit("internal/o9", session.ReqOpen, 9),
		unit("internal/o8", session.ReqOpen, 8),
		unit("internal/o7", session.ReqOpen, 7),
		unit("internal/s6", session.ReqSuperseded, 6),
		unit("internal/s5", session.ReqSuperseded, 5),
	}
	wantLines(t, session.OpenReqsLines(reqs), []string{"open: 'internal/o9', 'internal/o8' and 3 more", manyUnits})
}

// TestOpenReqs_KeyedDigestIsNotListed: the tie test. A real keyed digest,
// as record scope v4 stores a path, is the Arg of an open opened_unit row:
// no line names it, and beside a listed row it is not counted. Red when the
// digest's form becomes one the path slot's rule accepts.
func TestOpenReqs_KeyedDigestIsNotListed(t *testing.T) {
	d := intent.PathDigest([]byte("openreqs tie test key"), "internal/graph/session")
	if got := session.OpenReqsLines([]session.ReqState{unit(d, session.ReqOpen, 3)}); got != nil {
		t.Fatalf("a keyed digest is listed: %q", got)
	}
	reqs := []session.ReqState{unit(d, session.ReqOpen, 3), unit("internal/recap", session.ReqOpen, 2)}
	wantLines(t, session.OpenReqsLines(reqs), soleRow("internal/recap"))
}

// TestOpenReqs_RefusedPathIsNotListed: a path the path slot's rule refuses
// is neither named nor counted, whatever the fold let through. A newline
// inside a printed name would forge a second line in the brief.
func TestOpenReqs_RefusedPathIsNotListed(t *testing.T) {
	for _, tc := range []struct{ name, arg string }{
		{"empty", ""},
		{"single_dot", "."},
		{"two_dots", ".."},
		{"climbs", "../internal/recap"},
		{"newline_inside", "internal/recap\nopen: 'cmd/rashomon'"},
		{"leading_slash", "/internal/recap"},
		{"leading_hyphen", "internal/-recap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, st := range []session.ReqStatus{session.ReqOpen, session.ReqSuperseded} {
				if got := session.OpenReqsLines([]session.ReqState{unit(tc.arg, st, 3)}); got != nil {
					t.Fatalf("%s %q is listed: %q", st, tc.arg, got)
				}
				reqs := []session.ReqState{unit(tc.arg, st, 3), unit("internal/verify", session.ReqOpen, 2)}
				wantLines(t, session.OpenReqsLines(reqs), soleRow("internal/verify"))
			}
		})
	}
}

// TestOpenReqs_MetOnceIsNeverRendered: met_once and dropped rows of
// opened_unit are present and never named or counted.
func TestOpenReqs_MetOnceIsNeverRendered(t *testing.T) {
	met := unit("internal/a", session.ReqMetOnce, 4)
	met.MetAt = 6
	reqs := []session.ReqState{met, unit("internal/b", session.ReqDropped, 5)}
	if got := session.OpenReqsLines(reqs); got != nil {
		t.Fatalf("met_once or dropped is listed: %q", got)
	}
	wantLines(t, session.OpenReqsLines(append(reqs, unit("internal/c", session.ReqOpen, 1))), soleRow("internal/c"))
}

// TestOpenReqs_OtherPredicatesAreNeverRendered: the predicates with no
// evidence rule in the fold are present, open or superseded, on a path the
// rule accepts, and never named or counted.
func TestOpenReqs_OtherPredicatesAreNeverRendered(t *testing.T) {
	var reqs []session.ReqState
	for _, pred := range []string{steer.PredCalledSymbol, steer.PredEditedOrVerified, steer.PredPathsUntouched} {
		reqs = append(reqs, row(pred, "internal/a", session.ReqOpen, 9), row(pred, "internal/b", session.ReqSuperseded, 8))
	}
	if got := session.OpenReqsLines(reqs); got != nil {
		t.Fatalf("another predicate is listed: %q", got)
	}
	wantLines(t, session.OpenReqsLines(append(reqs, unit("internal/c", session.ReqOpen, 1))), soleRow("internal/c"))
}

// TestOpenReqs_EightyColumns measures the lines under the indent. The
// second line of each form is at most 80 columns (77, 80, 77). The first
// line's fixed words, its names taken out, are at most 80 in every form
// but one: two marks with a count, which needs no open row and three or
// more superseded ones, is 81 with a one-digit count and 82 with 99, pinned
// here. Two paths of 60 characters print whole on the first line, both of
// them, and push it past 80: a path is never cut.
func TestOpenReqs_EightyColumns(t *testing.T) {
	rows := func(open, superseded int) []session.ReqState {
		var out []session.ReqState
		for i := range open + superseded {
			st := session.ReqOpen
			if i >= open {
				st = session.ReqSuperseded
			}
			out = append(out, unit(fmt.Sprintf("u%03d", i), st, 1))
		}
		return out
	}
	for _, tc := range []struct {
		name             string
		open, superseded int
		first, second    int
	}{
		{"1 row", 1, 0, 20, 77},
		{"2 rows", 2, 0, 27, 80},
		{"101 rows", 101, 0, 36, 77},
		{"1 row, marked", 0, 1, 43, 77},
		{"2 rows, one marked", 1, 1, 50, 80},
		{"101 rows, one marked", 1, 100, 59, 77},
		{"2 rows, both marked", 0, 2, 73, 80},
		{"3 rows, both marked", 0, 3, 81, 77},
		{"101 rows, both marked", 0, 101, 82, 77},
	} {
		lines := session.OpenReqsLines(rows(tc.open, tc.superseded))
		if len(lines) != 2 {
			t.Fatalf("%s: %d lines, want 2: %q", tc.name, len(lines), lines)
		}
		named := min(tc.open+tc.superseded, 2)
		first := indent + len(lines[0]) - named*len("u000")
		second := indent + len(lines[1])
		t.Logf("%s: line 1 fixed words %d columns, line 2 %d columns, under the indent", tc.name, first, second)
		if first != tc.first || second != tc.second {
			t.Fatalf("%s: columns %d and %d, want %d and %d: %q", tc.name, first, second, tc.first, tc.second, lines)
		}
		bothMarkedWithCount := tc.open == 0 && tc.superseded > 2
		if second > lineColumns || (first > lineColumns && !bothMarkedWithCount) {
			t.Fatalf("%s: past %d columns: %q", tc.name, lineColumns, lines)
		}
	}

	a := "internal/graph/session/testdata/fold-v2/recorded-sess-a.json"
	b := "internal/graph/session/testdata/fold-v2/recorded-sess-b.json"
	if len(a) != 60 || len(b) != 60 {
		t.Fatalf("the long paths are %d and %d characters, want 60", len(a), len(b))
	}
	lines := session.OpenReqsLines([]session.ReqState{unit(a, session.ReqOpen, 2), unit(b, session.ReqOpen, 1)})
	wantLines(t, lines, []string{"open: '" + a + "' and '" + b + "'", twoUnits})
	if cols := indent + len(lines[0]); cols <= lineColumns || !strings.Contains(lines[0], "'"+a+"'") || !strings.Contains(lines[0], "'"+b+"'") {
		t.Fatalf("two 60-character paths: %d columns, want both whole and past %d: %q", cols, lineColumns, lines[0])
	}
	t.Logf("two 60-character paths: line 1 %d columns under the indent, both whole", indent+len(lines[0]))
}
