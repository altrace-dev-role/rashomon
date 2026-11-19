package report

import (
	"regexp"
	"testing"
)

// TestRH7_LessonsLineNamesNoMiner: the lessons line never prints "0 proposed".
// No code proposes a lesson from a session: learn.Candidate, the gate a miner
// would feed, has no caller, and the one writer that adds a lesson is
// Ledger.Import behind `rashomon learn import`. So with none proposed the line
// says that no lesson miner exists, and with some proposed it names where
// they came from. The approved and demoted counts, the fired clause and the
// two lines with no counts to read (no ledger opened, a ledger that did not
// check) are as before.
func TestRH7_LessonsLineNamesNoMiner(t *testing.T) {
	zeroProposed := regexp.MustCompile(`(^|[^0-9])0 proposed`)
	learned := func(seq int64) AutosteerNote { return AutosteerNote{Pattern: "learned-fact", Seq: seq} }
	for _, c := range []struct {
		name string
		a    Autosteer
		want string
	}{
		// Controls: the same line before and after item 7.
		{"not_read", Autosteer{}, "not read here"},
		{"unchecked", Autosteer{Lessons: &AutosteerLessons{Proposed: 1, Unchecked: true}},
			"the ledger did not check, so all 1 read proposed (this project)"},
		// None proposed: the line names the missing miner in place of a zero.
		{"empty_ledger", Autosteer{Lessons: &AutosteerLessons{}},
			"0 approved, 0 demoted (this project); no lesson miner exists; none fired this session"},
		{"none_proposed_one_fired", Autosteer{Lessons: &AutosteerLessons{Approved: 2, Demoted: 1},
			Delivered: []AutosteerNote{learned(4), {Pattern: "regression-location", Seq: 5}}},
			"2 approved, 1 demoted (this project); no lesson miner exists; 1 fired this session"},
		// Some proposed: every one came in by learn import.
		{"imported", Autosteer{Lessons: &AutosteerLessons{Approved: 1, Proposed: 3}},
			"1 approved, 3 proposed by learn import, 0 demoted (this project); none fired this session"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := lessonsText(&c.a)
			if got != c.want {
				t.Errorf("lessons line = %q, want %q", got, c.want)
			}
			if zeroProposed.MatchString(got) {
				t.Errorf("lessons line %q prints 0 proposed, a count no code produces", got)
			}
		})
	}
}
