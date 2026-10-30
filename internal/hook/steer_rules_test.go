package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Spec rules as a repository file declares them: no-force forbids a forced
// git push, frozen-migrations protects db/migrations in every mode, and
// no-go-test forbids go test outright.
const (
	srNoForce   = `{"id": "no-force", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force", "-f"]}, "why": "history is shared", "hard_control": "branch protection"}`
	srFrozen    = `{"id": "frozen-migrations", "kind": "protect_path", "paths": ["db/migrations/**"], "why": "applied migrations are immutable", "hard_control": "CODEOWNERS"}`
	srNoGoTest  = `{"id": "no-go-test", "kind": "forbid_command", "command": {"argv": ["go", "test"]}, "why": "w", "hard_control": "h"}`
	srForceLine = "rashomon record: this 'git' call matched spec rule no-force."
	srPathLine  = "rashomon record: this call wrote 'db/migrations/001.sql', which spec rule frozen-migrations protects."
)

// srSpec is a spec of rules, one JSON object each.
func srSpec(rules ...string) string {
	doc := `{"rashomon_spec": 1, "rules": [`
	for i, r := range rules {
		if i > 0 {
			doc += ", "
		}
		doc += r
	}
	return doc + `]}`
}

// newSRFix is newST with its cwd at the root of a .git tree holding doc as
// the repository spec, which Discover reads as untrusted.
func newSRFix(t *testing.T, doc string) *stFix {
	t.Helper()
	s := newSpecTree(t)
	s.mkdir(".git")
	s.put(s.base, doc)
	f := newST(t)
	f.cwd = s.base
	return f
}

// TestSR1w_TheRulesTriggerIsABashOrFileToolCallThatLanded: the rules step
// runs on an execution that landed in the ordered stream, at either post
// event, of Bash or a tool that names the file it writes; never on a read,
// an MCP tool or a shell tool that runs no line.
func TestSR1w_TheRulesTriggerIsABashOrFileToolCallThatLanded(t *testing.T) {
	for _, c := range []struct {
		event string
		seq   int64
		tool  string
		want  bool
	}{
		{"PostToolUse", 7, "Bash", true},
		{FailureEvent, 7, "Bash", true},
		{"PostToolUse", 7, "Edit", true},
		{"PostToolUse", 7, "MultiEdit", true},
		{"PostToolUse", 7, "NotebookEdit", true},
		{FailureEvent, 7, "Write", true},
		{"PostToolUse", 0, "Bash", false},
		{FailureEvent, 0, "Write", false},
		{"", 7, "Bash", false},
		{"PreToolUse", 7, "Edit", false},
		{"PostToolUse", 7, "Read", false},
		{"PostToolUse", 7, "BashOutput", false},
		{"PostToolUse", 7, "mcp__db__run", false},
	} {
		if got := ruled(c.event, c.seq, c.tool); got != c.want {
			t.Errorf("ruled(%q, %d, %q) = %v, want %v", c.event, c.seq, c.tool, got, c.want)
		}
	}
}

