package spec

import "testing"

// TestPF1l_MatchPathClauses: one matcher for Rule.Paths, Gate.AppliesTo and
// Rule.InvalidatedBy, which the preflight and SPEC-RULES1 share. Its input is
// a project-relative, slash-separated, cleaned path; one row group per clause
// of the contract (director, 10-29):
//   - `*` matches within one segment, `?` one character, `**` zero or more
//     whole segments;
//   - no character classes and no negation: `[`, `]` and a leading `!` are
//     plain characters; matching is case-sensitive;
//   - a pattern with no slash matches any segment of the path, its basename
//     or any parent directory, so it covers what is under a matching
//     directory (gitignore's rule);
//   - a leading `/` anchors to the project root, and so does a slash inside
//     the pattern;
//   - a trailing `/` means a directory of that name and everything under it,
//     at any depth unless another slash anchors it;
//   - a pattern that matches a parent directory covers what is under it
//     (gitignore's rule), anchored or not.
//
// Break: let `**` collapse to `*`, and the rows marked so fail.
func TestPF1l_MatchPathClauses(t *testing.T) {
	for _, c := range []struct {
		clause, pattern, rel string
		want                 bool
	}{
		{"star", "internal/*.go", "internal/a.go", true},
		{"star", "internal/*.go", "internal/x/a.go", false},
		{"star", "cmd/*/main.go", "cmd/rashomon/main.go", true},
		{"question", "cmd/?.go", "cmd/a.go", true},
		{"question", "cmd/?.go", "cmd/ab.go", false},
		{"question", "cmd?a.go", "cmd/a.go", false},
		{"globstar", "internal/**", "internal/verify/stop.go", true},
		{"globstar (collapse)", "internal/**/a.go", "internal/x/y/a.go", true},
		{"globstar", "internal/**/a.go", "internal/a.go", true},
		{"globstar", "internal/**", "internal", true},
		{"globstar", "**/*.go", "main.go", true},
		{"globstar (collapse)", "**/testdata/**", "internal/verify/testdata/x/y.json", true},
		{"globstar", "internal/**", "cmd/main.go", false},
		{"no class", "internal/[ab].go", "internal/[ab].go", true},
		{"no class", "internal/[ab].go", "internal/a.go", false},
		{"no negation", "!a.go", "a.go", false},
		{"no negation", "!a.go", "x/!a.go", true},
		{"case", "Internal/**", "internal/a.go", false},
		{"case", "*.GO", "a.go", false},
		{"basename", "*.go", "cmd/rashomon/main.go", true},
		{"basename", "main.go", "cmd/rashomon/main.go", true},
		{"basename", "main.go", "cmd/rashomon/main.go.orig", false},
		{"segment", "docs", "docs/a.md", true},
		{"segment", "docs", "x/docs/b.md", true},
		{"segment", "docs", "docsite/a.md", false},
		{"segment", "*.md", "x/docs/b.md", true},
		{"segment", "testdata", "internal/verify/testdata/x.json", true},
		{"leading slash", "/go.mod", "go.mod", true},
		{"leading slash", "/go.mod", "sub/go.mod", false},
		{"leading slash", "go.mod", "sub/go.mod", true},
		{"inner slash", "internal/*.go", "x/internal/a.go", false},
		{"parent", "internal/verify", "internal/verify/stop.go", true},
		{"parent", "internal/ver", "internal/verify/stop.go", false},
		{"trailing slash", "docs/", "docs", true},
		{"trailing slash", "docs/", "docs/guide/a.md", true},
		{"trailing slash", "docs/", "docsite/a.md", false},
		{"trailing slash", "docs/", "site/docs/a.md", true},
		{"trailing slash", "/docs/", "site/docs/a.md", false},
		{"trailing slash", ".github/workflows/", ".github/workflows/ci.yaml", true},
		{"input", "**", "/etc/passwd", false},
		{"input", "**", "../a.go", false},
		{"input", "../**", "a.go", false},
		{"input", "", "a.go", false},
		{"input", "*", "", false},
	} {
		if got := MatchPath(c.pattern, c.rel); got != c.want {
			t.Errorf("%s: MatchPath(%q, %q) = %v, want %v", c.clause, c.pattern, c.rel, got, c.want)
		}
	}
}

// TestPF1l_CoversHonoursExcludePaths: a rule covers a path one of its Paths
// matches and none of its ExcludePaths does; a rule with no Paths covers no
// path.
func TestPF1l_CoversHonoursExcludePaths(t *testing.T) {
	r := Rule{Paths: []string{"internal/**", "*.sql"}, ExcludePaths: []string{"internal/testdata/", "*_gen.go"}}
	for _, c := range []struct {
		rel  string
		want bool
	}{
		{"internal/verify/stop.go", true},
		{"db/migrations/001.sql", true},
		{"internal/testdata/x.json", false},
		{"internal/store/record_gen.go", false},
		{"cmd/main.go", false},
	} {
		if got := r.Covers(c.rel); got != c.want {
			t.Errorf("Covers(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	if (Rule{}).Covers("internal/a.go") {
		t.Error("a rule with no paths covers a path")
	}
}
