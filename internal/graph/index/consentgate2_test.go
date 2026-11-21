package index_test

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
)

// keptHead opens the consent paragraph that says what a yes keeps of a
// session's edits (defaultParagraphs, internal/disclose/paragraphs.go).
const keptHead = "KEPT FROM YOUR SESSIONS"

// keptAnyLanguage is that paragraph's clause naming the files whose edits
// are kept: from scope 9, a file in any language the index of the
// repository reads, where scopes 6 to 8 named a Go file alone.
const keptAnyLanguage = "for each Write, Edit or MultiEdit there of a file in a language the index of that repository was built to read"

// anyLanguageScope is the first consent scope whose screen names, in the
// words of keptAnyLanguage, the files of a language other than Go. It is a
// fact of the consent history (the scope 9 entry above ScopeVersion in
// internal/disclose/disclose.go), never a reader's statement, and never
// disclose.ScopeVersion, which each later bump moves.
const anyLanguageScope = 9

// readerFiles is, for each language of the registry a hook builds, a file
// its reader claims. A language registered with no row here fails the
// test, so a new reader comes with its row.
var readerFiles = map[string]string{
	"go": "a/x.go",
	"py": "a/x.py",
}

// firstNamed is the first consent scope whose screen names the files of
// language l among those whose edits a yes keeps: Go's from the scope that
// first kept a session's content (disclose.SessionContentScope), any other
// language's from anyLanguageScope.
func firstNamed(l string) int {
	if l == "go" {
		return disclose.SessionContentScope
	}
	return anyLanguageScope
}

// TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph: every reader of the
// registry a hook builds (index.Wire) that states a session's edits of its
// files are kept (lang.Kept, read through Registry.SessionKept, the overlay
// writer's own question) states them kept from a scope at or past the first
// whose screen names its language: Go from 6, any other from 9. A reader
// states that scope with a SessionKeptFrom method; one that states none is
// held to scope 6, the first scope that kept a session's content. The
// expected scopes are the consent history's, never a reader's. Breaks:
// state pylite kept, or let the registry call kept every reader that
// states anything.
func TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, keptHead) {
			texts = append(texts, p.Text)
		}
	}
	if len(texts) != 1 {
		t.Fatalf("premise: %d paragraphs open with %q; want one", len(texts), keptHead)
	}
	if n := strings.Count(texts[0], keptAnyLanguage); n != 1 {
		t.Fatalf("premise: the kept paragraph says %q %d times; want once", keptAnyLanguage, n)
	}
	if disclose.ScopeVersion < anyLanguageScope {
		t.Fatalf("premise: ScopeVersion = %d, before scope %d, the first whose screen names a language other than Go", disclose.ScopeVersion, anyLanguageScope)
	}
	reg := index.Wire()
	root := t.TempDir()
	kept := 0
	for _, l := range reg.Langs() {
		file, ok := readerFiles[l]
		if !ok {
			t.Errorf("the registry's %s reader has no row in readerFiles: add a file it claims", l)
			continue
		}
		r, ok := reg.For(root, file)
		if !ok || r.Lang() != l {
			t.Errorf("premise: %s is not claimed by the %s reader", file, l)
			continue
		}
		if !reg.SessionKept(root, file) {
			continue
		}
		kept++
		from := disclose.SessionContentScope
		if k, ok := r.(interface{ SessionKeptFrom() int }); ok {
			from = k.SessionKeptFrom()
		}
		if want := firstNamed(l); from < want {
			t.Errorf("the %s reader states a session's edits of its files are kept from scope %d; the first scope whose screen names them is %d", l, from, want)
		}
	}
	if kept == 0 {
		t.Errorf("premise: no reader of the registry states its session edits are kept")
	}
}
