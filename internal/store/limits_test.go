package store

import (
	"strings"
	"testing"
)

func TestCapID_OverCapGetsADistinctDigest(t *testing.T) {
	a := strings.Repeat("a", MaxIDBytes+1)
	b := strings.Repeat("b", MaxIDBytes+1)
	ca, cb := CapID(a), CapID(b)
	if ca == "" || cb == "" {
		t.Fatalf("CapID(a)=%q CapID(b)=%q: an over-cap id must not collapse to the empty string", ca, cb)
	}
	if ca == cb {
		t.Fatalf("CapID(a) == CapID(b) == %q: two different over-cap ids must not collide", ca)
	}
	within := strings.Repeat("c", MaxIDBytes)
	if got := CapID(within); got != within {
		t.Errorf("CapID(within-cap) = %q, want it unchanged", got)
	}
}
