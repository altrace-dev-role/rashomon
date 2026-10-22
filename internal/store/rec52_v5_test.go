package store

import (
	"strings"
	"testing"
)

// withheldAs is the reason a *_withheld key carries, "" for null.
func withheldAs(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestREC52c_V5CapsAtNAndNPlusOne: every v5 field a payload can fill has its
// cap of 1.2.11 at the store, whatever capture does. At N the value is kept;
// at N+1 it is null and its *_withheld key says "oversize", and CapDeclaration
// reports the withholding. A withheld written_paths also reads incomplete,
// and a withheld command takes its cut and its marker spans with it.
func TestREC52c_V5CapsAtNAndNPlusOne(t *testing.T) {
	str := func(n int) *string { s := strings.Repeat("x", n); return &s }
	paths := func(entries, each int) []string {
		out := make([]string, entries)
		for i := range out {
			out[i] = strings.Repeat("p", each)
		}
		return out
	}
	yes := true
	cases := []struct {
		name  string
		fill  func(d *Declaration, over bool)
		check func(d *Declaration) (kept bool, reason string)
	}{
		{"command", func(d *Declaration, over bool) {
			d.Command = str(MaxCommandBytes + b2i(over))
			d.CommandCut = &Cut{Offset: 1, RemovedBytes: 1}
			d.Masked = []MaskSpan{{Rule: "env-assign", Start: 0, End: 1}}
		}, func(d *Declaration) (bool, string) {
			if d.Command == nil && (d.CommandCut != nil || d.Masked != nil) {
				return false, "the cut or the spans outlived the command"
			}
			return d.Command != nil, withheldAs(d.CommandWithheld)
		}},
		{"cwd", func(d *Declaration, over bool) { d.CWD = str(MaxPathBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.CWD != nil, withheldAs(d.CWDWithheld) }},
		{"project", func(d *Declaration, over bool) { d.Project = str(MaxPathBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.Project != nil, withheldAs(d.ProjectWithheld) }},
		{"file_path", func(d *Declaration, over bool) { d.FilePath = str(MaxPathBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.FilePath != nil, withheldAs(d.FilePathWithheld) }},
		{"written_paths entries", func(d *Declaration, over bool) {
			d.WrittenPaths, d.WrittenPathsComplete = paths(MaxWrittenPaths+b2i(over), 1), &yes
		}, writtenKept},
		{"written_paths bytes", func(d *Declaration, over bool) {
			d.WrittenPaths, d.WrittenPathsComplete = paths(4, MaxWrittenPathsBytes/4), &yes
			if over {
				d.WrittenPaths[3] += "p"
			}
		}, writtenKept},
		{"target value", func(d *Declaration, over bool) {
			d.Target = &Target{AWSProfile: str(1), KubeContext: str(MaxTargetValueBytes + b2i(over))}
		}, func(d *Declaration) (bool, string) { return d.Target != nil, withheldAs(d.TargetWithheld) }},
		{"prompt_id", func(d *Declaration, over bool) { d.PromptID = str(MaxIDBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.PromptID != nil, withheldAs(d.PromptIDWithheld) }},
		{"agent_id", func(d *Declaration, over bool) { d.AgentID = str(MaxIDBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.AgentID != nil, withheldAs(d.AgentIDWithheld) }},
		{"agent_type", func(d *Declaration, over bool) { d.AgentType = str(MaxIDBytes + b2i(over)) },
			func(d *Declaration) (bool, string) { return d.AgentType != nil, withheldAs(d.AgentTypeWithheld) }},
		{"hosts", func(d *Declaration, over bool) { d.Hosts = paths(MaxHosts+b2i(over), 3) },
			func(d *Declaration) (bool, string) { return d.Hosts != nil, withheldAs(d.HostsWithheld) }},
		{"ssh_hosts", func(d *Declaration, over bool) { d.SSHHosts = paths(MaxHosts+b2i(over), 3) },
			func(d *Declaration) (bool, string) { return d.SSHHosts != nil, withheldAs(d.SSHHostsWithheld) }},
		{"harness_tool_name", func(d *Declaration, over bool) { d.HarnessToolName = str(MaxHarnessToolNameBytes + b2i(over)) },
			func(d *Declaration) (bool, string) {
				if d.HarnessToolName == nil {
					return false, "oversize" // no _withheld key of its own: null is its "else" (tech 6.2.1)
				}
				return true, ""
			}},
	}
	for _, c := range cases {
		for _, over := range []bool{false, true} {
			var d Declaration
			c.fill(&d, over)
			withheld := CapDeclaration(&d)
			kept, reason := c.check(&d)
			wantReason := ""
			if over {
				wantReason = withheldOversize
			}
			if withheld != over || kept == over || reason != wantReason {
				t.Errorf("%s over the cap %v: CapDeclaration reported %v, kept %v, reason %q; want %v, %v, %q",
					c.name, over, withheld, kept, reason, over, !over, wantReason)
			}
		}
	}
}

// writtenKept is the check of the written_paths cases: a withheld list is
// null, says oversize, and reads incomplete, never complete.
func writtenKept(d *Declaration) (bool, string) {
	if d.WrittenPaths == nil && (d.WrittenPathsComplete == nil || *d.WrittenPathsComplete) {
		return false, "a withheld list reads complete"
	}
	return d.WrittenPaths != nil, withheldAs(d.WrittenPathsWithheld)
}

// TestREC52d_MaskSpansThatCannotBeTheCommandsWithholdIt: a marker span is
// the evidence of what was masked. One that falls outside the stored
// command, or names a rule longer than any rule id, or one span too many,
// cannot be evidence of that command, so the command is withheld rather than
// stored beside spans that misdescribe it.
func TestREC52d_MaskSpansThatCannotBeTheCommandsWithholdIt(t *testing.T) {
	cmd := strings.Repeat("c", 100)
	ok := MaskSpan{Rule: strings.Repeat("r", MaxMaskRuleBytes), Start: 0, End: 100}
	many := make([]MaskSpan, MaxMaskSpans+1)
	for i := range many {
		many[i] = MaskSpan{Rule: "r", Start: 0, End: 1}
	}
	for _, c := range []struct {
		name  string
		spans []MaskSpan
		kept  bool
	}{
		{"valid", []MaskSpan{ok}, true},
		{"at the count", many[:MaxMaskSpans], true},
		{"one too many", many, false},
		{"rule too long", []MaskSpan{{Rule: ok.Rule + "r", Start: 0, End: 1}}, false},
		{"past the end", []MaskSpan{{Rule: "r", Start: 0, End: 101}}, false},
		{"reversed", []MaskSpan{{Rule: "r", Start: 5, End: 4}}, false},
		{"negative", []MaskSpan{{Rule: "r", Start: -1, End: 4}}, false},
	} {
		d := Declaration{}
		s := cmd
		d.Command, d.Masked = &s, c.spans
		withheld := CapDeclaration(&d)
		if (d.Command != nil) != c.kept || withheld == c.kept {
			t.Errorf("%s: command kept %v, withheld reported %v; want kept %v", c.name, d.Command != nil, withheld, c.kept)
		}
	}
}

// TestREC52e_ExecutionV5CapsAtNAndNPlusOne is REC52c for the execution's
// v5 fields: the executed command, the root of the git state after a
// HEAD-moving call, and the harness's own tool name.
func TestREC52e_ExecutionV5CapsAtNAndNPlusOne(t *testing.T) {
	for _, over := range []bool{false, true} {
		cmd, root := strings.Repeat("c", MaxCommandBytes+b2i(over)), strings.Repeat("r", MaxPathBytes+b2i(over))
		x := Execution{}
		x.ExecutedCommand, x.ExecutedCommandCut = &cmd, &Cut{Offset: 1, RemovedBytes: 1}
		x.ExecutedMasked = []MaskSpan{{Rule: "r", Start: 0, End: 1}}
		x.HeadAfter = &GitState{Root: &root, State: "ok"}
		name := strings.Repeat("n", MaxHarnessToolNameBytes+b2i(over))
		x.HarnessToolName = &name
		withheld := CapExecution(&x)
		want := ""
		if over {
			want = withheldOversize
		}
		if withheld != over || (x.ExecutedCommand == nil) != over || withheldAs(x.ExecutedCommandWithheld) != want {
			t.Errorf("executed_command over %v: reported %v, null %v, reason %q", over, withheld, x.ExecutedCommand == nil, withheldAs(x.ExecutedCommandWithheld))
		}
		if over && (x.ExecutedCommandCut != nil || x.ExecutedMasked != nil) {
			t.Error("the cut or the spans outlived the executed command")
		}
		if (x.HarnessToolName == nil) != over {
			t.Errorf("harness_tool_name over %v: null %v", over, x.HarnessToolName == nil)
		}
		if (x.HeadAfter.Root == nil) != over || withheldAs(x.HeadAfter.RootWithheld) != want {
			t.Errorf("head_after.root over %v: null %v, reason %q", over, x.HeadAfter.Root == nil, withheldAs(x.HeadAfter.RootWithheld))
		}
	}
}

// b2i is 1 for true: the one byte, entry or span past a cap.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
