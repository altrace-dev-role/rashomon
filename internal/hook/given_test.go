package hook

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestGiven_OneRuleForEveryCaller (ruling 04:02 on D6): the three notes said
// once per key read the steer log through the one walk, and one table drives
// all three over the same states: delivered, held out, withheld by the off
// arm at nudge and below it, withheld for another cause, superseded, another
// subject's key (a unit whose id runs on past "@" among them), another
// pattern, and no line. The callers part only where their parameter says:
// an arm_off line below nudge is given to blast and plan and owed to the
// unit brief, which counts a withheld note as measure does.
func TestGiven_OneRuleForEveryCaller(t *testing.T) {
	const (
		blastKey = "example.com/a@0123456789ab"
		planKey  = "plan@p1"
		ubPrefix = "unitbrief:g:r#unit:go:a@"
		ubKey    = ubPrefix + "0123456789ab"
	)
	type keys struct{ blast, plan, ub string }
	own := keys{blastKey, planKey, ubKey}
	cases := []struct {
		name                   string
		state, delivery, level string
		by                     steer.SuppressedBy
		keys                   keys
		pat                    pattern.ID // "" for each caller's own
		none                   bool
		blast, plan, ub        bool
	}{
		{name: "delivered", state: "delivered", delivery: "delivered", level: "nudge", keys: own, blast: true, plan: true, ub: true},
		{name: "held out", state: "delivered", delivery: "holdout", level: "nudge", keys: own, blast: true, plan: true, ub: true},
		{name: "arm_off at nudge", state: "suppressed", level: "nudge", by: steer.SuppressedByArmOff, keys: own, blast: true, plan: true, ub: true},
		{name: "arm_off at flag", state: "suppressed", level: "flag", by: steer.SuppressedByArmOff, keys: own, blast: true, plan: true, ub: false},
		{name: "withheld for another cause", state: "suppressed", level: "nudge", by: steer.SuppressedByBudgetPrompt, keys: own},
		{name: "superseded", state: "superseded", level: "nudge", keys: own},
		{name: "another subject", state: "delivered", delivery: "delivered", level: "nudge",
			keys: keys{"example.com/ab@0123456789ab", "plan@p2", "unitbrief:g:r#unit:go:ab@0123456789ab"}},
		{name: "a unit id past @", state: "delivered", delivery: "delivered", level: "nudge",
			keys: keys{"example.com/a@v1@0123456789ab", "plan@p1@x", ubPrefix + "v1@0123456789ab"}},
		{name: "another pattern", state: "delivered", delivery: "delivered", level: "nudge", keys: own, pat: pattern.RegressionLocation},
		{name: "no line", none: true},
	}
	line := func(t *testing.T, pat pattern.ID, key, state, delivery, level string, by steer.SuppressedBy) [][]byte {
		m := map[string]any{"type": "steer", "pattern": pat, "key": key, "state": state, "level": level, "suppressed_by": nil}
		if delivery != "" {
			m["delivery"] = delivery
		}
		if by != "" {
			m["suppressed_by"] = by
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return [][]byte{b}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := func(own pattern.ID, key string) [][]byte {
				if c.none {
					return nil
				}
				pat := own
				if c.pat != "" {
					pat = c.pat
				}
				return line(t, pat, key, c.state, c.delivery, c.level, c.by)
			}
			last := ""
			Given(lines(pattern.BlastRadius, c.keys.blast), pattern.BlastRadius, "example.com/a@", false, func(rest string) { last = rest })
			if got := last == "0123456789ab"; got != c.blast {
				t.Errorf("blast: given %v, want %v", got, c.blast)
			}
			if got := planGiven(lines(pattern.PlanCheck, c.keys.plan), planKey); got != c.plan {
				t.Errorf("plan: given %v, want %v", got, c.plan)
			}
			ub := false
			Given(lines(pattern.TaskPreflight, c.keys.ub), pattern.TaskPreflight, ubPrefix, true, func(string) { ub = true })
			if ub != c.ub {
				t.Errorf("unit brief: given %v, want %v", ub, c.ub)
			}
		})
	}
}
