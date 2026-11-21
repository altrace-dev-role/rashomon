package index_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
)

// keptHead opens the consent paragraph that says what a yes keeps of a
// session's edits (internal/disclose/paragraphs.go, line 67 at bef6cedb).
const keptHead = "KEPT FROM YOUR SESSIONS"

// keptFiles is that paragraph's clause naming the files whose edits are
// kept, "for each Write, Edit or MultiEdit of a Go file there"; its group is
// the language words, joined by a comma or "or".
var keptFiles = regexp.MustCompile(`for each Write, Edit or MultiEdit of an? ([A-Z][A-Za-z]*(?:(?:, | or )[A-Z][A-Za-z]*)*) files? there`)

// keptJoin splits keptFiles' group into its words.
var keptJoin = regexp.MustCompile(`, | or `)

// readerWords is, for each language of the registry a hook builds, a file
// its reader claims and the word the consent text names its files by. A
// language registered with no row here fails the test, so a new reader
// comes with its word.
var readerWords = map[string]struct{ file, word string }{
	"go": {"a/x.go", "Go"},
	"py": {"a/x.py", "Python"},
}

// TestConsentGate2_AKeptReaderIsNamedByTheKeptParagraph: every reader of the
// registry a hook builds (index.Wire) that states a session's edits of its
// files are kept (lang.Kept, read through Registry.SessionKept, the overlay
// writer's own question) is of a language the consent paragraph saying what
// a yes keeps names. The expected words are the paragraph's, never a
// reader's. Breaks: state pylite kept, or let the registry call kept every
// reader that states anything.
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
	m := keptFiles.FindAllStringSubmatch(texts[0], -1)
	if len(m) != 1 {
		t.Fatalf("premise: the kept paragraph names its files %d times by %v; want once", len(m), keptFiles)
	}
	words := keptJoin.Split(m[0][1], -1)
	reg := index.Wire()
	root := t.TempDir()
	kept := 0
	for _, l := range reg.Langs() {
		row, ok := readerWords[l]
		if !ok {
			t.Errorf("the registry's %s reader has no row in readerWords: add the word the consent text names its files by", l)
			continue
		}
		if r, ok := reg.For(root, row.file); !ok || r.Lang() != l {
			t.Errorf("premise: %s is not claimed by the %s reader", row.file, l)
			continue
		}
		if !reg.SessionKept(root, row.file) {
			continue
		}
		kept++
		if !slices.Contains(words, row.word) {
			t.Errorf("the %s reader states a session's edits of its files are kept; the consent paragraph keeps those of %q alone", l, words)
		}
	}
	if kept == 0 {
		t.Errorf("premise: no reader of the registry states its session edits are kept; the paragraph names %q", words)
	}
}
