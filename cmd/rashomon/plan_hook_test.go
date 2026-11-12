package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const (
	planHookWord  = "zqxvwurble"
	planHookHeld  = "qwertyui/zorkheld.go"
	planHookFresh = "qwertyui/zorkfresh.go"
	planHookSpec  = `{"rashomon_spec": 1, "rules": [{"id": "r1", "kind": "protect_path", "paths": ["qwertyui/*.go"], "why": "w", "hard_control": "h"}]}`
)

// planHookProject is a .git project with an index holding one file under one
// unit, and the file the spec's rule covers.
func planHookProject(t *testing.T) string {
	t.Helper()
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".git", "qwertyui"} {
		if err := os.MkdirAll(filepath.Join(project, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, body := range map[string]string{planHookHeld: "package x\n"} {
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(p)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rev := graph.RevCommit("c1")
	s := graph.NewStore()
	fid := graph.FileID(planHookHeld)
	s.AssertNode(graph.Node{ID: fid, Kind: "file", Rev: rev})
	s.AssertEdge(graph.Edge{Src: graph.UnitID("go", "qwertyui"), Pred: graph.Contains, Dst: fid, Rev: rev})
	if err := graph.SaveFile(project, s, graph.Header{Repo: planRID, Rev: graph.HeaderRev{Commit: "c1"}}); err != nil {
		t.Fatal(err)
	}
	return project
}

// planSession is one install, consented, with a project; each call runs the
// real entry the harness calls.
type planSession struct {
	t       *testing.T
	root    string
	id      string
	project string
	// cwd is the hook's working directory, the project's when "".
	cwd string
}

func newPlanSession(t *testing.T, scope string) *planSession {
	t.Helper()
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("AI_AGENT", "claude-code_2-1-280_harness")
	t.Setenv("CI", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	root, id := benchStarted(t)
	if err := store.WriteRecordScopeMarker(root, scope); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		t.Fatal(err)
	}
	project := planHookProject(t)
	// The project's spec, bound in the store as `rashomon spec trust` binds it:
	// an unbound project spec is untrusted and raises no rule.
	path := filepath.Join(project, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(planHookSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	bound := spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digestOf(t, planHookSpec), BoundAtMS: 1}
	if err := hook.WriteSpecBindings(root, &spec.Bindings{Bindings: []spec.Binding{bound}}); err != nil {
		t.Fatal(err)
	}
	return &planSession{t: t, root: root, id: id, project: project}
}

// call runs one hook entry (pre or post) and returns its stdout.
func (s *planSession) call(post bool, tool, useID string, input map[string]any) string {
	s.t.Helper()
	in, _ := json.Marshal(input)
	event := "PreToolUse"
	var resp json.RawMessage
	if post {
		event, resp = "PostToolUse", json.RawMessage(`{"stdout":"","stderr":"","interrupted":false,"isImage":false}`)
	}
	head := newBenchHead(event)
	head.CWD = s.project
	if s.cwd != "" {
		head.CWD = s.cwd
	}
	body := benchJSON(s.t, benchToolPayload{benchHead: head, PromptID: "prompt-1", ToolName: tool, ToolInput: in, ToolUseID: useID, ToolResponse: resp})
	var out, errOut bytes.Buffer
	args := []string{install.Marker, s.id}
	code := 0
	if post {
		code = cmdPost(args, bytes.NewReader(body), &out, &errOut)
	} else {
		code = cmdHook(args, bytes.NewReader(body), &errOut)
	}
	if code != exitOK || errOut.Len() != 0 {
		s.t.Fatalf("%s %s exited %d, stderr %q", event, tool, code, errOut.String())
	}
	return out.String()
}

func (s *planSession) plan(post bool, useID, text string) string {
	return s.call(post, "ExitPlanMode", useID, map[string]any{"plan": text, "planFilePath": "/tmp/plan.md"})
}

func planHookText() string {
	return "# Plan\nChange " + planHookHeld + " and add " + planHookFresh + ". The " + planHookWord + " approach."
}

func (s *planSession) steerLines() []steer.Line {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	raw, _, _ := st.ReadSteer(benchSessionID)
	var out []steer.Line
	for _, b := range raw {
		var l steer.Line
		if err := json.Unmarshal(b, &l); err != nil {
			s.t.Fatal(err)
		}
		if l.Pattern == "plan-check" {
			out = append(out, l)
		}
	}
	return out
}

// TestPlan_HoldsNoWordOfThePlan: the real post entry on an ExitPlanMode whose
// plan names an existing path under a matching rule, a planned path under an
// existing directory and a sentence word, at record scope v4 and v5: every
// file under the store root holds none of the paths, the word, or any of
// their tokens of four letters or more. The control half: the delivered
// note does name the existing path, or the pin passes for nothing.
func TestPlan_HoldsNoWordOfThePlan(t *testing.T) {
	tok := regexp.MustCompile(`[A-Za-z]{4,}`)
	for _, scope := range []string{store.RecordScopeV4, store.RecordScopeV5} {
		t.Run(scope, func(t *testing.T) {
			s := newPlanSession(t, scope)
			out := s.plan(true, "toolu_plan_a", planHookText())
			if !strings.Contains(out, planHookHeld) || !strings.Contains(out, planHookFresh) || !strings.Contains(out, "r1") {
				t.Fatalf("control: the delivered note = %q, want it to name %q, %q and r1", out, planHookHeld, planHookFresh)
			}
			lines := s.steerLines()
			if len(lines) != 1 {
				t.Fatalf("%d plan-check steer lines, want 1", len(lines))
			}
			t.Logf("scope %s delivered: %s", scope, out)
			t.Logf("scope %s stored: model_text=%q slots=%v", scope, lines[0].ModelText, lines[0].Slots)
			needles := []string{planHookWord, planHookHeld, planHookFresh}
			for _, w := range []string{planHookWord, planHookHeld, planHookFresh} {
				needles = append(needles, tok.FindAllString(w, -1)...)
			}
			err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				b, _ := os.ReadFile(p)
				for _, n := range needles {
					if bytes.Contains(b, []byte(n)) {
						t.Errorf("%s holds %q", p, n)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPlan_DeliversOnPostOnly: the Pre of ExitPlanMode gives no plan line, the
// Post gives one, the same plan again none (its key), an Edit Post is
// untouched, a plan naming no covered path gives nothing; and a planned file
// and a held file under one pattern are one line, not two.
func TestPlan_DeliversOnPostOnly(t *testing.T) {
	s := newPlanSession(t, store.RecordScopeV5)
	if out := s.plan(false, "toolu_plan_a", planHookText()); out != "" || len(s.steerLines()) != 0 {
		t.Fatalf("Pre printed %q and left %d plan lines, want none", out, len(s.steerLines()))
	}
	if out := s.plan(true, "toolu_plan_a", planHookText()); !strings.Contains(out, planHookFresh) || len(s.steerLines()) != 1 {
		t.Fatalf("Post printed %q with %d plan lines, want the note and one line", out, len(s.steerLines()))
	}
	if out := s.plan(true, "toolu_plan_b", planHookText()); out != "" || len(s.steerLines()) != 1 {
		t.Errorf("the same plan again printed %q, %d lines, want none and 1", out, len(s.steerLines()))
	}
	if out := s.call(true, "Edit", "toolu_edit", map[string]any{"file_path": filepath.Join(s.project, "qwertyui", "zorkheld.go"), "old_string": "a", "new_string": "b"}); strings.Contains(out, "the plan names") {
		t.Errorf("an Edit Post printed a plan note: %q", out)
	}
	if out := s.plan(true, "toolu_plan_c", "# Plan\nNothing covered here, only prose."); out != "" || len(s.steerLines()) != 1 {
		t.Errorf("a plan naming no covered path printed %q, %d lines, want none and 1", out, len(s.steerLines()))
	}
	if n := len(s.steerLines()); n != 1 {
		t.Errorf("%d plan lines in all, want 1 (held and planned files are one line)", n)
	}
}

// TestPlan_RecordEventIsNonCritical: the plan_check record reads back through
// the store's reader with its counts, and says critical false, so a reader
// without the type counts it Ignored.
func TestPlan_RecordEventIsNonCritical(t *testing.T) {
	s := newPlanSession(t, store.RecordScopeV5)
	s.plan(true, "toolu_plan_a", planHookText())
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.ReadRun(benchSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.PlanChecks) != 1 || run.Skipped != 0 || run.Ignored != 0 {
		t.Fatalf("PlanChecks %d, Skipped %d, Ignored %d, want 1, 0, 0", len(run.PlanChecks), run.Skipped, run.Ignored)
	}
	pc := run.PlanChecks[0]
	if pc.Named != 2 || pc.Planned != 1 || pc.Matched != 2 || len(pc.RuleIDs) != 1 || pc.Critical {
		t.Errorf("record = %+v, want named 2, planned 1, matched 2, one rule, critical false", pc)
	}
}
