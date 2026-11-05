package disclose

import (
	"strings"
	"testing"
)

// TestAI1_ScreenSaysTheGraphIsReadAfterAnEdit: the screen's list of what is
// read in memory and never stored names the blast-radius read (ARCH-INLINE1,
// the DE's condition (b)): after an edit, the hook reads the platform graph
// file pgraph wrote to name the packages calling the edited one. That read
// is new at hook time and rides on the same scope version (5 covers the
// release), so this clause is its consent. Break: drop the clause, and a
// hook reads a project file the screen does not name.
func TestAI1_ScreenSaysTheGraphIsReadAfterAnEdit(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const clause = "after an edit, the platform graph file pgraph wrote in the project, to name the packages that call the edited one"
	said := 0
	for _, p := range inv.Paragraphs {
		if !strings.Contains(p.Text, clause) {
			continue
		}
		said++
		if p.Owner != OwnerVerify || p.Cite != CiteSection2 || !strings.HasPrefix(p.Text, "READ IN MEMORY, NEVER STORED:") {
			t.Errorf("the clause is in %s/%s %q, want the read-in-memory list (%s/%s)", p.Owner, p.Cite, p.Text, OwnerVerify, CiteSection2)
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs name the platform graph read after an edit, want 1", said)
	}
}

// TestAI2_ScreenSaysPgraphWritesEachSymbolsPlace (W-M1.2): pgraph writes a
// third file under .pgraph, each declared symbol's file, line and doc
// sentence, that rashomon reads with the log and the digests
// (platform.ReadSnapshot). It is code the user owns, written beside what the
// screen already names, so the pgraph paragraph says it and the scope
// version stays. Break: drop the clause, and a file is written in the
// project that the screen does not name.
func TestAI2_ScreenSaysPgraphWritesEachSymbolsPlace(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const clause = "the file and line that declares it with the first sentence of its doc comment"
	said := 0
	for _, p := range inv.Paragraphs {
		if !strings.Contains(p.Text, clause) {
			continue
		}
		said++
		if p.Owner != OwnerRecord || p.Cite != CiteSection2 || !strings.HasPrefix(p.Text, "With the plugin,") {
			t.Errorf("the clause is in %s/%s %q, want the pgraph paragraph (%s/%s)", p.Owner, p.Cite, p.Text, OwnerRecord, CiteSection2)
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say pgraph writes each symbol's place, want 1", said)
	}
}
