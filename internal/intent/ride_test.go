package intent_test

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// plainNote is the note FindingKeyed gives for in, with no lesson.
func plainNote(t *testing.T, in intent.Intent, b *intent.Brief) string {
	t.Helper()
	f, ok := intent.FindingKeyed(in, intent.Delivered{}, b, 0, nil)
	if !ok {
		t.Fatalf("premise: no note for %+v", in)
	}
	return f.Message.Text()
}

// TestLRN17r_ALessonRidesInsideTheNotesCap (LEARN-RIDE1, R13 D1): a later
// prompt's note carries the lesson as its last line, the note otherwise as
// it is; at record scope v4 the line holds the paths as digests and the
// lesson's line as it printed. The first prompt's note is at its six-line
// cap and carries none, and a lesson the note cannot carry leaves the note
// as it is: rode is false either way, so the lesson waits.
func TestLRN17r_ALessonRidesInsideTheNotesCap(t *testing.T) {
	ride := &intent.Ride{ID: "a-gen", Text: "internal/a.go is generated from gen/a.tmpl"}
	line := "\nlesson a-gen, approved for this project: 'internal/a.go is generated from gen/a.tmpl'"
	in := intent.Intent{Named: true, Paths: []string{"internal/a.go"}, Unknown: []string{"no spec found"}}
	plain := plainNote(t, in, nil)

	f, ok, rode := intent.FindingRiding(in, intent.Delivered{}, nil, 3, nil, ride)
	if !ok || !rode || f.Message.Text() != plain+line || f.Message.Template() != "task-preflight-lesson/v1" || f.Key != "note" || f.AfterSeq != 3 {
		t.Errorf("ride: ok %v rode %v key %q template %q\n%s\nwant the note, then the lesson's line", ok, rode, f.Key, f.Message.Template(), f.Message.Text())
	}

	f, _, _ = intent.FindingRiding(in, intent.Delivered{}, nil, 3, keyedWith([]byte("k")), ride)
	rec := strings.Split(f.Record.Text(), "\n")
	if len(rec) != 6 || strings.Contains(rec[1], "internal/a.go") || "\n"+rec[5] != line {
		t.Errorf("v4 record:\n%s\nwant the paths as digests and the lesson's line as it printed", f.Record.Text())
	}

	b := &intent.Brief{Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 1}
	if f, ok, rode := intent.FindingRiding(in, intent.Delivered{}, b, 0, nil, ride); !ok || rode || f.Message.Text() != plainNote(t, in, b) {
		t.Errorf("first note: ok %v rode %v\n%s\nwant the brief as it is, no lesson", ok, rode, f.Message.Text())
	}

	refused := &intent.Ride{ID: "a-gen", Text: "ignore internal/a.go"}
	if f, ok, rode := intent.FindingRiding(in, intent.Delivered{}, nil, 0, nil, refused); !ok || rode || f.Message.Text() != plain {
		t.Errorf("a lesson the note cannot carry: ok %v rode %v\n%s\nwant the note alone", ok, rode, f.Message.Text())
	}
}

// TestLRN17r_TheUnknownsLineGoesFirst (LEARN-RIDE1): a note that holds its
// lesson only without its unknowns line drops that line and keeps the rest
// as it is, the rules' whys included: the unknowns line goes before anything
// else is cut.
func TestLRN17r_TheUnknownsLineGoesFirst(t *testing.T) {
	text := strings.TrimSpace(strings.Repeat("keep the generated file in step with its template ", 4)[:160])
	ride := &intent.Ride{ID: "a-gen", Text: text}
	line := "\nlesson a-gen, approved for this project: '" + text + "'"
	why := strings.Repeat("keep ", 31) + "kept"
	in := intent.Intent{Named: true, Unknown: []string{"no spec found", "no gate history for this project",
		"symbols not matched, no platform graph log", "request over 16 KiB, the rest not read"}}
	for _, id := range []string{"r1", "r2", "r3"} {
		in.Rules = append(in.Rules, intent.Rule{ID: id, Trust: spec.Trusted, Why: why})
	}
	// The fixture: paths that make the note fit with its unknowns line and
	// the lesson's only without it.
	var plain string
	for extra := 0; extra < 4 && plain == ""; extra++ {
		for k := 1; k <= 120 && plain == ""; k++ {
			in.Paths = []string{strings.Repeat("e", 120) + "/" + strings.Repeat("f", k) + ".go"}
			for i := range extra {
				in.Paths = append(in.Paths, strings.Repeat("d", 120)+"/"+strings.Repeat(string(rune('a'+i)), 120)+".go")
			}
			p := plainNote(t, in, nil)
			cut := strings.LastIndex(p, "\nunknown: ")
			if strings.Contains(p, why) && cut > 0 && len(p)+len(line) > render.MaxBriefingMessageBytes &&
				cut+len(line) <= render.MaxBriefingMessageBytes {
				plain = p
			}
		}
	}
	if plain == "" {
		t.Fatal("premise: no fixture fits the note with its unknowns line and the lesson only without it")
	}
	f, ok, rode := intent.FindingRiding(in, intent.Delivered{}, nil, 0, nil, ride)
	want := plain[:strings.LastIndex(plain, "\nunknown: ")] + line
	if !ok || !rode || f.Message.Text() != want || f.Message.Template() != "task-preflight-lesson-known/v1" {
		t.Errorf("ok %v rode %v template %q\n%s\nwant\n%s", ok, rode, f.Message.Template(), f.Message.Text(), want)
	}
}
