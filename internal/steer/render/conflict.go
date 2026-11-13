package render

// The conflict note (W-M2.4 hook seam, card m2-seam1 D6): at the post of an
// edit that raises a conflict under a declared rule, the unit the edit is
// in, the conflict in the Stop line's words (slot.EnumConflictPhrase, the
// phrase internal/recap keeps for the kind) and the rule's id, in the Stop
// line's order: the rule, then what the edit did. A rule id the slot refuses is left out by the unruled
// rows, which say "a spec rule" as the Stop's line does. The record rows
// name no unit: at record scope v4 the steer line holds no path (the steer
// key names the conflict as a keyed digest). Kept apart from render.go so
// that file stays inside Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// ConflictBinding is a conflict's note: the unit's directory relative to the
// hook's working directory, the kind, and the rule.
type ConflictBinding struct {
	Unit slot.RelPath `slot:"unit"`
	Kind slot.Enum    `slot:"conflict"`
	Rule slot.RuleID  `slot:"rule"`
}

// ConflictUnruledBinding is ConflictBinding with no rule slot.
type ConflictUnruledBinding struct {
	Unit slot.RelPath `slot:"unit"`
	Kind slot.Enum    `slot:"conflict"`
}

// ConflictRecordBinding is the note's record form: no unit.
type ConflictRecordBinding struct {
	Kind slot.Enum   `slot:"conflict"`
	Rule slot.RuleID `slot:"rule"`
}

// ConflictUnruledRecordBinding is ConflictRecordBinding with no rule slot.
type ConflictUnruledRecordBinding struct {
	Kind slot.Enum `slot:"conflict"`
}

// templatesConflict is the conflict rows. Each states what the record holds
// about the edit, never an order (STR-14).
func templatesConflict() map[templateKey]template {
	return map[templateKey]template{
		{pattern.DeclaredFact, reflect.TypeOf(ConflictBinding{})}: {
			id:     "declared-fact-conflict/v1",
			format: "rashomon record: spec rule {rule}: this edit in unit {unit} {conflict}.",
		},
		{pattern.DeclaredFact, reflect.TypeOf(ConflictUnruledBinding{})}: {
			id:     "declared-fact-conflict-unruled/v1",
			format: "rashomon record: a spec rule: this edit in unit {unit} {conflict}.",
		},
		{pattern.DeclaredFact, reflect.TypeOf(ConflictRecordBinding{})}: {
			id:     "declared-fact-conflict-record/v1",
			format: "rashomon record: spec rule {rule}: an edit {conflict}.",
		},
		{pattern.DeclaredFact, reflect.TypeOf(ConflictUnruledRecordBinding{})}: {
			id:     "declared-fact-conflict-record-unruled/v1",
			format: "rashomon record: a spec rule: an edit {conflict}.",
		},
	}
}
