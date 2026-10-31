package verify

import (
	"encoding/json"
	"testing"
	"unsafe"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// TestMO1_TheRunStructsDidNotGrow holds what the per-run structs cost to what
// they cost before call schema 8 (D209, D173d): runner_name, masked_by and
// outcome_from are bytes in padding, not strings, since gateRun is held once
// for every call of a turn and RunRef once per gate state. Break: make any of
// the three a string field again.
func TestMO1_TheRunStructsDidNotGrow(t *testing.T) {
	if got, want := unsafe.Sizeof(gateRun{}), uintptr(144); got != want {
		t.Errorf("gateRun is %d bytes, want %d (its size before call schema 8)", got, want)
	}
	if got, want := unsafe.Sizeof(RunRef{}), uintptr(96); got != want {
		t.Errorf("RunRef is %d bytes, want %d (its size class before call schema 8)", got, want)
	}
}

// TestMO1_RunWordsKeepTheirJSON: the three one-byte words marshal as the
// words the keys always held, are left out when they are none, and read back.
func TestMO1_RunWordsKeepTheirJSON(t *testing.T) {
	ref := RunRef{Seq: 3, OutcomeFrom: SourceOutput,
		MaskedBy: StageWord(shape.MaskedByID("tail")), RunnerName: RunnerWord(shape.RunnerNameID("python3 -m pytest"))}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["outcome_from"] != "output_summary" || m["masked_by"] != "tail" || m["runner_name"] != "python3 -m pytest" {
		t.Errorf("run ref marshals %s, want the three words", b)
	}
	var back RunRef
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.OutcomeFrom != ref.OutcomeFrom || back.MaskedBy != ref.MaskedBy || back.RunnerName != ref.RunnerName {
		t.Errorf("run ref reads back %+v, want %+v", back, ref)
	}
	b, _ = json.Marshal(RunRef{})
	m = nil
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"outcome_from", "masked_by", "runner_name"} {
		if _, ok := m[k]; ok {
			t.Errorf("a run with none of the words marshals %q: %s", k, b)
		}
	}
}
