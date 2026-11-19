package editmarks

// Version is the version of the pattern table below, the Version every mark
// this package produces carries. A change to a row, or an added row, is a new
// version; the fold reads marks of any version alike (verify.MarkerTableVersion).
// Version 1 is the conservative, line-anchored default of issue 207, for Go and
// Python: archive 20.2.6's own table is not in the repository, and if it
// turns up it replaces this one under a version bump.
const Version = 1

// Basis is the basis of every mark here: an Edit's old_string against its
// new_string, summed over a MultiEdit's edits (edit_delta; D42).
const Basis = "edit_delta"

// lang is the language a row's patterns are read in, by the file's extension.
type lang uint8

const (
	langGo lang = iota + 1
	langPy
)

// boundary is what must follow a prefix for a line to match it.
type boundary uint8

const (
	// The zero boundary is no boundary: the prefix is the whole anchor (it
	// ends in "(" or ".").
	//
	// boundWord: the next byte, if any, is not an identifier byte, so
	// "@pytest.mark.skip" is not "@pytest.mark.skipped".
	boundWord boundary = iota + 1
	// boundGoTest: a Go test's name after "func Test" does not start with a
	// lower case letter, and is not TestMain, which is no test.
	boundGoTest
)

// row is one marker id and the line heads that count for it. A line counts at
// its head: after its leading blanks it starts with one of the prefixes. A
// line a comment marker starts therefore counts for nothing, so an assertion
// an edit comments out is a removal and a skip that sits in a comment is not a
// skip (issue 207). Each id has one language; its closed name is the id the
// fold's markerTable holds (verify, integrity.go), class and all.
type row struct {
	id       string
	lang     lang
	bound    boundary
	prefixes []string
}

// table is the version-1 pattern table, one row per id. The Go test value is
// t, tb or b; the Go assertions are its Error and Fatal methods and the calls
// into testify's assert and require packages, and a bare comparison such as
// "if got != want" is deliberately no assertion: no line-anchored pattern
// separates it from ordinary code, and a false "assertions removed" costs
// more than a miss. The pytest and unittest rows are the skip and skipif
// marks, the skip call, importorskip, the unittest skip decorators and
// skipTest, and xfail. A slice of constants, laid out in the binary with no
// code run at init (RES-2).
var table = [...]row{
	{id: "go.test_def", lang: langGo, bound: boundGoTest, prefixes: []string{"func Test"}},
	{id: "go.t_skip", lang: langGo, prefixes: []string{
		"t.Skip(", "t.Skipf(", "t.SkipNow(",
		"tb.Skip(", "tb.Skipf(", "tb.SkipNow(",
		"b.Skip(", "b.Skipf(", "b.SkipNow(",
	}},
	{id: "go.assert", lang: langGo, prefixes: []string{
		"t.Error(", "t.Errorf(", "t.Fatal(", "t.Fatalf(",
		"tb.Error(", "tb.Errorf(", "tb.Fatal(", "tb.Fatalf(",
		"b.Error(", "b.Errorf(", "b.Fatal(", "b.Fatalf(",
		"assert.", "require.",
	}},
	{id: "py.test_def", lang: langPy, prefixes: []string{"def test_", "async def test_"}},
	{id: "py.pytest_skip", lang: langPy, bound: boundWord, prefixes: []string{
		"@pytest.mark.skip", "@pytest.mark.skipif", "pytest.skip(", "pytest.importorskip(",
	}},
	{id: "py.unittest_skip", lang: langPy, bound: boundWord, prefixes: []string{
		"@unittest.skip", "@unittest.skipIf", "@unittest.skipUnless", "self.skipTest(",
	}},
	{id: "py.xfail", lang: langPy, bound: boundWord, prefixes: []string{"@pytest.mark.xfail", "pytest.xfail("}},
	{id: "py.assert", lang: langPy, prefixes: []string{"assert ", "assert(", "self.assert"}},
}
