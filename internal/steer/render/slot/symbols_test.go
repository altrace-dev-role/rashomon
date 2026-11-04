package slot

import (
	"errors"
	"testing"
)

// TestPW1b3_SymbolFactsScreensAndCounts (PW1b-ON-230 step 3): a symbol
// renders quoted with its package's directory and its direct callers, "1
// package" in the singular and "no direct callers" for none; a name that is
// no identifier, or is secret-shaped (by the entropy test or by the deny
// list alone), is left out and counted; caller
// packages that cannot fit the callers refuse the fact; the record form is
// the count alone; none reads "none". Break: render a refused name, or the
// counted form's names, and this fails.
func TestPW1b3_SymbolFactsScreensAndCounts(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    SymbolFacts
		want string
	}{
		{"two", NewSymbolFacts([]SymbolFact{{Name: "CountRows", Dir: "a", Callers: 2, Packages: 2}, {Name: "T.M", Dir: "b/c", Callers: 1, Packages: 1}}, 0),
			"'CountRows' declared in 'a', direct callers 2 in 2 packages; 'T.M' declared in 'b/c', direct callers 1 in 1 package"},
		{"no callers", NewSymbolFacts([]SymbolFact{{Name: "Pair", Dir: "b", Callers: 0, Packages: 0}}, 0), "'Pair' declared in 'b', no direct callers"},
		{"not an identifier", NewSymbolFacts([]SymbolFact{{Name: "a b", Dir: "a"}, {Name: "Pair", Dir: "b"}}, 0), "'Pair' declared in 'b', no direct callers and 1 more"},
		{"secret-shaped", NewSymbolFacts([]SymbolFact{{Name: "AKIAIOSFODNN7EXAMPLEZQ", Dir: "a"}}, 0), "1 left out"},
		// Low entropy and no token run: only the deny list refuses it.
		{"on the deny list alone", NewSymbolFacts([]SymbolFact{{Name: "AKIAAAAAAAAAAAAAAAAA", Dir: "a"}}, 0), "1 left out"},
		{"a root package", NewSymbolFacts([]SymbolFact{{Name: "Pair", Dir: "."}}, 0), "1 left out"},
		{"counted", NewSymbolFacts([]SymbolFact{{Name: "CountRows", Dir: "a", Callers: 2, Packages: 2}}, 1).Counted(), "2 from the platform graph"},
		{"none", NewSymbolFacts(nil, 0), "none"},
	} {
		if got := bound(t, tc.name, tc.s); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	bad := NewSymbolFacts([]SymbolFact{{Name: "Pair", Dir: "b", Callers: 1, Packages: 2}}, 0)
	if err := bad.Validate(); err == nil {
		t.Error("two caller packages for one caller: valid")
	}
	if err := NewSymbolFacts([]SymbolFact{{Name: "AKIAIOSFODNN7EXAMPLEZQ", Dir: "a"}}, 0).Validate(); !errors.Is(err, ErrSecretShaped) {
		t.Errorf("a secret-shaped name: %v, want ErrSecretShaped", err)
	}
}
