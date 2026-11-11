package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEngine_NoLanguageLiterals: contracts-v2 section 6.3. The engine
// reads records and never names a language: no source file of this
// package holds a language name or a source extension as a string
// literal. The one place a language name exists is its lang.Resolver.
func TestEngine_NoLanguageLiterals(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`"(py|go|ts|js|java|rs|python|golang|typescript|javascript)"|"\.(py|go|ts|js|java|rs)"`)
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := re.Find(b); m != nil {
			t.Fatalf("%s names a language: %s", f, m)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no engine source checked")
	}
}
