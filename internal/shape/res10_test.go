package shape

import (
	"strings"
	"testing"
)

// TestRES10_RunnerPastTheFoldIsNotPaired: the cwd digest holds at most
// MaxLeadingCd leading cd steps (the hook folds no more, and records the
// directory as unknown past them), so the masked-status walk counts no more
// as held either. At 64 steps the runner after them is digested as running
// where the cwd digest says; at 65 where it ran is not known, and its runner
// digest is null rather than one that pairs with a run somewhere else.
func TestRES10_RunnerPastTheFoldIsNotPaired(t *testing.T) {
	line := func(n int) string { return strings.Repeat("cd a && ", n) + "make test" }
	if got := leadingSteps(dirsOf(line(MaxLeadingCd))); got != MaxLeadingCd {
		t.Errorf("%d steps: leadingSteps %d", MaxLeadingCd, got)
	}
	if got := leadingSteps(dirsOf(line(MaxLeadingCd + 1))); got != 0 {
		t.Errorf("%d steps: leadingSteps %d, want 0: past the bound no step is held", MaxLeadingCd+1, got)
	}
	if s := Derive("Bash", bashInput(t, line(MaxLeadingCd)), benchKey); s.RunnerDigest == nil {
		t.Errorf("%d steps: no runner digest", MaxLeadingCd)
	}
	if s := Derive("Bash", bashInput(t, line(MaxLeadingCd+1)), benchKey); s.RunnerDigest != nil {
		t.Errorf("%d steps: runner digest %s, want null", MaxLeadingCd+1, *s.RunnerDigest)
	}
}

// TestRES10_ProgramPastManyDirectoryChanges: the program search moves past
// every leading `cd DIR &&` however many there are -- the cd bound is the
// fold's, not the program's -- and does it in a loop: 14,000 steps, the most
// the token bound lets through, name go.
func TestRES10_ProgramPastManyDirectoryChanges(t *testing.T) {
	s := Derive("Bash", bashInput(t, strings.Repeat("cd a && ", 14_000)+"go test ./..."), benchKey)
	if s.Program == nil || *s.Program != "go" {
		t.Errorf("program %v, want go", show(s.Program))
	}
}

// TestRES10_LeadingStepsPastTheTokenBound: a line that runs on past the
// token bound (RES-9) still has its leading steps read, since they come first
// and the longest run read, MaxLeadingCd+1 steps of three tokens, lies far
// inside the bound. The masked-status walk counts the same steps the hook
// folds into the cwd digest: one cd in front of a 50,000-word here-document
// is one step held, and 100 in front of 50,000 words are past the fold.
func TestRES10_LeadingStepsPastTheTokenBound(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 50_000))
	heredoc := "cd sub && python3 - <<'EOF'\n" + long + "\nEOF\nmake test"
	if _, err := tokenizeProgram(heredoc); err != errTokenBound {
		t.Fatalf("the here-document line reads with %v, want errTokenBound", err)
	}
	if got := leadingSteps(dirsOf(heredoc)); got != 1 {
		t.Errorf("one cd before a long here-document: leadingSteps %d, want 1", got)
	}
	dirs, ok := leadingDirectories(strings.Repeat("cd a && ", 100) + long)
	if !ok || len(dirs) != MaxLeadingCd+1 {
		t.Errorf("100 cds before 50,000 words: %d directories, %v; want %d, true", len(dirs), ok, MaxLeadingCd+1)
	}
}

// dirsOf is a line's leading directories, as Derive reads them.
func dirsOf(line string) []string {
	dirs, _ := leadingDirectories(line)
	return dirs
}
