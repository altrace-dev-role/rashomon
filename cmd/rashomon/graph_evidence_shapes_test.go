package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGraphBrief_EvidenceFileHoldsNoWordOfAnyShape (W-M2.11 1d, D160): a
// prompt word that names nothing in the repository, as a path, a dotted
// symbol, a quoted string or a flag-like word, is in neither Evidence nor
// the bytes of the file the prompt hook writes under the store; the roles
// line keeps the class of the reason and not the word. The table repeats
// the engine pin's: want is whether Explain, without its prompt line,
// carries the whole word.
func TestGraphBrief_EvidenceFileHoldsNoWordOfAnyShape(t *testing.T) {
	for _, tc := range []struct {
		name, word string
		want       bool
		tokens     []string
	}{
		{"path", "nosuchdir/zqxv.go", true, []string{"zqxv", "nosuchdir"}},
		{"dotted symbol", "zqxvpkg.Wurble", false, []string{"zqxv", "zqxvpkg", "wurble"}},
		{"quoted string", `"zqxv wurble"`, false, []string{"zqxv", "wurble"}},
		{"flag-like word", "--zqxv-wurble", false, []string{"zqxv", "wurble"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			graphProject(t, dir)
			storeRoot := t.TempDir()
			prompt := "fix " + tc.word + " while purging the sessions"
			gb, ok := graphBrief(storeRoot, filepath.Join(dir, "services", "auth"), "s1", "p1", []byte(prompt))
			if !ok {
				t.Fatal("no brief")
			}
			for _, w := range gb.Roles.Named.Words {
				if w.Unit != "" && strings.Contains(strings.ToLower(w.Word), "zqxv") {
					t.Errorf("the word names a unit in the specimen: %+v", w)
				}
			}
			var explain []string
			for _, l := range strings.Split(gb.Explain(), "\n") {
				if !strings.HasPrefix(l, "prompt: ") {
					explain = append(explain, l)
				}
			}
			if carried := strings.Contains(strings.Join(explain, "\n"), tc.word); carried != tc.want {
				t.Errorf("Explain without its prompt line carries %q = %v, want %v", tc.word, carried, tc.want)
			}
			raw, err := os.ReadFile(filepath.Join(storeRoot, "graph", "s1-p1.txt"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			for _, w := range append([]string{tc.word}, tc.tokens...) {
				if strings.Contains(gb.Evidence(), w) {
					t.Errorf("Evidence holds %q", w)
				}
				if strings.Contains(strings.ToLower(text), strings.ToLower(w)) {
					t.Errorf("the evidence file holds %q:\n%s", w, text)
				}
			}
			if !strings.Contains(text, gb.Evidence()) {
				t.Errorf("the file lacks the brief's Evidence:\n%s", text)
			}
			for _, l := range strings.Split(text, "\n") {
				if rest, ok := strings.CutPrefix(l, "roles ("); ok {
					reason, _, ok := strings.Cut(rest, "): unit ")
					if !ok || strings.ContainsAny(reason, "0123456789/.") {
						t.Errorf("roles line reason is not a class: %q", l)
					}
				}
			}
		})
	}
}
