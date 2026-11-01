package spec

import "testing"

// TestSTD1d_OfIsTheBindingOfAProjectAndPath (card SPEC-TRUST-DROP1): Of is
// the binding of the project and the path whatever digest it names, so the
// trust screen and the Stop can tell a spec edited since it was trusted
// from one never trusted. The same path under another project is another
// binding, and a nil Bindings has none.
// Break: match the path alone, or the project alone.
func TestSTD1d_OfIsTheBindingOfAProjectAndPath(t *testing.T) {
	var b Bindings
	b.Bind(Binding{ProjectRoot: "/a", SpecPath: "/a/.claude/rashomon/spec.json", SpecDigest: "sha256:aa", BoundAtMS: 1})
	b.Bind(Binding{ProjectRoot: "/b", SpecPath: "/b/.claude/rashomon/spec.json", SpecDigest: "sha256:bb", BoundAtMS: 2})
	for _, c := range []struct {
		name, project, path, digest string
		ok                          bool
	}{
		{"the first project's", "/a", "/a/.claude/rashomon/spec.json", "sha256:aa", true},
		{"the second project's", "/b", "/b/.claude/rashomon/spec.json", "sha256:bb", true},
		{"a path bound under another project", "/b", "/a/.claude/rashomon/spec.json", "", false},
		{"a project bound at another path", "/a", "/a/spec.json", "", false},
	} {
		got, ok := b.Of(c.project, c.path)
		if ok != c.ok || got.SpecDigest != c.digest {
			t.Errorf("%s: Of = %+v, %v; want digest %q, %v", c.name, got, ok, c.digest, c.ok)
		}
	}
	if _, ok := (*Bindings)(nil).Of("/a", "/a/.claude/rashomon/spec.json"); ok {
		t.Error("a nil Bindings has a binding")
	}
}
