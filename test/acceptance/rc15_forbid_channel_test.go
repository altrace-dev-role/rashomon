package acceptance

import (
	"path/filepath"
	"strings"
	"testing"
)

// Card RECALL-15 (b): the forbid_command channel, proven on two failure
// ledger rows' own calls. B237 (eng-2, line 6768) ran an unrequested `git
// fetch -q` of all of origin; B245 (eng-3, line 3115) ran `rm -rf ./*` after
// a cd and sat three hours on the permission prompt. Each row's rule lived
// only as prose, so the L1 replay held no spec and printed nothing that names
// it. Declared as the ledger's trigger would declare it (LEDGER-SEED1's
// part), the rule must reach the agent at the call, in the post note, and
// the person at the Stop, by its id. The commands are the transcripts' word
// for word but for the cd target, which is the test's own directory or one
// beside it, so the run is the same on any machine.
const rc15Spec = `{"rashomon_spec": 1,
 "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}],
 "rules": [{"id": "no-fetch-all", "kind": "forbid_command", "command": {"argv": ["git", "fetch"]},
            "why": "a lane fetches only the ref it was given"},
           {"id": "no-rm-glob", "kind": "forbid_command", "command": {"argv": ["rm"], "flags": ["-rf", "-fr"]},
            "why": "a glob delete stalls an unattended lane on its prompt"}]}`

func TestRC15_ALedgerRowsRuleIsNamedAtTheCallAndAtTheStop(t *testing.T) {
	outside := t.TempDir() // on disk, under no .git
	for _, c := range []struct {
		name, rule string
		cmd        func(dir string) string
	}{
		{"B237 fetch", "no-fetch-all", func(dir string) string {
			return "cd " + dir + " && git fetch -q 2>/dev/null; git status -sb | head -2"
		}},
		{"B245 rm glob, cd not yet on disk", "no-rm-glob", func(dir string) string {
			art := filepath.Join(dir, "art183")
			return "mkdir -p " + art + " && cd " + art + " && rm -rf ./* && gh run download 41256731295 -R altrace-dev-role/rashomon -n bench-macos -D macos 2>&1 | tail -2"
		}},
		{"B245 rm glob, cd outside every repository", "no-rm-glob", func(string) string {
			return "mkdir -p " + outside + " && cd " + outside + " && rm -rf ./* && gh run download 41256731295 -R altrace-dev-role/rashomon -n bench-macos -D macos 2>&1 | tail -2"
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, rc15Spec)
			e.trust(path)
			out := e.dghCall(dir, "toolu_row", "Bash", map[string]any{"command": c.cmd(dir)})
			if !strings.Contains(out, "matched spec rule "+c.rule) {
				t.Errorf("post stdout %q, want the call to name rule %s", out, c.rule)
			}
			if got := e.dghStop(dir); !strings.Contains(got, c.rule) {
				t.Errorf("Stop line %q, want it to name rule %s", got, c.rule)
			}
		})
	}

	// Control: the same session's calls that match no rule name none.
	t.Run("control", func(t *testing.T) {
		e := newSteerEnv(t)
		dir, path := dghProject(t, rc15Spec)
		e.trust(path)
		out := e.dghCall(dir, "toolu_ok", "Bash", map[string]any{"command": "cd " + dir + " && git status -sb | head -2"})
		if strings.Contains(out, "matched spec rule") {
			t.Errorf("post stdout %q, want no rule named", out)
		}
		if got := e.dghStop(dir); strings.Contains(got, "no-fetch-all") || strings.Contains(got, "no-rm-glob") {
			t.Errorf("Stop line %q, want no rule named", got)
		}
	})
}
