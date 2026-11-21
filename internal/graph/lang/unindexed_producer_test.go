package lang

import "testing"

// versioned is a fake whose reader states p as the producer string the
// index it writes names in its header.
type versioned struct {
	fake
	p string
}

func (v versioned) Producer() string { return v.p }

// TestRegistry_UnindexedComparesTheProducerString: an indexed claimant that
// states its producer string is unindexed where the producers lack its
// language and where they name a string other than its own, and is indexed
// only where they name its own; an indexed claimant that states none keeps
// the key test, indexed wherever the producers name its language. No step
// runs. Break: drop the comparison (pylite/1 reads as indexed), invert it
// (pylite/2 reads as unindexed), or compare by prefix or by order
// (pylite/20 reads as indexed).
func TestRegistry_UnindexedComparesTheProducerString(t *testing.T) {
	runs := map[string]int{}
	var g Registry
	g.RegisterIndexed(versioned{fake: fake{l: "py", ext: ".py"}, p: "pylite/2"}, counted(fake{l: "py", ext: ".py"}, runs))
	g.RegisterIndexed(fake{l: "rb", ext: ".rb"}, counted(fake{l: "rb", ext: ".rb"}, runs))
	for _, c := range []struct {
		rel       string
		producers map[string]string
		want      string
	}{
		{"m.py", map[string]string{"go": "pgraph/v0"}, "py"},
		{"m.py", map[string]string{"py": "pylite/2"}, ""},
		{"m.py", map[string]string{"py": "pylite/1"}, "py"},
		{"m.py", map[string]string{"py": "pylite/20"}, "py"},
		{"m.py", map[string]string{"py": ""}, "py"},
		{"m.rb", map[string]string{"rb": "rblite/9"}, ""},
		{"m.rb", map[string]string{"go": "pgraph/v0"}, "rb"},
	} {
		if l, ok := g.Unindexed("/a", c.rel, c.producers); l != c.want || ok != (c.want != "") {
			t.Errorf("Unindexed(%q, %v) = %q, %v; want %q", c.rel, c.producers, l, ok, c.want)
		}
	}
	if len(runs) != 0 {
		t.Errorf("Unindexed ran the steps: %v", runs)
	}
}
