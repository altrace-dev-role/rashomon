package report

import "testing"

// TestOverlapping: the second of two id lists is not printed again when it is
// the same set as the first, or contained in it.
//
// These two lists are identical whenever the recorder was installed after the
// calls ran, which is the ordinary first-use case. In a real report that was
// the same 858 ids printed twice -- four hundred characters for one fact, and
// a reader left to compare two walls of opaque ids to notice they matched.
func TestOverlapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		items   []string
		printed []string
		want    string
		why     string
	}{
		{name: "identical sets collapse", items: []string{"a", "b"}, printed: []string{"a", "b"},
			want: `the same 2, listed under "above list" above`},
		{name: "subset names its size only", items: []string{"a"}, printed: []string{"a", "b"},
			want: `1, all of them among those under "above list" above`},
		{name: "not a subset is printed in full", items: []string{"a", "z"}, printed: []string{"a", "b"},
			want: "a, z",
			why:  "an id the reader has not already seen has to appear"},
		{name: "empty stays empty", items: []string{}, printed: []string{"a"}, want: none},
		{name: "nil stays unknown", items: nil, printed: []string{"a"}, want: unknown,
			why: "nil is 'we could not tell', and must never render as a measurement"},
		{name: "nothing printed above, so print in full", items: []string{"a"}, printed: nil, want: "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlapping(tc.items, tc.printed, "above list"); got != tc.want {
				t.Errorf("overlapping(%v, %v) = %q, want %q%s", tc.items, tc.printed, got, tc.want, because(tc.why))
			}
		})
	}
}

func because(why string) string {
	if why == "" {
		return ""
	}
	return "\n  " + why
}
