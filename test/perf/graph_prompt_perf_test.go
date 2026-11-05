//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// graphPromptCase is the prompt's graph read (W-M1.8, AC-M1.2): the
// recap pending entry over an index of PayloadBytes, the size of this
// repository's own graph.jsonl at the first M1 index (8.2 MB for 22,011
// records), so the row says what the engine's load and brief cost the
// prompt hook on a repository this size.
const graphPromptCase = "recap.pending.graph8MiB"

// graphPromptHeader names the synthetic index; the commit is graphtest's.
var graphPromptHeader = graph.Header{Repo: "github.com/perf/graph", Rev: graph.HeaderRev{Commit: graphtest.Commit}}

// syntheticAtLeast is a graphtest.Synthetic store whose saved form is at
// least size bytes and at most an eighth over: the record count is scaled
// by the measured bytes per record toward a sixteenth over size until the
// file lands in that band, within eight rounds.
func syntheticAtLeast(t *testing.T, size int) (*graph.Store, int) {
	t.Helper()
	records := size / 400
	for round := 0; round < 8; round++ {
		s := graphtest.Synthetic(records)
		var buf bytes.Buffer
		if err := graph.Save(&buf, s, graphPromptHeader); err != nil {
			t.Fatal(err)
		}
		if n := buf.Len(); n >= size && n <= size+size/8 {
			return s, n
		}
		records = records*(size+size/16)/buf.Len() + 1
	}
	t.Fatalf("no synthetic store between %d and %d bytes within eight rounds", size, size+size/8)
	return nil, 0
}

// graphPrompt returns the payload builder and the control of the prompt's
// graph read case: a project whose .rashomon/graph.jsonl is a synthetic
// index of at least size bytes, and a prompt in the index's vocabulary.
// The payload is recap.pending's with the project as its cwd. A premise
// run checks the steer log holds a graph line (the slot's source_digest
// key, which only the graph brief writes), so a case whose read stood
// down fails rather than reads light. control moves .rashomon away: the
// same prompt with nothing to read.
func (r rig) graphPrompt(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	proj, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, n := syntheticAtLeast(t, size)
	if err := graph.SaveFile(proj, s, graphPromptHeader); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: index of %d nodes, %d edges, %d bytes", graphPromptCase, len(s.Nodes(graph.RevCommit(graphtest.Commit))), len(s.Edges(graph.RevCommit(graphtest.Commit))), n)
	var fields map[string]any
	if err := json.Unmarshal(payload.Prompt("make session expiry respect the org-level timeout override."), &fields); err != nil {
		t.Fatal(err)
	}
	fields["cwd"] = proj
	prompt, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rashomonBin, r.args([]string{"recap", "pending"})...)
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(prompt), r.env, &stderr
	out, err := cmd.Output()
	st, serr := store.Open(r.home)
	if serr != nil {
		t.Fatal(serr)
	}
	steerLog, _ := os.ReadFile(filepath.Join(st.RunDir(payload.Session), store.FileSteer))
	if err != nil || !bytes.Contains(steerLog, []byte(`"source_digest"`)) {
		t.Fatalf("premise: the prompt's note holds no graph line, so the read proved nothing (%v):\n%s\nstderr: %s", err, out, stderr.Bytes())
	}
	state := filepath.Join(proj, ".rashomon")
	return func(int) []byte { return prompt }, func() {
		if err := os.Rename(state, state+".off"); err != nil {
			t.Fatal(err)
		}
	}
}
