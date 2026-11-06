package steer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestGS3_TheLineRecordsAGraphLineAsFlatArrays (card GRAPH-SLOT1): a graph
// line's slot is one object of flat arrays and counts, keys sorted, so the
// delivered-set reader finds its knowledge ids with jsonscan; the record
// form (record scope v4) holds the path as its keyed digest, each list as
// its count and the ids and clause as they are. The two forms of INC-212's
// line, the slot's record and the rendered sentence, are logged side by
// side: the owner's "show me both before it is used".
func TestGS3_TheLineRecordsAGraphLineAsFlatArrays(t *testing.T) {
	know := slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: "les:INC-212",
		Path:   "../acme-ops/jobs/purge_sessions.py",
		Clause: "the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",
		Source: "incidents/INC-212.md"})
	code := slot.NewCodeLine(slot.CodeFact{Path: "../acme-ops/jobs/purge_sessions.py", Names: []slot.Ident{"run"},
		Consts:  []slot.ConstFact{{Name: "BATCH", Value: 500}},
		Writes:  []slot.TableRef{{Table: "public.sessions", Cols: []slot.Ident{"created_at"}}},
		Imports: []slot.Ident{"auth.sessions"}, Lessons: []slot.KnowledgeID{"les:INC-212"}})
	m, err := render.New(pattern.TaskPreflight, render.NewGraphBinding("C1", 2, 9, []slot.GraphLine{code, know}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := slotsOf(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(m.Text(), "\n")[1:] {
		t.Logf("rendered line: %s", line)
	}
	for _, name := range []string{"line1", "line2"} {
		t.Logf("slot record %s: %s", name, got[name])
	}
	wantCode := `{"calls":[],"clause":"","consumers":[],"counts":{"calls":0,"consts":0,"consumers":0,"imports":0,"lessons":0,"names":0,"readers":0,"reads":0,"writers":0,"writes":0},` +
		`"digest":"","id":"","imports":["auth.sessions"],"kind":"code","lessons":["les:INC-212"],"names":["run","BATCH"],` +
		`"path":"../acme-ops/jobs/purge_sessions.py","readers":[],"reads":[],"role":"","source":"","source_digest":"","table":"","writers":[],"writes":["public.sessions"]}`
	if string(got["line1"]) != wantCode {
		t.Errorf("line1 = %s\nwant %s", got["line1"], wantCode)
	}
	wantKnow := `{"calls":[],"clause":"the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",` +
		`"consumers":[],"counts":{"calls":0,"consts":0,"consumers":0,"imports":0,"lessons":0,"names":0,"readers":0,"reads":0,"writers":0,"writes":0},` +
		`"digest":"","id":"les:INC-212","imports":[],"kind":"lesson","lessons":[],"names":[],"path":"../acme-ops/jobs/purge_sessions.py",` +
		`"readers":[],"reads":[],"role":"","source":"incidents/INC-212.md","source_digest":"","table":"","writers":[],"writes":[]}`
	if string(got["line2"]) != wantKnow {
		t.Errorf("line2 = %s\nwant %s", got["line2"], wantKnow)
	}
	if string(got["line3"]) != string(got["line5"]) || !strings.HasPrefix(string(got["line3"]), `{"calls":[],"clause":"","consumers":[],"counts":`) {
		t.Errorf("an empty line slot = %s", got["line3"])
	}
	if string(got["rev"]) != `"C1"` || string(got["matched"]) != "2" || string(got["reached"]) != "9" {
		t.Errorf("header slots = %s %s %s", got["rev"], got["matched"], got["reached"])
	}
	// The record form: digests and counts, no path and no name.
	digest := func(string) string { return strings.Repeat("0", 64) }
	rec, err := render.New(pattern.TaskPreflight, render.NewGraphBinding("C1", 2, 9, []slot.GraphLine{
		boundGraph(t, code).Record(digest), boundGraph(t, know).Record(digest)}))
	if err != nil {
		t.Fatalf("record form: %v", err)
	}
	recSlots, err := slotsOf(rec)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("record-scope v4 line1: %s", recSlots["line1"])
	t.Logf("record-scope v4 line2: %s", recSlots["line2"])
	var l1 struct {
		Path, Digest string
		Names        []string
		Counts       map[string]int
		Lessons      []string
	}
	if err := json.Unmarshal(recSlots["line1"], &l1); err != nil {
		t.Fatal(err)
	}
	if l1.Path != "" || l1.Digest != strings.Repeat("0", 64) || len(l1.Names) != 0 || l1.Counts["names"] != 1 || l1.Counts["consts"] != 1 ||
		l1.Counts["writes"] != 1 || len(l1.Lessons) != 1 {
		t.Errorf("record form line1 = %+v", l1)
	}
	if !strings.Contains(string(recSlots["line2"]), `"path":""`) || !strings.Contains(string(recSlots["line2"]), `"source_digest":"`+strings.Repeat("0", 64)+`"`) {
		t.Errorf("record form line2 = %s", recSlots["line2"])
	}
}

// boundGraph is l as a message holds it.
func boundGraph(t *testing.T, l slot.GraphLine) slot.GraphLine {
	t.Helper()
	v, err := slot.Bind(l)
	if err != nil {
		t.Fatal(err)
	}
	return v.(slot.GraphLine)
}
