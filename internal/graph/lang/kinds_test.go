package lang

import (
	"slices"
	"testing"
)

// TestKinds_TheRowsAreTheRuledRows: the table holds exactly the rows the
// rule in kinds.go names, .mod, .work, .cfg and .gitignore among them,
// in the order of the rule's classes, so a row cannot be added without
// this list and the rule in kinds.go changing with it.
func TestKinds_TheRowsAreTheRuledRows(t *testing.T) {
	want := []string{".md", ".rst", ".txt", ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".csv",
		".mod", ".work", ".sum", ".lock", ".png", ".jpg", ".gif", ".svg", ".gitignore", "LICENSE", "CODEOWNERS"}
	if got := nonSourceKinds[:]; !slices.Equal(got, want) {
		t.Errorf("the kinds table is %q\nwant %q", got, want)
	}
}

// TestKinds_ARowIsNonSource: a path of each row's kind is non-source,
// wherever it sits: an extension row by its extension, a name row by the
// path's base name. go.mod and go.work are rows (so an edit of go.mod
// prints nothing in a Go session). Break: invert the answer.
func TestKinds_ARowIsNonSource(t *testing.T) {
	for _, rel := range []string{"README.md", "docs/a.rst", "LICENSE.txt", "x/y.json", "ci.yaml", "ci.yml", "pyproject.toml",
		"tox.ini", "setup.cfg", "d.csv", "poetry.lock", "go.sum", "go.mod", "go.work", "a.png", "a.jpg", "a.gif", "a.svg",
		".gitignore", "sub/.gitignore", "LICENSE", "dir.v2/LICENSE", ".github/CODEOWNERS"} {
		if !NonSource(rel) {
			t.Errorf("NonSource(%q) = false, want true", rel)
		}
	}
}

// TestKinds_AProgramKindIsNotNonSource: a kind whose files are a
// program, read here or not, is not a row, so an edit of one that no
// resolver claims is disclosed; so is a kind the table has never heard of,
// and a build program whose name ends in a row's extension
// (CMakeLists.txt). Break: invert the answer, or key CMakeLists.txt by .txt.
func TestKinds_AProgramKindIsNotNonSource(t *testing.T) {
	for _, rel := range []string{"a.go", "a.py", "web/a.ts", "run.sh", "q.sql", "Makefile", "build/Dockerfile", "README",
		"a.MD", "x.unknownkind", "build/CMakeLists.txt"} {
		if NonSource(rel) {
			t.Errorf("NonSource(%q) = true, want false", rel)
		}
	}
}

// TestKinds_KeyIsTheExtensionElseTheBaseName: the key is the final
// element's extension with its dot, else its base name, whatever the
// directories hold; a named program (namedPrograms) keys as its name, so a
// line that names the kind names CMakeLists.txt, not .txt. Break: key every
// path by its base name, or check the name after the extension.
func TestKinds_KeyIsTheExtensionElseTheBaseName(t *testing.T) {
	for rel, want := range map[string]string{
		"a/b/c.tar.gz": ".gz", "go.mod": ".mod", "notes.md": ".md", ".gitignore": ".gitignore",
		"x/Makefile": "Makefile", "dir.v2/LICENSE": "LICENSE", "LICENSE": "LICENSE", "docs/md": "md",
		"build/CMakeLists.txt": "CMakeLists.txt",
	} {
		if got := KindKey(rel); got != want {
			t.Errorf("KindKey(%q) = %q, want %q", rel, got, want)
		}
	}
	if NonSource("docs/md") {
		t.Error(`NonSource("docs/md") = true: a file named md has no .md extension`)
	}
}

// TestKinds_AReadAllocatesNothing (A4, RES-2): asking the table allocates
// nothing, for a row and for a miss.
func TestKinds_AReadAllocatesNothing(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() { _ = NonSource("docs/guide.md"); _ = NonSource("web/app.ts") }); n != 0 {
		t.Errorf("NonSource allocates %v per run, want 0", n)
	}
}
