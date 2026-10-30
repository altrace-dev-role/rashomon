package verify

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The four one-way doors' user lines (archive 40.4.7; card P5B, 5.7), byte
// for byte, and the task file's path as the record names it.
const (
	ivTask        = ".claude/rashomon/task/current.json"
	ivStoreLine   = "rashomon record: this call wrote '" + ivTask + "', under .claude/rashomon, which holds the spec and the task file rashomon reads."
	ivUnnamedLine = "rashomon record: this call wrote a file under .claude/rashomon, which holds the spec and the task file rashomon reads."
	ivTestLine    = "rashomon record: this edit removed a test after a gate run in this session exited non-zero."
	ivWeakLine    = "rashomon record: this call ran a gate with a flag or variable that skips its checks."
	ivConfigLine  = "rashomon record: this edit changed a CI or test-configuration file."
)

var ivEvents = []string{string(steer.EventPostToolUse), string(steer.EventPostToolUseFailure)}

// The gate runs around call x: a red at seq 5 before it and a green at 12
// after it; or a green before and a red after only.
var (
	ivRedBefore = []gateRun{{Seq: 5, Outcome: store.ExecFailed}, {Seq: 12, Outcome: store.ExecOK}}
	ivRedAfter  = []gateRun{{Seq: 5, Outcome: store.ExecOK}, {Seq: 12, Outcome: store.ExecFailed}}
)

// ivRaise is what integrityRaises gives for the call named toolUseID, among
// "w", "x" and "y" declared at seqs 9, 10 and 11, beside runs, mapped and
// rendered at seq 12 as SteerAt maps them.
func ivRaise(runs []gateRun, event, toolUseID string, entries ...IntegrityEntry) ([]steer.Finding, int) {
	r := &store.Run{Declarations: []store.Declaration{{Seq: 9, ToolUseID: "w"}, {Seq: 10, ToolUseID: "x"}, {Seq: 11, ToolUseID: "y"}}}
	snap := Snapshot{Integrity: Integrity{Unresolved: entries}}
	return mapFacts(r, integrityRaises(r, snap, runs, event, toolUseID), Point{Seq: 12, SessionID: "s1", PromptID: strp("p1")}, 0)
}

// TestIV3a_OneWayDoorsAreTheRaiseSet: the checks raised are PF-IV3's closed
// list as IntegrityKinds names it, a test removed, a gate run weakened, a gate
// configuration edited and the store touched, and no other kind (card P5B,
// 5.7; the list grows only by ruling).
func TestIV3a_OneWayDoorsAreTheRaiseSet(t *testing.T) {
	doors := []IntegrityKind{TestRemoved, GateWeakened, GateConfigEdited, StoreTouched}
	for _, k := range IntegrityKinds {
		if got := oneWayDoor(k); got != slices.Contains(doors, k) {
			t.Errorf("oneWayDoor(%s) = %v, want %v", k, got, !got)
		}
	}
}

// TestIV3b_EachDoorIsAUserFlagWithItsLine (D139): each door's entry made by
// this call is one integrity candidate at either post event, kind mapped,
// capped at flag (user-only, N15), citing the entry's seq, keyed by it, and
// rendered as its own line. Three of the four have no producer on a real
// session yet (IV-PRODUCERS1); their entries here are built by hand.
func TestIV3b_EachDoorIsAUserFlagWithItsLine(t *testing.T) {
	for _, c := range []struct {
		entry IntegrityEntry
		kind  steer.Kind
		line  string
	}{
		{IntegrityEntry{Check: StoreTouched, Seq: 10, Path: ivTask}, "store-touched", ivStoreLine},
		{IntegrityEntry{Check: TestRemoved, Seq: 10, Marker: "go.t_func"}, "test-deleted", ivTestLine},
		{IntegrityEntry{Check: GateWeakened, Seq: 10}, "gate-weakened", ivWeakLine},
		{IntegrityEntry{Check: GateConfigEdited, Seq: 10}, "gate-config-edited", ivConfigLine},
	} {
		for _, event := range ivEvents {
			found, dropped := ivRaise(ivRedBefore, event, "x", c.entry)
			if dropped != 0 || len(found) != 1 {
				t.Errorf("%s at %s: %d raised, %d dropped; want one raised", c.entry.Check, event, len(found), dropped)
				continue
			}
			f := found[0]
			if f.Pattern != pattern.Integrity || f.Kind != c.kind || f.MaxLevel != steer.LevelFlag || f.StopForm != steer.StopFormNone ||
				!slices.Equal(f.Evidence, []int64{10}) || f.Key != "10" || f.Message.Text() != c.line {
				t.Errorf("%s at %s: %s/%s at %v, evidence %v, key %q, text %q; want integrity/%s at flag, [10], \"10\", %q",
					c.entry.Check, event, f.Pattern, f.Kind, f.MaxLevel, f.Evidence, f.Key, f.Message.Text(), c.kind, c.line)
			}
		}
	}
}

