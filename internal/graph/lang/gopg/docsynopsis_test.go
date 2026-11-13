package gopg

import (
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDocSynopsis_EqualsGoDoc: docSynopsis returns what go/doc's
// new(doc.Package).Synopsis returns (the test may import go/doc; the package
// may not), for a table of the rule's edges and for every comment in the
// module's internal/ tree.
func TestDocSynopsis_EqualsGoDoc(t *testing.T) {
	bad := 0
	check := func(where, text string) {
		t.Helper()
		got, want := docSynopsis(text), new(doc.Package).Synopsis(text)
		if got != want && bad < 20 {
			bad++
			t.Errorf("%s: docSynopsis(%q) = %q, go/doc says %q", where, text, got, want)
		}
	}
	edges := []string{
		"",
		"Copyright 2026 The Authors. All rights reserved.",
		"COPYRIGHT notice. More.",
		"All rights reserved.",
		"Author: someone. More.",
		"Foo does x. Bar does y.",
		"Foo returns the U.S. total. More.",
		"Foo is a T. Next.",
		"Use e.g. this one. Then.",
		"Mr. Smith goes. Next.",
		"A.B.C. done. Next.",
		"Version 1.2.3 is out. Next.",
		"Ends with an ideographic stop。Next.",
		"Ends with a fullwidth stop．Next.",
		"Line one\nline two. Next.",
		"Tabs\tinside.\tNext.",
		"CRLF\r\nline.\r\nNext.",
		"No period at all",
		"Ends with period.",
		"  Leading spaces. More.",
		"Trailing space . More.",
		"See [fmt.Println] for output. More.",
		"See [Foo] and [Foo.Bar]. More.",
		"See [os] and [*os.File]. More.",
		"See [encoding/json.Marshal]. More.",
		"See [] and [ x ] and [nosuchpkg.F]. More.",
		"Has a URL https://example.com/a.b. More.",
		"Quotes ``like this'' here. More.",
		"Unicode café résumé naïve. More.",
		"# Heading\n\nText. More.",
		"\tcode block\n\nText. More.",
		"- item one\n- item two",
		"1. first\n2. second",
		"Text with [link] here.\n\n[link]: https://example.com\n",
		"[link]: https://example.com\n\nText.",
		"A long first sentence that runs well past any width a printer would wrap at, with words and more words and still more words. Next.",
	}
	for i, s := range edges {
		check("edge "+strconv.Itoa(i), s)
	}
	n := 0
	root := filepath.Join("..", "..", "..") // internal/
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ParseComments)
		if err != nil {
			return nil // a testdata file may not parse; its comments are not needed
		}
		for _, g := range f.Comments {
			n++
			check(path, g.Text())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 5000 {
		t.Fatalf("compared %d comments under %s, want at least 5000: the walk did not reach the tree", n, root)
	}
	t.Logf("%d edge cases and %d comments compared", len(edges), n)
}

// TestGopg_ImportsNoGoDoc: no non-test file of the package imports go/doc,
// whose package variables allocate at init in every process that links gopg
// (RES-2's exact darwin budget; docSynopsis mirrors the one function used).
func TestGopg_ImportsNoGoDoc(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		seen++
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "go/doc" {
				t.Errorf("%s imports go/doc: its init allocates in every process linking gopg (RES-2); use docSynopsis", name)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no non-test .go file found: the test is not in the package directory")
	}
}
