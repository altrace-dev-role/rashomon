package intent_test

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
)

// TestTB1_ANewEvidenceKindIsThePersonsAlone (card TOKEN-BUDGET1): a request
// that names only what the session was given and an evidence kind new to it
// still gets a note, for the person: the evidence a done claim will need is
// the aside's, and every line for the model lists only items given earlier,
// so the model's text is "". Control: a new path is the model's line.
// Break: drop the new kind from the note's freshness, and the person is
// never told a "deploy" after a "fix" of the same file needs another kind
// of evidence; or send the model a note of counts.
func TestTB1_ANewEvidenceKindIsThePersonsAlone(t *testing.T) {
	d := intent.Delivered{Paths: map[string]bool{"internal/a.go": true}, Evidence: map[string]bool{"judgment_disclosed": true}}
	in := intent.Intent{Verb: intent.VerbDeploy, Named: true, Paths: []string{"internal/a.go"}, Evidence: []string{"mechanism_stated"}}
	f, ok := intent.FindingKeyed(in, d, nil, 1, nil)
	if !ok || f.Message.Text() != "" || !strings.Contains(f.Message.Aside(), "mechanism_stated") {
		t.Errorf("ok %v, model %q, aside %q; want a note with no model text and the new kind in the aside", ok, f.Message.Text(), f.Message.Aside())
	}
	in.Paths = append(in.Paths, "internal/b.go")
	if f, ok := intent.FindingKeyed(in, d, nil, 1, nil); !ok || !strings.Contains(f.Message.Text(), "paths named: 'internal/b.go'; 1 given earlier") {
		t.Errorf("control: ok %v, model %q; want the new path's line", ok, f.Message.Text())
	}
}
