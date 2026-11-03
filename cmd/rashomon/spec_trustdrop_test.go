package main

import (
	"strings"
	"testing"
)

// TestSTD1_TheTrustScreenSaysTheBindingsState (card SPEC-TRUST-DROP1, #153
// D1, D221 item 2): the trust screen says the spec's state as the bindings
// file holds it. With no binding it is untrusted; with a binding of this
// digest it is trusted; after an edit, the binding names another digest, so
// it is untrusted because it changed since it was trusted.
// Break: print "now untrusted" whatever the bindings hold.
func TestSTD1_TheTrustScreenSaysTheBindingsState(t *testing.T) {
	p := newSpecProject(t, specUnit)
	screen := func(doc string) string {
		t.Helper()
		shown := &person{answer: typedPrefix(t, doc)}
		atSpecTerminal(t, shown, nil)
		if code, out, errOut := specRun(t, "", "trust"); code != 0 || !strings.HasPrefix(out, "trusted "+p.path) {
			t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
		}
		return shown.shown.String()
	}
	if s := screen(specUnit); !strings.Contains(s, "now untrusted: flags to you only") {
		t.Errorf("first trust: the screen %q does not say the spec is untrusted", s)
	}
	if s := screen(specUnit); strings.Contains(s, "now untrusted") || !strings.Contains(s, "now trusted") {
		t.Errorf("a trust of the digest already bound: the screen %q, want it to say the spec is trusted", s)
	}
	p.edit(specVet)
	if s := screen(specVet); !strings.Contains(s, "now untrusted: edited since it was trusted") {
		t.Errorf("a trust after an edit: the screen %q, want it to say the spec changed since it was trusted", s)
	}
}
