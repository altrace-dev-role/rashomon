package shape

import (
	"encoding/json"
	"testing"
)

// FuzzRES5_ToolInputFieldsMatchTheirOldDecode: each field shape reads from a
// tool input is what its old decode gave. run_in_background was a struct
// field, so it is matched in any case; command and the named string fields
// were read from a map, and are matched exactly. Break: match
// run_in_background exactly, and Run_In_Background is lost.
func FuzzRES5_ToolInputFieldsMatchTheirOldDecode(f *testing.F) {
	for _, s := range []string{
		`{"command":"go test ./...","run_in_background":true}`,
		`{"Command":"ls","Run_In_Background":true}`,
		`{"run_in_background":"true"}`,
		`{"run_in_background":true,"RUN_IN_BACKGROUND":false}`,
		`{"url":"https://example.com","URL":"https://other.example"}`,
		`{"command":null}`,
		`[1]`,
		`null`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if !json.Valid(raw) {
			return // a tool input is a subslice of a validated payload
		}
		var obj struct {
			RunInBackground json.RawMessage `json:"run_in_background"`
		}
		var b bool
		wantBg := json.Unmarshal(raw, &obj) == nil && json.Unmarshal(obj.RunInBackground, &b) == nil && b
		if got := backgrounded(raw); got != wantBg {
			t.Fatalf("backgrounded(%q) = %v, struct decode %v", raw, got, wantBg)
		}
		for _, name := range []string{"command", "url"} {
			var m map[string]json.RawMessage
			var want string
			wantOK := json.Unmarshal(raw, &m) == nil
			if v, ok := m[name]; wantOK && ok {
				wantOK = json.Unmarshal(v, &want) == nil
			} else {
				wantOK = false
			}
			got, ok := stringField(raw, name)
			if ok != wantOK || got != want {
				t.Fatalf("%s of %q = %q %v, map decode %q %v", name, raw, got, ok, want, wantOK)
			}
		}
	})
}

// TestRES5_RunInBackgroundMatchesItsOldField: run_in_background in any case
// marks the call backgrounded, as its struct field matched it; command is
// still matched exactly, as the map it was read from matched it.
func TestRES5_RunInBackgroundMatchesItsOldField(t *testing.T) {
	if !backgrounded([]byte(`{"command":"go test","Run_In_Background":true}`)) {
		t.Error("Run_In_Background was not read")
	}
	if _, ok := commandField([]byte(`{"Command":"go test"}`)); ok {
		t.Error("Command matched command")
	}
}
