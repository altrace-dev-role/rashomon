package report

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// filesystemImports are the packages that let code find or open a file.
// io/fs is not among them: its sentinel errors are values, and naming
// fs.ErrNotExist opens nothing.
var filesystemImports = map[string]bool{"os": true, "path/filepath": true}

// TestRubricHLD4_OnlyTheAdapterFilesReachTheFilesystem holds report's
// transcript readers to the io.Reader they take: of this package's own
// files, only transcript_files.go may import a package that opens files.
// digest and spend link the readers and the verdict helpers; a reader that
// opened its own file again would hand them a path to a transcript they
// promise not to read.
func TestRubricHLD4_OnlyTheAdapterFilesReachTheFilesystem(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "transcript_files.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if filesystemImports[path] {
				t.Errorf("%s imports %s; opening a file belongs in transcript_files.go", name, path)
			}
		}
	}
}
