package verify

import "testing"

// BenchmarkCut is VER51b's snapshot cut to the budget: 200 green gates, 20
// loose claims and a practice score, the stress case of card 1.10 R6, where
// some 170 cuts each re-marshal the record. Cut runs once per Stop and per
// subagent stop (2.4), never on a query point; ns/op is ungated, and the
// card's 10 ms is a note, not a gate.
func BenchmarkCut(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name string
		snap Snapshot
	}{{"gates_200", cpGates200()}} {
		if cp := Cut(c.snap, cpAt); cp.GatesOmitted == 0 {
			b.Fatal("the fixture fits the budget: nothing to cut")
		}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				Cut(c.snap, cpAt)
			}
		})
	}
}
