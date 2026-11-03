package disclose

import (
	"strings"
	"testing"
)

// TestOL1d_RecordsRowNamesTheThenCheck (card ORDER-LIVE1): the inventory's
// row for a run's records.ndjson says it holds a then_check for each Bash
// call read under an order rule, and what it keeps: the rule's id, its
// file's digest, the seq, the one word, and no command text. Break: drop the
// clause.
func TestOL1d_RecordsRowNamesTheThenCheck(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range inv.Files {
		if !strings.Contains(r.Holds, "a spec_event with the rule's id") {
			continue
		}
		found++
		i := strings.Index(r.Holds, "a then_check with the rule's id")
		if i < 0 {
			t.Fatalf("the records row's Holds names no then_check: %q", r.Holds)
		}
		for _, want := range []string{"the content digest of the spec file", "the seq the call's execution landed at",
			"whether the line runs the rule's then (yes, no or maybe)", "no command text"} {
			if !strings.Contains(r.Holds[i:], want) {
				t.Errorf("the records row's then_check clause does not say %q: %q", want, r.Holds[i:])
			}
		}
	}
	if found != 1 {
		t.Errorf("%d rows hold the spec_event sentence, want 1", found)
	}
}
