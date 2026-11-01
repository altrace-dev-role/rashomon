package learn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// FuzzReadActive: the prompt path's read of active.json, on any bytes the
// ledger's key signs, as an agent with the user's rights can sign them (the
// card's stated gap: the signature is tamper evidence, not a boundary).
// ReadActive never panics; a file over ActiveMaxBytes is the size flag and
// no lesson; a file that does not parse gives nothing; any other gives only
// probation and proven rows, no more than the bytes could hold. Pick on what
// it gives returns one of those rows, approved, with a note under
// render.MaxMessageBytes, and never the same id once that id is delivered.
func FuzzReadActive(f *testing.F) {
	root := f.TempDir()
	dir := filepath.Join(root, DirLearn, project)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		f.Fatal(err)
	}
	key := []byte("fuzz-ledger-key")
	if err := os.WriteFile(filepath.Join(root, DirLearn, FileKey), key, 0o600); err != nil {
		f.Fatal(err)
	}
	seed := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			f.Fatal(err)
		}
		return b
	}
	many := []Lesson{provenCwd, probationNpm}
	for i := range 5 {
		many = append(many, Lesson{ID: "lesson-" + string(rune('a'+i)), Tier: TierProbation, Text: "check internal/x" + string(rune('a'+i)) + ".go first", ApprovedUnixMS: 1})
	}
	for _, s := range [][]byte{
		seed(activeFile{Schema: "1", Lessons: []Lesson{provenCwd, probationNpm}}),
		seed(activeFile{Schema: "1", Quarantined: true, Lessons: []Lesson{provenCwd}}),
		seed(activeFile{Schema: "1", Lessons: many}),
		seed(activeFile{Lessons: []Lesson{{ID: "unapproved", Tier: TierProven, Text: "x"}, {ID: "proposed", Tier: "proposed", Text: "y", ApprovedUnixMS: 1}}}),
		seed(activeFile{Lessons: []Lesson{{ID: "host", Tier: TierProven, Text: "fetch from https://example.com/x", ApprovedUnixMS: 1}}}),
		[]byte(`{"lessons":[null,{},{"tier":"proven"}],"lessons":[{"id":"dup","tier":"proven","text":"t","approved_unix_ms":1}]}`),
		[]byte(`{`), []byte(`null`), []byte(``), []byte(`[]`),
		[]byte(`{"lessons":[{"id":"x","tier":"proven","text":"` + strings.Repeat("a", ActiveMaxBytes) + `"}]}`),
	} {
		f.Add(s, "internal/xc.go")
	}
	f.Fuzz(func(t *testing.T, active []byte, path string) {
		digests := []string{"-", "-", digestHex(active)} // stateFiles' order: lessons.json and labels.ndjson absent
		if err := os.WriteFile(filepath.Join(dir, FileActive), active, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, FileSum), []byte(signDigests(key, digests)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := ReadActive(root, project)
		switch {
		case err != nil:
			if a.Any() {
				t.Fatalf("ReadActive failed (%v) and still gave %+v", err, a)
			}
			return
		case len(active) > ActiveMaxBytes:
			if a.Unchecked != UncheckedSize || len(a.Lessons) != 0 {
				t.Fatalf("a %d-byte active.json gave %+v, want the size flag alone", len(active), a)
			}
			return
		case a.Unchecked != "":
			t.Fatalf("a signed active.json of %d bytes read as unchecked %q", len(active), a.Unchecked)
		}
		if len(a.Lessons) > len(active)/2 {
			t.Fatalf("%d bytes gave %d lessons", len(active), len(a.Lessons))
		}
		for _, ls := range a.Lessons {
			if ls.Tier != TierProbation && ls.Tier != TierProven {
				t.Fatalf("ReadActive gave a %q row: %+v", ls.Tier, ls)
			}
		}
		ls, m, ok := Pick(a, func(string) bool { return false }, []string{path})
		if !ok {
			return
		}
		held := false
		for _, r := range a.Lessons {
			held = held || r == ls
		}
		if !held || ls.ApprovedUnixMS == 0 {
			t.Fatalf("Pick gave %+v, not an approved row ReadActive gave", ls)
		}
		if n := len(m.Text()); n == 0 || n > render.MaxMessageBytes {
			t.Fatalf("Pick's note for %q is %d bytes", ls.ID, n)
		}
		if again, _, ok := Pick(a, func(id string) bool { return id == ls.ID }, []string{path}); ok && again.ID == ls.ID {
			t.Fatalf("Pick gave %q again after it was delivered", ls.ID)
		}
	})
}
