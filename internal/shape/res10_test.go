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
	if got := leadingSteps(line(MaxLeadingCd)); got != MaxLeadingCd {
		t.Errorf("%d steps: leadingSteps %d", MaxLeadingCd, got)
	}
	if got := leadingSteps(line(MaxLeadingCd + 1)); got != 0 {
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
