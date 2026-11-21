package lang

// IndexedOf is the resolvers g holds as registered by RegisterIndexed, in
// the order registered, for the tests of package lang_test, which may import
// the package that wires this build's registry (index.Wire) where a test of
// package lang may not.
func IndexedOf(g *Registry) []Resolver {
	var out []Resolver
	for _, e := range g.es {
		if e.indexed {
			out = append(out, e.r)
		}
	}
	return out
}
