package install

import "testing"

// TestTRU44_OnlyTakesOutRefusesAnAddedOrMovedByte: the edited-since-watch
// line is printed only when the result is the file as found with bytes
// taken out. A byte added or moved anywhere refuses it.
func TestTRU44_OnlyTakesOutRefusesAnAddedOrMovedByte(t *testing.T) {
	before := []byte("{\n  \"a\": 1,\n  \"hooks\": {}\n}\n")
	if !onlyTakesOut(before, []byte("{\n  \"a\": 1\n}\n")) {
		t.Error("a pure deletion was refused")
	}
	for _, after := range []string{"{\n\t\"a\": 1\n}\n", "{\n  \"a\": 2\n}\n", "{\"a\":1}\r\n", "\n{\n  \"a\": 1\n}\n"} {
		if onlyTakesOut(before, []byte(after)) {
			t.Errorf("onlyTakesOut accepted %q, which adds or moves a byte", after)
		}
	}
}

// TestTRU44_OnlyOursGoneRefusesAnyOtherDeletion: deleting bytes is not
// enough; what was deleted has to be our entries. Taking out another
// member, or another tool's entry, refuses the line.
func TestTRU44_OnlyOursGoneRefusesAnyOtherDeletion(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	ours := `{"hooks":[{"type":"command","command":"/bin/rashomon recap --install ` + id + `","timeout":10}]}`
	oursEnd := `{"hooks":[{"type":"command","command":"/bin/rashomon probe end --install ` + id + `","timeout":5}]}`
	theirs := `{"hooks":[{"type":"command","command":"echo hi","timeout":5}]}`
	before := []byte(`{"a":1,"hooks":{"Stop":[` + ours + `,` + theirs + `],"SessionEnd":[` + oursEnd + `]}}`)
	if !onlyOursGone(before, []byte(`{"a":1,"hooks":{"Stop":[`+theirs+`]}}`), id) {
		t.Error("removing exactly our entries was refused")
	}
	for _, after := range []string{
		`{"hooks":{"Stop":[` + theirs + `]}}`,
		`{"a":1}`,
		`{"a":1,"hooks":{"Stop":[` + theirs + `],"SessionEnd":[]}}`,
	} {
		if onlyOursGone(before, []byte(after), id) {
			t.Errorf("onlyOursGone accepted %s", after)
		}
	}
}
