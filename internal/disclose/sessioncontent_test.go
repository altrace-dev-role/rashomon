package disclose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSessionContent_TheScreensSayWhatASessionKeeps (W-M2.11 step 1d): scope
// 6 is the session's content, and each text a yes is given on says so: the
// full screen's KEPT paragraph names what an edit's log keeps, the conflict
// base, and that the size cap does not cover the store's graph directory; the
// plugin's description names what each edit of a file in a language its index
// reads declares, and the short screen, at its 24 rows (TestCSH1), what each
// edit in an indexed repository declares.
// No paragraph puts the size cap beside the graph directory without saying it
// does not cover it. The marker, its temp, the session log and the evidence
// file each match a row, none of them removed by the size cap.
// TestSessionContent_EveryTextNamesTheIndexedRepository: the session's
// content is kept under a condition on a file, .rashomon/graph.jsonl at or
// up to four directories above the working directory (graph.FindIndex),
// whoever wrote it: a cloned repository may carry one. The short screen, the
// full screen's KEPT paragraph and the README each name it as an indexed
// repository, the two long texts by its file too, and no text a yes is
// given on says "you indexed", which claims the person ran the index. The
// witness: the check finds the words this release replaced. Break: a text
// that says "you indexed" or names the condition in other words.
func TestSessionContent_EveryTextNamesTheIndexedRepository(t *testing.T) {
	for _, old := range []string{
		"In a repository you indexed with `rashomon graph index`, once you have agreed to this build's screen",
		"KEPT FROM YOUR SESSIONS after a yes here: in a repository with .rashomon/graph.jsonl, for each Write",
	} {
		if len(indexedConditionFaults("old", old, true)) == 0 {
			t.Errorf("witness: the check finds nothing in the replaced words %q", old)
		}
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var readme []string
	for _, p := range strings.Split(string(b), "\n\n") {
		if p = strings.Join(strings.Fields(p), " "); strings.Contains(p, "what each Go edit declares") {
			readme = append(readme, p)
		}
	}
	var kept, short []string
	for _, p := range defaultParagraphs() {
		switch {
		case strings.HasPrefix(p.Text, "KEPT FROM YOUR SESSIONS"):
			kept = append(kept, p.Text)
		case p.Cite == CiteShort && strings.HasPrefix(p.Text, "RECORDED ON THIS MACHINE"):
			short = append(short, p.Text)
		}
	}
	if len(readme) != 1 || len(kept) != 1 || len(short) != 1 {
		t.Fatalf("%d README paragraphs name what each Go edit declares, %d KEPT and %d short RECORDED paragraphs, want one of each", len(readme), len(kept), len(short))
	}
	var faults []string
	faults = append(faults, indexedConditionFaults("the README", readme[0], true)...)
	faults = append(faults, indexedConditionFaults("the full screen's KEPT paragraph", kept[0], true)...)
	faults = append(faults, indexedConditionFaults("the short screen's RECORDED paragraph", short[0], false)...)
	if strings.Contains(strings.ToLower(PluginDescription), "you indexed") {
		faults = append(faults, `the plugin's description says "you indexed"`)
	}
	for _, f := range faults {
		t.Error(f)
	}
}

// indexedConditionFaults is how text fails to name the indexed repository:
// it says "you indexed", it does not say "an indexed repository", or, for a
// long text, it does not name .rashomon/graph.jsonl.
func indexedConditionFaults(name, text string, long bool) []string {
	var out []string
	low := strings.ToLower(text)
	if strings.Contains(low, "you indexed") {
		out = append(out, name+` says "you indexed"`)
	}
	if !strings.Contains(low, "an indexed repository") {
		out = append(out, name+` does not say "an indexed repository"`)
	}
	if long && !strings.Contains(text, ".rashomon/graph.jsonl") {
		out = append(out, name+" does not name .rashomon/graph.jsonl")
	}
	return out
}

func TestSessionContent_TheScreensSayWhatASessionKeeps(t *testing.T) {
	if SessionContentScope != 6 || ScopeVersion < SessionContentScope {
		t.Errorf("SessionContentScope = %d and ScopeVersion = %d; want 6, and this build at or past it", SessionContentScope, ScopeVersion)
	}
	var kept, short int
	for _, p := range defaultParagraphs() {
		if strings.Contains(p.Text, "size cap") && strings.Contains(p.Text, "graph directory") && !strings.Contains(p.Text, "does not cover") {
			t.Errorf("%s/%s puts the size cap beside the graph directory and does not say it does not cover it: %q", p.Owner, p.Cite, p.Text)
		}
		switch {
		case strings.HasPrefix(p.Text, "KEPT FROM YOUR SESSIONS"):
			kept++
			if p.Owner != OwnerRecord || p.Cite != CiteSection2 {
				t.Errorf("the KEPT paragraph is %s/%s, want %s/%s", p.Owner, p.Cite, OwnerRecord, CiteSection2)
			}
			for _, want := range []string{"Write, Edit or MultiEdit there of a file in a language the index of that repository was built to read",
				"never any other text of the file", "conflict base",
				"does not cover the store's graph directory", "`rashomon purge`"} {
				if !strings.Contains(p.Text, want) {
					t.Errorf("the KEPT paragraph does not say %q", want)
				}
			}
		case p.Cite == CiteShort && strings.HasPrefix(p.Text, "RECORDED ON THIS MACHINE"):
			short++
			if !strings.Contains(p.Text, "In an indexed repository: what each edit declares") {
				t.Errorf("the short screen's RECORDED paragraph does not name what each edit in an indexed repository declares: %q", p.Text)
			}
		}
	}
	if kept != 1 || short != 1 {
		t.Errorf("%d KEPT paragraphs and %d short RECORDED paragraphs, want one of each", kept, short)
	}
	if !strings.Contains(PluginDescription, "what each edit of a file in a language its index reads declares") {
		t.Errorf("the plugin's description does not name what each edit of a file in a language its index reads declares: %q", PluginDescription)
	}

	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{store.FileSessionContent, "." + store.FileSessionContent + ".tmp-x1",
		"graph/0123456789abcdef.jsonl", "graph/sess1-prompt1.txt"} {
		if !matchesAFileRule(rel, inv.Files) {
			t.Errorf("%s matches no row of the inventory", rel)
		}
	}
	for _, r := range sessionContentFiles() {
		if r.Mode != "0600" || !strings.Contains(r.RemovedBy, "purge") || strings.Contains(r.RemovedBy, "cap") {
			t.Errorf("%s/%s's row is mode %q, removed by %q; want 0600, purge, and no size cap", r.Base, r.Pattern, r.Mode, r.RemovedBy)
		}
	}
}

// TestSessionContent_TheLogRowNamesEachLanguageTheIndexReads: the session
// log's inventory row says the log is appended at an edit of a file in any
// language the index of the repository reads, in the KEPT paragraph's words,
// and its Holds names what a reader other than Go's records of such a file,
// each a thing that paragraph names: a class among the declarations, the
// line of each declaration and of each call, and the module of the file.
// Break: the row as it was at scope 8, which named a Go file alone.
func TestSessionContent_TheLogRowNamesEachLanguageTheIndexReads(t *testing.T) {
	const files = "Write, Edit or MultiEdit there of a file in a language the index of that repository was built to read"
	var kept []string
	for _, p := range defaultParagraphs() {
		if strings.HasPrefix(p.Text, "KEPT FROM YOUR SESSIONS") {
			kept = append(kept, p.Text)
		}
	}
	if len(kept) != 1 {
		t.Fatalf("%d KEPT paragraphs, want one", len(kept))
	}
	for _, w := range []string{files, "class", "its line", "the line and number of arguments of each call", "module of the file"} {
		if !strings.Contains(kept[0], w) {
			t.Fatalf("premise: the KEPT paragraph does not say %q", w)
		}
	}
	row := slRow(t)
	if !strings.Contains(row.Purpose, files) || strings.Contains(row.Purpose, "Go file") {
		t.Errorf("the session log row's Purpose does not name the files the KEPT paragraph names (%q): %q", files, row.Purpose)
	}
	for _, w := range []string{"each function, class, type, method, constant and variable by name and kind",
		"the line of each declaration and of each call", "the module of the file"} {
		if !strings.Contains(row.Holds, w) {
			t.Errorf("the session log row's Holds does not say %q", w)
		}
	}
}
