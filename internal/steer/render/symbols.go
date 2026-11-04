package render

// The task-preflight rows with the symbols line (PW1b-ON-230 step 3,
// WED-SLICE row 1(c)): their bindings and templates, kept apart from
// render.go so each file stays inside Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TaskPreflightSymbolsBinding is the later-prompt note with the symbols the
// request named that the platform graph proves (PW1b-ON-230 step 3) as the
// line before its unknowns.
type TaskPreflightSymbolsBinding struct {
	Paths    slot.PathSet     `slot:"paths"`
	Rules    slot.RuleRefs    `slot:"rules"`
	Gates    slot.GateRefs    `slot:"gates"`
	Source   slot.Enum        `slot:"source"`
	Evidence slot.EnumList    `slot:"evidence"`
	Symbols  slot.SymbolFacts `slot:"symbols"`
	Unknown  slot.EnumList    `slot:"unknown"`
}

// TaskPreflightSymbolsKnownBinding is TaskPreflightSymbolsBinding without
// the unknowns line, which goes first when the note does not fit.
type TaskPreflightSymbolsKnownBinding struct {
	Paths    slot.PathSet     `slot:"paths"`
	Rules    slot.RuleRefs    `slot:"rules"`
	Gates    slot.GateRefs    `slot:"gates"`
	Source   slot.Enum        `slot:"source"`
	Evidence slot.EnumList    `slot:"evidence"`
	Symbols  slot.SymbolFacts `slot:"symbols"`
}

// TaskPreflightSymbolsLessonBinding is the symbols note with a lesson as its
// last line in place of the unknowns (LEARN-RIDE1's ride, at the six-line
// cap).
type TaskPreflightSymbolsLessonBinding struct {
	Paths    slot.PathSet     `slot:"paths"`
	Rules    slot.RuleRefs    `slot:"rules"`
	Gates    slot.GateRefs    `slot:"gates"`
	Source   slot.Enum        `slot:"source"`
	Evidence slot.EnumList    `slot:"evidence"`
	Symbols  slot.SymbolFacts `slot:"symbols"`
	Lesson   slot.LessonID    `slot:"lesson"`
	Text     slot.Approved    `slot:"text"`
}

// TaskPreflightFirstSymbolsBinding is the first prompt's note with the
// symbols line in place of the unknowns: its six lines are the cap, and a
// fact the graph proves goes to the model before what the preflight could
// not read.
type TaskPreflightFirstSymbolsBinding struct {
	Instructions slot.PathSet     `slot:"instructions"`
	Manifests    slot.PathSet     `slot:"manifests"`
	GoDirs       slot.Count       `slot:"go_dirs"`
	Paths        slot.PathSet     `slot:"paths"`
	Rules        slot.RuleRefs    `slot:"rules"`
	Gates        slot.GateRefs    `slot:"gates"`
	Source       slot.Enum        `slot:"source"`
	Evidence     slot.EnumList    `slot:"evidence"`
	Symbols      slot.SymbolFacts `slot:"symbols"`
}

// symbolsLine is the line the symbols the request named take, after the
// gates and before the unknowns (PW1b-ON-230 step 3, WED-SLICE row 1(c)).
const symbolsLine = "\nsymbols named: {symbols}"

// templatesSymbols is the four task-preflight rows that carry the symbols
// line: the later prompts' note with and without its unknowns, with a lesson
// in the unknowns' place, and the first prompt's in the unknowns' place.
func templatesSymbols() map[templateKey]template {
	return map[templateKey]template{
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightSymbolsBinding{})}: {
			id:     "task-preflight-symbols/v1",
			format: preflightHead + preflightKnown + symbolsLine + "\nunknown: {unknown}",
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightSymbolsKnownBinding{})}: {
			id:     "task-preflight-symbols-known/v1",
			format: preflightHead + preflightKnown + symbolsLine,
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightSymbolsLessonBinding{})}: {
			id:     "task-preflight-symbols-lesson/v1",
			format: preflightHead + preflightKnown + symbolsLine + lessonRide,
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightFirstSymbolsBinding{})}: {
			id:     "task-preflight-first-symbols/v1",
			format: firstHead + preflightKnown + symbolsLine,
		},
	}
}
