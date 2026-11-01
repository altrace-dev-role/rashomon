package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specEditAt is an agent's Edit of the spec at path in dir: the file
// rewritten to doc, then the call's hook and post.
func (e *env) specEditAt(dir, path, id, doc string) {
	e.t.Helper()
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		e.t.Fatal(err)
	}
	e.dghCall(dir, id, "Edit", map[string]any{"file_path": path, "old_string": "a", "new_string": "b"})
}

// TestSTD1b_TheStopNamesASpecAnEditUnbound (card SPEC-TRUST-DROP1, d4): an
// agent edit of a trusted spec changes its digest, so its binding no longer
// holds and what it declares flags to the user only. The Stop line names the
// spec and the verb that trusts it again. Controls name neither: an edit
// that leaves the digest as it was (the same document reformatted); an edit
// of a spec never trusted (the brief names that one); the same change made
// outside the session, by no call of its own; and an edit after a session
// write under the store root, after which no trust holds for the rest of
// the session (D173), so the verb would not give it back.
// Break: build the Stop line without the bindings, or name a spec on any
// one of the controls.
func TestSTD1b_TheStopNamesASpecAnEditUnbound(t *testing.T) {
	const edited = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./internal/..."]}]}`
	const reformatted = "{\n  \"rashomon_spec\": 1,\n  \"gates\": [\n    {\"id\": \"unit\", \"kind\": \"test\", \"argv\": [\"go\", \"test\", \"./...\"]}\n  ]\n}\n"
	for _, c := range []struct {
		name    string
		trusted bool
		doc     string
		outside bool // the file changes with no call of the session's
		store   bool // a call of the session's writes under the store root first
		named   bool
	}{
		{"edited after a trust", true, edited, false, false, true},
		{"reformatted, the same digest", true, reformatted, false, false, false},
		{"never trusted", false, edited, false, false, false},
		{"edited outside the session", true, edited, true, false, false},
		{"edited after a store write", true, edited, false, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, dghSpec)
			if c.trusted {
				e.trust(path)
			}
			if c.store {
				e.dghCall(dir, "toolu_w", "Write", map[string]any{"file_path": filepath.Join(e.home, "note.txt"), "content": "x"})
			}
			if c.outside {
				if err := os.WriteFile(path, []byte(c.doc), 0o600); err != nil {
					t.Fatal(err)
				}
				e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			} else {
				e.specEditAt(dir, path, "toolu_s", c.doc)
			}
			e.dghCall(dir, "toolu_g", "Bash", dghRun)
			got := e.dghStop(dir)
			t.Logf("Stop line: %q", got)
			named := strings.Contains(got, ".claude/rashomon/spec.json") && strings.Contains(got, "rashomon spec trust")
			if named != c.named {
				t.Errorf("Stop line %q names the spec and the trust verb: %v, want %v", got, named, c.named)
			}
		})
	}
}
