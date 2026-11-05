package graph

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestHeader_RoundTrip (W-M1.4, PLAN B6): the file's first line is the
// schema, the repository id, the revision, the producers and the record
// count under those names; Load gives the header back and the records
// under it; a header with no repository or no commit is refused.
func TestHeader_RoundTrip(t *testing.T) {
	s := NewStore()
	rev := RevCommit("c1")
	s.AssertNode(Node{ID: "unit:go:a", Kind: "unit", Tier: Proven, Conf: 1, Rev: rev})
	s.AssertNode(Node{ID: "file:a/a.go", Kind: "file", Tier: Proven, Conf: 1, Rev: rev})
	s.AssertEdge(Edge{Src: "unit:go:a", Pred: Contains, Dst: "file:a/a.go", Tier: Proven, Conf: 1, Rev: rev})
	var buf bytes.Buffer
	h := Header{Repo: "github.com/x/y", Rev: HeaderRev{Commit: "c1"}, Producers: map[string]string{"go": "pgraph/v0"}, Records: 99}
	if err := Save(&buf, s, h); err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(buf.String(), "\n")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(first), &raw); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"schema": "1", "repo": `"github.com/x/y"`, "rev": `{"commit":"c1"}`, "producers": `{"go":"pgraph/v0"}`, "records": "3"} {
		if string(raw[k]) != want {
			t.Errorf("header %s = %s; want %s (line %s)", k, raw[k], want, first)
		}
	}
	got, gh, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if gh.Schema != Schema || gh.Repo != "github.com/x/y" || gh.Rev.Commit != "c1" || gh.Producers["go"] != "pgraph/v0" || gh.Records != 3 {
		t.Errorf("loaded header = %+v", gh)
	}
	if _, ok := got.Edge("unit:go:a", Contains, "file:a/a.go", rev); !ok {
		t.Errorf("the edge did not survive the round trip")
	}
	for name, bad := range map[string]Header{"no repo": {Rev: HeaderRev{Commit: "c1"}}, "no commit": {Repo: "r"}} {
		if err := Save(&bytes.Buffer{}, s, bad); err == nil {
			t.Errorf("%s: Save accepted %+v", name, bad)
		}
	}
}

// TestLoad_RefusesOtherSchema (W-M1.4): a file under another schema, or
// the headerless form this package wrote before the schema line, is an
// error that says so, never a store with an unknown repository.
func TestLoad_RefusesOtherSchema(t *testing.T) {
	for name, head := range map[string]string{
		"schema 2":  `{"schema":2,"repo":"r","rev":{"commit":"c1"},"producers":{},"records":0}`,
		"old form":  `{"Commit":"c1","Records":0}`,
		"no schema": `{"repo":"r","rev":{"commit":"c1"}}`,
	} {
		_, _, err := Load(strings.NewReader(head + "\n"))
		if err == nil || !strings.Contains(err.Error(), "schema") {
			t.Errorf("%s: err = %v; want a schema error", name, err)
		}
	}
}
