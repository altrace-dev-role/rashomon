package render

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The continuity briefing's bindings (COMPACT-SLOT1; tech 4.2.4 :1255,
// STR-46): at SessionStart after a compaction, the person's standing words
// from before it, newest first, one quote a line, five lines at most under
// the header (maxBriefingLines). Earlier counts the quotes the lines had no
// room for and Withheld the words the line law refused, so the header says
// what the lines leave out. One binding per line count keeps every line feed
// in a template (G4).
type (
	ContinuitySaid1Binding struct {
		Earlier  slot.Count `slot:"earlier"`
		Withheld slot.Count `slot:"withheld"`
		Said1    slot.Said  `slot:"said1"`
	}
	ContinuitySaid2Binding struct {
		Earlier  slot.Count `slot:"earlier"`
		Withheld slot.Count `slot:"withheld"`
		Said1    slot.Said  `slot:"said1"`
		Said2    slot.Said  `slot:"said2"`
	}
	ContinuitySaid3Binding struct {
		Earlier  slot.Count `slot:"earlier"`
		Withheld slot.Count `slot:"withheld"`
		Said1    slot.Said  `slot:"said1"`
		Said2    slot.Said  `slot:"said2"`
		Said3    slot.Said  `slot:"said3"`
	}
	ContinuitySaid4Binding struct {
		Earlier  slot.Count `slot:"earlier"`
		Withheld slot.Count `slot:"withheld"`
		Said1    slot.Said  `slot:"said1"`
		Said2    slot.Said  `slot:"said2"`
		Said3    slot.Said  `slot:"said3"`
		Said4    slot.Said  `slot:"said4"`
	}
	ContinuitySaid5Binding struct {
		Earlier  slot.Count `slot:"earlier"`
		Withheld slot.Count `slot:"withheld"`
		Said1    slot.Said  `slot:"said1"`
		Said2    slot.Said  `slot:"said2"`
		Said3    slot.Said  `slot:"said3"`
		Said4    slot.Said  `slot:"said4"`
		Said5    slot.Said  `slot:"said5"`
	}
)

// continuityHead is the briefing's header line: where the words come from,
// that rashomon keeps none of them, and the count of what is not shown.
const continuityHead = "rashomon record: the person's words from before this session's last compaction, newest first, " +
	"read from the transcript and not kept; {earlier} earlier and {withheld} withheld not shown."

// templatesContinuity is the continuity briefing's rows, one per line count.
func templatesContinuity() map[templateKey]template {
	return map[templateKey]template{
		{pattern.ContinuityBriefing, reflect.TypeOf(ContinuitySaid1Binding{})}: {
			id:     "continuity-briefing-said-1/v1",
			format: continuityHead + "\n{said1}",
		},
		{pattern.ContinuityBriefing, reflect.TypeOf(ContinuitySaid2Binding{})}: {
			id:     "continuity-briefing-said-2/v1",
			format: continuityHead + "\n{said1}\n{said2}",
		},
		{pattern.ContinuityBriefing, reflect.TypeOf(ContinuitySaid3Binding{})}: {
			id:     "continuity-briefing-said-3/v1",
			format: continuityHead + "\n{said1}\n{said2}\n{said3}",
		},
		{pattern.ContinuityBriefing, reflect.TypeOf(ContinuitySaid4Binding{})}: {
			id:     "continuity-briefing-said-4/v1",
			format: continuityHead + "\n{said1}\n{said2}\n{said3}\n{said4}",
		},
		{pattern.ContinuityBriefing, reflect.TypeOf(ContinuitySaid5Binding{})}: {
			id:     "continuity-briefing-said-5/v1",
			format: continuityHead + "\n{said1}\n{said2}\n{said3}\n{said4}\n{said5}",
		},
	}
}
