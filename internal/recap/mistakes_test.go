package recap

import (
	"slices"
	"testing"
)

// TestRP3b_MistakesPartitionTheFindings (card REPORT-REQS1): every finding
// lands in exactly one list, by the first rule that applies, and whether the
// agent was told is read from delivery, never from the finding's state. A
// line withheld and then resolved was fixed with no line, not caught; an open
// one told to the agent reached the person only when the last printed clause
// counted it, and was missed otherwise; a line delivered at a Stop reached
// the person whatever its state; a flag reached the person when open and was
// fixed with no line when resolved. The lists are disjoint and hold every
// finding, so no finding falls through two rules.
func TestRP3b_MistakesPartitionTheFindings(t *testing.T) {
	fs := []found{
		{FindingRef{ID: "told-resolved", Event: "PostToolUseFailure", State: FindingResolved, Seq: 1, ResolvedBy: 3}, true, false},
		{FindingRef{ID: "withheld-resolved", Event: "PostToolUseFailure", State: FindingResolved, Seq: 1, ResolvedBy: 3}, false, false},
		{FindingRef{ID: "stop-resolved", Event: "Stop", State: FindingResolved, Seq: 2, ResolvedBy: 3}, true, false},
		{FindingRef{ID: "stopfailure-open", Event: "StopFailure", State: FindingOpen, Seq: 4}, true, false},
		{FindingRef{ID: "told-open", Event: "PreToolUse", State: FindingOpen, Seq: 5}, true, false},
		{FindingRef{ID: "withheld-open", Event: "PostToolUse", State: FindingOpen, Seq: 6}, false, false},
		{FindingRef{ID: "told-unchecked", Event: "UserPromptSubmit", State: FindingUnchecked, Seq: 7}, true, false},
		{FindingRef{ID: "stop-unchecked", Event: "Stop", State: FindingUnchecked, Seq: 8}, true, false},
		{FindingRef{ID: "flag-open", Event: "PostToolUse", State: FindingOpen, Seq: 9}, false, true},
		{FindingRef{ID: "flag-resolved", Event: "PostToolUse", State: FindingResolved, Seq: 9, ResolvedBy: 10}, false, true},
	}
	for _, c := range []struct {
		carried bool
		reached []string
		missed  []string
	}{
		{true, []string{"stop-resolved/stop", "stopfailure-open/stop", "told-open/open", "stop-unchecked/stop", "flag-open/flag"},
			[]string{"withheld-open"}},
		{false, []string{"stop-resolved/stop", "stopfailure-open/stop", "stop-unchecked/stop", "flag-open/flag"},
			[]string{"told-open", "withheld-open"}},
	} {
		m := classify(fs, c.carried)
		var reached []string
		for _, f := range m.Reached {
			reached = append(reached, f.ID+"/"+string(f.Why))
		}
		for name, got := range map[string][2][]string{
			"caught":    {findingIDs(m.Caught), {"told-resolved"}},
			"reached":   {reached, c.reached},
			"missed":    {findingIDs(m.Missed), c.missed},
			"unsettled": {findingIDs(m.Unsettled), {"told-unchecked"}},
			"fixed":     {findingIDs(m.Fixed), {"withheld-resolved", "flag-resolved"}},
		} {
			if !slices.Equal(got[0], got[1]) {
				t.Errorf("carried %v: %s = %v, want %v", c.carried, name, got[0], got[1])
			}
		}
		lists := map[string]int{}
		for _, l := range [][]FindingRef{m.Caught, m.Missed, m.Unsettled, m.Fixed} {
			for _, id := range findingIDs(l) {
				lists[id]++
			}
		}
		for _, f := range m.Reached {
			lists[f.ID]++
		}
		for _, f := range fs {
			if lists[f.ID] != 1 {
				t.Errorf("carried %v: %s is in %d lists, want exactly 1", c.carried, f.ID, lists[f.ID])
			}
		}
		if len(lists) != len(fs) {
			t.Errorf("carried %v: the lists name %d findings, want the %d given", c.carried, len(lists), len(fs))
		}
	}
}

func findingIDs(fs []FindingRef) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}
