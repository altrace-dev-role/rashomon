package shape

import (
	"strconv"
	"strings"
	"testing"
)

// TestStatusMaskedRunnerGroupsAreCapped: statusMasked's abstract run re-reads
// the whole parsed line once per distinctly-worded runner group (runShells),
// so a line of many groups is the product of group count and line size. A
// line of 2,000 runners each piped to a later stage -- `make t0|tail;make
// t1|tail;...` -- measured 1,104 MiB and 574 ms before maxRunnerGroups
// existed; capped, the same line allocates a few MiB. The gate is
// allocation, not wall time, as the rest of this package's RES-29 tests are.
func TestStatusMaskedRunnerGroupsAreCapped(t *testing.T) {
	line := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString("make t")
			b.WriteString(strconv.Itoa(i))
			b.WriteString("|tail;")
		}
		return b.String()
	}
	const bound = 64 << 20 // measured ~5.6 MiB capped; 2,000 groups uncapped measured 1,104 MiB
	in := bashInput(t, line(2000))
	if alloc := allocated(func() { Derive("Bash", in, benchKey) }); alloc > bound {
		t.Errorf("2,000 distinctly-worded runners allocated %d MiB, want at most %d: past maxRunnerGroups this is the square of the line, not its size", alloc>>20, bound>>20)
	}
	s := Derive("Bash", in, benchKey)
	if s.StatusMasked != nil {
		t.Errorf("status_masked %v past maxRunnerGroups, want null", show(s.StatusMasked))
	}
	// At and one past maxRunnerGroups' own value (64, written here as a
	// literal so a mutant that changes the constant does not also change
	// how many groups this case builds): a plain runner's status reaches
	// the line, so joined entirely by && it is MaskedNone up to the cap,
	// and null one past it.
	andLine := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = "make t" + strconv.Itoa(i)
		}
		return strings.Join(parts, " && ")
	}
	if s := Derive("Bash", bashInput(t, andLine(64)), benchKey); s.StatusMasked == nil || *s.StatusMasked != MaskedNone {
		t.Errorf("64 distinctly-worded runners joined by &&: status_masked %v, want %q", show(s.StatusMasked), MaskedNone)
	}
	if s := Derive("Bash", bashInput(t, andLine(65)), benchKey); s.StatusMasked != nil {
		t.Errorf("65 distinctly-worded runners: status_masked %v, want null", show(s.StatusMasked))
	}
}
