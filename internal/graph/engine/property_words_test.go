package engine

import "testing"

// propertyWords is a literal of stems, not a loop over Tokens: a call in a
// package-level var's initializer, a func literal's included, is what
// RES-2 (test/perf RegexpCompilesAtInit) forbids, and the walk from Tokens
// reaches a regexp through stem's get. This test does the stemming the
// literal cannot, so the set holds exactly the stems of the disclosed
// words and drifts from Tokens in neither direction (W-M1.34).
func TestWords_PropertyWordsAreTheStemsOfTheDisclosedWords(t *testing.T) {
	want := map[string]bool{}
	for _, w := range []string{"format", "schema", "output", "layout", "contract", "interface", "api", "behaviour", "behavior", "signature", "encoding"} {
		for _, tok := range Tokens(w) {
			want[tok] = true
		}
	}
	got := propertyWords.get()
	for tok := range want {
		if !got[tok] {
			t.Errorf("propertyWords lacks %q", tok)
		}
	}
	for tok := range got {
		if !want[tok] {
			t.Errorf("propertyWords holds %q, the stem of no disclosed word", tok)
		}
	}
}
