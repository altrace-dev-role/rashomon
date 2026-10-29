package verify

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
)

// TestVER60b_EveryFailureWordLoosensAVerifyClaim: verify's disclosure list
// covers report's failure vocabulary, so a summary report reads as
// acknowledging a failure never holds a strict claim in the comparator (spec
// 20.2.5 step 8). Each word is a claimDisclosures entry in its word form, and
// in the next sentence it loosens the claim before it. Here, not in report:
// verify imports report (integrity.go), so a report test importing verify is
// a cycle.
func TestVER60b_EveryFailureWordLoosensAVerifyClaim(t *testing.T) {
	gates := []GateState{{Gate: "learned:test:r:x@d:repo", Kind: "test", Required: true, State: Green}}
	for _, w := range report.FailureVocabulary() {
		if !slices.ContainsFunc(claimDisclosures[:], func(d []string) bool { return slices.Equal(d, words(w)) }) {
			t.Errorf("%q (words %q) is not a claimDisclosures entry", w, words(w))
		}
		msg := "All tests pass. Then " + w + "."
		s := Claims([]byte(msg), nil, []byte("key"), gates, false, SourceFinalMessage, nil)
		if len(s.Claims) != 1 || s.Claims[0].Strict {
			t.Errorf("%q: claims %+v, want one loose tests_pass", msg, s.Claims)
		}
	}
}
