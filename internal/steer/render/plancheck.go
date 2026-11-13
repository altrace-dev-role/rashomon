package render

// The plan-check note (W-M2.6 hook half): after an accepted plan lands, the
// plan's named paths that a spec rule covers, with the rules' ids, in one
// line. Its path slot binds slot.PathSet; the record form binds the same slot
// as keyed digests (slot.NewDigestPathSet), so the steer line holds a count
// where the delivered note names paths. Kept apart from render.go so that
// file stays inside Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// PlanCheckBinding is plan-check's slots: the paths the plan names that a
// rule covers, at most three listed and the rest counted as "and N more", and
// the rules that cover them, their ids in the clear.
type PlanCheckBinding struct {
	Paths slot.PathSet  `slot:"paths"`
	Rules slot.RuleRefs `slot:"rules"`
}

// templatesPlanCheck is the one plan-check row. It says the plan names a
// path and a rule covers it, never that the plan creates anything (ruling 4).
func templatesPlanCheck() map[templateKey]template {
	return map[templateKey]template{
		{pattern.PlanCheck, reflect.TypeOf(PlanCheckBinding{})}: {
			id:     "plan-check/v1",
			format: "rashomon record: the plan names {paths}; rule {rules} covers it.",
		},
	}
}
