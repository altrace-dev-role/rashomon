package digest

import (
	"encoding/json"
	"fmt"
	"testing"
)

func emptyDeclarations() Declarations {
	return Declarations{
		WithoutExecution: []Unexecuted{},
		Unterminated:     []string{},
		Dropped:          []string{},
		ByTool:           map[string]int{},
		ByLabel:          map[string]int{},
	}
}

// TestTruncate_HoldsTheCeilingWithACorrectOmittedCount is H-88. A turn wide
// in ALL FIVE growable fields at once must still fit under CapBytes, and each
// field's own omitted count must equal what was actually removed from it.
func TestTruncate_HoldsTheCeilingWithACorrectOmittedCount(t *testing.T) {
	d := &Digest{Declarations: emptyDeclarations()}
	const n = 400
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("toolu_%04d", i)
		d.Declarations.Unterminated = append(d.Declarations.Unterminated, id)
		d.Declarations.Dropped = append(d.Declarations.Dropped, id)
		d.Declarations.WithoutExecution = append(d.Declarations.WithoutExecution,
			Unexecuted{ToolUseID: id, PermissionMode: "default"})
		d.Declarations.ByTool[fmt.Sprintf("Tool%04d", i)] = 1
		d.Declarations.ByLabel[fmt.Sprintf("label-%04d", i)] = 1
	}
	unterminatedBefore := len(d.Declarations.Unterminated)
	droppedBefore := len(d.Declarations.Dropped)
	withoutExecBefore := len(d.Declarations.WithoutExecution)
	byToolBefore := len(d.Declarations.ByTool)
	byLabelBefore := len(d.Declarations.ByLabel)

	truncate(d)

	if !d.Truncated {
		t.Fatal("Truncated = false for a document built to exceed the ceiling in all five fields")
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > CapBytes {
		t.Fatalf("marshalled size = %d, want <= %d (CapBytes)", len(b), CapBytes)
	}

	if got, want := len(d.Declarations.Unterminated)+d.Declarations.UnterminatedOmitted, unterminatedBefore; got != want {
		t.Errorf("unterminated kept+omitted = %d, want %d (nothing invented or double-counted)", got, want)
	}
	if got, want := len(d.Declarations.Dropped)+d.Declarations.DroppedOmitted, droppedBefore; got != want {
		t.Errorf("dropped kept+omitted = %d, want %d", got, want)
	}
	if got, want := len(d.Declarations.WithoutExecution)+d.Declarations.WithoutExecutionOmitted, withoutExecBefore; got != want {
		t.Errorf("without_execution kept+omitted = %d, want %d", got, want)
	}
	if got, want := len(d.Declarations.ByTool)+d.Declarations.ByToolOmitted, byToolBefore; got != want {
		t.Errorf("by_tool kept+omitted = %d, want %d", got, want)
	}
	if got, want := len(d.Declarations.ByLabel)+d.Declarations.ByLabelOmitted, byLabelBefore; got != want {
		t.Errorf("by_label kept+omitted = %d, want %d", got, want)
	}
}

// TestTruncate_BreakOnIndependentPerFieldCapsStillExceedsTheTotal is the
// break H-88 names: capping each field independently (at, say, 100 entries)
// lets a turn wide in five dimensions AT ONCE exceed the total while every
// field individually looks fine.
func TestTruncate_BreakOnIndependentPerFieldCapsStillExceedsTheTotal(t *testing.T) {
	const perFieldCap = 100
	d := &Digest{Declarations: emptyDeclarations()}
	for i := 0; i < perFieldCap; i++ {
		id := fmt.Sprintf("toolu_%04d", i)
		d.Declarations.Unterminated = append(d.Declarations.Unterminated, id)
		d.Declarations.Dropped = append(d.Declarations.Dropped, id)
		d.Declarations.WithoutExecution = append(d.Declarations.WithoutExecution,
			Unexecuted{ToolUseID: id, PermissionMode: "default"})
		d.Declarations.ByTool[fmt.Sprintf("Tool%04d", i)] = 1
		d.Declarations.ByLabel[fmt.Sprintf("label-%04d", i)] = 1
	}
	// Every field here is already AT a per-field cap of 100 and none of them
	// would be trimmed further by an independent-cap scheme.
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) <= CapBytes {
		t.Fatalf("premise: this fixture should exceed CapBytes (%d) to demonstrate the break; got %d", CapBytes, len(b))
	}
	// The real truncate() -- which enforces the TOTAL -- must still bring it
	// under budget, proving the per-field-cap scheme is the one that fails.
	truncate(d)
	b, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > CapBytes {
		t.Errorf("the real truncate() still exceeds CapBytes at %d bytes", len(b))
	}
}

// TestTruncate_OrderIsFixedAndDocumented pins the order truncate.go cuts in:
// dropped, unterminated, without_execution, by_label, by_tool. A document
// oversized in ONLY the first field in that order must lose nothing else.
func TestTruncate_OrderIsFixedAndDocumented(t *testing.T) {
	d := &Digest{Declarations: emptyDeclarations()}
	for i := 0; i < 2000; i++ {
		d.Declarations.Dropped = append(d.Declarations.Dropped, fmt.Sprintf("toolu_%05d", i))
	}
	d.Declarations.ByTool["Bash"] = 1
	d.Declarations.ByLabel["source"] = 1
	d.Declarations.Unterminated = append(d.Declarations.Unterminated, "toolu_x")
	d.Declarations.WithoutExecution = append(d.Declarations.WithoutExecution,
		Unexecuted{ToolUseID: "toolu_y"})

	truncate(d)

	if d.Declarations.DroppedOmitted == 0 {
		t.Fatal("premise: dropped should have been the field cut here")
	}
	if d.Declarations.UnterminatedOmitted != 0 || len(d.Declarations.Unterminated) != 1 {
		t.Error("unterminated was touched although dropped alone was enough to fit the ceiling")
	}
	if d.Declarations.WithoutExecutionOmitted != 0 || len(d.Declarations.WithoutExecution) != 1 {
		t.Error("without_execution was touched although dropped alone was enough to fit the ceiling")
	}
	if d.Declarations.ByToolOmitted != 0 || d.Declarations.ByLabelOmitted != 0 {
		t.Error("by_tool/by_label were touched although dropped alone was enough to fit the ceiling")
	}
}

// TestTruncate_NoOpUnderTheCeiling: a small, ordinary turn is left untouched.
func TestTruncate_NoOpUnderTheCeiling(t *testing.T) {
	d := &Digest{Declarations: emptyDeclarations()}
	d.Declarations.ByTool["Bash"] = 3
	before, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}

	truncate(d)

	if d.Truncated {
		t.Error("Truncated = true for a document well under the ceiling")
	}
	after, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("a document under the ceiling was modified:\nbefore: %s\nafter:  %s", before, after)
	}
}
