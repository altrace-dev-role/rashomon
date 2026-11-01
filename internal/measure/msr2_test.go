package measure

import (
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestMSR2_NoNetworkInMeasure: no file of the package imports a network or
// subprocess package (MSR-2): the pair reads the record and the transcripts,
// and the driver that starts sessions and runs the oracle is outside the
// binary.
func TestMSR2_NoNetworkInMeasure(t *testing.T) {
	banned := []string{"net", "net/http", "os/exec", "syscall"}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for _, im := range f.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			if slices.Contains(banned, path) || strings.HasPrefix(path, "net/") {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
	if read == 0 {
		t.Fatal("no source file read")
	}
}
