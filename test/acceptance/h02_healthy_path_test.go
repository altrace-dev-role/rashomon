package acceptance

import "testing"

// TestH2_HealthyPath exists so that H-1 cannot be satisfied by a program that
// does nothing. A shell script consisting of `exit 0` passes every exit-code
// assertion in this package; it fails this one.
func TestH2_HealthyPath(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	res := e.hook(defaultPayload().build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
	}
	assertNoTraceback(t, res)

	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want exactly 1", len(decls))
	}
	terms := e.terminals(testSession)
	if len(terms) != 1 {
		t.Fatalf("got %d terminal records, want exactly 1", len(terms))
	}
	if all := e.records(testSession); len(all) != 2 {
		t.Errorf("records hold %d entries, want exactly the 2 above", len(all))
	}

	if got := decls[0].str("tool_use_id"); got != testToolUseID {
		t.Errorf("declaration tool_use_id is %q, want %q", got, testToolUseID)
	}
	if got := terms[0].str("tool_use_id"); got != testToolUseID {
		t.Errorf("terminal tool_use_id is %q, want %q; a terminal that cannot be joined to its declaration closes nothing", got, testToolUseID)
	}
	if got := terms[0].str("outcome"); got != "ok" {
		t.Errorf("terminal outcome is %q, want \"ok\"", got)
	}
	if decls[0].fields["seq"] != float64(1) || terms[0].fields["seq"] != float64(2) {
		t.Errorf("seq is %v then %v, want 1 then 2", decls[0].fields["seq"], terms[0].fields["seq"])
	}

	assertCallCoverage(t, e, testSession, "verified", "")
}

// TestH2_UnwatchedRunIsNotVerified is the other side of the healthy path: a
// handler that runs without its entry installed as watch installs it must say
// so, not claim coverage it cannot have.
func TestH2_UnwatchedRunIsNotVerified(t *testing.T) {
	e := newEnv(t)
	e.mustHook(defaultPayload().build(t))
	assertCallCoverage(t, e, testSession, "unverified", "hook_entry_absent")
}

// TestH2_AbsentFieldsAreNullNotEmpty covers the distinction the pointer fields
// exist for. prompt_id is absent until the first input of a session and the
// agent fields only appear inside a subagent call, so an empty string would
// record that we saw a value and it was empty -- a different and false claim.
func TestH2_AbsentFieldsAreNullNotEmpty(t *testing.T) {
	e := newEnv(t)
	p := defaultPayload()
	p.PromptID, p.AgentID, p.AgentType = "", "", ""
	e.mustHook(p.build(t))

	decls := e.declarations(testSession)
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
	e := newEnv(t)
	p := defaultPayload()
	p.AgentID, p.AgentType = "agent-7", "Explore"
	e.mustHook(p.build(t))

	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	if got := decls[0].str("agent_id"); got != "agent-7" {
		t.Errorf("agent_id is %q, want \"agent-7\"", got)
	}
	if got := decls[0].str("agent_type"); got != "Explore" {
		t.Errorf("agent_type is %q, want \"Explore\"", got)
	}
}

// TestH2_DeclarationWithoutIDIsStillClosed: a payload with no tool_use_id is
// still a declaration, and a declaration that landed is closed whatever its
// id -- otherwise every such call reads as a killed handler.
func TestH2_DeclarationWithoutIDIsStillClosed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolUseID = ""
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	if got := e.terminals(testSession); len(got) != 1 {
		t.Fatalf("got %d terminal records, want 1", len(got))
	}
	rep := e.report(testSession)
	if len(rep.Declarations.Unterminated) != 0 {
		t.Errorf("a closed declaration reads as unterminated: %v", rep.Declarations.Unterminated)
	}
}
