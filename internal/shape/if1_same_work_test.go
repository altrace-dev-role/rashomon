package shape

import (
	"slices"
	"testing"
)

// TestIF1_SameWorkIsRows (card INFER-FALSE1): every name sameWork pairs is a
// row of testCommands, so a renamed row cannot drop out of the table
// unseen, and no left side is a right side, so RunnerKey's one pass maps
// every form to the one name; the two forms the FHV split, `pytest` and
// `python3 -m pytest`, meet at one key, while a launcher row keeps its
// digest. Break: drop the mapping from RunnerKey.
func TestIF1_SameWorkIsRows(t *testing.T) {
	rows := testCommandNames()
	for _, w := range sameWork {
		for _, n := range w {
			if !slices.Contains(rows, n) {
				t.Errorf("sameWork names %q, which is no testCommands row", n)
			}
		}
		if slices.ContainsFunc(sameWork, func(v [2]string) bool { return v[1] == w[0] }) {
			t.Errorf("%q is both a form and the name forms map to", w[0])
		}
	}
	d := "digest"
	a, _ := RunnerKey("pytest", &d)
	b, _ := RunnerKey("python3 -m pytest", &d)
	if a != b || a != "\x00pytest" {
		t.Errorf("keys %q and %q, want both %q", a, b, "\x00pytest")
	}
	if k, _ := RunnerKey("make", &d); k != d {
		t.Errorf("a launcher row: key %q, want its digest", k)
	}
}
