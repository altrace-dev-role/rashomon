package report

import "testing"

// TestRP3d_TheCountLineKeepsItsShape (card REPORT-REQS1): the count line
// prints every term with its seqs, "missed" even at 0, "found later" as
// unknown while nothing records a later find (a null list) and as a count
// once something does, and "not settled" and "fixed with no line" only when
// any. No term is ever dropped, so an absent one never reads as zero.
func TestRP3d_TheCountLineKeepsItsShape(t *testing.T) {
	none := []AutosteerMistake{}
	for _, c := range []struct {
		name string
		m    AutosteerMistakes
		want string
	}{
		{"no finding", AutosteerMistakes{Caught: none, Reached: none, Missed: none, Unsettled: none, FixedUnprompted: none},
			"0 caught at the act, 0 reached the person, 0 missed; found later unknown (no trace is recorded)"},
		{"one of each", AutosteerMistakes{Caught: []AutosteerMistake{{Seq: 1}}, Reached: []AutosteerMistake{{Seq: 1}, {Seq: 99}},
			Missed: []AutosteerMistake{{Seq: 99}}, Unsettled: []AutosteerMistake{{Seq: 2}}, FixedUnprompted: []AutosteerMistake{{Seq: 1}}},
			"1 caught at the act (at #1), 2 reached the person (at #1, #99), 1 missed (at #99); found later unknown (no trace is recorded); " +
				"1 not settled (at #2); 1 fixed with no line (at #1)"},
		{"a later list read", AutosteerMistakes{Caught: none, Reached: none, Missed: none, Unsettled: none, FixedUnprompted: none, Later: none},
			"0 caught at the act, 0 reached the person, 0 missed; 0 found later"},
	} {
		if got := mistakesText(&c.m); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
