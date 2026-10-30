package learn

import "testing"

// TestLRN17h_RelevanceRule: one lesson per prompt (L4, PREFLIGHT1's step 6).
// With three approved lessons or fewer, the first in order not yet given
// arrives whatever the prompt names: proven before probation, then the
// ledger's order. With more, only a lesson whose text holds one of the
// prompt's path tokens arrives, and a prompt naming nothing gets none.
func TestLRN17h_RelevanceRule(t *testing.T) {
	verify := Lesson{ID: "vet-race", Tier: TierProbation, Text: "run internal/verify tests with -race", SinceUnixMS: 1_761_700_005_000}
	tidy := Lesson{ID: "tidy", Tier: TierProbation, Text: "run go mod tidy after editing go.mod", SinceUnixMS: 1_761_700_006_000}
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
	if want := "rashomon record: lesson go-test-cwd, approved for this project at a terminal: 'run go test from the module root'."; err != nil || m.Text() != want {
		t.Errorf("Message = %q, %v; want %q", m.Text(), err, want)
	}
	if _, _, ok := Pick(Active{Lessons: []Lesson{provenCwd}, Unchecked: UncheckedSignature}, none, nil); ok {
		t.Error("Pick gave a lesson from an unchecked state")
	}
}
