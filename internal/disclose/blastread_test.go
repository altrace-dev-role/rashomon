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

// TestAI3_ScreenSaysGraphIndexReadsTheOrigin (W-M1.4): the screen names the
// repository graph's reads and writes: `rashomon graph index` reads the
// configuration, the units' files, the pgraph sidecars and the origin url,
// writes .rashomon/graph.jsonl headed by that url as the repository's id;
// each prompt reads the graph in memory and the selection lands under the
// store's graph directory. One paragraph says so. Every paragraph that names
// the origin read names every file it reads (baseline.GitDirs and
// originURL: the config, the .git pointer, the commondir file), what the id
// drops (any user name and password, and an empty or default port) and the
// states that give the path's hash instead (no origin, a relative origin, an
// @ after the host part: baseline.NormalizeOrigin), in one wording; no
// paragraph keeps the wording of scope 7, which named neither the port nor a
// relative origin.
func TestAI3_ScreenSaysGraphIndexReadsTheOrigin(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const clause = "the url of the git remote named origin"
	every := []string{
		"the url of the git remote named origin from the repository's git files (the config, and for a worktree the .git pointer and commondir files)",
		"with its scheme, any user name and password, and an empty or default port removed",
		"when it has no origin, when its origin is a relative path, or when an @ follows its host part",
	}
	gone := []string{
		"with its scheme and any user name and password removed",
		"when it has no origin or when an @ follows its host part",
	}
	said, index := 0, 0
	for _, p := range inv.Paragraphs {
		for _, w := range gone {
			if strings.Contains(p.Text, w) {
				t.Errorf("a paragraph still says %q", w)
			}
		}
		if !strings.Contains(p.Text, clause) {
			continue
		}
		said++
		for _, w := range every {
			if !strings.Contains(p.Text, w) {
				t.Errorf("a paragraph names the origin read and does not say %q: %q", w, p.Text)
			}
		}
		if !strings.HasPrefix(p.Text, "With `rashomon graph index") {
			continue
		}
		index++
		if p.Owner != OwnerRecord || p.Cite != CiteSection2 {
			t.Errorf("the graph index paragraph is in %s/%s, want %s/%s", p.Owner, p.Cite, OwnerRecord, CiteSection2)
		}
		for _, also := range []string{".rashomon/graph.jsonl", "At each prompt", "with no word of the prompt", "rashomon graph knowledge import", ".rashomon/knowledge/"} {
			if !strings.Contains(p.Text, also) {
				t.Errorf("the graph index paragraph does not say %q", also)
			}
		}
	}
	if index != 1 {
		t.Errorf("%d graph index paragraphs say the index reads the origin url, want 1", index)
	}
	if said < 2 {
		t.Errorf("%d paragraphs name the origin read, want the graph index paragraph and the kept session log's", said)
	}
}

// TestAI3b_ScreenSaysWhenGraphIndexWritesTheConfiguration (x42.6, SETUP1):
// the graph index paragraph, and no other, says that `rashomon graph index`
// writes the configuration it inferred to .rashomon/graph.json only when
// nothing is at that name, and each state in which it writes nothing. The
// behaviour's tests are SETUP1's. Break: drop the sentence, change a state,
// or say it in another paragraph or a second one.
func TestAI3b_ScreenSaysWhenGraphIndexWritesTheConfiguration(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const absent = "When nothing is at .rashomon/graph.json in the repository, rashomon graph index writes there, as a new file, the configuration it inferred from the repository's manifest files; it writes nothing when a file, a directory or a link is already at that name, when .rashomon is a link, when it infers no unit, or when that configuration would be over 64 KiB."
	said := 0
	for _, p := range inv.Paragraphs {
		if !strings.Contains(p.Text, absent) {
			continue
		}
		said++
		if p.Owner != OwnerRecord || p.Cite != CiteSection2 || !strings.HasPrefix(p.Text, "With `rashomon graph index") {
			t.Errorf("the graph.json sentence is in %s/%s, not the graph index paragraph", p.Owner, p.Cite)
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say when graph index writes .rashomon/graph.json, want 1", said)
	}
}
