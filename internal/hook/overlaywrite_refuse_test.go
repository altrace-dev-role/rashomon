package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestKeptGoEdit_RefusedGitConfigCapturesNothing: a
// kept Go edit (scope v5, the marker set) in a repository whose git config
// is one byte over baseline.MaxGitFileBytes captures nothing through either
// writer: no session log under any id, so no group under the path form,
// which would fork the repository's log key. The hook says nothing of it:
// a refused git read stops capture silently. The control, the same tree
// with the config at the cap: the edit is kept in the origin's log, the
// one log there is. Break: let a refusal give the path form, and a log
// appears under it.
func TestKeptGoEdit_RefusedGitConfigCapturesNothing(t *testing.T) {
	text := "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n"
	for _, tc := range []struct {
		name string
		size int
	}{
		{"control, config at the cap", baseline.MaxGitFileBytes},
		{"config one byte over", baseline.MaxGitFileBytes + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOvFix(t, "1\n")
			config := filepath.Join(f.repo, ".git", "config")
			if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config, []byte(text+"#"+strings.Repeat("x", tc.size-len(text)-2)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
			f.pre("toolu_rgc", "Edit", in)
			f.post("toolu_rgc", "Edit", in)
			logs, err := filepath.Glob(filepath.Join(f.st.Root(), graph.StoreDir, "*.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.size > baseline.MaxGitFileBytes {
				if len(logs) != 0 {
					t.Errorf("config one byte over: session logs %v; want none, under no id", logs)
				}
				return
			}
			f.rid = "host.example/org/repo"
			groups, _, exists := f.read()
			if len(logs) != 1 || logs[0] != f.logPath() || !exists || len(groups) == 0 {
				t.Errorf("control: session logs %v, origin's log exists %v with %d groups; want the origin's log alone, with the edit kept", logs, exists, len(groups))
			}
		})
	}
}
