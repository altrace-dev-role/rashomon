package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRV1_4t_TheControlTaskFileMakesANudge is the acceptance row for the
// measure1 harness's channel control. It reads the harness's own task file and
// prompt from scripts/ and runs them through the real UserPromptSubmit and Stop
// hooks on a project that holds nothing else, so an edit to either file that
// stops the control's note from reaching the model fails here first.
//
// With the task file at its path, the note lists the file's builtin rule and is
// a nudge the model is given. With no file there, the prompt's path names
// nothing and there is no note. With a file the spec refuses ({}), there is no
// rule, so the note lists only the path and is the user's flag: the nudge comes
// from the parsed rule, not from the prompt naming a file.
//
// Every steer line the two hooks leave is held to what measure1_check.py's
// control reads: surface cli and a prompt id on each.
func TestRV1_4t_TheControlTaskFileMakesANudge(t *testing.T) {
	task, err := os.ReadFile(filepath.Join(moduleRoot, "scripts", "measure1_control_task.json"))
	if err != nil {
		t.Fatal(err)
	}
	promptBytes, err := os.ReadFile(filepath.Join(moduleRoot, "scripts", "measure1_control_prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	const taskRel = ".claude/rashomon/task/current.json"
	if !strings.Contains(prompt, taskRel) {
		t.Fatalf("the control prompt does not name %s: %q", taskRel, prompt)
	}

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"task", task},
		{"absent", nil},
		{"invalid", []byte("{}")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.body != nil {
				p := filepath.Join(dir, filepath.FromSlash(taskRel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, tc.body, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			r, out := e.prompt(dir, prompt, "prompt-1")
			t.Logf("prompt exit %d stdout: %s", r.exitCode, r.stdout)
			if r.exitCode != 0 {
				t.Fatalf("prompt hook exit %d, stderr %s", r.exitCode, r.stderr)
			}
			s := e.recap(stopEvent("Stop", "ok", dir), e.steerPins()...)
			t.Logf("stop exit %d stdout: %s", s.exitCode, s.stdout)
			t.Logf("stop stderr: %s", s.stderr)
			if s.exitCode != 0 {
				t.Fatalf("stop hook exit %d, stderr %s", s.exitCode, s.stderr)
			}

			for i, l := range e.records(testSession) {
				t.Logf("record %d: %s", i, l.raw)
			}
			var steer []record
			for i, l := range e.steerLines() {
				t.Logf("steer.ndjson %d: %s", i, l.raw)
				if l.typ() == "steer" {
					steer = append(steer, l)
				}
			}
			for _, l := range steer {
				if l.str("surface") != "cli" || l.str("prompt_id") == "" {
					t.Errorf("a steer line has surface %q and prompt_id %q, not cli and an id: %s", l.str("surface"), l.str("prompt_id"), l.raw)
				}
			}
			at := func(level, delivery string) []record {
				var got []record
				for _, l := range steer {
					if l.str("level") == level && l.str("delivery") == delivery {
						got = append(got, l)
					}
				}
				return got
			}
			ctx := out.HookSpecificOutput.AdditionalContext

			switch tc.name {
			case "task":
				if !strings.Contains(ctx, "spec rules that apply: 'task-file' (builtin)") {
					t.Errorf("the model was not given the task file's rule: additionalContext %q", ctx)
				}
				got := at("nudge", "delivered")
				if len(got) == 0 || got[0].str("model_text") == "" {
					t.Fatalf("no delivered nudge with model text among %d steer lines", len(steer))
				}
				slots, _ := got[0].fields["slots"].(map[string]any)
				rules, _ := slots["rules"].(map[string]any)
				if ids, trust := fmt.Sprint(rules["ids"]), fmt.Sprint(rules["trust"]); ids != "[task-file]" || trust != "[builtin]" {
					t.Errorf("the nudge's rules slot is ids %s trust %s, not the one builtin task-file rule", ids, trust)
				}
			case "absent":
				if ctx != "" || out.SystemMessage != "" {
					t.Errorf("a note with no task file: additionalContext %q, systemMessage %q", ctx, out.SystemMessage)
				}
				if got := at("nudge", "delivered"); len(got) != 0 {
					t.Errorf("a delivered nudge with no task file: %s", got[0].raw)
				}
			case "invalid":
				if ctx != "" {
					t.Errorf("the model was given a note under a refused task file: %q", ctx)
				}
				if !strings.Contains(out.SystemMessage, taskRel) {
					t.Errorf("the flag does not name the path: systemMessage %q", out.SystemMessage)
				}
				if got := at("nudge", "delivered"); len(got) != 0 {
					t.Errorf("a delivered nudge under a refused task file: %s", got[0].raw)
				}
				if got := at("flag", "flag_only"); len(got) == 0 {
					t.Errorf("no flag line under a refused task file among %d steer lines", len(steer))
				}
			}
		})
	}
}
