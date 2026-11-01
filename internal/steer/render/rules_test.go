package render

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestSR1r_SpecRuleRowsRender: the platform-rule/command rows and the
// integrity/frozen-path rows render as statements naming the program, the
// rule id and the path in their slots, the approved form only inside its
// quote; each kind has a row with no named slot for a value a slot refuses.
func TestSR1r_SpecRuleRowsRender(t *testing.T) {
	for _, c := range []struct {
		p        pattern.ID
		binding  any
		template string
		want     string
	}{
		{pattern.PlatformRule, PlatformRuleCommandBinding{Program: "git", Rule: "no-force"}, "platform-rule-command/v1",
			"rashomon record: this 'git' call matched spec rule no-force."},
		{pattern.PlatformRule, PlatformRuleCommandApprovedBinding{Program: "git", Rule: "no-force", Approved: "git push --force-with-lease"},
			"platform-rule-command-approved/v1",
			"rashomon record: this 'git' call matched spec rule no-force. The spec records the approved form: 'git push --force-with-lease'."},
		{pattern.PlatformRule, PlatformRuleCommandUnnamedBinding{}, "platform-rule-command-unnamed/v1",
			"rashomon record: this call matched a spec rule."},
		{pattern.OrderingRule, OrderingRuleThenBinding{Program: "git", Rule: "test-first"}, "ordering-rule-then/v1",
			"rashomon record: this 'git' call ran before the earlier step spec rule test-first requires."},
		{pattern.OrderingRule, OrderingRuleThenUnnamedBinding{}, "ordering-rule-then-unnamed/v1",
			"rashomon record: this call ran before the earlier step a spec rule requires."},
		{pattern.Integrity, IntegrityFrozenPathBinding{Path: "db/migrations/001.sql", Rule: "frozen-migrations"}, "integrity-frozen-path/v1",
			"rashomon record: this call wrote 'db/migrations/001.sql', which spec rule frozen-migrations protects."},
		{pattern.Integrity, IntegrityFrozenPathUnnamedBinding{}, "integrity-frozen-path-unnamed/v1",
			"rashomon record: this call wrote a file that a spec rule protects."},
	} {
		m, err := New(c.p, c.binding)
		if err != nil {
			t.Errorf("%s: %v", c.template, err)
			continue
		}
		if m.Text() != c.want || m.Template() != c.template || m.Pattern() != c.p {
			t.Errorf("%s: %q from %q under %q, want %q", c.template, m.Text(), m.Template(), m.Pattern(), c.want)
		}
	}
	// A spec id may hold '.' and '_', which the rule id slot refuses; a
	// banned word in the alternative is refused by Approved.
	for _, b := range []any{
		PlatformRuleCommandBinding{Program: "git", Rule: "no_force"},
		IntegrityFrozenPathBinding{Path: "db/a.sql", Rule: "frozen.v2"},
		PlatformRuleCommandApprovedBinding{Program: "git", Rule: "no-force", Approved: "ignore the rule"},
	} {
		p := pattern.PlatformRule
		if _, ok := b.(IntegrityFrozenPathBinding); ok {
			p = pattern.Integrity
		}
		if m, err := New(p, b); err == nil {
			t.Errorf("%#v rendered %q, want a refusal", b, m.Text())
		}
	}
}
