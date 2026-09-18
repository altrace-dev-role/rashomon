package report

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// forbiddenWords are claims about INTENT. The silent-failure line is a fact
// about text -- this many calls failed, these words are absent from the summary
// -- and the moment the renderer says why, it has asserted something this
// program cannot know.
//
// It is not squeamishness. The line's whole value is that a reader can check
// it: the count comes from records and the absent words are verifiable against
// the message in front of them. "The agent hid two failures" is not checkable,
// and one unprovable accusation costs the reader's trust in every line that
// was provable.
var forbiddenWords = []string{"lied", "lie", "hid", "hiding", "gaslit", "gaslight", "deceived", "concealed", "dishonest"}

// TestReportRendererMakesNoClaimAboutIntent scans the STRING LITERALS of this
// package's non-test source.
//
// Literals rather than raw file bytes, and that distinction is the test being
// correct rather than merely strict. What a user reads is what the renderer
// prints, and only a literal can be printed. A comment has to be free to
// explain the rule -- saying why a word is forbidden requires naming it, as
// this file does throughout, and the first version of this test failed on its
// own rationale and on Luis's unrelated prose about hiding a deletion.
//
// Whole-word matching inside each literal, so a field named `hidden` or a
// sentence about a forbidden word is not a finding while an actual accusation
// is.
func TestReportRendererMakesNoClaimAboutIntent(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	var scanned, literals int
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		scanned++

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			literals++
			lower := strings.ToLower(text)
			for _, w := range forbiddenWords {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`)
				if re.MatchString(lower) {
					t.Errorf("%s prints the word %q: %q\nThe report states facts about text "+
						"-- how many calls failed, which words are absent -- and never why. "+
						"An unprovable claim costs the reader's trust in every line that was "+
						"provable.", fset.Position(lit.Pos()), w, text)
				}
			}
			return true
		})
	}
	// Guard the premise twice: a glob that matched nothing, or an AST walk that
	// found no literals, would both pass in silence.
	if scanned < 4 {
		t.Fatalf("scanned only %d non-test files in this package; the glob is not looking "+
			"at the renderer any more", scanned)
	}
	if literals < 20 {
		t.Fatalf("found only %d string literals across %d files; the walk is not reaching "+
			"the rendered text", literals, scanned)
	}
}

// TestFailureVocabularyIsBroadEnoughToBeConservative pins the direction of the
// list's error.
//
// A WIDER vocabulary makes the silent-failure line harder to fire, because any
// one of these words counts as acknowledgement. So breadth is the defence
// against a false positive, and a future edit that trims the list to the
// "real" failure words would make the line fire more often, not less.
func TestFailureVocabularyIsBroadEnoughToBeConservative(t *testing.T) {
	if len(failureVocabulary) < 25 {
		t.Errorf("the vocabulary has %d words. It is deliberately broad: every word in it "+
			"is a way for the agent to be judged honest, so trimming the list makes "+
			"the line fire MORE often and on weaker grounds.", len(failureVocabulary))
	}
	// The words a real summary is most likely to use must be present, or the
	// line fires on summaries that plainly acknowledged the problem.
	for _, w := range []string{"fail", "error", "could not", "unable", "issue", "problem", "skipped"} {
		var found bool
		for _, v := range failureVocabulary {
			if v == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the vocabulary is missing %q, a word an honest summary commonly uses", w)
		}
	}
	// Lower-case only: the comparison lower-cases the message, so a
	// capitalised entry here could never match.
	for _, w := range failureVocabulary {
		if w != strings.ToLower(w) {
			t.Errorf("vocabulary entry %q is not lower-case and can never match", w)
		}
	}
}
