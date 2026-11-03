package acceptance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// PREFLIGHT1b: the prompt path's steer line follows the record scope, a
// delivered note prints even when the Stop view errors, and attendance on the
// prompt path follows the surface.

const pf1bPath = "internal/verify/stop.go"

var pf1bHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// steerPins is the hermetic form of the steer fixtures' process environment
// (card PREFLIGHT1b, G): the variables the steer environment reads from the
// process are cleared (CI, AI_AGENT, CLAUDE_PROJECT_DIR) or pinned to off
// (RASHOMON_HOLDOUT, since an empty value leaves the draw live and each
// test's fresh store key makes the draw a random input). The four the
// harness's environ always sets are asserted, not reset. A test that wants
// one of them passes it in extra, which wins.
func (e *env) steerPins(extra ...string) []string {
	e.t.Helper()
	have := strings.Join(e.environ(), "\n")
	for _, name := range []string{"RASHOMON_MANAGED_SETTINGS_PATH=", "CLAUDE_CODE_ENTRYPOINT=", "CLAUDE_CONFIG_DIR=", "RASHOMON_HOME="} {
		if !strings.Contains("\n"+have, "\n"+name) {
			e.t.Fatalf("premise: the fixture does not set %s", strings.TrimSuffix(name, "="))
		}
	}
	return append([]string{"CI=", "AI_AGENT=", "CLAUDE_PROJECT_DIR=", "RASHOMON_HOLDOUT=off"}, extra...)
}

// preflightOut is the catch-up's stdout object.
type preflightOut struct {
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// prompt runs the UserPromptSubmit catch-up for a prompt from cwd and returns
// the result and the parsed object (zero when nothing printed).
func (e *env) prompt(cwd, text, promptID string, extraEnv ...string) (result, preflightOut) {
	e.t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id": testSession, "hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/transcripts/" + testSession + ".jsonl",
		"cwd": cwd, "permission_mode": "default", "prompt": text, "prompt_id": promptID,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	res := e.run(string(payload), e.steerPins(extraEnv...), append([]string{"recap", "pending"}, e.installArgs()...)...)
	var out preflightOut
	if res.stdout != "" {
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
			e.t.Fatalf("stdout %q is not one JSON object: %v", res.stdout, err)
		}
	}
	return res, out
}

// slotItems is a steer line's slots.paths.items.
func slotItems(l record) []string {
	slots, _ := l.fields["slots"].(map[string]any)
	paths, _ := slots["paths"].(map[string]any)
	return strs(paths["items"])
}

// storeText is every file under the store, by path.
func (e *env) storeText() map[string]string {
	e.t.Helper()
	all := map[string]string{}
	err := filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		all[p] = string(b)
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return all
}

func (e *env) preflightProject() {
	e.t.Helper()
	e.writeProject("go.mod", "CLAUDE.md", pf1bPath, "internal/verify/index.go")
}

