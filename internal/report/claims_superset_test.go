package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestVER60b_EveryFailureWordLoosensAVerifyClaim: verify's disclosure list
// covers failureVocabulary, so a summary this package reads as acknowledging a
// failure never holds a strict claim in the comparator (spec 20.2.5 step 8).
// Here, not in verify: the list is unexported, and verify may not import
// report (2.1's boundary).
func TestVER60b_EveryFailureWordLoosensAVerifyClaim(t *testing.T) {
	gates := []verify.GateState{{Gate: "learned:test:r:x@d:repo", Kind: "test", Required: true, State: verify.Green}}
	for _, w := range failureVocabulary {
		msg := "All tests pass. Then " + w + "."
		s := verify.Claims([]byte(msg), nil, []byte("key"), gates, false, verify.SourceFinalMessage, nil)
		if len(s.Claims) != 1 || s.Claims[0].Strict {
			t.Errorf("%q: claims %+v, want one loose tests_pass", msg, s.Claims)
		}
	}
}