// TestSR1x_APostRaisesItsRulesFromTheCallAlone: under an untrusted spec, a
// forced push raises platform-rule/command and an edit of a protected path
// integrity/frozen-path, each a flag at its own post, success or failure:
// the user's systemMessage carries the line, no model-visible key prints, and
// the steer line is the pattern's flag. A plain push, an edit elsewhere and a
// read of the path raise nothing and log nothing. The raise reads no run: it
// stands with the session's run made unreadable.
func TestSR1x_APostRaisesItsRulesFromTheCallAlone(t *testing.T) {
	for _, c := range []struct {
		name       string
		tool       string
		input      func(base string) map[string]any
		exit       int
		line       string
		pat        pattern.ID
		kind       string
		raisesNone bool
	}{
		{"a forced push", "Bash", func(string) map[string]any { return map[string]any{"command": "git push --force origin main"} },
			0, srForceLine, pattern.PlatformRule, "command", false},
		{"a forced push that failed", "Bash", func(string) map[string]any { return map[string]any{"command": "cd sub && git push -f"} },
			1, srForceLine, pattern.PlatformRule, "command", false},
		{"an edit of a migration", "Edit", func(base string) map[string]any {
			return map[string]any{"file_path": filepath.Join(base, "db", "migrations", "001.sql"), "old_string": "a", "new_string": "b"}
		}, 0, srPathLine, pattern.Integrity, "frozen-path", false},
		{"a relative write of a migration", "Write", func(string) map[string]any {
			return map[string]any{"file_path": "db/migrations/001.sql", "content": "x"}
		}, 0, srPathLine, pattern.Integrity, "frozen-path", false},
		{"a plain push", "Bash", func(string) map[string]any { return map[string]any{"command": "git push origin main"} },
			0, "", "", "", true},
		{"an edit elsewhere", "Edit", func(base string) map[string]any {
			return map[string]any{"file_path": filepath.Join(base, "db", "seed.sql"), "old_string": "a", "new_string": "b"}
		}, 0, "", "", "", true},
		{"a read of a migration", "Read", func(base string) map[string]any {
			return map[string]any{"file_path": filepath.Join(base, "db", "migrations", "001.sql")}
		}, 0, "", "", "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSRFix(t, srSpec(srNoForce, srFrozen))
			p := f.call("toolu_r", c.tool, c.input(f.cwd), c.exit)
			b := p.Steer(stEnv(0))
			lines := steerLines(t, f.st)
			if c.raisesNone {
				if b != nil || len(lines) != 0 {
					t.Fatalf("printed %q with %d steer lines, want nothing", b, len(lines))
				}
				// No trigger read the run, so the call ends before any
				// context is built.
				if _, _, ok := p.judge(stEnv(0)); ok {
					t.Error("judge is ok for a call that raised nothing and read no run")
				}
				return
			}
			var out map[string]any
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("printed %q: %v", b, err)
			}
			if msg, _ := out["systemMessage"].(string); msg != c.line {
				t.Errorf("systemMessage %q, want %q", msg, c.line)
			}
			if _, model := out["hookSpecificOutput"]; model {
				t.Errorf("a model-visible key printed for an untrusted rule: %s", b)
			}
			if len(lines) != 1 {
				t.Fatalf("%d steer lines, want 1", len(lines))
			}
			if l := lines[0]; l.Pattern != string(c.pat) || l.Kind != c.kind || l.Level != steer.LevelFlag.String() || l.ModelText != "" || l.UserText != c.line {
				t.Errorf("steer line %s/%s at %s, model %q, user %q; want %s/%s, flag, no model text, %q",
					l.Pattern, l.Kind, l.Level, l.ModelText, l.UserText, c.pat, c.kind, c.line)
			}

			// The same call again with the session's run unreadable: the
			// raise stands, since it never reads the run.
			g := newSRFix(t, srSpec(srNoForce, srFrozen))
			q := g.call("toolu_r", c.tool, c.input(g.cwd), c.exit)
			dir := g.st.RunDir(stSession)
			if err := os.Rename(dir, dir+".moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dir, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			_, view, ok := q.judge(stEnv(0))
			if !ok || len(view.Findings) != 1 || view.Findings[0].Pattern != c.pat {
				t.Errorf("judge over an unreadable run: ok %v, findings %+v; want one %s finding", ok, view.Findings, c.pat)
			}
		})
	}
}

