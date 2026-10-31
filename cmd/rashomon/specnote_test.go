package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

const specNoteSpec = `{"rashomon_spec": 1, "rules": [{"id": "no-force", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force"]}, "why": "w", "hard_control": "h"}]}`

// specNoteTurn runs a UserPromptSubmit catch-up for prompt in a .git project
// with no spec, then makes a spec appear and runs the turn's forced push post
// (the same prompt id), and returns the post's stdout. It is the end to end of D210's note: the prompt's one discovery
// writes the note, and the turn's rules-only post honours it.
func specNoteTurn(t *testing.T, prompt string, sessionID string) (post string) {
	t.Helper()
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	_, installID := benchStarted(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	prompts, err := json.Marshal(map[string]any{
		"session_id": sessionID, "hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/t.jsonl",
		"cwd": project, "permission_mode": "default", "prompt": prompt, "prompt_id": payload.PromptID(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := recapTurn([]string{"pending", install.Marker, installID}, bytes.NewReader(prompts), &stdout, &stderr); err != nil {
		t.Fatalf("recap pending: %v", err)
	}
	spec := filepath.Join(project, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(spec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte(specNoteSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]any{"command": "git push --force origin main"})
	if err != nil {
		t.Fatal(err)
	}
	body := benchToolCallIn(t, project, "PostToolUse", "Bash", input, []byte(`{"stdout":"","stderr":"","interrupted":false,"isImage":false}`))
	stdout.Reset()
	if code := cmdPost([]string{install.Marker, installID}, bytes.NewReader(body), &stdout, &stderr); code != exitOK {
		t.Fatalf("post exited %d: %s", code, stderr.String())
	}
	return stdout.String()
}

// TestSR1af_EveryPromptNotesItsTurnsNoSpec (D210): a prompt that names no
// path or identifier still makes the one discovery that writes the turn's
// no-spec note, so its turn's rules-only posts skip theirs; a prompt that
// names one makes its note from the preflight's own discovery, not a second;
// and a prompt with no session writes nothing. The spec that appears after
// the prompt is the narrowing the note accepts: its rule is raised at the
// Stop, not at the call. Break: write the note only for a prompt that names a
// path, or discover twice.
func TestSR1af_EveryPromptNotesItsTurnsNoSpec(t *testing.T) {
	for _, c := range []struct {
		name, prompt, session string
		noted                 bool
	}{
		{"a prompt naming nothing", "Now add a test for a key that is too long.", benchSessionID, true},
		{"a prompt naming a path", "fix internal/verify/stop.go", benchSessionID, true},
		{"no session", "Now add a test for a key that is too long.", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			post := specNoteTurn(t, c.prompt, c.session)
			raised := strings.Contains(post, "no-force")
			if raised == c.noted {
				t.Errorf("the post printed %q; the rule raised = %v, want %v", post, raised, !c.noted)
			}
		})
	}
}

// TestSR1ag_ThePreflightsDiscoveryIsTheTurnsOnlyOne (D210): when the
// preflight discovered the spec for a prompt that names a path, it made the
// turn's note from that discovery, and noteTurnSpec adds none; one that did
// not discover leaves it to noteTurnSpec, which makes the one. Break: let
// noteTurnSpec discover again after the preflight did.
func TestSR1ag_ThePreflightsDiscoveryIsTheTurnsOnlyOne(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's own allocations are counted")
	}
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	root, _ := benchStarted(t)
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := &store.Run{}
	pp := pendingPayload{recapPayload: recapPayload{SessionID: benchSessionID, CWD: project}, PromptID: payload.PromptID(0)}
	fresh := testing.AllocsPerRun(20, func() { pp.specNoted = false; noteTurnSpec(st, run, &pp) })
	if fresh == 0 || !pp.specNoted {
		t.Fatalf("noteTurnSpec with no discovery behind it: %v allocations, specNoted %v; want it to discover and note", fresh, pp.specNoted)
	}
	if n := testing.AllocsPerRun(20, func() { pp.specNoted = true; noteTurnSpec(st, run, &pp) }); n != 0 {
		t.Errorf("noteTurnSpec after the preflight's discovery allocates %v times: it discovers a second time", n)
	}
	named := pendingPayload{recapPayload: recapPayload{SessionID: benchSessionID, CWD: project}, PromptID: payload.PromptID(0),
		Prompt: []byte("fix internal/verify/stop.go")}
	preflightNote(st, run, &named, harness.Identity{ID: harness.ClaudeCode}, time.Now())
	if !named.specNoted {
		t.Error("a prompt that names a path made its note from no discovery")
	}
}
