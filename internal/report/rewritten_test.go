package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// B9 -- name the calls that executed differently, not just how many.
//
// The report said "executed differently from declared: 2" and stopped. That is
// the honest limit of a digest for the CONTENT of the change -- neither input
// is stored, so the report cannot say what differed -- but it is not the limit
// for WHICH CALL. The ids, the tool name and the derived shape are all in the
// store already, and withholding them made the reader's next question
// unanswerable from the artifact.
//
// What a row may carry is bounded by the same rule as everything else: no
// command strings, because none exist. Tool name, program, verb class, and the
// fact that two digests differ.

func declWithShape(id, tool, program, verb, digest string) store.Declaration {
	p := program
	d := store.Declaration{
		ToolUseID: id,
		ToolName:  tool,
		SessionID: "s1",
		Shape:     shape.Shape{VerbClass: verb, Digest: digest},
	}
	if program != "" {
		d.Shape.Program = &p
	}
	return d
}

// TestRewritten_NamesTheCall is the item: which one, not how many.
func TestRewritten_NamesTheCall(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			declWithShape("toolu_a", "Bash", "curl", "network", "digest-declared"),
			declWithShape("toolu_b", "Bash", "ls", "read", "same"),
		},
		Executions: []store.Execution{
			{ToolUseID: "toolu_a", ExecutedDigest: "digest-executed"},
			{ToolUseID: "toolu_b", ExecutedDigest: "same"},
		},
	}

	rows := rewrittenCalls(run)

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the one call whose digests differ: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.ToolUseID != "toolu_a" {
		t.Errorf("tool_use_id = %q, want toolu_a", r.ToolUseID)
	}
	if r.ToolName != "Bash" || r.Program != "curl" || r.VerbClass != "network" {
		t.Errorf("row = %+v; it should carry the tool name and the derived shape, which are "+
			"already in the store", r)
	}
}

// TestRewritten_CarriesNoInput. The row is assembled from records that hold no
// command string, and this asserts the obvious thing directly so that a future
// field cannot quietly add one.
func TestRewritten_CarriesNoInput(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			declWithShape("toolu_a", "Bash", "curl", "network", "declared"),
		},
		Executions: []store.Execution{{ToolUseID: "toolu_a", ExecutedDigest: "executed"}},
	}

	rows := rewrittenCalls(run)
	if len(rows) != 1 {
		t.Fatal("premise: one row expected")
	}
	// The digests themselves are not carried either: they are keyed, so they
	// are not content, but they are also not information a reader can act on,
	// and a row that prints two opaque hashes invites treating them as one.
	for _, s := range []string{"declared", "executed"} {
		if strings.Contains(rows[0].ToolUseID+rows[0].ToolName+rows[0].Program+rows[0].VerbClass, s) {
			t.Errorf("a digest leaked into the row: %+v", rows[0])
		}
	}
}

// TestRewritten_UnknownIsNotADifference preserves the existing rule. An
// execution with no digest -- a payload that carried no tool_input -- is
// unknown, and unknown is not a difference.
func TestRewritten_UnknownIsNotADifference(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			declWithShape("toolu_a", "Bash", "curl", "network", "declared"),
			declWithShape("toolu_b", "Bash", "ls", "read", "declared-b"),
		},
		Executions: []store.Execution{
			{ToolUseID: "toolu_a", ExecutedDigest: ""}, // no input in the payload
			{ToolUseID: "toolu_b", ExecutedDigest: ""}, // likewise
		},
	}

	if rows := rewrittenCalls(run); len(rows) != 0 {
		t.Errorf("got %d rows for executions with no digest; unknown is not a difference: %+v",
			len(rows), rows)
	}
}

// TestRewritten_WordingDependsOnTheTool is the constraint from the review of
// the rule-match spec, and it is a correctness issue rather than a style one.
//
// shape.Derive digests the WHOLE canonicalised tool_input for every tool except
// Bash. So for an Edit or a Write, a reworded `description` -- which changes
// nothing about what the call did to the file -- moves the digest and fires
// this line. Saying "command changed" there would be plainly false: there is no
// command, and what changed may be a field that does not affect the effect.
func TestRewritten_WordingDependsOnTheTool(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			declWithShape("toolu_bash", "Bash", "curl", "network", "d1"),
			declWithShape("toolu_edit", "Edit", "", "write", "d2"),
		},
		Executions: []store.Execution{
			{ToolUseID: "toolu_bash", ExecutedDigest: "x1"},
			{ToolUseID: "toolu_edit", ExecutedDigest: "x2"},
		},
	}

	var b bytes.Buffer
	writeRewritten(&b, rewrittenCalls(run))
	out := b.String()

	if !strings.Contains(out, "command changed") {
		t.Errorf("the Bash row does not say the command changed:\n%s", out)
	}
	if !strings.Contains(out, "input changed") {
		t.Errorf("the Edit row does not say the INPUT changed. The digest covers the whole "+
			"canonicalised input for every tool but Bash, so a reworded description moves "+
			"it -- calling that a changed command is false twice over:\n%s", out)
	}
	if strings.Count(out, "command changed") != 1 {
		t.Errorf("the non-Bash row was described as a command:\n%s", out)
	}
}

// TestRewritten_CountEqualsTheRows. The count and the list answer "how many"
// and "which" about the same predicate, and a reader who sees a count of 2
// above one row cannot tell which is wrong.
//
// Found by a mutation: adding one to the count broke nothing, because nothing
// compared them. The count is now DEFINED as the length of the list, and this
// is what keeps it that way rather than a second walk drifting from the first.
func TestRewritten_CountEqualsTheRows(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			declWithShape("toolu_a", "Bash", "curl", "network", "d1"),
			declWithShape("toolu_b", "Edit", "", "write", "d2"),
			declWithShape("toolu_c", "Bash", "ls", "read", "same"),
		},
		Executions: []store.Execution{
			{ToolUseID: "toolu_a", ExecutedDigest: "x1"},
			{ToolUseID: "toolu_b", ExecutedDigest: "x2"},
			{ToolUseID: "toolu_c", ExecutedDigest: "same"},
		},
	}

	d := buildDestinations(run, observed(), "", nil)

	if got, want := d.ExecutedNotAsDeclared, len(d.Rewritten); got != want {
		t.Errorf("executed_not_as_declared = %d and rewritten has %d rows. They answer one "+
			"question and must not be able to disagree.", got, want)
	}
	if len(d.Rewritten) != 2 {
		t.Errorf("rewritten = %+v, want the two calls whose digests differ", d.Rewritten)
	}
}
