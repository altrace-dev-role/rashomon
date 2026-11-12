package disclose

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestUB12_InventoryNamesTheUnitBriefFiles (W-M2.3 hook half, D12): when
// the session's content is kept, the prompt hook writes each unit's brief
// under graph/unitbrief/<session>, through a temp file of the writer's own
// pattern. The names the writer makes (graph's UnitBriefDir and
// UnitBriefFileName, and its temp pattern as os.CreateTemp fills it) each
// fall under exactly one row, private, removed by purge, and the brief's
// row says it holds no word of the prompt. The rows are literals, since
// disclose does not import graph; this test is where they meet graph's
// constants. Break: drop unitBriefFiles from Default, or change graph's
// directory or temp pattern without the rows.
func TestUB12_InventoryNamesTheUnitBriefFiles(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	dir := graph.UnitBriefDir("", "0b6a9c52-1111-4222-8333-444455556666")
	brief := filepath.Join(dir, graph.UnitBriefFileName("/src/proj", "unit:py:services/auth"))
	temp := filepath.Join(dir, strings.Replace(graph.UnitBriefTempPattern, "*", "2718281828", 1))
	for _, rel := range []string{brief, temp} {
		var rows []FileRule
		for _, r := range inv.Files {
			if matchesAFileRule(rel, []FileRule{r}) {
				rows = append(rows, r)
			}
		}
		if len(rows) != 1 {
			t.Errorf("%d rows cover %s, want one", len(rows), rel)
			continue
		}
		r := rows[0]
		if r.Mode != "0600" || !strings.Contains(r.RemovedBy, "purge") || r.Owner != "cmd" {
			t.Errorf("%s's row is mode %q, removed by %q, owner %q; want 0600, purge, cmd", rel, r.Mode, r.RemovedBy, r.Owner)
		}
		if !strings.Contains(r.NeverHolds, "any word of the prompt") {
			t.Errorf("%s's row does not say it never holds a word of the prompt: %q", rel, r.NeverHolds)
		}
	}
}

// TestUB12b_ScreenSaysEachUnitsBriefIsKept (D12): the graph index
// paragraph, which says the selection is written under the store's graph
// directory, goes on to say that where the session's content is kept the
// brief for the first edit in each selected unit is written there too,
// with what it holds and no word of the prompt. One paragraph says so, and
// after the graph directory it says "there" of. Break: drop the sentence,
// or move it before the graph directory is named.
func TestUB12b_ScreenSaysEachUnitsBriefIsKept(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const clause = "under unitbrief/<session>, the brief for the first edit in each unit the selection's lines lie in, up to eight units"
	said := 0
	for _, p := range inv.Paragraphs {
		i := strings.Index(p.Text, clause)
		if i < 0 {
			continue
		}
		said++
		if p.Owner != OwnerRecord || p.Cite != CiteSection2 || !strings.HasPrefix(p.Text, "With `rashomon graph index") {
			t.Errorf("the clause is in %s/%s %q, want the graph index paragraph (%s/%s)", p.Owner, p.Cite, p.Text, OwnerRecord, CiteSection2)
		}
		j := strings.Index(p.Text, "under the store's graph directory.")
		if j < 0 || j > i {
			t.Errorf("the unit brief's sentence does not follow the graph directory's: %q", p.Text)
			continue
		}
		after := p.Text[j:]
		for _, also := range []string{"Where the session's content is kept", "the files of other units that use the unit",
			"the recorded decisions and lessons that apply to it", "offers for reuse there"} {
			if !strings.Contains(after, also) {
				t.Errorf("the unit brief's sentence does not say %q", also)
			}
		}
		if !strings.Contains(after, "with no word of the prompt") {
			t.Errorf("the unit brief's sentence does not say it keeps no word of the prompt: %q", p.Text)
		}
		// No prompt word is stored, yet the set of units with a brief is
		// derived from the prompts: the screen says so (ruling 03:50 (i)).
		const shows = "which units have a brief shows what the session's prompts pointed at."
		if !strings.HasSuffix(p.Text, shows) {
			t.Errorf("the graph index paragraph does not end on %q: %q", shows, p.Text)
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say each unit's brief is kept, want 1", said)
	}
}
