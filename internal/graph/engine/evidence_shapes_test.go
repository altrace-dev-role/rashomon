package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// evidenceShapes is the table of the no-word pin (W-M2.11 1d; the cmd pin repeats it): each
// prompt word names nothing in the specimen, in a different shape, and
// want is whether Explain, the form for the person who typed it, carries
// the whole word anywhere but its prompt line (measured at 0e9c6002: only
// the path is read as a named word, so only its refusal text holds it).
// Evidence carries it in no row.
var evidenceShapes = []struct {
	name, word string
	want       bool
	tokens     []string
}{
	{"path", "nosuchdir/zqxv.go", true, []string{"zqxv", "nosuchdir"}},
	{"dotted symbol", "zqxvpkg.Wurble", false, []string{"zqxv", "zqxvpkg", "wurble"}},
	{"quoted string", `"zqxv wurble"`, false, []string{"zqxv", "wurble"}},
	{"flag-like word", "--zqxv-wurble", false, []string{"zqxv", "wurble"}},
}

// TestEvidence_HoldsNoWordOfAnyShape (W-M2.11 1d, D160): a prompt word
// that names nothing in the repository reaches the roles line's reason
// ("<word> is not in the index") and the pick's refusal; the evidence file
// keeps the reason's class and not the word, for a path, a dotted symbol,
// a quoted string and a flag-like word.
func TestEvidence_HoldsNoWordOfAnyShape(t *testing.T) {
	sp := graphtest.New(t)
	for _, tc := range evidenceShapes {
		t.Run(tc.name, func(t *testing.T) {
			prompt := "fix " + tc.word + " while purging the sessions"
			b := Build(sp.Store, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true})
			if b.Empty() || len(b.Rows) == 0 {
				t.Fatalf("the brief is empty: %d rows", len(b.Rows))
			}
			for _, w := range b.Roles.Named.Words {
				if w.Unit != "" && strings.Contains(strings.ToLower(w.Word), "zqxv") {
					t.Errorf("the word names a unit in the specimen: %+v", w)
				}
			}
			var explain []string
			for _, l := range strings.Split(b.Explain(), "\n") {
				if !strings.HasPrefix(l, "prompt: ") {
					explain = append(explain, l)
				}
			}
			carried := strings.Contains(strings.Join(explain, "\n"), tc.word)
			if carried != tc.want {
				t.Errorf("Explain without its prompt line carries %q = %v, want %v", tc.word, carried, tc.want)
			}
			ev := b.Evidence()
			for _, w := range append([]string{tc.word}, tc.tokens...) {
				if strings.Contains(ev, w) {
					t.Errorf("Evidence holds %q:\n%s", w, ev)
				}
			}
			for _, l := range strings.Split(ev, "\n") {
				rest, ok := strings.CutPrefix(l, "roles (")
				if !ok {
					continue
				}
				reason, _, ok := strings.Cut(rest, "): unit ")
				if !ok {
					t.Fatalf("roles line has no reason field: %q", l)
				}
				if reason != "" && !slices.Contains(whyPrefixes, reason) && reason != "other" {
					t.Errorf("roles line reason %q is not a whyClass table value", reason)
				}
			}
			for _, p := range b.Roles.Picks {
				if got := whyClass(p.Refused); got != "" && !slices.Contains(whyPrefixes, got) && got != "other" {
					t.Errorf("whyClass(%q) = %q, not in the table", p.Refused, got)
				}
			}
		})
	}
}

// TestWhyClass_IsTheLongestTablePrefix: the class is the longest table
// prefix, "" for "" and "other" for a reason the table lacks; the word
// after the prefix never survives.
func TestWhyClass_IsTheLongestTablePrefix(t *testing.T) {
	for in, want := range map[string]string{
		"":                                  "",
		"zqxv.go is not in the index":       "other",
		"named 'zqxv': zqxv is in no unit":  "named '",
		"refused: named 'zqxv': x":          "refused: named '",
		"refused: ambiguous: 3 units tie":   "refused: ambiguous: ",
		"refused: 'it' stands for a thing":  "refused: '",
		"unit; tie settled by zqxv":         "unit; tie settled by ",
		"unit":                              "unit",
		"anchor full over named 'zqxv' (u)": "anchor ",
		"no unit names the head 'zqxv'":     "no unit names the head '",
	} {
		if got := whyClass(in); got != want {
			t.Errorf("whyClass(%q) = %q, want %q", in, got, want)
		}
	}
}
