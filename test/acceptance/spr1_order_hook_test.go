package acceptance

// SPEC-RULES1 1b, the hook's half (#216 ruling 1): at the After call's post,
// under a spec that declares an order rule, a call that matches the rule's
// After pattern leaves an after_hit record, as a forbid_command match leaves
// a rule_hit. The order judge reads those records and never an After call's
// command line: a Then call with no qualifying earlier hit is a violation; an
// After that left no hit, or whose line may not be the After, is undecided.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const spr1Spec = `{"rashomon_spec": 1,
 "rules": [{"id": "test-first", "kind": "order", "then": {"argv": ["git", "push"]}, "after": {"argv": ["go", "test"]},
            "invalidated_by": ["**/*.go"], "why": "push only tested code", "hard_control": "CI"}]}`

var (
	spr1Test = map[string]any{"command": "go test ./..."}
	spr1Push = map[string]any{"command": "git push origin main"}
)

// spr1Project is a project whose spec declares the order rule, trusted when
// trusted.
func spr1Project(t *testing.T, trusted bool) (*env, string) {
	t.Helper()
	e := newSteerEnv(t)
	dir, path := dghProject(t, spr1Spec)
	if trusted {
		e.trust(path)
	}
	return e, dir
}

// spr1Raised reports a post's stdout that carries the ordering-rule line.
func spr1Raised(out string) bool {
	return strings.Contains(out, "ran before the earlier step spec rule test-first requires")
}

// spr1Hits is the session's after_hit records.
func spr1Hits(e *env) []map[string]any { return e.recordsOfType("after_hit") }

// TestSPR1a_AnAfterCallLeavesAHitAndAThenWithOneIsMet: the After's post
// writes one after_hit: the rule's id, exit 0, foreground, the call's
// directory digest and no command text. A later Then raises nothing. (The
// Stop's row reads unknown live: this build records no command line, so the
// Stop cannot see the Then; the judge's Stop reading is the verify tests'.)
func TestSPR1a_AnAfterCallLeavesAHitAndAThenWithOneIsMet(t *testing.T) {
	e, dir := spr1Project(t, true)
	e.dghCall(dir, "toolu_a", "Bash", spr1Test)
	hits := spr1Hits(e)
	if len(hits) != 1 {
		t.Fatalf("after_hit records %v, want one", hits)
	}
	h := hits[0]
	if h["rule_id"] != "spec:test-first" || h["exit_code"] != float64(0) || h["foreground"] != true || h["maybe"] != false ||
		h["critical"] != true || h["event"] != "PostToolUse" || h["cwd_digest"] == "" || h["cwd_digest"] == nil || h["landed_seq"] == float64(0) {
		t.Errorf("after_hit %v, want spec:test-first, exit 0, foreground, a directory digest, not maybe, critical", h)
	}
	for k, v := range h {
		if s, ok := v.(string); ok && strings.Contains(s, "go test") {
			t.Errorf("after_hit key %s holds command text: %q", k, s)
		}
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); spr1Raised(out) {
		t.Errorf("a Then after a qualifying hit raised: %q", out)
	}
}

// TestSPR1b_AThenWithNoHitRaises: no After call, so no hit: the Then's post
// raises the ordering-rule note. A call that is neither leaves no hit.
func TestSPR1b_AThenWithNoHitRaises(t *testing.T) {
	e, dir := spr1Project(t, true)
	e.dghCall(dir, "toolu_b", "Bash", map[string]any{"command": "go build ./..."})
	out := e.dghCall(dir, "toolu_t", "Bash", spr1Push)
	if !spr1Raised(out) {
		t.Errorf("a Then with no hit: stdout %q, want the ordering-rule note", out)
	}
	if hits := spr1Hits(e); len(hits) != 0 {
		t.Errorf("after_hit records %v, want none", hits)
	}
}

// TestSPR1c_AFailedAfterIsRecordedAndDoesNotQualify: an After that exited 1
// writes its hit at PostToolUseFailure with exit_code 1, and the Then after
// it raises.
func TestSPR1c_AFailedAfterIsRecordedAndDoesNotQualify(t *testing.T) {
	e, dir := spr1Project(t, true)
	e.dghFailedCall(dir, "toolu_a", "Bash", spr1Test, 1)
	hits := spr1Hits(e)
	if len(hits) != 1 || hits[0]["exit_code"] != float64(1) || hits[0]["event"] != "PostToolUseFailure" || hits[0]["foreground"] != true {
		t.Fatalf("after_hit records %v, want one at PostToolUseFailure with exit 1", hits)
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); !spr1Raised(out) {
		t.Errorf("a Then after a failed After: stdout %q, want the ordering-rule note", out)
	}
}

// TestSPR1d_AnAfterInAnotherDirectoryDoesNotQualify: the hit carries the
// After's directory digest, and a Then in another directory is not met by it.
func TestSPR1d_AnAfterInAnotherDirectoryDoesNotQualify(t *testing.T) {
	e, dir := spr1Project(t, true)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	e.dghCall(sub, "toolu_a", "Bash", spr1Test)
	if hits := spr1Hits(e); len(hits) != 1 {
		t.Fatalf("after_hit records %v, want one", hits)
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); !spr1Raised(out) {
		t.Errorf("a Then in another directory: stdout %q, want the ordering-rule note", out)
	}
	// In the After's own directory it is met.
	if out := e.dghCall(sub, "toolu_u", "Bash", spr1Push); spr1Raised(out) {
		t.Errorf("a Then in the After's directory raised: %q", out)
	}
}

// TestSPR1e_AMaybeAfterIsUndecided: an After the line only may be (bash -c)
// leaves a maybe hit, and the Then after it is undecided, never violated: no
// note. (An After whose hit was lost while its line is recorded is the
// verify tests' case: this build records no line.)
func TestSPR1e_AMaybeAfterIsUndecided(t *testing.T) {
	e, dir := spr1Project(t, true)
	e.dghCall(dir, "toolu_a", "Bash", map[string]any{"command": "bash -c 'go test ./...'"})
	if hits := spr1Hits(e); len(hits) != 1 || hits[0]["maybe"] != true {
		t.Fatalf("after_hit records %v, want one maybe", hits)
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); spr1Raised(out) {
		t.Errorf("an undecided Then raised: %q", out)
	}
}

// TestSPR1f_AnUntrustedSpecFlagsTheUserAndOwesNoRow: an untrusted spec's
// Then with no hit is a flag for the user alone (no note reaches the agent)
// and owes no ledger row; a hit is still recorded, so the flag judges from
// the record as the nudge does.
func TestSPR1f_AnUntrustedSpecFlagsTheUserAndOwesNoRow(t *testing.T) {
	e, dir := spr1Project(t, false)
	e.dghCall(dir, "toolu_a", "Bash", spr1Test)
	if hits := spr1Hits(e); len(hits) != 1 {
		t.Fatalf("after_hit records %v, want the one hit", hits)
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); spr1Raised(out) {
		t.Errorf("an untrusted Then after a hit raised: %q", out)
	}
	e2, dir2 := spr1Project(t, false)
	out := e2.dghCall(dir2, "toolu_t", "Bash", spr1Push)
	if strings.Contains(out, "additionalContext") || !strings.Contains(out, "systemMessage") || !spr1Raised(out) {
		t.Errorf("an untrusted Then with no hit: stdout %q, want the flag in systemMessage alone", out)
	}
	e2.dghStop(dir2)
	if row := e2.dghLedgerRow("spec:test-first"); row != nil {
		t.Errorf("ledger row %v for an untrusted rule, want none", row)
	}
}

// TestSPR1g_ABackgroundedAfterIsRecordedNotForeground: a go test whose
// PostToolUse fired while the command ran in the background (its response
// carries backgroundTaskId) leaves a hit that is not foreground, and the Then
// after it raises: the launch's exit is not the command's.
func TestSPR1g_ABackgroundedAfterIsRecordedNotForeground(t *testing.T) {
	e, dir := spr1Project(t, true)
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(t, "PreToolUse", "toolu_a", "Bash", spr1Test, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	more := map[string]any{"cwd": dir, "tool_response": map[string]any{"stdout": "", "stderr": "", "backgroundTaskId": "bg1"}}
	if res := e.post(stPayload(t, "PostToolUse", "toolu_a", "Bash", spr1Test, more), envs...); res.exitCode != 0 {
		t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	hits := spr1Hits(e)
	if len(hits) != 1 || hits[0]["foreground"] != false || hits[0]["exit_code"] != float64(0) {
		t.Fatalf("after_hit records %v, want one with exit 0 and foreground false", hits)
	}
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); !spr1Raised(out) {
		t.Errorf("a Then after a backgrounded After: stdout %q, want the ordering-rule note", out)
	}
}
