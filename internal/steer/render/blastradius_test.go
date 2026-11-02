package render

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestAI1_BlastRadiusRowsSayALowerBoundAndNoOrder: blast-radius's two rows
// (card ARCH-INLINE1) render the count as a lower bound ("at least": PROVEN
// calls only, PF-PL11), the callers' directories as a path list, and how
// many hold a test file, as a statement of fact with no "run" in it (the DE
// directive: an edit-time note never says "run Y" unless Y is bound). The
// degraded row is the full row without the list. Break: drop "at least", or
// word the note as an order, and this fails.
func TestAI1_BlastRadiusRowsSayALowerBoundAndNoOrder(t *testing.T) {
	cases := []struct {
		binding  any
		template string
		slots    []string
		want     string
	}{
		{
			BlastRadiusBinding{Count: 5, Dirs: slot.NewPathList([]slot.Path{"internal/verify", "cmd/rashomon", "internal/hook", "internal/recap"}, 1), Tested: 4},
			"blast-radius/v1", []string{"count", "dirs", "tested"},
			"rashomon record: the platform graph names other packages with code calling into the package this call edited " +
				"(at least 5): 'internal/verify', 'cmd/rashomon', 'internal/hook' and 2 more; test files are in 4 of them.",
		},
		{
			BlastRadiusDegradedBinding{Count: 1, Tested: 0},
			"blast-radius-degraded/v1", []string{"count", "tested"},
			"rashomon record: the platform graph names other packages with code calling into the package this call edited " +
				"(at least 1); test files are in 0 of them.",
		},
	}
	for _, c := range cases {
		m, err := New(pattern.BlastRadius, c.binding)
		if err != nil {
			t.Fatalf("New(%T): %v", c.binding, err)
		}
		if m.Text() != c.want {
			t.Errorf("Text() = %q\nwant      %q", m.Text(), c.want)
		}
		if m.Pattern() != pattern.BlastRadius || m.Template() != c.template {
			t.Errorf("Pattern, Template = %q, %q; want blast-radius, %q", m.Pattern(), m.Template(), c.template)
		}
		var names []string
		for _, s := range m.Slots() {
			names = append(names, s.Name)
		}
		if !slices.Equal(names, c.slots) {
			t.Errorf("Slots() names = %q, want %q", names, c.slots)
		}
	}
	if _, err := New(pattern.BlastRadius, BlastRadiusBinding{Count: 1, Dirs: slot.NewPathList(nil, 1), Tested: 0}); err == nil {
		t.Error("a list with no directory rendered; want slot.ErrNoPathsLeft, the degraded row's case")
	}
}
