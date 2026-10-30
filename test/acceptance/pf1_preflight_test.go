package acceptance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PF1d -- the request is read in memory and never lands.
//
// The UserPromptSubmit catch-up reads the new prompt to resolve the paths it
// names against the project and delivers the task-preflight note. The words
// of the request that are not paths the project holds survive nowhere: not
// in any file under the store, not in report --json, not on either hook
// stream. The matched path does survive, in the note and its steer line.
//
// Break: store the prompt (or any token of it) in a steer line, a record or
// a log, and a sentinel is found.

const (
	pf1Sentinel1 = "zebracornflake"
	pf1Sentinel2 = "QuokkaMarmalade"
)

// writeProject makes e.cwd a small Go project with one named file.
func (e *env) writeProject(files ...string) {
	e.t.Helper()
	for _, f := range files {
		p := filepath.Join(e.cwd, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			e.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
}

func TestPF1d_PromptWordsNeverLand(t *testing.T) {
	e := newSteerEnv(t)
	e.writeProject("go.mod", "CLAUDE.md", "internal/verify/stop.go")

	prompt := "fix the " + pf1Sentinel1 + " bug in internal/verify/stop.go and rename " + pf1Sentinel2 + " in quokka/" + pf1Sentinel1 + ".go"
	payload, err := json.Marshal(map[string]any{
		"session_id": testSession, "hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/transcripts/" + testSession + ".jsonl",
		"cwd": e.cwd, "permission_mode": "default", "prompt": prompt, "prompt_id": "prompt-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	res := e.run(string(payload), []string{"RASHOMON_HOLDOUT=off"}, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 {
		t.Fatalf("recap pending: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var out struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if res.stdout == "" || json.Unmarshal([]byte(res.stdout), &out) != nil {
		t.Fatalf("stdout = %q, want the preflight note as one JSON object", res.stdout)
	}
	const named = "'internal/verify/stop.go'"
	if !strings.Contains(out.HookSpecificOutput.AdditionalContext, named) || !strings.Contains(out.SystemMessage, named) {
		t.Errorf("the note does not name the matched file in both channels: %s", res.stdout)
	}

	report := e.run("", nil, "report", "--json", "--session", testSession)
	surfaces := map[string]string{"stdout": res.stdout, "stderr": res.stderr, "report --json": report.stdout + report.stderr}
	steerHasPath := false
	err = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		surfaces[p] = string(b)
		if strings.HasSuffix(p, "steer.ndjson") && strings.Contains(string(b), "internal/verify/stop.go") {
			steerHasPath = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for where, text := range surfaces {
		for _, w := range []string{pf1Sentinel1, pf1Sentinel2, strings.ToLower(pf1Sentinel2), "quokka"} {
			if strings.Contains(text, w) {
				t.Errorf("%q from the request is in %s", w, where)
			}
		}
	}
	if !steerHasPath {
		t.Error("no steer line under the store names the matched file")
	}
}
