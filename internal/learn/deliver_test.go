package learn

import (
	"fmt"
	"testing"
)

// TestLRN17h_RelevanceRule: one lesson per prompt (L4, PREFLIGHT1's step 6).
// With three approved lessons or fewer, the first in order not yet given
// arrives whatever the prompt names: proven before probation, then the
// ledger's order. With more, only a lesson whose text holds one of the
// prompt's path tokens arrives, and a prompt naming nothing gets none.
func TestLRN17h_RelevanceRule(t *testing.T) {
	verify := Lesson{ID: "vet-race", Tier: TierProbation, Text: "run internal/verify tests with -race", SinceUnixMS: 1_761_700_005_000, ApprovedUnixMS: 1_761_700_005_500}
	tidy := Lesson{ID: "tidy", Tier: TierProbation, Text: "run go mod tidy after editing go.mod", SinceUnixMS: 1_761_700_006_000, ApprovedUnixMS: 1_761_700_006_500}
	none := func(string) bool { return false }
	for _, tc := range []struct {
		name    string
		lessons []Lesson
		given   func(string) bool
		paths   []string
		want    string
	}{
		{"three, naming nothing: proven first", []Lesson{probationNpm, provenCwd, tidy}, none, nil, "go-test-cwd"},
		{"three, the first given: the next", []Lesson{probationNpm, provenCwd, tidy}, func(id string) bool { return id == "go-test-cwd" }, nil, "npm-ci"},
		{"four, one naming the path", []Lesson{provenCwd, probationNpm, tidy, verify}, none, []string{"cmd/x.go", "internal/verify"}, "vet-race"},
		{"four, naming nothing", []Lesson{provenCwd, probationNpm, tidy, verify}, none, nil, ""},
		{"four, naming a path no lesson holds", []Lesson{provenCwd, probationNpm, tidy, verify}, none, []string{"internal/hook"}, ""},
		{"three, every one given", []Lesson{provenCwd, probationNpm}, func(string) bool { return true }, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ls, m, ok := Pick(Active{Lessons: tc.lessons}, tc.given, tc.paths)
			if ok != (tc.want != "") || ls.ID != tc.want {
				t.Fatalf("Pick = %q, %v; want %q", ls.ID, ok, tc.want)
			}
			if !ok {
				return
			}
			want, err := Message(ls)
			if err != nil || m.Text() != want.Text() || m.Text() == "" {
				t.Errorf("Pick's message %q, Message's %q (%v)", m.Text(), want.Text(), err)
			}
		})
	}
	m, err := Message(provenCwd)
	if want := "rashomon record: lesson go-test-cwd, approved for this project: 'run go test from the module root'."; err != nil || m.Text() != want {
		t.Errorf("Message = %q, %v; want %q", m.Text(), err, want)
	}
	unapproved := provenCwd
	unapproved.ApprovedUnixMS = 0
	if ls, _, ok := Pick(Active{Lessons: []Lesson{unapproved}}, none, nil); ok {
		t.Errorf("Pick gave %q, a lesson with no approval time: a signed file is not proof a person approved it", ls.ID)
	}
	if _, _, ok := Pick(Active{Lessons: []Lesson{provenCwd}, Unchecked: UncheckedSignature}, none, nil); ok {
		t.Error("Pick gave a lesson from an unchecked state")
	}
}

// TestLRN17r_ALessonRidesOnlyAPathItNames (LEARN-RIDE1, #197 F2): a lesson
// rides when its text names one of the request's paths whole, read as intent
// reads the request (a run of path bytes, trailing dots dropped), a leading
// "./" dropped. The request's paths are root-relative, so a longer token that
// holds one is another file: main.go is not cmd/domain.go, cmd/main.go or
// main.go.bak, and a directory does not name a file inside it.
// Break: match by substring, or by a /-separated suffix, and a lesson about
// one file rides, and spends its once a session, on a prompt about another.
func TestLRN17r_ALessonRidesOnlyAPathItNames(t *testing.T) {
	none := func(string) bool { return false }
	for _, c := range []struct {
		text  string
		paths []string
		want  bool
	}{
		{"cmd/domain.go is generated from gen/domain.tmpl", []string{"main.go"}, false},
		{"cmd/main.go must not import net/http", []string{"main.go"}, false},
		{"keep main.go.bak out of the build", []string{"main.go"}, false},
		{"internal/learn/ledger.go never decodes a line", []string{"internal/learn"}, false},
		{"cmd/domain.go is generated from gen/domain.tmpl", []string{"main.go", "cmd/domain.go"}, true},
		{"never edit gen/domain.tmpl.", []string{"gen/domain.tmpl"}, true},
		{"never edit ./cmd/domain.go by hand", []string{"cmd/domain.go"}, true},
		{"see cmd/domain.go:12 (generated)", []string{"cmd/domain.go"}, true},
		{"main.go, then cmd/main.go", []string{"main.go"}, true},
	} {
		ls := Lesson{ID: "dom-gen", Tier: TierProbation, Text: c.text, SinceUnixMS: 1, ApprovedUnixMS: 2}
		if _, err := Message(ls); err != nil {
			t.Fatalf("premise: %q renders: %v", c.text, err)
		}
		if _, ok := PickRide(Active{Lessons: []Lesson{ls}}, none, c.paths); ok != c.want {
			t.Errorf("PickRide(%q) for paths %q = %v, want %v", c.text, c.paths, ok, c.want)
		}
	}
}

// TestLRN17r_DeliveredReadsALessonThatRodeTheNote (LEARN-RIDE1, R13 D1): a
// lesson that rode the request's preflight note was given on that prompt, so
// the given set holds the lesson its task-preflight line names in its lesson
// slot, under the rule a learned-fact line has: delivered, or withheld by arm
// off; a superseded note gave nothing, and a note with no lesson names none.
func TestLRN17r_DeliveredReadsALessonThatRodeTheNote(t *testing.T) {
	line := func(pattern, key, state, by, slots string) []byte {
		b := "null"
		if by != "" {
			b = fmt.Sprintf("%q", by)
		}
		return []byte(fmt.Sprintf(`{"pattern":%q,"key":%q,"state":%q,"suppressed_by":%s,"slots":%s}`, pattern, key, state, b, slots))
	}
	ride := func(id string) string {
		return fmt.Sprintf(`{"paths":{"items":["internal/a.go"],"more":0,"earlier":0},"lesson":%q,"text":"internal/a.go is generated from gen/a.tmpl"}`, id)
	}
	got := Delivered([][]byte{
		line("task-preflight", "note", "delivered", "", ride("a-gen")),
		line("task-preflight", "note", "superseded", "budget_batch", ride("b-gen")),
		line("task-preflight", "brief", "suppressed", "arm_off", ride("c-gen")),
		line("task-preflight", "note", "delivered", "", `{"paths":{"items":["internal/b.go"],"more":0,"earlier":0}}`),
		line("task-preflight", "note", "delivered", "", `{"lesson":7}`),
		line("learned-fact", "d-gen", "delivered", "", `{"lesson":"d-gen","text":"run go test from the module root"}`),
	})
	want := map[string]bool{"a-gen": true, "c-gen": true, "d-gen": true}
	if len(got) != len(want) {
		t.Fatalf("Delivered = %v, want %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("Delivered = %v, want %s given", got, id)
		}
	}
}
