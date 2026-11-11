package jsonscan

import (
	"errors"
	"testing"
)

// TestElements_WalksAnArrayWithoutDecoding: Elements hands back each
// element's span in order, over strings, numbers, nested objects and
// arrays, stops when fn says so, and refuses a span that is not an array.
func TestElements_WalksAnArrayWithoutDecoding(t *testing.T) {
	buf := []byte(`{"a":[ "x", 2 , {"k":[1,2]}, [3,"]"], null ], "b":"s"}`)
	var sp [2]Span
	if err := Top(buf, NewKeySet("a", "b"), sp[:]); err != nil {
		t.Fatal(err)
	}
	var got []string
	var kinds []Kind
	if err := Elements(buf, sp[0], func(s Span) bool {
		got = append(got, string(s.Raw(buf)))
		kinds = append(kinds, s.Kind)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{`"x"`, `2`, `{"k":[1,2]}`, `[3,"]"]`, `null`}
	if len(got) != len(want) {
		t.Fatalf("elements = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d = %q, want %q", i, got[i], want[i])
		}
	}
	if kinds[0] != String || kinds[1] != Number || kinds[2] != Object || kinds[3] != Array || kinds[4] != Null {
		t.Errorf("kinds = %v", kinds)
	}
	n := 0
	_ = Elements(buf, sp[0], func(Span) bool { n++; return n < 2 })
	if n != 2 {
		t.Errorf("a false return did not stop the walk: %d", n)
	}
	if err := Elements(buf, sp[1], func(Span) bool { return true }); !errors.Is(err, ErrNotArray) {
		t.Errorf("a string span = %v, want ErrNotArray", err)
	}
	empty := []byte(`[]`)
	if err := Elements(empty, ValueSpan(empty), func(Span) bool { t.Error("an element of []"); return true }); err != nil {
		t.Error(err)
	}
}
