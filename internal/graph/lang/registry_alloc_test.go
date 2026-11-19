package lang

import (
	"slices"
	"testing"
)

// TestRegistry_RegisterAllocatesNothing: a Go Stop
// builds a registry of its producers' resolvers (index.UnitOf), so Register
// allocates nothing of its own: into a list with room for them, two
// Registers allocate nothing. The room is made first, so the growth of the
// list is not counted. Break: hold each entry by pointer, one allocation
// per Register.
func TestRegistry_RegisterAllocatesNothing(t *testing.T) {
	var r Resolver = fake{l: "go", ext: ".go"}
	var g Registry
	g.es = slices.Grow(g.es, 2)
	n := testing.AllocsPerRun(100, func() {
		g.es = g.es[:0]
		g.Register(r)
		g.Register(r)
	})
	if n != 0 {
		t.Fatalf("two Registers into a list with room allocate %v times; want 0", n)
	}
}
