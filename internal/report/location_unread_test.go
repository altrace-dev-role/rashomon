package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lu1Line is the sentence the masked block says of the runs whose location the
// hook could not read (5.14b): a closed template with one number, the count.
const lu1Line = "    %s of these ran after a leading `cd` step the hook does not read (`cd DIR;`, a variable, quoting or `-`), so the directory they ran in is not known; a later run is matched only under the same unread step\n"

// TestLU1b_TheReportSaysNotCredited: an uncredited masked run that started
// after a cd the hook did not read is counted as such, from the declaration's
// location_unread (the cwd digest cannot say it: the report has no key). It is
// still a masked run, never a pass and never zero; a run after a read cd, a
// run credited by a later run from the same unread fold, and a record before
// schema 9 are not counted. Break: never count it, count every masked run,
// or count a run a later run credited.
func TestLU1b_TheReportSaysNotCredited(t *testing.T) {
	const ok = store.ExecOK
	const msg = "Done. All tests pass."
	for _, tc := range []struct {
		name         string
		calls        [][3]string
		runs, unread int
	}{
		{"cd DIR; then a piped test", [][3]string{{"cd sub; make test 2>&1 | tail -40", "/repo", ok}}, 1, 1},
		{"a variable cd", [][3]string{{"cd $REPO && make test | tail", "/repo", ok}}, 1, 1},
		{"a read cd is located", [][3]string{{"cd sub && make test | tail", "/repo", ok}}, 1, 0},
		{"no cd", [][3]string{{"make test | tail", "/repo", ok}}, 1, 0},
		{"one of two", [][3]string{{"cd sub; make test | tail", "/repo", ok}, {"make test | tail", "/repo", ok}}, 2, 1},
		{"a later run from the same unread fold credits it",
			[][3]string{{"cd sub; make test | tail", "/repo", ok}, {"cd sub; make test", "/repo", ok}}, 0, 0},
		{"a later run from another directory does not",
			[][3]string{{"cd sub; make test | tail", "/repo", ok}, {"cd sub; make test", "/other", ok}}, 1, 1},
	} {
		run := derivedRun(t, tc.calls...)
		got := BuildMaskedRuns(run, AccountFromMessage(msg))
		if got.Runs != tc.runs || got.UnreadLocation != tc.unread {
			t.Errorf("%s: runs %d, unread location %d; want %d, %d", tc.name, got.Runs, got.UnreadLocation, tc.runs, tc.unread)
		}
		var b bytes.Buffer
		writeMaskedRuns(&b, got)
		want := ""
		if tc.unread > 0 {
			want = strings.Replace(lu1Line, "%s", string(rune('0'+tc.unread)), 1)
		}
		if has := strings.Contains(b.String(), "of these ran after a leading `cd` step"); has != (tc.unread > 0) ||
			(want != "" && !strings.Contains(b.String(), want)) {
			t.Errorf("%s: text\n%s\nwant the line %q: %v", tc.name, b.String(), want, tc.unread > 0)
		}
		if tc.runs > 0 && !strings.Contains(b.String(), "masked exit status: ") {
			t.Errorf("%s: an uncredited run is not reported as masked:\n%s", tc.name, b.String())
		}
	}
	// A schema-8 record that carries the key anyway is not counted.
	s8 := derivedRun(t, [3]string{"cd sub; make test | tail", "/repo", ok})
	s8.Declarations[0].SchemaVersion = 8
	if !s8.Declarations[0].LocationUnread {
		t.Fatal("setup: the call should carry the bit")
	}
	if got := BuildMaskedRuns(s8, AccountFromMessage(msg)); got.Runs != 1 || got.UnreadLocation != 0 {
		t.Errorf("a schema-8 record carrying location_unread: runs %d, unread %d; want 1, 0", got.Runs, got.UnreadLocation)
	}
	// A record before schema 9 has no bit and reads false, whatever its digest.
	old := derivedRun(t, [3]string{"cd sub; make test | tail", "/repo", ok})
	old.Declarations[0].LocationUnread = false
	if got := BuildMaskedRuns(old, AccountFromMessage(msg)); got.Runs != 1 || got.UnreadLocation != 0 {
		t.Errorf("a record with no location_unread: runs %d, unread %d; want 1, 0", got.Runs, got.UnreadLocation)
	}
}
