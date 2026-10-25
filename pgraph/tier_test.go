package pgraph

import "testing"

func TestPL4_TierClosedSet(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Tier
		ok   bool
	}{
		{"PROVEN", Proven, true},
		{"ALGORITHMIC", Algorithmic, true},
		{"model-proposed", Proposed, true},
		{"proven", 0, false}, // case is part of the spelling
		{"", 0, false},
		{"TRUSTED", 0, false},
	} {
		got, err := ParseTier(c.in)
		if (err == nil) != c.ok || (c.ok && got != c.want) {
			t.Errorf("ParseTier(%q) = %v, %v; want %v ok=%v", c.in, got, err, c.want, c.ok)
		}
		if c.ok && got.String() != c.in {
			t.Errorf("round trip %q -> %q", c.in, got.String())
		}
	}
	if Tier(0).String() == "PROVEN" || Tier(99).String() == "PROVEN" {
		t.Error("out-of-range tier must not render as a real tier")
	}
}

func TestPL4_PredicateClosedSet(t *testing.T) {
	for _, p := range []Predicate{Declares, Calls, Embeds, Implements, ImplementsViaPointer} {
		got, err := ParsePredicate(p.String())
		if err != nil || got != p {
			t.Errorf("round trip %v: %v %v", p, got, err)
		}
	}
	if _, err := ParsePredicate("causes"); err == nil {
		t.Error("unknown predicate accepted")
	}
}

// A producer outside the package can only ever be model-proposed: the PROVEN
// constructor is unexported, so no caller can mint a PROVEN fact.
func TestPL4_OutsideProducerIsProposedOnly(t *testing.T) {
	p, err := ProposedProducer("llm", "1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Tier() != Proposed {
		t.Fatalf("tier = %v", p.Tier())
	}
	for _, bad := range [][2]string{{"", "1"}, {"a@b", "1"}, {"a", ""}, {"a\tb", "1"}, {"a", "1\n"}} {
		if _, err := ProposedProducer(bad[0], bad[1]); err == nil {
			t.Errorf("ProposedProducer(%q,%q) accepted", bad[0], bad[1])
		}
	}
}
