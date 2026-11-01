package intent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestPF1w_RecordFormNeverStepsTheNoteDown: FindingKeyed renders the record
// form beside the note at each step, and a record form that failed where the
// note rendered would step the note down, or drop it, with no trace (#170
// F7). It cannot: the record form is the note's binding with each path list
// as a count and keyed digests, which are 64 hex by construction, and a count
// is never longer than the paths it counts. So at record scope v4 the note
// that prints is the one v5 prints, at each of the three steps, first note
// or not, paths given earlier or not.
func TestPF1w_RecordFormNeverStepsTheNoteDown(t *testing.T) {
	root := projectTree(t)
	var long []string
	for i := range 8 {
		rel := "internal/" + strings.Repeat("d", 120) + "/" + strings.Repeat(string(rune('a'+i)), 120) + ".go"
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		long = append(long, rel)
	}
	var instructions []string
	for i := range 3 {
		instructions = append(instructions, "docs/"+strings.Repeat("i", 120)+"/"+strings.Repeat(string(rune('a'+i)), 120)+".md")
	}
	short := projectOf(root, specOf(root, spec.Trusted), indexOf())
	wide := projectOf(root, longSpec(root), indexOf())
	digest := keyedWith([]byte("key-one-key-one-key-one-key-one!"))
	brief := &intent.Brief{Instructions: []string{"CLAUDE.md"}, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 2}
	wideBrief := &intent.Brief{Instructions: instructions, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 2}
	given := intent.Delivered{Paths: map[string]bool{"internal/verify/stop.go": true, digest("internal/verify/stop.go"): true}}
	for _, c := range []struct {
		name   string
		p      intent.Project
		prompt string
		d      intent.Delivered
		b      *intent.Brief
	}{
		{"one path", short, "fix internal/verify/stop.go", intent.Delivered{}, nil},
		{"four paths, a workflow and a test data file among them", short,
			"fix internal/verify/stop.go internal/verify/index.go .github/workflows/ci.yaml internal/testdata/x.json", intent.Delivered{}, nil},
		{"the first note, with its brief", short, "fix internal/verify/stop.go and internal/verify/index.go", intent.Delivered{}, brief},
		{"a path given earlier", short, "fix internal/verify/stop.go and internal/verify/index.go", given, nil},
		{"step 1: whys dropped", wide, "fix " + strings.Join(long, " "), intent.Delivered{}, brief},
		{"step 2: one item per list", wide, "fix " + strings.Join(long, " "), intent.Delivered{}, wideBrief},
	} {
		in := intent.Resolve([]byte(c.prompt), false, c.p)
		plain, ok := intent.Finding(in, c.d, c.b, 1)
		if !ok {
			t.Fatalf("%s: no note at v5; the fixture names nothing new", c.name)
		}
		keyed, ok := intent.FindingKeyed(in, c.d, c.b, 1, digest)
		if !ok {
			t.Errorf("%s: a note at v5 and none at v4", c.name)
			continue
		}
		if plain.Message.Text() != keyed.Message.Text() {
			t.Errorf("%s: the record form changed the note that prints:\nv5 %q\nv4 %q", c.name, plain.Message.Text(), keyed.Message.Text())
		}
		if keyed.Record.Text() == "" || keyed.Record.Text() == keyed.Message.Text() {
			t.Errorf("%s: v4 holds no record form apart from the note: %q", c.name, keyed.Record.Text())
		}
	}
}
