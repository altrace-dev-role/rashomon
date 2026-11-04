package learn

import (
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestLW1_GivenGraphReadsTheDeliveredLinesAndJudgeLabelsThem (card
// LABEL-WRITER1): a delivered task-preflight graph line gives each knowledge
// record with the paths its lines named, its own line's and each code line
// it applies to, as the line holds them (the digest at v4, the path at v5);
// a withheld line gives nothing; a session that wrote one of those paths
// confirms the record, one that wrote others refutes it, one that wrote
// none leaves it unresolved, and the row's basis says which.
func TestLW1_GivenGraphReadsTheDeliveredLinesAndJudgeLabelsThem(t *testing.T) {
	line := func(state string) []byte {
		return []byte(`{"pattern":"task-preflight","state":"` + state + `","key":"graph","suppressed_by":"","slots":{"rev":"C1",` +
			`"line1":{"kind":"code","id":"","digest":"d-db","lessons":["les:INC-212"]},` +
			`"line2":{"kind":"lesson","id":"les:INC-212","digest":"d-purge","lessons":[]},` +
			`"line3":{"kind":"decision","id":"dec:ADR-0007","digest":"","lessons":[]},` +
			`"line4":{"kind":"table","id":"","digest":"","lessons":["les:INC-212"]}}}`)
	}
	v5 := []byte(`{"pattern":"task-preflight","state":"delivered","key":"graph","suppressed_by":"","slots":{"rev":"C1",` +
		`"line1":{"kind":"lesson","id":"les:INC-212","digest":"","path":"../acme-ops/jobs/purge_sessions.py","lessons":[]}}}`)
	given := GivenGraph([][]byte{line("delivered"), line("suppressed"), []byte(`{"pattern":"learned-fact","state":"delivered","key":"k"}`), v5})
	if len(given) != 2 || given[0].ID != "les:INC-212" || strings.Join(given[0].Paths, ",") != "d-db,d-purge,../acme-ops/jobs/purge_sessions.py" ||
		given[1].ID != "dec:ADR-0007" || len(given[1].Paths) != 0 {
		t.Fatalf("given = %+v", given)
	}
	for _, c := range []struct {
		w     Written
		g     Given
		want  steer.Label
		basis string
	}{
		{Written{map[string]bool{"d-purge": true, "d-x": true}, 2}, given[0], steer.LabelConfirmed, "wrote a path the record's line named"},
		{Written{map[string]bool{"d-x": true, "x.py": true, "d-y": true, "y.py": true}, 2}, given[0], steer.LabelRefuted, "wrote 2 path(s) and none"},
		{Written{map[string]bool{}, 0}, given[0], steer.LabelUnresolved, "wrote no file"},
		{Written{map[string]bool{"d-x": true}, 1}, given[1], steer.LabelUnresolved, "named no path"},
	} {
		if got, basis := Judge(c.g, c.w); got != c.want || !strings.Contains(basis, c.basis) {
			t.Errorf("Judge(%s, %v) = %s %q, want %s containing %q", c.g.ID, c.w, got, basis, c.want, c.basis)
		}
	}
	if FoldID("les:INC-212") != "les:inc-212" || !idForm(FoldID("les:INC-212")) || idForm("les:INC-212") {
		t.Error("a graph id is folded to the ledger's charset at the door")
	}
}

// TestLW1_TheLedgerKeepsWhichSessionLabeledAndTheLastRow: after two rows,
// Labeled answers by lesson and session, Last is the later row with its
// basis, and both survive a reopen from the files.
func TestLW1_TheLedgerKeepsWhichSessionLabeledAndTheLastRow(t *testing.T) {
	root, now := t.TempDir(), time.UnixMilli(1_700_000_000_000)
	l, err := Open(root, "p", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Import(strings.NewReader(`{"schema":"lessons/1","lessons":[{"id":"les:inc-212","text":"the purge predicate changes too"}]}`), now); err != nil {
		t.Fatal(err)
	}
	if err := l.Approve("les:inc-212", now); err != nil {
		t.Fatal(err)
	}
	for _, lb := range []Label{
		{LessonID: "les:inc-212", SessionID: "s1", AtUnixMS: 1, Label: steer.LabelRefuted, Basis: "the session wrote 2 path(s) and none the record's lines named"},
		{LessonID: "les:inc-212", SessionID: "s2", AtUnixMS: 2, Label: steer.LabelConfirmed, Basis: "the session wrote a path the record's line named"},
	} {
		if err := l.AppendLabel(lb); err != nil {
			t.Fatal(err)
		}
	}
	check := func(l *Ledger, when string) {
		t.Helper()
		last, ok := l.Last("les:inc-212")
		if !l.Labeled("les:inc-212", "s1") || !l.Labeled("les:inc-212", "s2") || l.Labeled("les:inc-212", "s3") || l.Labeled("other", "s1") ||
			!ok || last.SessionID != "s2" || last.Label != steer.LabelConfirmed || !strings.HasPrefix(last.Basis, "the session wrote a path") {
			t.Errorf("%s: labeled s1 %v s2 %v s3 %v; last %+v %v", when, l.Labeled("les:inc-212", "s1"), l.Labeled("les:inc-212", "s2"), l.Labeled("les:inc-212", "s3"), last, ok)
		}
		if _, ok := l.Last("other"); ok {
			t.Errorf("%s: an unlabeled lesson has a last row", when)
		}
	}
	check(l, "in memory")
	re, err := Open(root, "p", now)
	if err != nil || re.Quarantined {
		t.Fatalf("reopen: %v quarantined %v", err, re.Quarantined)
	}
	check(re, "reopened")
}
