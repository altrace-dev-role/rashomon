package pgraph

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func tri(s string, p Predicate, o string) Triple { return Triple{s, p, o} }

func mustProposed(t testing.TB, name string) Producer {
	t.Helper()
	p, err := ProposedProducer(name, "1")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

const cfgA = "linux/amd64/cgo=0/tags="

func TestPL5_ReconcileAssertsRetractsAndCounts(t *testing.T) {
	var l Log
	p := mustProposed(t, "x")
	d, err := l.Reconcile(p, cfgA, "c1", []Triple{tri("a", Calls, "b"), tri("a", Calls, "c")})
	if err != nil || d != (Delta{Asserted: 2}) {
		t.Fatalf("first: %+v %v", d, err)
	}
	// Same set again: nothing appended.
	d, _ = l.Reconcile(p, cfgA, "c2", []Triple{tri("a", Calls, "c"), tri("a", Calls, "b")})
	if d != (Delta{}) || l.Len() != 2 {
		t.Fatalf("idempotent: %+v len=%d", d, l.Len())
	}
	// One out, one in.
	d, _ = l.Reconcile(p, cfgA, "c3", []Triple{tri("a", Calls, "c"), tri("a", Calls, "d")})
	if d != (Delta{Asserted: 1, Retracted: 1}) || l.Len() != 4 {
		t.Fatalf("delta: %+v len=%d", d, l.Len())
	}
	live := l.Live()
	if len(live) != 2 || live[0].Triple().Object != "c" || live[1].Triple().Object != "d" {
		t.Fatalf("live = %+v", live)
	}
	recs := l.Records()
	if recs[2].Op() != Retract || recs[2].Fact().Triple().Object != "b" || recs[2].Fact().Commit() != "c3" {
		t.Fatalf("tombstone = %+v", recs[2])
	}
}

// Tier is part of a fact's identity: the same triple from two tiers is two
// facts, and one tier's reconcile never retracts the other's.
func TestPL4_TierIsIdentityAndNothingUpgrades(t *testing.T) {
	var l Log
	proven := gotypesProducer()
	// Same name as the PROVEN producer: only the tier tells them apart.
	prop, err := ProposedProducer("gotypes", "1")
	if err != nil {
		t.Fatal(err)
	}
	x := tri("a", Calls, "b")
	if _, err := l.Reconcile(proven, cfgA, "c1", []Triple{x}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reconcile(prop, cfgA, "c1", []Triple{x}); err != nil {
		t.Fatal(err)
	}
	live := l.Live()
	if len(live) != 2 || live[0].Tier() == live[1].Tier() {
		t.Fatalf("want two facts, two tiers: %+v", live)
	}
	// The proposer withdraws; the PROVEN fact survives and keeps its tier.
	if _, err := l.Reconcile(prop, cfgA, "c2", nil); err != nil {
		t.Fatal(err)
	}
	live = l.Live()
	if len(live) != 1 || live[0].Tier() != Proven {
		t.Fatalf("after proposer withdraws: %+v", live)
	}
}

func TestPL5_ReconcileScopeIsConfigAndProducer(t *testing.T) {
	var l Log
	p, q := mustProposed(t, "p"), mustProposed(t, "q")
	l.Reconcile(p, cfgA, "c", []Triple{tri("a", Calls, "b")})
	l.Reconcile(p, "windows/amd64/cgo=0/tags=", "c", []Triple{tri("a", Calls, "w")})
	l.Reconcile(q, cfgA, "c", []Triple{tri("a", Calls, "q")})
	// p under cfgA now empty: only p/cfgA is retracted.
	d, _ := l.Reconcile(p, cfgA, "c2", nil)
	if d != (Delta{Retracted: 1}) || len(l.Live()) != 2 {
		t.Fatalf("scope leak: %+v live=%d", d, len(l.Live()))
	}
}

func TestPL5_ReconcileRejectsBadInputAtomically(t *testing.T) {
	var l Log
	p := mustProposed(t, "x")
	for name, bad := range map[string][]Triple{
		"tab":      {tri("a", Calls, "b"), tri("a\tx", Calls, "c")},
		"newline":  {tri("a", Calls, "b"), tri("a", Calls, "c\n")},
		"empty":    {tri("a", Calls, "b"), tri("", Calls, "c")},
		"badutf8":  {tri("a", Calls, "b"), tri("a", Calls, "\xff")},
		"toolong":  {tri("a", Calls, "b"), tri(strings.Repeat("x", MaxFieldBytes+1), Calls, "c")},
		"predzero": {tri("a", 0, "b")},
	} {
		if _, err := l.Reconcile(p, cfgA, "c", bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if l.Len() != 0 {
			t.Fatalf("%s: partial append, len=%d", name, l.Len())
		}
	}
	if _, err := l.Reconcile(p, "bad\tcfg", "c", nil); err == nil {
		t.Error("bad config accepted")
	}
	if _, err := l.Reconcile(p, cfgA, "", nil); err == nil {
		t.Error("empty commit accepted")
	}
}

// The view is a function of the live set only: input order, commit labels and
// history do not reach it.
func TestPL5_ViewDeterministicAndHistoryFree(t *testing.T) {
	p := mustProposed(t, "x")
	set := []Triple{tri("a", Calls, "b"), tri("a", Calls, "c"), tri("b", Embeds, "c"), tri("z", Declares, "y")}
	var want []byte
	for seed := 0; seed < 20; seed++ {
		in := append([]Triple(nil), set...)
		rand.New(rand.NewSource(int64(seed))).Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		var l Log
		l.Reconcile(p, cfgA, "c1", in)
		v := l.View()
		if seed == 0 {
			want = v
		} else if !bytes.Equal(v, want) {
			t.Fatalf("seed %d: view differs", seed)
		}
	}
	// Reaching the same live set through history gives the same bytes.
	var h Log
	h.Reconcile(p, cfgA, "c1", []Triple{tri("q", Calls, "r")})
	h.Reconcile(p, cfgA, "c2", set)
	if !bytes.Equal(h.View(), want) {
		t.Fatal("history reached the view")
	}
	if !bytes.HasPrefix(want, []byte(ViewHeader+"\n")) || !bytes.HasSuffix(want, []byte("\n")) {
		t.Fatalf("view framing: %q", want)
	}
}

func TestPL5_RecordsAndLiveAreCopies(t *testing.T) {
	var l Log
	l.Reconcile(mustProposed(t, "x"), cfgA, "c", []Triple{tri("a", Calls, "b")})
	r := l.Records()
	r[0] = Record{}
	lv := l.Live()
	lv[0] = Fact{}
	if l.Records()[0].Fact().Triple().Subject != "a" || l.Live()[0].Triple().Subject != "a" {
		t.Fatal("caller mutation reached the log")
	}
}

// Records are appended in sorted order, so two logs built from the same input
// are byte-equal even though Reconcile walks maps.
func TestPL5_EncodeDeterministic(t *testing.T) {
	var want []byte
	for run := 0; run < 10; run++ {
		var l Log
		p := mustProposed(t, "x")
		var first, second []Triple
		for i := 0; i < 60; i++ {
			first = append(first, tri(fmt.Sprintf("s%02d", i), Calls, "o"))
			if i%3 != 0 {
				second = append(second, tri(fmt.Sprintf("s%02d", i), Calls, "o"), tri(fmt.Sprintf("n%02d", i), Calls, "o"))
			}
		}
		l.Reconcile(p, cfgA, "c1", first)
		l.Reconcile(p, cfgA, "c2", second)
		if run == 0 {
			want = l.Encode()
		} else if !bytes.Equal(l.Encode(), want) {
			t.Fatalf("run %d: encoding differs", run)
		}
	}
}
