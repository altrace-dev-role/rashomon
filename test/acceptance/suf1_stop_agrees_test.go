package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSUF1_TheStopAgreesWithTheBriefOnAnUntrustedGate (STOP-UNBOUND-FIRST,
// QA-2 e2e/live/PR-196.txt items 1 and 13): when the brief names a spec's
// gate, the Stop never says that no test or build is known: it says "'unit'
// does not count." and, on the next line, why. A spec never trusted, or one
// the person edited after a trust, is not trusted, and the line names the
// step that counts it; a spec the session edited after a trust gets the two
// lines that say so (SPEC-TRUST-DROP1) and no second step. Controls: a project with no spec
// still reads "no test or build is known for this project yet", and after a
// session write under the store root, when no trust holds for the rest of
// the session (D173), no line offers the trust verb.
// The brief is the session's, as QA-2's run had it: SessionStart's tests
// line, "go test ./..." (spec, untrusted).
// Break: leave the no-gate row's sentence as it was, or print the review
// line under an unbound spec's own lines.
func TestSUF1_TheStopAgreesWithTheBriefOnAnUntrustedGate(t *testing.T) {
	const edited = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./internal/..."]}]}`
	const review = "The spec is not trusted; review it, then run rashomon spec trust."
	for _, c := range []struct {
		name    string
		spec    bool
		trusted bool
		brief   string // the start brief's tests line reads so; "" asks nothing
		early   bool   // before runs ahead of the first prompt, as the person's edit
		before  func(e *env, dir, path string)
		want    []string
		not     []string
	}{
		{"never trusted", true, false, `"go test ./..." (spec, untrusted)`, false, nil,
			[]string{"1 file edited; 'unit' does not count.\n", review},
			[]string{"no test or build is known", "was edited after you trusted it"}},
		{"edited by the person after a trust", true, true, `"go test ./internal/..." (spec, untrusted)`, true,
			func(e *env, dir, path string) {
				if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
					e.t.Fatal(err)
				}
			},
			[]string{"1 file edited; 'unit' does not count.\n", review},
			[]string{"no test or build is known", "was edited after you trusted it"}},
		{"edited by the session after a trust", true, true, `"go test ./..." (spec, trusted)`, false,
			func(e *env, dir, path string) { e.specEditAt(dir, path, "toolu_s", edited) },
			[]string{"'unit' does not count.\n", "was edited after you trusted it, so it",
				"no longer counts; review the edit, then run rashomon spec trust."},
			[]string{"no test or build is known", review}},
		{"no spec", false, false, "", false, nil,
			[]string{"1 file edited; no test or build is known for this project yet."},
			[]string{"does not count", "rashomon spec trust"}},
		{"a store write ended the trust", true, true, "", false,
			func(e *env, dir, path string) {
				e.dghCall(dir, "toolu_w", "Write", map[string]any{"file_path": filepath.Join(e.home, "note.txt"), "content": "x"})
			},
			nil,
			[]string{"rashomon spec trust", review}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, dghSpec)
			if !c.spec {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if c.trusted {
				e.trust(path)
			}
			if c.early {
				c.before(e, dir, path)
			}
			if brief := e.sufStart(dir); c.brief != "" && !strings.Contains(brief, "tests: "+c.brief) {
				t.Fatalf("premise: the session's brief names the gate %q:\n%s", c.brief, brief)
			}
			e.prompt(dir, "fix internal/a.go", "prompt-1")
			if c.before != nil && !c.early {
				c.before(e, dir, path)
			}
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			got := e.dghStop(dir)
			t.Logf("Stop line: %q", got)
			gb1Columns(t, got)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("Stop line %q, want it to hold %q", got, w)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("Stop line %q, want it not to hold %q", got, n)
				}
			}
		})
	}
}

// sufStart is the session's start brief in dir, through probe start, as the
// person sees it.
func (e *env) sufStart(dir string) string {
	e.t.Helper()
	payload := fmt.Sprintf(`{"session_id":%q,"hook_event_name":"SessionStart","source":"startup","transcript_path":"/tmp/none.jsonl","cwd":%q}`,
		testSession, dir)
	res := e.run(payload, []string{stAgent, "CI="}, append([]string{"probe", "start"}, e.installArgs()...)...)
	var out struct {
		SystemMessage string `json:"systemMessage"`
	}
	if res.exitCode != 0 || json.Unmarshal([]byte(res.stdout), &out) != nil {
		e.t.Fatalf("probe start: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	return out.SystemMessage
}
