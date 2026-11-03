package acceptance

// SRS1d4 (card SPEC-RULES-STOP1): an agent edit of a trusted repository
// spec drops the spec's trust, so its rules bind nothing at Stop. The record
// still reads not green: the edit is a write under .claude/rashomon, which
// integrity/store-touched raises and leaves unresolved. Pinned here so that
// the gap read on c7fce266 (none) stays closed. The rule is require_evidence
// at done over a declared gate never run: it is a violation at Stop with no
// command capture. The lint gate is not required, so the control is red only
// by bindingViolations; the edit variant's go test green is the learned
// family, every family required with no index (D38).

import (
	"os"
	"strings"
	"testing"
)

const srs1d4Spec = `{"rashomon_spec": 1,
 "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]},
           {"id": "lint", "kind": "lint", "argv": ["golangci-lint", "run"], "required": false}],
 "rules": [{"id": "lint-before-done", "kind": "require_evidence", "at": ["done"], "evidence": ["lint"],
            "why": "lint findings block review", "hard_control": "CI lint job"}]}`

// srs1d4Edited is srs1d4Spec with one semantic change, the rule's why: a new
// content digest, so the binding no longer holds.
var srs1d4Edited = strings.Replace(srs1d4Spec, "lint findings block review", "lint is advisory", 1)

// dghSpecEdit is the agent's edit of the spec at path to doc: the file
// written as the tool writes it, then the call's hook and post. It returns
// the post's stdout and does not fail on output, which the caller reads.
func (e *env) dghSpecEdit(dir, path, id, doc string) string {
	e.t.Helper()
	old, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		e.t.Fatal(err)
	}
	input := map[string]any{"file_path": path, "old_string": string(old), "new_string": doc}
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Edit", input, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	more := map[string]any{"cwd": dir, "tool_response": map[string]any{"filePath": path}}
	res := e.post(stPayload(e.t, "PostToolUse", id, "Edit", input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// TestSRS1d4_AnAgentSpecEditStaysARecordFinding: control, a trusted spec
// whose rule is unmet at done: the Stop line is not verified. Edit, the same
// session with the agent editing the spec before the last green run: the
// post raises store-touched, and the Stop line is not verified, counting it.
func TestSRS1d4_AnAgentSpecEditStaysARecordFinding(t *testing.T) {
	t.Run("control", func(t *testing.T) {
		e := newSteerEnv(t)
		dir, path := dghProject(t, srs1d4Spec)
		e.trust(path)
		e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
		e.dghCall(dir, "toolu_g", "Bash", dghRun)
		got := e.dghStop(dir)
		t.Logf("control Stop line: %q", got)
		if got == "" || strings.Contains(got, "verified - ") {
			t.Errorf("control Stop line %q, want a line that is not verified (lint-before-done violated)", got)
		}
	})
	t.Run("edit", func(t *testing.T) {
		e := newSteerEnv(t)
		dir, path := dghProject(t, srs1d4Spec)
		e.trust(path)
		e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
		if out := e.dghSpecEdit(dir, path, "toolu_s", srs1d4Edited); !strings.Contains(out, "under .claude/rashomon") {
			t.Errorf("spec edit post stdout %q, want the store-touched line", out)
		}
		e.dghCall(dir, "toolu_g", "Bash", dghRun)
		got := e.dghStop(dir)
		t.Logf("edit Stop line: %q", got)
		if strings.Contains(got, "verified - ") || !strings.Contains(got, "1 record finding(s)") {
			t.Errorf("edit Stop line %q, want not verified with 1 record finding(s)", got)
		}
	})
}

// srs1mSpec protects internal/gen and declares no gate (QA-2's beat 4d).
const srs1mSpec = `{"rashomon_spec": 1,
 "rules": [{"id": "gen-owned", "kind": "protect_path", "paths": ["internal/gen/**"],
            "why": "generated code is owned by the generator", "hard_control": "CODEOWNERS"}]}`

// TestSRS1m_AProtectPathViolationIsNamedAtStop: a trusted protect_path rule
// fires at the call and is a violation at Stop; the Stop line counts it, in
// a project where no test or build is known.
func TestSRS1m_AProtectPathViolationIsNamedAtStop(t *testing.T) {
	e := newSteerEnv(t)
	dir, path := dghProject(t, srs1mSpec)
	e.trust(path)
	gen := map[string]any{"file_path": dir + "/internal/gen/x.go", "old_string": "a", "new_string": "b"}
	if out := e.dghCall(dir, "toolu_e", "Edit", gen); !strings.Contains(out, "spec rule gen-owned protects") {
		t.Errorf("post stdout %q, want the frozen-path line", out)
	}
	got := e.dghStop(dir)
	t.Logf("Stop line: %q", got)
	if !strings.Contains(got, "1 record finding(s)") {
		t.Errorf("Stop line %q, want it to count the violation: 1 record finding(s)", got)
	}
}
