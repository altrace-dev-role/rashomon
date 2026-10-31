package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleKnowledge() *Knowledge {
	seq, at := int64(30), int64(1_761_700_000_000)
	return &Knowledge{
		Project:  "/Users/k/src/customer-app",
		Baseline: &KnowledgeBaseline{SessionID: "41aa09", RecordedAtUnixMS: at, PointSeq: 88},
		Retained: Retained{RequiredFamilies: 2, Facts: "read at each session start, not stored (rashomon brief)",
			Specs:          []SpecHeld{{Path: "/Users/k/src/customer-app/.claude/rashomon/spec.json", FileDigest: "sha256:ab12cd34ef56", BoundAtMS: at}},
			SpecAtLastStop: &SpecAtStop{Trust: "trusted", Compiled: "sha256:5e6f70818293", PointSeq: 30},
			Lessons:        []LessonRef{{ID: "npm-ci", Tier: "probation"}}},
		Discovered:  []Change{{What: "spec trusted at the Stop after #30 (compiled sha256:5e6f70818293)", To: "trusted", Source: "checkpoint #30", PointSeq: &seq}},
		Invalidated: []Change{},
		GateStates:  []GateStates{{Gate: "learned:go-test", Changes: 2, LastFrom: "red", LastTo: "green", LastSeq: 44}},
	}
}

// TestRP2p_RedactDropsNamesKeepsCounts (card REPORT1b K8 a, REPORT1b-3): under
// --redact, in the JSON and the text alike, neither the project root nor a
// bound spec's path appears, and the ids, counts, seqs, gate names, digests
// and times are what they were; the original report still holds its names.
// Break: skip the knowledge pass, so --json keeps the spec path.
func TestRP2p_RedactDropsNamesKeepsCounts(t *testing.T) {
	rep := &Report{Sessions: []Session{{SessionID: "s1", Knowledge: sampleKnowledge()}}}
	red := Redact(rep, []byte("install-key"))
	body, err := json.Marshal(red)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	if err := Text(&text, red); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"customer-app", "/Users/k", ".claude/rashomon/spec.json"} {
		if strings.Contains(string(body), name) || strings.Contains(text.String(), name) {
			t.Errorf("%q survived --redact:\n%s\n%s", name, body, text.String())
		}
	}
	flat := strings.Join(strings.Fields(text.String()), " ")
	for _, keep := range []string{"session 41aa09", "at #88", "2 gate families required", "file sha256:ab12cd34ef56", "bound 10-29 ",
		"compiled sha256:5e6f70818293", "npm-ci probation", "learned:go-test 2 changes, last red to green at #44", "(checkpoint #30)"} {
		if !strings.Contains(flat, keep) {
			t.Errorf("--redact dropped %q:\n%s", keep, text.String())
		}
	}
	orig, _ := json.Marshal(rep)
	if !strings.Contains(string(orig), "customer-app") || !strings.Contains(string(orig), ".claude/rashomon/spec.json") {
		t.Error("Redact wrote through to the original report")
	}
	// The same fields, plain: the redacted copy differs from it only in the two names.
	if k := red.Sessions[0].Knowledge; k.Baseline == rep.Sessions[0].Knowledge.Baseline || k.Retained.SpecAtLastStop == rep.Sessions[0].Knowledge.Retained.SpecAtLastStop ||
		k.Discovered[0].PointSeq == rep.Sessions[0].Knowledge.Discovered[0].PointSeq {
		t.Error("the copy shares a pointer with the original")
	}
}
