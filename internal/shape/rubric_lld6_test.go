package shape

import (
	"strings"
	"testing"
	"time"
)

// TestRubricLLD6_JoinsAreSearchedNotScanned: the masked-status walk reads a
// here-document's body line by line, and asked of each line whether a
// backslash-newline was removed inside it by scanning every offset the
// joining kept, so a line of continuations followed by a body of short lines
// took time in the product of the two: 160,000 of each, 800 KB, took 13.8 s
// against the hook's 5 s timeout. The offsets are kept in order and are
// searched. The answers are unchanged: the line's runner is still found after
// the body, and a continuation inside the body still gives up the reading.
func TestRubricLLD6_JoinsAreSearchedNotScanned(t *testing.T) {
	n := 160_000
	line := "echo " + strings.Repeat("a\\\n", n) + "; cat <<E\n" + strings.Repeat("x\n", n) + "E\nmake test 2>&1 | tail"
	in := bashInput(t, line)
	var s Shape
	start := time.Now()
	s = Derive("Bash", in, benchKey)
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("an 800 KB line of continuations and a here-document took %v, want at most 2s", took)
	}
	if s.StatusMasked == nil || *s.StatusMasked != MaskedTest {
		t.Errorf("status_masked %v, want %q: the runner after the body is read", show(s.StatusMasked), MaskedTest)
	}
	joinedBody := "echo a\\\nb; cat <<E\nx\\\ny\nE\nmake test 2>&1 | tail"
	if s := Derive("Bash", bashInput(t, joinedBody), benchKey); s.StatusMasked != nil {
		t.Errorf("a continuation inside the body: status_masked %v, want null", show(s.StatusMasked))
	}
}

// TestRubricLLD6_JoinedInKeepsItsBounds: the search answers what the scan
// did -- a join just before a byte after from and no later than to -- at each
// edge, and the delimiter line is still read across the joins inside it.
func TestRubricLLD6_JoinedInKeepsItsBounds(t *testing.T) {
	r := joined{s: "0123456789", joins: []int{3, 5, 5}}
	for _, tc := range []struct {
		from, to int
		want     bool
	}{
		{2, 3, true}, {3, 4, false}, {2, 2, false}, {3, 5, true}, {4, 5, true}, {5, 9, false}, {0, 9, true},
	} {
		if got := r.joinedIn(tc.from, tc.to); got != tc.want {
			t.Errorf("joinedIn(%d, %d) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
	// The line s[2:5] is EOF. A join before the line is not the line's: read
	// as one, its text would run backwards. One at the line's end ends a
	// line that began EOF and ended in a backslash as written -- not the
	// delimiter, and not certain -- and one inside it leaves a line, F, that
	// is certainly not the delimiter.
	d := heredoc{delim: "EOF"}
	for _, tc := range []struct {
		joins     []int
		found, ok bool
	}{
		{[]int{1, 6}, true, true},
		{[]int{1, 5}, false, false},
		{[]int{1, 4}, false, true},
	} {
		r = joined{s: "x\nEOF\ny", joins: tc.joins}
		if found, ok := r.delimLine(2, 5, d); found != tc.found || ok != tc.ok {
			t.Errorf("joins %v: found %v, ok %v; want %v, %v", tc.joins, found, ok, tc.found, tc.ok)
		}
	}
}
