package platform

// PW1b-ON-230 step 2: the Stop reads the log after every reply that edits
// Go, so decode holds only the facts of the packages the reply wrote
// (keepFor), not the repository's whole graph. These hold that the families
// are the ones the whole log gives, that a refusal anywhere in the log is
// still a refusal, and that a dropped record costs no allocation.

import (
	"bytes"
	"context"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

// allocsNoGC is testing.AllocsPerRun with the collector held off for its
// window. A cycle there can cost a malloc the code never made (a pool's
// refill, a cleanup's run), and whether one lands follows the heap the
// earlier tests left: Impact's equality failed 54 against 53 after the
// keep tests alone, every time, and held run by itself and with GOGC=off
// (pw230c-flake, pw230c-flake2).
func allocsNoGC(runs int, f func()) float64 {
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	return testing.AllocsPerRun(runs, f)
}

// keepLog is a log with every subject kind pgraph writes facts under (pkg,
// type, method, func), a package whose path is another's prefix without a
// dot (example.com/m/ab beside example.com/m/a), and a retracted fact.
func keepLog(tier string) string {
	const a, ab = "example.com/m/a", "example.com/m/ab"
	rec := func(op, s, p, o, tier string) string {
		return op + "\t" + s + "\t" + p + "\t" + o + "\t" + tier + "\tlinux/amd64/cgo=0/tags=\tc1\tgotypes@v2\n"
	}
	return LogHeader + "\n" +
		rec("+", "pkg:"+a, "declares", "func:"+a+".F", tierProven) +
		rec("+", "pkg:"+a, "declares", "type:"+a+".T", tierProven) +
		rec("+", "pkg:"+a, "declares", "const:"+a+".C", tierProven) +
		rec("+", "type:"+a+".T", "declares", "method:"+a+".T.M", tierProven) +
		rec("+", "method:"+a+".T.M", "calls", "func:"+a+".F", tierProven) +
		rec("+", "func:"+a+".F", "calls", "func:"+ab+".G", tierProven) +
		rec("+", "func:"+a+".F", "calls", "func:"+a+".Old", tierProven) +
		rec("-", "func:"+a+".F", "calls", "func:"+a+".Old", tierProven) +
		rec("+", "pkg:"+ab, "declares", "func:"+ab+".G", tierProven) +
		rec("+", "func:"+ab+".G", "calls", "func:"+a+".F", tier)
}

// bySubjectOf is impact's index of a live set.
func bySubjectOf(live map[fact]struct{}) map[string][]fact {
	out := map[string][]fact{}
	for k := range live {
		out[k.subject] = append(out[k.subject], k)
	}
	return out
}

// TestPW1b2_KeepGivesTheWholeLogsFamilies: for each package, the facts
// decode keeps for it alone are exactly its own subjects', and its family
// hash is the one the whole log gives. Break: keep only pkg and func
// subjects (the type's and the method's facts drop out of a's hash), or
// match a path without its dot (ab's facts held for a), and this fails.
func TestPW1b2_KeepGivesTheWholeLogsFamilies(t *testing.T) {
	log := keepLog(tierProven)
	whole, err := decode(context.Background(), strings.NewReader(log), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"example.com/m/a":  {"func:example.com/m/a.F", "method:example.com/m/a.T.M", "pkg:example.com/m/a", "type:example.com/m/a.T"},
		"example.com/m/ab": {"func:example.com/m/ab.G", "pkg:example.com/m/ab"},
	}
	for p, subjects := range want {
		kept, err := decode(context.Background(), strings.NewReader(log), nil, keepFor([]string{p}))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for s := range bySubjectOf(kept) {
			got = append(got, s)
		}
		slices.Sort(got)
		if !slices.Equal(got, subjects) {
			t.Errorf("%s: kept subjects %q, want %q", p, got, subjects)
		}
		if g, w := familyHash(bySubjectOf(kept), "pkg:"+p), familyHash(bySubjectOf(whole), "pkg:"+p); g != w {
			t.Errorf("%s: family hash %s from the kept facts, %s from the whole log", p, g, w)
		}
	}
}

// TestPW1b2_ARefusalUnderADroppedSubjectStillRefuses: a record keep drops is
// still checked whole, so a log with one bad record anywhere is refused
// whatever the read wants. Break: test keep before the record's grammar.
func TestPW1b2_ARefusalUnderADroppedSubjectStillRefuses(t *testing.T) {
	log := keepLog("PROVENISH")
	if live, err := decode(context.Background(), strings.NewReader(log), nil, keepFor([]string{"example.com/m/a"})); err == nil {
		t.Errorf("a bad tier under example.com/m/ab, read for example.com/m/a: %d facts, no error", len(live))
	}
}

// TestPW1b2_ADroppedRecordAllocatesNothing: a decode that keeps no subject
// allocates as much for 1,024 records as for 2 (its reader, its buffer and
// the empty set), so the Stop's memory follows the written packages, not
// the log. Break: build the fact before keep is asked, or copy the line to
// a string to parse it, and the larger log allocates once a record.
func TestPW1b2_ADroppedRecordAllocatesNothing(t *testing.T) {
	none := func([]byte) bool { return false }
	allocs := func(n int) float64 {
		b := synthLog(t, n)
		return allocsNoGC(5, func() {
			if _, err := decode(context.Background(), bytes.NewReader(b), nil, none); err != nil {
				t.Fatal(err)
			}
		})
	}
	if small, big := allocs(2), allocs(4*checkEvery); big != small {
		t.Errorf("a decode that keeps nothing: %v allocations for %d records, %v for 2", big, 4*checkEvery, small)
	}
}

// TestPW1b2_ImpactHoldsTheWrittenPackagesOnly: Impact of a write to package
// a allocates as much over a log with 1,024 more records of other packages
// as over one with 2: it hands decode its wanted packages. Break: pass
// decode no keep, and the larger log allocates once a record.
func TestPW1b2_ImpactHoldsTheWrittenPackagesOnly(t *testing.T) {
	allocs := func(n int) float64 {
		root, lp := project(t, synthLog(t, n))
		return allocsNoGC(5, func() {
			if fams, err := Impact(context.Background(), root, lp, []string{"a/a.go"}); err != nil || len(fams) != 1 {
				t.Fatalf("Impact: %+v, %v; want package a's family", fams, err)
			}
		})
	}
	if small, big := allocs(2), allocs(4*checkEvery); big != small {
		t.Errorf("Impact of a write to a: %v allocations over %d more records, %v over 2", big, 4*checkEvery, small)
	}
}
