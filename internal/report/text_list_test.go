package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestReasonTextCoversTheVocabulary: every reason in the vocabulary --
// store.Reasons() and report.Reasons() -- has a sentence, and every sentence
// belongs to one of them.
//
// It checks the vocabulary constants and nothing else. A coverage reason is
// read off disk as stored, so a record from another build or a hand-edited
// store can still carry a code outside the vocabulary; this test does not
// cover that, and writeReasons prints such a code bare.
//
// writeReasons falls back to the bare code when a reason has no sentence, so
// a vocabulary that grows without this map stays green everywhere and only a
// person reading a report notices the one line with no explanation beside it.
// That is how records_unreadable first arrived. The reverse direction catches
// a key that matches no code -- a typo that silently never renders.
func TestReasonTextCoversTheVocabulary(t *testing.T) {
	want := map[string]bool{}
	for _, r := range append(store.Reasons(), Reasons()...) {
		want[r] = true
	}
	for r := range want {
		if reasonText[r] == "" {
			t.Errorf("reason %q has no sentence in reasonText, so the report prints the bare code", r)
		}
	}
	for r := range reasonText {
		if !want[r] {
			t.Errorf("reasonText explains %q, which is in neither store.Reasons() nor report.Reasons()", r)
		}
	}
}

// ids returns n distinct tool-use ids of realistic width.
func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("toolu_01%022d", i)
	}
	return out
}

// TestListIsBounded: a list too long to read is summarised, and the summary
// keeps the count and says where the rest are.
//
// Reported from a real first run: 858 ids on one line of about twenty-five
// thousand characters, burying every other line of the report.
func TestListIsBounded(t *testing.T) {
	got := list(ids(858))
	if len(got) > listWidth+len(" and 858 more of 858 (--json lists them all)") {
		t.Errorf("858 ids rendered as %d characters, want about %d", len(got), listWidth)
	}
	if !strings.HasSuffix(got, "of 858 (--json lists them all)") {
		t.Errorf("a shortened list must keep the total and point at --json: %q", got)
	}

	// A handful of short words is not shortened at all.
	words := []string{"read", "write", "network"}
	if got := list(words); got != "read, write, network" {
		t.Errorf("list(%v) = %q, want every item", words, got)
	}

	// Short items are bounded by count as well as width.
	many := strings.Split("a b c d e f g h i j k l m", " ")
	if got := list(many); !strings.HasSuffix(got, fmt.Sprintf("and %d more of %d (--json lists them all)", len(many)-listMax, len(many))) {
		t.Errorf("list of %d short items = %q, want it cut at %d", len(many), got, listMax)
	}
}

// TestOverlappingNeverClaimsAShortenedListIsListed: when the list above was
// shortened, the line below must not say its ids are "listed" there.
//
// The two changes this file tests collided: the overlap line said "the same
// 858, listed under ... above" while the line above showed a dozen of them.
func TestOverlappingNeverClaimsAShortenedListIsListed(t *testing.T) {
	all := ids(858)
	above := list(all)
	if above == strings.Join(all, ", ") {
		t.Fatal("premise broken: the list above was not shortened")
	}
	for _, items := range [][]string{all, all[:400]} {
		got := overlapping(items, all, "missing from store")
		if strings.Contains(got, "listed") {
			t.Errorf("overlapping says %q, but the line above shows only %q", got, above)
		}
		if !strings.Contains(got, fmt.Sprint(len(items))) {
			t.Errorf("overlapping says %q, which does not carry the count %d", got, len(items))
		}
	}
}
