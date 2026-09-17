package acceptance

import "testing"

// TestH2_HealthyPath exists so that H-1 cannot be satisfied by a program that
// does nothing. A shell script consisting of `exit 0` passes every exit-code
// assertion in this package; it fails this one.
func TestH2_HealthyPath(t *testing.T) {
	home := t.TempDir()

	res := runHook(t, home, defaultPayload().build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
	}
	assertNoTraceback(t, res)

	recs := readRecords(t, home, testSession, "records.ndjson")

	decls := recordsOfType(recs, "declaration")
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want exactly 1", len(decls))
	}
	terms := recordsOfType(recs, "terminal")
	if len(terms) != 1 {
		t.Fatalf("got %d terminal records, want exactly 1", len(terms))
	}
	if len(recs) != 2 {
		t.Errorf("records file holds %d records, want exactly the 2 above", len(recs))
	}

	if got := decls[0].fields["tool_use_id"]; got != testToolUseID {
		t.Errorf("declaration tool_use_id is %v, want %q", got, testToolUseID)
	}
	if got := terms[0].fields["tool_use_id"]; got != testToolUseID {
		t.Errorf("terminal tool_use_id is %v, want %q; a terminal record that cannot be joined to its declaration closes nothing", got, testToolUseID)
	}
	if got := terms[0].fields["outcome"]; got != "ok" {
		t.Errorf("terminal outcome is %v, want \"ok\"", got)
	}

	assertCoverage(t, home, testSession, "verified", "")
}

// TestH2_AbsentFieldsAreNullNotEmpty covers the distinction the pointer fields
// exist for. prompt_id is absent until the first input of a session and the
// agent fields only appear inside a subagent call, so an empty string would
// record that we saw a value and it was empty -- a different and false claim.
func TestH2_AbsentFieldsAreNullNotEmpty(t *testing.T) {
	home := t.TempDir()

	p := defaultPayload()
	p.PromptID = ""
	p.AgentID = ""
	p.AgentType = ""

	if res := runHook(t, home, p.build(t)); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	decls := recordsOfType(readRecords(t, home, testSession, "records.ndjson"), "declaration")
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}

	for _, key := range []string{"prompt_id", "agent_id", "agent_type"} {
		v, present := decls[0].fields[key]
		if !present {
			t.Errorf("%s is missing from the record; absent has to be recorded, not omitted", key)
			continue
		}
		if v != nil {
			t.Errorf("%s is %#v, want null", key, v)
		}
	}
}

// TestH2_SubagentFieldsArePersisted covers the join key that subagent
// accounting needs and that nothing else supplies.
func TestH2_SubagentFieldsArePersisted(t *testing.T) {
	home := t.TempDir()

	p := defaultPayload()
	p.AgentID = "agent-7"
	p.AgentType = "Explore"

	if res := runHook(t, home, p.build(t)); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	decls := recordsOfType(readRecords(t, home, testSession, "records.ndjson"), "declaration")
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	if got := decls[0].fields["agent_id"]; got != "agent-7" {
		t.Errorf("agent_id is %v, want \"agent-7\"", got)
	}
	if got := decls[0].fields["agent_type"]; got != "Explore" {
		t.Errorf("agent_type is %v, want \"Explore\"", got)
	}
}
