package digest

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func testRun(seq int64, id, promptID, digest string) store.Declaration {
	d := decl(seq, id, "Bash", promptID, "/t.jsonl", 100+seq)
	d.Shape = shape.Shape{VerbClass: shape.VerbTest, Digest: digest}
	return d
}

func testEdit(seq int64, id, promptID, label string) store.Declaration {
	d := decl(seq, id, "Edit", promptID, "/t.jsonl", 100+seq)
	d.Shape = shape.Shape{VerbClass: shape.VerbWrite, Digest: "e-" + id}
	d.FileLabel = &label
	return d
}

func outcome(id, o string) store.Execution {
	return store.Execution{ToolUseID: id, Outcome: o}
}

// TestBuild_TestBendingIsTurnScoped: the digest finds the patterns among THIS
// turn's calls, a subagent's included, and not across turns. Break: run the
// detection over the whole session, and a flaky pair from an earlier turn is
// raised again on every later one; key the turn by transcript as well, and
// the subagent's passing run is lost.
func TestBuild_TestBendingIsTurnScoped(t *testing.T) {
	sub := testRun(6, "b3", "p2", "d")
	sub.TranscriptPath = "/sub.jsonl"
	a := "agent-x"
	sub.AgentID = &a
	run := &store.Run{
		Declarations: []store.Declaration{
			// Turn 1: the same command passes and fails, nothing edited.
			testRun(1, "a1", "p1", "f"), testRun(2, "a2", "p1", "f"),
			// Turn 2: fails, only a test file is edited, a subagent's run
			// of the same command passes.
			testRun(4, "b1", "p2", "d"), testEdit(5, "b2", "p2", shape.LabelTestFile), sub,
		},
		Executions: []store.Execution{
			outcome("a1", store.ExecOK), outcome("a2", store.ExecFailed),
			outcome("b1", store.ExecFailed), outcome("b2", store.ExecOK), outcome("b3", store.ExecOK),
		},
		Coverage: []store.Coverage{startCoverage("inst-1")},
	}

	d1 := build(run, nil, "p1", "", time.Now())
	if want := []report.FlakyPair{{Seqs: report.SeqPair{1, 2}}}; !reflect.DeepEqual(d1.TestBending.Flaky, want) {
		t.Errorf("turn 1 flaky = %v, want %v", d1.TestBending.Flaky, want)
	}
	if len(d1.TestBending.TestsOnlyThenGreen) != 0 {
		t.Errorf("turn 1 carries turn 2's pattern: %v", d1.TestBending.TestsOnlyThenGreen)
	}

	d2 := build(run, nil, "p2", "", time.Now())
	if want := []report.SeqPair{{4, 6}}; !reflect.DeepEqual(d2.TestBending.TestsOnlyThenGreen, want) {
		t.Errorf("turn 2 tests_only_then_green = %v, want %v", d2.TestBending.TestsOnlyThenGreen, want)
	}
	if len(d2.TestBending.Flaky) != 0 {
		t.Errorf("turn 2 carries turn 1's pattern: %v", d2.TestBending.Flaky)
	}
}

// TestBuild_TestBendingListsAreNeverNull: an empty turn, and a store with
// nothing in it, both marshal two empty lists.
func TestBuild_TestBendingListsAreNeverNull(t *testing.T) {
	for name, d := range map[string]*Digest{
		"built": build(&store.Run{}, nil, "", "", time.Now()),
		"empty": Empty(time.Now(), "s", "p"),
	} {
		b, err := json.Marshal(d.TestBending)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(b); got != `{"tests_only_then_green":[],"tests_only_then_green_omitted":0,"flaky":[],"flaky_omitted":0}` {
			t.Errorf("%s: test_bending = %s", name, got)
		}
	}
}

// TestTruncate_TestBendingIsCutLastAndCounted: the two lists are cut after
// every other growable field, from the tail, with an omitted count that adds
// back to what was there. Break: cut them first, and the pair the end-of-turn
// line names is the first thing a wide turn loses.
func TestTruncate_TestBendingIsCutLastAndCounted(t *testing.T) {
	d := &Digest{Declarations: emptyDeclarations()}
	for i := int64(0); i < 20; i++ {
		d.TestBending.Flaky = append(d.TestBending.Flaky, report.FlakyPair{Seqs: report.SeqPair{i, i + 1}})
	}
	d.TestBending.TestsOnlyThenGreen = []report.SeqPair{{7, 9}}
	// Over the cap with padding in Dropped, and under it once Dropped goes.
	for {
		b, _ := json.Marshal(d)
		if len(b) > CapBytes {
			break
		}
		d.Declarations.Dropped = append(d.Declarations.Dropped, "toolu_dropped_padding_padding")
	}
	truncate(d)
	if len(d.TestBending.Flaky) != 20 || d.TestBending.FlakyOmitted != 0 {
		t.Errorf("flaky cut to %d (+%d omitted) while Dropped still had room to give",
			len(d.TestBending.Flaky), d.TestBending.FlakyOmitted)
	}

	// Now a document whose test-bending lists alone exceed the cap.
	d = &Digest{Declarations: emptyDeclarations()}
	const n = 1000
	for i := int64(0); i < n; i++ {
		d.TestBending.Flaky = append(d.TestBending.Flaky, report.FlakyPair{Seqs: report.SeqPair{1_000_000 + i, 2_000_000 + i}})
		d.TestBending.TestsOnlyThenGreen = append(d.TestBending.TestsOnlyThenGreen,
			report.SeqPair{3_000_000 + i, 4_000_000 + i})
	}
	truncate(d)
	b, _ := json.Marshal(d)
	if len(b) > CapBytes {
		t.Fatalf("marshalled size = %d, want <= %d", len(b), CapBytes)
	}
	if !d.Truncated {
		t.Error("Truncated = false")
	}
	if got := len(d.TestBending.Flaky) + d.TestBending.FlakyOmitted; got != n {
		t.Errorf("flaky kept+omitted = %d, want %d", got, n)
	}
	if got := len(d.TestBending.TestsOnlyThenGreen) + d.TestBending.TestsOnlyThenGreenOmitted; got != n {
		t.Errorf("tests_only_then_green kept+omitted = %d, want %d", got, n)
	}
	if len(d.TestBending.TestsOnlyThenGreen) == 0 ||
		d.TestBending.TestsOnlyThenGreen[0] != (report.SeqPair{3_000_000, 4_000_000}) {
		t.Errorf("the first tests_only_then_green pair did not survive: %v", d.TestBending.TestsOnlyThenGreen)
	}
	if d.TestBending.TestsOnlyThenGreenOmitted > 0 && len(d.TestBending.Flaky) > 0 {
		t.Errorf("tests_only_then_green was cut while flaky still had %d pairs: flaky goes first",
			len(d.TestBending.Flaky))
	}
}