// TestIV3c_OnlyThisCallsDoorsRaise (D-B9): an entry another call made, a
// kind off the list, a test removed with no failed gate run before it, or a
// call the record holds no declaration for raises nothing.
func TestIV3c_OnlyThisCallsDoorsRaise(t *testing.T) {
	store11 := IntegrityEntry{Check: StoreTouched, Seq: 11, Path: ivTask}
	for _, c := range []struct {
		name  string
		runs  []gateRun
		id    string
		entry IntegrityEntry
	}{
		{"another call's store write", ivRedBefore, "x", store11},
		{"an earlier call's store write", ivRedBefore, "y", IntegrityEntry{Check: StoreTouched, Seq: 10, Path: ivTask}},
		{"a skip marker", ivRedBefore, "x", IntegrityEntry{Check: SkipAdded, Seq: 10, Marker: "go.t_skip"}},
		{"tests only then green", ivRedBefore, "x", IntegrityEntry{Check: TestsOnlyThenGreen, Seq: 10}},
		{"a test removed with no gate run", nil, "x", IntegrityEntry{Check: TestRemoved, Seq: 10, Marker: "go.t_func"}},
		{"a test removed before the red", ivRedAfter, "x", IntegrityEntry{Check: TestRemoved, Seq: 10, Marker: "go.t_func"}},
		{"a call with no declaration", ivRedBefore, "z", store11},
		{"a call with no id", ivRedBefore, "", store11},
	} {
		for _, event := range ivEvents {
			if found, dropped := ivRaise(c.runs, event, c.id, c.entry); len(found) != 0 || dropped != 0 {
				t.Errorf("%s at %s: %d raised, %d dropped; want none", c.name, event, len(found), dropped)
			}
		}
	}
	if found, _ := ivRaise(ivRedBefore, ivEvents[0], "y", store11); len(found) != 1 {
		t.Errorf("call y's own store write: %d raised, want one", len(found))
	}
}

// TestIV3d_AStorePathTheSlotsRefuseRendersUnnamed: a store path outside the
// fact slot's charset is counted, not named, and a path the fact slot takes
// but the render slot refuses as secret-shaped falls to the same no-path
// line; neither is dropped.
func TestIV3d_AStorePathTheSlotsRefuseRendersUnnamed(t *testing.T) {
	for _, p := range []string{".claude/rashomon/my task.json", ".claude/rashomon/AKIAABCDEFGHIJKLMNOP.json"} {
		found, dropped := ivRaise(nil, ivEvents[0], "x", IntegrityEntry{Check: StoreTouched, Seq: 10, Path: p})
		if dropped != 0 || len(found) != 1 || found[0].Message.Text() != ivUnnamedLine {
			t.Errorf("%q: %d raised, %d dropped; want one with %q", p, len(found), dropped, ivUnnamedLine)
		}
	}
}

// TestIV3e_SteerAtRaisesTheStoreWriteOfThisCall: end to end over a built run,
// SteerAt at a call that wrote under .claude/rashomon raises that write, at
// either post event; at a call that wrote elsewhere it raises nothing, and a
// later store write names its own path, not the earlier one.
func TestIV3e_SteerAtRaisesTheStoreWriteOfThisCall(t *testing.T) {
	specPath := ".claude/rashomon/spec.json"
	r := runOf(t, edit(1, ivTask), edit(2, "pkg/a.go"), edit(3, specPath))
	at := func(n int64, event string) []steer.Finding {
		return SteerAt(r, nil, nil, nil, callPoint(t, r, n), time.Time{}, event, "t"+strconv.FormatInt(n, 10)).Findings
	}
	for _, event := range ivEvents {
		if got := at(1, event); len(got) != 1 || got[0].Message.Text() != ivStoreLine {
			t.Errorf("call 1 at %s: %d raised; want the task file's line", event, len(got))
		}
		if got := at(2, event); len(got) != 0 {
			t.Errorf("call 2 at %s: %d raised, want none", event, len(got))
		}
		want := "rashomon record: this call wrote '" + specPath + "', under .claude/rashomon, which holds the spec and the task file rashomon reads."
		if got := at(3, event); len(got) != 1 || got[0].Message.Text() != want {
			t.Errorf("call 3 at %s: %+v; want one naming %s", event, got, specPath)
		}
	}
}
