package disclose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestTRU23_RecordsRowNamesTheRuleHit (card SPEC-RULES-STOP1): a call that
// matched a spec's forbid_command rule leaves a spec_event in the run's
// records.ndjson, holding the rule's id, its spec file's digest, the hook
// event and the landed seq, so the one row that covers the file says so in
// its Holds; the written bytes are the proof it holds them.
func TestTRU23_RecordsRowNamesTheRuleHit(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendRecord("s1", store.NewRuleHit("s1", "PostToolUse", "spec:no-force", "sha256:"+strings.Repeat("11", 32), 4, 100)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.RunDir("s1"), store.FileRecords)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"spec_event"`, `"rule_id":"spec:no-force"`, `"source_digest":`, `"event":"PostToolUse"`, `"landed_seq":4`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("premise: the records file lacks %s: %s", want, raw)
		}
	}
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []FileRule
	for _, r := range inv.Files {
		if matchesAFileRule(rel, []FileRule{r}) {
			rows = append(rows, r)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows cover %s, want one", len(rows), rel)
	}
	if h := rows[0].Holds; !strings.Contains(h, "spec_event with the rule's id, the content digest of the spec file that declared it") {
		t.Errorf("the records row's Holds does not name the rule hit it holds: %q", h)
	}
}