// TestPF1w_V4LineKeepsNoPath: at record scope v4 a request naming a file
// writes a steer line with project "", every path slot a 64-hex keyed digest,
// and no path or cwd in its slots or text or in any file under the store; the
// note on stdout still names the path. The control, v5, writes the path in
// the slot.
func TestPF1w_V4LineKeepsNoPath(t *testing.T) {
	t.Run("v4", func(t *testing.T) {
		e := newSteerEnv(t)
		if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
			t.Fatal(err)
		}
		e.preflightProject()
		res, out := e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-2")
		if res.exitCode != 0 {
			t.Fatalf("exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if !strings.Contains(out.HookSpecificOutput.AdditionalContext, "'"+pf1bPath+"'") || !strings.Contains(out.SystemMessage, "'"+pf1bPath+"'") {
			t.Fatalf("the note on stdout does not name the path in both channels: %s", res.stdout)
		}
		lines := e.steerLines()
		if len(lines) != 1 {
			t.Fatalf("%d steer lines, want 1", len(lines))
		}
		l := lines[0]
		if l.str("project") != "" {
			t.Errorf("project %q at v4, want \"\"", l.str("project"))
		}
		items := slotItems(l)
		if len(items) != 1 || !pf1bHex64.MatchString(items[0]) {
			t.Errorf("path slot %q, want one 64-hex keyed digest", items)
		}
		if l.str("state") != "delivered" || !strings.Contains(l.str("model_text"), "paths named: 1\n") {
			t.Errorf("line state %q, model_text %q, want delivered with the count form", l.str("state"), l.str("model_text"))
		}
		cwds := []string{e.cwd}
		if r, err := filepath.EvalSymlinks(e.cwd); err == nil {
			cwds = append(cwds, r)
		}
		for where, text := range e.storeText() {
			for _, canary := range append([]string{pf1bPath, "verify/stop.go"}, cwds...) {
				// The session's coverage record keeps the cwd at every scope:
				// the SessionStart probe wrote it, as session metadata, before
				// any prompt; only the steer line is this card's.
				if strings.HasSuffix(where, "coverage.ndjson") && canary != pf1bPath && canary != "verify/stop.go" {
					continue
				}
				if strings.Contains(text, canary) {
					t.Errorf("%q is in %s at record scope v4", canary, where)
				}
			}
		}
	})
	t.Run("v5 control", func(t *testing.T) {
		e := newSteerEnv(t)
		e.preflightProject()
		_, out := e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-2")
		if !strings.Contains(out.HookSpecificOutput.AdditionalContext, "'"+pf1bPath+"'") {
			t.Fatalf("no note: %+v", out)
		}
		lines := e.steerLines()
		if len(lines) != 1 || len(slotItems(lines[0])) != 1 || slotItems(lines[0])[0] != pf1bPath {
			t.Fatalf("v5 lines %v, want the path in the slot", lines)
		}
		if lines[0].str("project") == "" {
			t.Error("project is \"\" at v5, want the project's key")
		}
	})
}

// TestPF1w_V4DeliveredSetDedupes: at v4, two prompts naming one path: the
// second note says it was given earlier, so the keyed digests compare.
func TestPF1w_V4DeliveredSetDedupes(t *testing.T) {
	e := newSteerEnv(t)
	if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
		t.Fatal(err)
	}
	e.preflightProject()
	if _, out := e.prompt(e.cwd, "look at "+pf1bPath, "prompt-2"); out.HookSpecificOutput.AdditionalContext == "" {
		t.Fatal("premise: the first prompt printed no note")
	}
	_, out := e.prompt(e.cwd, "look at "+pf1bPath+" and internal/verify/index.go", "prompt-3")
	got := out.HookSpecificOutput.AdditionalContext
	if !strings.Contains(got, "paths named: 'internal/verify/index.go'; 1 given earlier") {
		t.Errorf("the second note does not say the first path was given earlier:\n%s", got)
	}
}

// TestPF1y_StopViewErrorStillPrintsTheNote: a fault makes stopView fail after
// Route logged the note as delivered: stdout still carries the note, and the
// steer line and stdout agree. The control, no fault, prints the same note.
func TestPF1y_StopViewErrorStillPrintsTheNote(t *testing.T) {
	var notes [2]string
	for i, extra := range [][]string{nil, {"RASHOMON_FAULT=recap.stop_view:fail=1"}} {
		e := newSteerEnv(t)
		e.preflightProject()
		e.greenTurn(t, "prompt-1")
		_, out := e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-2", extra...)
		notes[i] = out.HookSpecificOutput.AdditionalContext
		if notes[i] == "" {
			t.Fatalf("run %d (%v): no note on stdout", i, extra)
		}
		lines := e.steerLines()
		if len(lines) == 0 || lines[len(lines)-1].str("state") != "delivered" || lines[len(lines)-1].str("model_text") != notes[i] {
			t.Errorf("run %d: the steer line and stdout disagree: stdout %q", i, notes[i])
		}
	}
	if notes[0] != notes[1] {
		t.Errorf("the faulted run's note differs from the control's:\n%s\nvs\n%s", notes[1], notes[0])
	}
}

