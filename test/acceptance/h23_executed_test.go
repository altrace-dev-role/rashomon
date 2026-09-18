package acceptance

// H-23 — what EXECUTED, set against what was declared.
//
// Measured on Claude Code 2.1.258 (the PostToolUse experiment): PreToolUse
// carries tool_input as the model asked for it, and PostToolUse carries
// tool_input AFTER any hook rewrote it, with the same tool_use_id. So a
// rewriting hook is visible from these two records and from nowhere else -- the
// transcript keeps only the pre-rewrite input, which is exactly the record an
// agent with shell access could have shaped.
//
// Only the DIGESTS are compared and only the digests are stored. Neither input
// is persisted, so the report can say that the executed call differed from the
// declared one without being able to say how.

import (
	"strings"
	"testing"
)

func TestH23_RewrittenInputRecordsADifferentDigest(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Declared: one command. Executed: another. Same tool_use_id, which is what
	// makes them the same call rather than two calls.
	decl := defaultPayload()
	decl.ToolInput = map[string]any{"command": "echo ORIGINAL"}
	e.mustHook(decl.build(t))

	post := defaultPost()
	post.ToolInput = map[string]any{"command": "echo REWRITTEN"}
	e.mustPost(post.build(t))

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1", len(execs))
	}
	executed, _ := execs[0].fields["executed_digest"].(string)
	if executed == "" {
		t.Fatal("the execution record carries no executed_digest, so a rewriting hook " +
			"is invisible: the transcript keeps only the pre-rewrite input")
	}
	if !hex64.MatchString(executed) {
		t.Errorf("executed_digest = %q, want 64 hex characters", executed)
	}

	declared, _ := nested(e.declarations(testSession)[0], "shape.digest")
	if executed == declared {
		t.Error("the executed digest equals the declared digest although the commands " +
			"differ; the two sides are not being digested comparably")
	}

	rep := e.report(testSession)
	if rep.Destinations.ExecutedNotAsDeclared != 1 {
		t.Errorf("executed_not_as_declared = %d, want 1", rep.Destinations.ExecutedNotAsDeclared)
	}
}

// TestH23_UnchangedInputRecordsTheSameDigest is the negative fixture, and it is
// the case that runs on every ordinary call. A count that drifted above zero on
// unmodified sessions would make the line meaningless.
func TestH23_UnchangedInputRecordsTheSameDigest(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	same := map[string]any{"command": "git status --short"}
	decl := defaultPayload()
	decl.ToolInput = same
	e.mustHook(decl.build(t))

	post := defaultPost()
	post.ToolInput = same
	e.mustPost(post.build(t))

	executed, _ := e.executions(testSession)[0].fields["executed_digest"].(string)
	declared, _ := nested(e.declarations(testSession)[0], "shape.digest")
	if executed != declared {
		t.Errorf("executed_digest %q != declared %v for identical input; the two sides "+
			"must digest the same bytes the same way or every call looks rewritten",
			executed, declared)
	}

	rep := e.report(testSession)
	if rep.Destinations.ExecutedNotAsDeclared != 0 {
		t.Errorf("executed_not_as_declared = %d, want 0 on an unmodified session",
			rep.Destinations.ExecutedNotAsDeclared)
	}
}

// TestH23_MissingToolInputLeavesTheDigestEmpty covers a payload shape that
// carries no tool_input. The digest is then unknown, and unknown must not
// compare unequal: a null digest counted as a difference would report every
// such call as rewritten.
func TestH23_MissingToolInputLeavesTheDigestEmpty(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(`{"hook_event_name":"PostToolUse","session_id":"` + testSession +
		`","tool_name":"Bash","tool_use_id":"` + testToolUseID + `"}`)

	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("got %d execution records, want 1", len(execs))
	}
	if got := execs[0].fields["executed_digest"]; got != nil && got != "" {
		t.Errorf("executed_digest = %v, want null when the payload carried no tool_input", got)
	}

	rep := e.report(testSession)
	if rep.Destinations.ExecutedNotAsDeclared != 0 {
		t.Errorf("executed_not_as_declared = %d, want 0: an unknown digest is not a "+
			"difference, and counting it as one reports every such call as rewritten",
			rep.Destinations.ExecutedNotAsDeclared)
	}
}

// TestH23_ExecutedInputNeverReachesDisk is the content guarantee for the field
// this item adds. The post path now reads tool_input, which is the model's own
// text; only a digest may survive it.
func TestH23_ExecutedInputNeverReachesDisk(t *testing.T) {
	const canary = "CANARY-23f81a-executed-input"

	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	post := defaultPost()
	post.ToolInput = map[string]any{
		"command":     "echo " + canary,
		"description": canary,
		"nested":      map[string]any{"deep": canary},
	}
	res := e.post(post.build(t))
	if res.exitCode != 0 {
		t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.probe("end", testSession)

	for rel, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), canary) {
			t.Errorf("%s contains the executed input; only its digest may survive the "+
				"post path", rel)
		}
	}
	for name, out := range map[string]string{
		"post stdout":   res.stdout,
		"post stderr":   res.stderr,
		"report text":   e.run("", nil, "report", "--session", testSession).stdout,
		"report --json": e.run("", nil, "report", "--json", "--session", testSession).stdout,
	} {
		if strings.Contains(out, canary) {
			t.Errorf("%s contains the executed input", name)
		}
	}
}