// TestSR1y_TheStepAndTheRulesShareOnePost: a red go test under a spec that
// forbids go test is judged once by both: the step's findings as a spec
// without the rule gives them, then the rule's platform-rule/command. A run
// the step cannot read drops the step's findings alone: an edit of a
// protected path whose input names .claude/rashomon, so the step fires too,
// still prints its frozen-path flag with the session's records unreadable.
func TestSR1y_TheStepAndTheRulesShareOnePost(t *testing.T) {
	control := newSRFix(t, srSpec())
	_, base, ok := control.red("toolu_r").judge(stEnv(0))
	if !ok {
		t.Fatal("premise: the red call's step did not run")
	}
	f := newSRFix(t, srSpec(srNoGoTest))
	_, view, ok := f.red("toolu_r").judge(stEnv(0))
	if !ok || len(view.Findings) != len(base.Findings)+1 {
		t.Fatalf("judge: ok %v, %d findings; want the step's %d and the rule's", ok, len(view.Findings), len(base.Findings))
	}
	for i, b := range base.Findings {
		if view.Findings[i].Pattern != b.Pattern {
			t.Errorf("finding %d is %s, want the step's %s", i, view.Findings[i].Pattern, b.Pattern)
		}
	}
	if last := view.Findings[len(view.Findings)-1]; last.Pattern != pattern.PlatformRule {
		t.Errorf("the last finding is %s, want platform-rule", last.Pattern)
	}

	g := newSRFix(t, srSpec(srFrozen))
	edit := map[string]any{"file_path": filepath.Join(g.cwd, "db", "migrations", "001.sql"),
		"old_string": "a", "new_string": "see .claude/rashomon/spec.json"}
	p := g.call("toolu_m", "Edit", edit, 0)
	if !steps(p.event, p.landedSeq, p.landedRunner, p.landedStore) {
		t.Fatal("premise: the step does not fire for an input naming .claude/rashomon")
	}
	records := filepath.Join(g.st.RunDir(stSession), store.FileRecords)
	if err := os.Rename(records, records+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(records, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := g.st.ReadRunConsistent(stSession, SteerReadBudget); err == nil {
		t.Fatal("premise: the run still reads with a directory for its records")
	}
	b := p.Steer(stEnv(0))
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("a store fault silenced the frozen-path flag: printed %q (%v)", b, err)
	}
	if msg, _ := out["systemMessage"].(string); msg != srPathLine {
		t.Errorf("systemMessage %q, want %q", msg, srPathLine)
	}
}

// TestSR1z_ACallIsReadOnlyAsFarAsARuleReadsIt: the post decodes a Bash
// call's line only under a forbid_command rule, and reads a file tool's path
// and the project key only under a protect_path rule; with neither, or no
// spec, it reads nothing from the input.
func TestSR1z_ACallIsReadOnlyAsFarAsARuleReadsIt(t *testing.T) {
	push := map[string]any{"command": "git push --force"}
	for _, c := range []struct {
		name               string
		rules              []string
		tool               string
		command, path, key bool
	}{
		{"a line under a command rule", []string{srNoForce}, "Bash", true, false, false},
		{"a line under a path rule", []string{srFrozen}, "Bash", false, false, false},
		{"a path under a path rule", []string{srFrozen}, "Edit", false, true, true},
		{"a path under a command rule", []string{srNoForce}, "Edit", false, false, false},
		{"no rule", nil, "Bash", false, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSRFix(t, srSpec(c.rules...))
			input := push
			if c.tool == "Edit" {
				input = map[string]any{"file_path": filepath.Join(f.cwd, "db", "migrations", "001.sql"), "old_string": "a", "new_string": "b"}
			}
			p := f.call("toolu_r", c.tool, input, 0)
			p.sp = DiscoverSpec(p.cwd, "", "")
			got := p.ruleCall()
			if (got.Command != "") != c.command || (got.FilePath != "") != c.path || (got.Project != "") != c.key {
				t.Errorf("ruleCall read command %q, path %q, project %q; want command %v, path %v, project %v",
					got.Command, got.FilePath, got.Project, c.command, c.path, c.key)
			}
			if got.Seq != p.landedSeq || got.ToolName != c.tool || got.CWD != f.cwd {
				t.Errorf("ruleCall seq %d, tool %q, cwd %q; want %d, %q, %q", got.Seq, got.ToolName, got.CWD, p.landedSeq, c.tool, f.cwd)
			}
		})
	}
	f := newSRFix(t, srSpec(srNoForce))
	p := f.call("toolu_r", "Bash", push, 0)
	if got := p.ruleCall(); got.Command != "" {
		t.Errorf("with no spec discovered, ruleCall read %q", got.Command)
	}
}
