package disclose

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSessionContent_TheScreensSayWhatASessionKeeps (W-M2.11 step 1d): scope
// 6 is the session's content, and each text a yes is given on says so: the
// full screen's KEPT paragraph names what an edit's log keeps, the conflict
// base, and that the size cap does not cover the store's graph directory; the
// short screen and the plugin's description name what each Go edit declares.
// No paragraph puts the size cap beside the graph directory without saying it
// does not cover it. The marker, its temp, the session log and the evidence
// file each match a row, none of them removed by the size cap.
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
			for _, want := range []string{"Write, Edit or MultiEdit of a Go file", "never the file's other text", "conflict base",
				"does not cover the store's graph directory", "`rashomon purge`"} {
				if !strings.Contains(p.Text, want) {
					t.Errorf("the KEPT paragraph does not say %q", want)
				}
			}
		case p.Cite == CiteShort && strings.HasPrefix(p.Text, "RECORDED ON THIS MACHINE"):
			short++
			if !strings.Contains(p.Text, "what each Go edit declares") {
				t.Errorf("the short screen's RECORDED paragraph does not name what each Go edit declares: %q", p.Text)
			}
		}
	}
	if kept != 1 || short != 1 {
		t.Errorf("%d KEPT paragraphs and %d short RECORDED paragraphs, want one of each", kept, short)
	}
	if !strings.Contains(PluginDescription, "what each Go edit declares") {
		t.Errorf("the plugin's description does not name what each Go edit declares: %q", PluginDescription)
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
