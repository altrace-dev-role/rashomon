package report

import "testing"

// TestRubricNAMING12_TheZeroLineKindIsNoKind holds LineKind's zero value
// neutral: a kind never assigned must not read as a member. With LineBlank at
// zero, a reader that forgot to take DecodeHeader's answer treated every line
// as blank and skipped it in silence.
func TestRubricNAMING12_TheZeroLineKindIsNoKind(t *testing.T) {
	var zero LineKind
	for _, k := range []LineKind{LineBlank, LineDecoded, LineUndecodable, LineUndecodableSidechain} {
		if zero == k {
			t.Errorf("the zero LineKind is member %d; it must be none", k)
		}
	}
	var head struct {
		Type string `json:"type"`
	}
	for _, raw := range []string{"", "  ", `{"type":"user"}`, `{"type":1}`, `{"isSidechain":true,"type":1}`, `{`} {
		if DecodeHeader([]byte(raw), &head) == zero {
			t.Errorf("DecodeHeader(%q) returned the zero LineKind", raw)
		}
	}
}