// TestPF1z_AttendanceFollowsTheSurface: on the prompt path CI alone does not
// silence the note, and an SDK surface does; each with its positive control.
func TestPF1z_AttendanceFollowsTheSurface(t *testing.T) {
	runWith := func(t *testing.T, envOf func(e *env) []string) (preflightOut, record) {
		t.Helper()
		e := newSteerEnv(t)
		e.preflightProject()
		_, out := e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-2", envOf(e)...)
		lines := e.steerLines()
		if len(lines) != 1 {
			t.Fatalf("%d steer lines, want 1", len(lines))
		}
		return out, lines[0]
	}
	run := func(t *testing.T, extra ...string) (preflightOut, record) {
		t.Helper()
		return runWith(t, func(*env) []string { return extra })
	}
	t.Run("CI on an interactive surface", func(t *testing.T) {
		for _, extra := range [][]string{{"CI=1"}, nil} {
			out, l := run(t, extra...)
			if out.HookSpecificOutput.AdditionalContext == "" || l.str("state") != "delivered" || l.str("attendance") != "attended" {
				t.Errorf("env %v: note %q, state %q, attendance %q, want a delivered note, attended", extra, out.HookSpecificOutput.AdditionalContext, l.str("state"), l.str("attendance"))
			}
		}
	})
	t.Run("sdk-cli surface", func(t *testing.T) {
		out, l := run(t, "CLAUDE_CODE_ENTRYPOINT=sdk-cli")
		if out.HookSpecificOutput.AdditionalContext != "" {
			t.Errorf("an unattended surface printed a note: %q", out.HookSpecificOutput.AdditionalContext)
		}
		if l.str("attendance") != "unattended" || l.str("suppressed_by") != "no_mirror" {
			t.Errorf("line attendance %q, suppressed_by %q, want unattended, no_mirror", l.str("attendance"), l.str("suppressed_by"))
		}
	})
	// The cloud surface is Claude Code on the web (remote*) and the GitHub
	// Action, which exports CI: a person reads the session or the run, and
	// harness.Caps says a person is at the prompt there (CapPersonAtPrompt,
	// PREFLIGHT-CLOUD1). With no entrypoint but CLAUDE_PROJECT_DIR, which
	// Claude Code sets for every hook, the call is a presumed Claude Code,
	// attended as before.
	for _, c := range []struct {
		name  string
		envOf func(e *env) []string
	}{
		{"Claude Code on the web", func(*env) []string { return []string{"CLAUDE_CODE_ENTRYPOINT=remote"} }},
		{"the GitHub Action", func(*env) []string { return []string{"CLAUDE_CODE_ENTRYPOINT=claude-code-github-action", "CI=true"} }},
		{"no entrypoint", func(e *env) []string { return []string{"CLAUDE_CODE_ENTRYPOINT=", "CLAUDE_PROJECT_DIR=" + e.cwd} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, l := runWith(t, c.envOf)
			if out.HookSpecificOutput.AdditionalContext == "" || l.str("state") != "delivered" || l.str("attendance") != "attended" {
				t.Errorf("note %q, state %q, attendance %q, suppressed_by %q, want a delivered note, attended",
					out.HookSpecificOutput.AdditionalContext, l.str("state"), l.str("attendance"), l.str("suppressed_by"))
			}
		})
	}
	// Nothing Claude-shaped at all, no entrypoint and no CLAUDE_PROJECT_DIR:
	// Detect reads the Unknown harness, Caps claims no capability for it,
	// CapPersonAtPrompt included, so the note is withheld and the line says
	// why (a change from #170's surface rule, where it was attended).
	t.Run("no Claude Code evidence", func(t *testing.T) {
		out, l := run(t, "CLAUDE_CODE_ENTRYPOINT=")
		if out.HookSpecificOutput.AdditionalContext != "" {
			t.Errorf("the Unknown harness printed a note: %q", out.HookSpecificOutput.AdditionalContext)
		}
		if l.str("attendance") != "unattended" || l.str("suppressed_by") != "no_mirror" {
			t.Errorf("line attendance %q, suppressed_by %q, want unattended, no_mirror", l.str("attendance"), l.str("suppressed_by"))
		}
	})
}
