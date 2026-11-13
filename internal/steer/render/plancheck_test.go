package render

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestPC1_PlanCheckRowNamesPathsAndRulesAndClaimsNoCreation: plan-check's one
// row (W-M2.6) says the plan names paths a rule covers, with the rule ids in
// the clear, and never says the plan creates or will write anything: the
// plan's text is a statement of intent, not a record of a write. The record
// form binds the same slot as keyed digests and renders as a count, so the
// steer line holds no path. Break: word the row as a creation, or let the
// digest form list a path, and this fails.
func TestPC1_PlanCheckRowNamesPathsAndRulesAndClaimsNoCreation(t *testing.T) {
	rules := slot.NewRuleRefs([]slot.RuleRef{
		{ID: "no-edit-secrets", Trust: slot.NewEnum(slot.EnumTrust, "trusted")},
	}, 0, 0, 0)
	m, err := New(pattern.PlanCheck, PlanCheckBinding{
		Paths: slot.NewPathSet([]slot.Path{"internal/x/new.go", "docs/a.md"}, 0, 0),
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.Pattern() != pattern.PlanCheck || m.Template() != "plan-check/v1" {
		t.Errorf("Pattern, Template = %q, %q; want plan-check, plan-check/v1", m.Pattern(), m.Template())
	}
	text := m.Text()
	for _, want := range []string{"the plan names", "'internal/x/new.go'", "'docs/a.md'", "no-edit-secrets"} {
		if !strings.Contains(text, want) {
			t.Errorf("Text() = %q, missing %q", text, want)
		}
	}
	for _, bad := range []string{"creates", "will create", "will write", "is created"} {
		if strings.Contains(text, bad) {
			t.Errorf("Text() = %q, claims %q", text, bad)
		}
	}
	rec, err := New(pattern.PlanCheck, PlanCheckBinding{
		Paths: slot.NewDigestPathSet([]string{strings.Repeat("a1", 32), strings.Repeat("b2", 32)}, 0, 0),
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("New (digest form): %v", err)
	}
	if strings.Contains(rec.Text(), "a1a1a1a1") || strings.Contains(rec.Text(), "internal/") {
		t.Errorf("digest form Text() = %q, lists a digest or a path", rec.Text())
	}
}
