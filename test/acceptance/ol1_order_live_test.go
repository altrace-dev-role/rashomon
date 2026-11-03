package acceptance

// ORDER-LIVE1 (manager ruling 22:2x): in a live session an order rule's Stop
// row says met or unmet, never unknown where the record holds the answer,
// and a gap in the record reads unknown, never met. The post of every Bash
// call it parses for an order rule leaves one then_check record per rule:
// the rule's id, the spec file's digest, the landed seq, and whether the
// line runs the rule's Then (yes, no or maybe). No command text. The Stop
// reads "no" only from a "no" record; a call with none reads maybe.

import (
	"strings"
	"testing"
)

// ol1Checks is the session's then_check records.
func ol1Checks(e *env) []map[string]any { return e.recordsOfType("then_check") }

// ol1Row is the order rule's ledger state after a Stop in dir, "" with no
// row.
func ol1Row(e *env, dir string) string {
	e.t.Helper()
	e.dghStop(dir)
	if row := e.dghLedgerRow("spec:test-first"); row != nil {
		return row[4]
	}
	return ""
}

// TestOL1a_TheStopRowAgreesWithTheCallTimeNudge: a Then with no After raises
// the ordering-rule note at its post, and the Stop's row for the rule reads
// unmet; an After then a Then raises nothing, and the row reads met. Today
// the post raises (or not) as wanted, and the row reads unknown both times:
// the Stop sees no Bash line's command (Declaration.Command has no producer).
func TestOL1a_TheStopRowAgreesWithTheCallTimeNudge(t *testing.T) {
	e, dir := spr1Project(t, true)
	if out := e.dghCall(dir, "toolu_t", "Bash", spr1Push); !spr1Raised(out) {
		t.Fatalf("premise: a Then with no After: stdout %q, want the ordering-rule note", out)
	}
	if got := ol1Row(e, dir); got != "unmet" {
		t.Errorf("a Then the post raised as violated: Stop row %q, want unmet", got)
	}

	e2, dir2 := spr1Project(t, true)
	e2.dghCall(dir2, "toolu_a", "Bash", spr1Test)
	if out := e2.dghCall(dir2, "toolu_t", "Bash", spr1Push); spr1Raised(out) {
		t.Fatalf("premise: a Then after a qualifying After raised: %q", out)
	}
	if got := ol1Row(e2, dir2); got != "met" {
		t.Errorf("a Then after a qualifying After: Stop row %q, want met", got)
	}
}

// TestOL1b_EveryCheckedCallLeavesItsVerdict: each Bash call the post parses
// under the order rule leaves one then_check: no for a line that is not the
// Then (`go build`, and the After itself), yes for `git push`, maybe for a
// line that may run it (bash -c), with the rule's id, a landed seq, critical,
// and no command text.
func TestOL1b_EveryCheckedCallLeavesItsVerdict(t *testing.T) {
	e, dir := spr1Project(t, true)
	for _, c := range []struct {
		id      string
		command string
	}{
		{"toolu_1", "go build ./..."},
		{"toolu_2", "go test ./..."},
		{"toolu_3", "git push origin main"},
		{"toolu_4", "bash -c 'git push origin main'"},
	} {
		e.dghCall(dir, c.id, "Bash", map[string]any{"command": c.command})
	}
	checks := ol1Checks(e)
	var got []string
	for _, c := range checks {
		got = append(got, c["then"].(string))
		if c["rule_id"] != "spec:test-first" || c["critical"] != true || c["landed_seq"] == float64(0) || c["source_digest"] == "" {
			t.Errorf("then_check %v, want spec:test-first, critical, a landed seq and the spec's digest", c)
		}
		for k, v := range c {
			if s, ok := v.(string); ok && (strings.Contains(s, "git push") || strings.Contains(s, "go build")) {
				t.Errorf("then_check key %s holds command text: %q", k, s)
			}
		}
	}
	if strings.Join(got, ",") != "no,no,yes,maybe" {
		t.Errorf("then_check verdicts %v, want no,no,yes,maybe in call order", got)
	}
}

// TestOL1c_ACallWithNoVerdictReadsUnknown: a Bash call whose post never
// landed (its PreToolUse alone), then an After, then a qualifying Then: the
// first call has no then_check, so it may have been a Then with no After
// before it, and the row reads unknown, never met. Break: read a missing
// record as no (the row would read met). The lost call comes first: one
// after a qualifying After in its directory is met whatever it ran, so a
// missing record there decides nothing.
func TestOL1c_ACallWithNoVerdictReadsUnknown(t *testing.T) {
	e, dir := spr1Project(t, true)
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(t, "PreToolUse", "toolu_lost", "Bash", spr1Push, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.dghCall(dir, "toolu_a", "Bash", spr1Test)
	e.dghCall(dir, "toolu_t", "Bash", spr1Push)
	if got := ol1Row(e, dir); got != "unknown" {
		t.Errorf("a Bash call with no then_check before the After: row %q, want unknown", got)
	}
}

// TestOL1f_ACallWhosePostStepsKeepsItsVerdict: a call that names the store
// makes its post step (read the run and judge it, steps' inStore), and the
// stepped path keeps the call's then_check like the early return does: a
// "no" for a line that is not the Then, so a later After and Then read met.
// Break: drop the then_checks from the stepped view (the line would read
// maybe with no After before it, and the row unknown).
func TestOL1f_ACallWhosePostStepsKeepsItsVerdict(t *testing.T) {
	e, dir := spr1Project(t, true)
	e.dghCall(dir, "toolu_s", "Bash", map[string]any{"command": "cat .claude/rashomon/spec.json"})
	if checks := ol1Checks(e); len(checks) != 1 || checks[0]["then"] != "no" {
		t.Fatalf("premise: a stepped post's then_checks %v, want one no", checks)
	}
	e.dghCall(dir, "toolu_a", "Bash", spr1Test)
	e.dghCall(dir, "toolu_t", "Bash", spr1Push)
	if got := ol1Row(e, dir); got != "met" {
		t.Errorf("a stepped call that is no Then, then an After and a Then: row %q, want met", got)
	}
}
