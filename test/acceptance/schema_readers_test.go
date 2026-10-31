package acceptance

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSchemaReaders_EveryReaderTakesTheCurrentCallVersion guards the bump
// itself. A call-schema bump (D11: one version, read the same way, additive
// key) once left internal/verify's test builder refusing the new version,
// and nothing failed until 148 tests there did; no run of the bump's own
// packages covered it. Two halves, each failing by this name:
//
//  1. The store's reader accepts every call kind from 1 through
//     store.CallSchemaVersion and nothing beyond it, so bumping the constant
//     without the acceptance rule following (or the reverse) fails here.
//  2. No Go file under internal/ or cmd/ closes the range with a literal: a
//     call record's schema compared with <= or > (or, in a reader, == or !=) against a number, or
//     a list of versions ("5, 6, 7 or 8"). A gate that reads a key a version
//     adds is open-ended (>= N, < N) and passes. A closed range is exactly
//     the reader a bump leaves behind, whatever package it lives in.
//
// Break: close a reader's range with a literal (SchemaVersion == 9), or stop
// AcceptsKind at one below CallSchemaVersion.
func TestSchemaReaders_EveryReaderTakesTheCurrentCallVersion(t *testing.T) {
	for _, kind := range []string{store.TypeDeclaration, store.TypeExecution, store.TypeTerminal} {
		for v := 1; v <= store.CallSchemaVersion; v++ {
			if !store.AcceptsKind(kind, v) {
				t.Errorf("store.AcceptsKind(%q, %d) = false: a %s record at version %d (CallSchemaVersion is %d) is skipped", kind, v, kind, v, store.CallSchemaVersion)
			}
		}
		for _, v := range []int{0, store.CallSchemaVersion + 1} {
			if store.AcceptsKind(kind, v) {
				t.Errorf("store.AcceptsKind(%q, %d) = true: only 1 through %d are call versions", kind, v, store.CallSchemaVersion)
			}
		}
		if v, _ := store.KindVersion(kind); v != store.CallSchemaVersion {
			t.Errorf("store.KindVersion(%q) = %d, want CallSchemaVersion %d", kind, v, store.CallSchemaVersion)
		}
	}

	// A ceiling (<=, >) closes the range anywhere. An equality (==, !=)
	// does in a reader; in a test it is how an old-version fixture says which
	// version it is, and a zero means "the current one", so tests are held
	// to the ceilings only.
	ceiling := regexp.MustCompile(`(?i)\bschema(?:_?version)?\)?\s*(?:<=|>)\s*[0-9]+\b|\b[0-9]+\s*(?:>=|<)\s*[a-z.]*schema(?:_?version)?\b|\b5, 6, 7(?:,| or)`)
	equality := regexp.MustCompile(`(?i)\bschema(?:_?version)?\)?\s*(?:==|!=)\s*[1-9][0-9]*\b`)
	// Event and session records have their own versions, and these files
	// state those; a call-record reader is never among them.
	skip := map[string]bool{
		"internal/store/kinds.go":  true, // the acceptance rule itself, in terms of the constants
		"internal/store/record.go": true,
		"internal/store/read.go":   true,
	}
	root := filepath.Join("..", "..")
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if skip[rel] {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "//") || strings.Contains(line, "EventSchemaVersion") || strings.Contains(line, "SessionSchemaVersion") {
					continue
				}
				if ceiling.MatchString(line) || (!strings.HasSuffix(p, "_test.go") && equality.MatchString(line)) {
					t.Errorf("%s:%d closes a schema range with a literal, so the next bump skips this reader: %s", rel, i+1, trim)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
