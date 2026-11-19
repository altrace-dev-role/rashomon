package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// marksWith is edit marks with n ids in each map, each id idBytes long, and
// a basis of basisBytes.
func marksWith(n, idBytes, basisBytes int) *EditMarks {
	m := &EditMarks{Added: map[string]int{}, Removed: map[string]int{}, Basis: strings.Repeat("b", basisBytes), Version: 1 << 30}
	for i := 0; i < n; i++ {
		id := strings.Repeat("<", idBytes-2) + string(rune('a'+i/26)) + string(rune('a'+i%26))
		m.Added[id], m.Removed[id] = 1<<62, 1<<62
	}
	return m
}

// TestEM_ExecutionEditMarksCapAtNAndNPlusOne: the marks of an execution are
// kept at their caps and withheld, with the reason oversize, one past any of
// them: ids in a map, an id's bytes, the basis's bytes.
func TestEM_ExecutionEditMarksCapAtNAndNPlusOne(t *testing.T) {
	for _, c := range []struct {
		name string
		m    *EditMarks
		kept bool
	}{
		{"at every cap", marksWith(MaxEditMarkIDs, MaxEditMarkIDBytes, MaxEditMarkBasisBytes), true},
		{"one id too many", marksWith(MaxEditMarkIDs+1, MaxEditMarkIDBytes, MaxEditMarkBasisBytes), false},
		{"an id too long", marksWith(1, MaxEditMarkIDBytes+1, 1), false},
		{"a basis too long", marksWith(1, 1+2, MaxEditMarkBasisBytes+1), false},
		{"none", &EditMarks{Basis: "edit_delta", Version: 1}, true},
	} {
		x := Execution{}
		x.EditMarks = c.m
		withheld := CapExecution(&x)
		if kept := x.EditMarks != nil; kept != c.kept || withheld == c.kept {
			t.Errorf("%s: kept %v, reported withheld %v, want kept %v", c.name, kept, withheld, c.kept)
		}
		if got, want := withheldAs(x.EditMarksWithheld), map[bool]string{true: "", false: withheldOversize}[c.kept]; got != want {
			t.Errorf("%s: edit_marks_withheld %q, want %q", c.name, got, want)
		}
	}
	x := Execution{}
	if CapExecution(&x) || x.EditMarksWithheld != nil {
		t.Error("an execution with no marks was withheld, or gained a reason")
	}
}

// TestEM_WorstCaseExecutionWithMarksFits: the execution that fills every
// field a writer fills at its cap, with its marks at theirs and every byte of
// the escapable ones written as six, is a record the writer takes: under the
// derived worst case and MaxRecordBytes. The marks' own bound is derived from
// their caps (editMarksBytes), and the worst marks come to no more.
func TestEM_WorstCaseExecutionWithMarksFits(t *testing.T) {
	esc := func(n int) string { return strings.Repeat("<", n) }
	cmd, root, name := esc(MaxCommandBytes), esc(MaxPathBytes), esc(MaxHarnessToolNameBytes)
	big := 1 << 62
	reason := "masker_error"
	spans := make([]MaskSpan, MaxMaskSpans)
	for i := range spans {
		spans[i] = MaskSpan{Rule: esc(MaxMaskRuleBytes), Start: MaxCommandBytes, End: MaxCommandBytes}
	}
	x := Execution{Type: TypeExecution, SchemaVersion: SchemaVersion, ToolUseID: esc(MaxIDBytes), SessionID: esc(MaxIDBytes),
		ToolName: esc(MaxIDBytes), ExecutedDigest: strings.Repeat("f", 64)}
	x.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = &cmd, &Cut{Offset: big, RemovedBytes: big}, spans
	x.ExecutedCommandWithheld, x.EditMarksWithheld = &reason, &reason
	x.HeadAfter, x.HarnessToolName = &GitState{Root: &root, State: "ok"}, &name
	x.EditMarks = marksWith(MaxEditMarkIDs, MaxEditMarkIDBytes, MaxEditMarkBasisBytes)
	if CapExecution(&x) {
		t.Fatal("the worst-case execution is not at its caps: CapExecution withheld a field of it")
	}
	line, err := marshalLine(x)
	if err != nil {
		t.Fatalf("the worst-case execution was refused: %v", err)
	}
	marks, err := json.Marshal(marksWith(MaxEditMarkIDs, MaxEditMarkIDBytes, MaxEditMarkBasisBytes))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("execution at every cap: %d bytes; its marks %d of the %d they are bounded by; derived worst case %d", len(line), len(marks), editMarksBytes, worstCaseRecordBytes())
	if len(marks) > editMarksBytes {
		t.Errorf("marks at every cap are %d bytes, over editMarksBytes %d", len(marks), editMarksBytes)
	}
	if len(line) > worstCaseRecordBytes() || len(line) > MaxRecordBytes {
		t.Errorf("the worst-case execution is %d bytes, over the derived worst case %d", len(line), worstCaseRecordBytes())
	}
}

// TestEM_ConfigEditMarksSwitch: the edit_marks key is on when absent and off
// for any value but "on", as the steer switches read; UpdateConfig round
// trips it and keeps the others.
func TestEM_ConfigEditMarksSwitch(t *testing.T) {
	for v, off := range map[string]bool{"": false, "on": false, "off": true, "Off": true, "disabled": true} {
		if got := (Config{EditMarks: v}).EditMarksOff(); got != off {
			t.Errorf("edit_marks %q: off = %v, want %v", v, got, off)
		}
	}
	root := t.TempDir()
	if _, err := UpdateConfig(root, func(c *Config) { c.EditMarks = SwitchOff; c.SteerUser = SwitchOff }); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.EditMarksOff() || !c.SteerOffUser() {
		t.Errorf("round trip: %+v", c)
	}
	if c.extra["edit_marks"] != nil {
		t.Error("edit_marks is named by Config and was also kept as an unknown key")
	}
}
