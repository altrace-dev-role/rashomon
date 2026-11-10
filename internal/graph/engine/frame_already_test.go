package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// alreadyFixture is two units: cli declares detachTarget, and store is a
// unit a phrase names by its own name (classes2 NN01's shape: detachTarget
// in cmd/rashomon, "the store" naming internal/store).
func alreadyFixture(t *testing.T) *graph.Store {
	t.Helper()
	return synth(t, []synFile{
		{"cli", "cli/detach.py", []synSym{fn("detachTarget"), fn("parse_install")}},
		{"store", "store/latest.py", []synSym{fn("latest_install"), fn("open_store")}},
	}, [][2]string{{"cli/detach.py#detachTarget", "store/latest.py#latest_install"}}, nil)
}

// "already" after a phrase no place word opens marks a precedent the
// request points to, not where the change lands, so it does not override
// the named unit (W-M1.47). NN01's own text splits at the possessive, so
// "store" is the phrase "already" follows; before W-M1.47 it anchored and
// claimed store over the named detachTarget. The registered sentence is
// one phrase here, whose anchor picks cli itself, so it reads as the named
// unit before and after.
func TestNames_APrecedentThatAlreadyExistsLeavesTheNamedUnit(t *testing.T) {
	g := alreadyFixture(t)
	for _, prompt := range []string{
		"detachTarget falls back to whatever the store already points to.",
		"detachTarget quietly falls back to whatever install this machine's store already points to when I pass --install with an empty value.",
	} {
		r := roles(t, g, prompt)
		if r.Why != ClassNamed || r.Path != "cli" {
			t.Errorf("%q: why %q path %q, want cli by the named detachTarget", prompt, r.Why, r.Path)
		}
	}
}

// "already" anchors a phrase only where a place word opens it: the place
// part splitPlace cuts from a verb clause, and a clause a place word
// heads ("where", not only "wherever"). Elsewhere it marks the phrase
// definite and no anchor; a phrase "existing" opens anchors as before
// (W-M1.47).
func TestFrame_AlreadyAnchorsOnlyAPhraseAPlaceWordOpens(t *testing.T) {
	for _, tc := range []struct {
		prompt, token string
		anchor        bool
	}{
		{"Add the ttl to sessions/expiry.py wherever the org settings already appear.", "org", true},
		{"Add the ttl to sessions/expiry.py, where the org settings already appear.", "org", true},
		{"Fix existing retry loop in sessions/expiry.py.", "retr", true},
		{"detachTarget falls back to whatever the store already points to.", "stor", false},
		{"internal/store/capbytes.go already has a pattern for this.", "capby", false},
	} {
		f := Parse(tc.prompt)
		var got *Phrase
		for i := range f.Targets {
			for _, tok := range f.Targets[i].Tokens {
				if strings.HasPrefix(tok, tc.token) {
					got = &f.Targets[i]
				}
			}
		}
		if got == nil {
			t.Errorf("%q: no target holds %q; targets %v", tc.prompt, tc.token, f.Targets)
			continue
		}
		if got.Anchor != tc.anchor || !got.Definite {
			t.Errorf("%q: %v, want anchor %v and definite; targets %v", tc.prompt, got, tc.anchor, f.Targets)
		}
	}
}
