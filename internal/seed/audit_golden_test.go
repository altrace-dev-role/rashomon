package seed

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/audit/audit.golden from the current fold")

// goldenInput is one transcript the golden fold reads: a file under
// testdata/audit or the repository's Claude Code fixture, or one of this
// package's synthetic sessions.
type goldenInput struct {
	name, project string
	sub           bool
	text          func(t *testing.T) string
}

func goldenInputs() []goldenInput {
	file := func(path string) func(t *testing.T) string {
		return func(t *testing.T) string {
			t.Helper()
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	tx := func(build func(*testing.T) *tx) func(t *testing.T) string {
		return func(t *testing.T) string { return build(t).text() }
	}
	return []goldenInput{
		{"edges", "p-edges", false, file(filepath.Join("testdata", "audit", "edges.jsonl"))},
		{"edges as a subagent", "p-edges", true, file(filepath.Join("testdata", "audit", "edges.jsonl"))},
		{"claude-code 2.1.280 fixture", "p-fixture", false,
			file(filepath.Join("..", "..", "test", "fixtures", "claude-code", "2.1.280", "transcript.jsonl"))},
		{"session A", "p1", false, tx(sessionA)},
		{"session B", "p2", false, tx(sessionB)},
		{"subagent of B", "p2", true, tx(subagentB)},
	}
}

// goldenFold is everything `rashomon audit` prints and keeps for the inputs:
// the card, the kept card both ways, every summary field and row, and the
// kept file a fold with a cut and a project key writes.
func goldenFold(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	cut := time.Date(2026, 11, 10, 10, 30, 0, 0, time.UTC)
	for _, pass := range []struct {
		name string
		make func() *Tally
	}{
		{"plain", New},
		{"until and key", func() *Tally {
			tl := New()
			tl.Until = cut
			tl.Key = func(cwd string) string { return "key:" + cwd }
			return tl
		}},
	} {
		tl := pass.make()
		for _, in := range goldenInputs() {
			if err := tl.Read(strings.NewReader(in.text(t)), in.project, in.sub); err != nil {
				t.Fatalf("%s: %v", in.name, err)
			}
		}
		s := tl.Summary()
		fmt.Fprintf(&b, "=== pass %s\n--- summary\n", pass.name)
		rows := s.Rows
		s.Rows = nil
		fmt.Fprintf(&b, "%+v\n", s)
		for _, r := range rows {
			fmt.Fprintf(&b, "%3d %3d  %s\n", r.Times, r.Sessions, r.Text)
		}
		s.Rows = rows
		fmt.Fprintf(&b, "--- card\n%s--- kept card (kept)\n%s--- kept card (not kept)\n%s",
			Card(s, 14), KeptCard(s, 14, true), KeptCard(s, 14, false))
		k := tl.Kept(cut.Add(time.Hour), cut.Add(-14*24*time.Hour), cut, 14)
		kj, err := json.MarshalIndent(k, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "--- kept\n%s\n", kj)
	}
	return b.String()
}

// TestSEED_AuditGolden pins `rashomon audit`'s output byte for byte over a
// corpus that reaches every branch of the transcript read: each tool class,
// each tool-error opening, each denial kind, markers, markup, meta lines,
// questions, sidechains, odd and torn lines, and timestamps around a cut.
// The golden was written by the reader before the trace adapter replaced it,
// so this test is the proof that the move changed nothing.
func TestSEED_AuditGolden(t *testing.T) {
	got := goldenFold(t)
	path := filepath.Join("testdata", "audit", "audit.golden")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("the audit fold differs from %s; first difference:\n%s", path, firstDiff(string(want), got))
	}
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\n want: %q\n  got: %q", i+1, wl, gl)
		}
	}
	return "(none)"
}
