package lang

import (
	"slices"
	"testing"
)

// inferring is a fake resolver that is an Inferrer too.
type inferring struct{ fake }

func (inferring) Manifests() []string     { return []string{"m.toml"} }
func (inferring) Infer([]string) Inferred { return Inferred{} }

// TestSETUP1_LI1_InferrersAreTheRegisteredInferrersInOrder: Inferrers holds
// each registered resolver that is an Inferrer, in registration order,
// each language once, configured or not, and none that is not one.
func TestSETUP1_LI1_InferrersAreTheRegisteredInferrersInOrder(t *testing.T) {
	var g Registry
	g.Register(fake{l: "plain", ext: ".p"})
	g.RegisterConfigured(inferring{fake{l: "b", ext: ".b"}}, func(string) (Resolver, error) { return nil, nil })
	g.Register(inferring{fake{l: "a", ext: ".a"}})
	g.Register(inferring{fake{l: "b", ext: ".bb"}})
	var got []string
	for _, i := range g.Inferrers() {
		got = append(got, i.(Resolver).Lang())
	}
	if want := []string{"b", "a"}; !slices.Equal(got, want) {
		t.Errorf("Inferrers' languages = %v; want %v", got, want)
	}
}
