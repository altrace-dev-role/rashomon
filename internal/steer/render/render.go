// Package render is STEER's output gate for model-visible text: the closed
// Message every Finding carries, New that builds it from a pattern's slot
// binding, and the whole-string rules (tech 4.2.7; archive 40.3.8).
//
// A pattern binds its slots in a struct of internal/steer/render/slot types
// (tech 4.2.7: "Each pattern binds its slots in a struct of
// `internal/steer/render/slot` types only"). New fills the pattern's fixed
// template from the bound values, runs every slot check and the
// whole-string check, and returns the Message or an error, never both.
// Message's fields are unexported, so New is the only way to build one that
// is not the zero Message, and a Finding can carry only text that passed
// the gate. A pattern's PR adds its binding struct and its template row,
// never a message type (tech, Contracts and names, gaps table: "each struct
// in its pattern's PR").
//
// render imports internal/steer/pattern and internal/steer/render/slot,
// and nothing outside the standard library and this module.
package render

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// Message is a pattern's rendered claim, the closed
// `render.Message{Pattern, Template, Slots, Text}` of tech 4.2.7, read
// through its methods. The zero Message has no text and is never
// delivered.
type Message struct {
	pattern  pattern.ID
	template string
	slots    []Slot
	text     string
}

// Pattern is the pattern whose template made the message.
func (m Message) Pattern() pattern.ID { return m.pattern }

// Template is the id of the template New filled.
func (m Message) Template() string { return m.template }

// Slots returns a copy of the bound slots, in the binding struct's field
// order: what the steer line's `slots` key records.
func (m Message) Slots() []Slot { return slices.Clone(m.slots) }

// Text is the rendered string, which passed the whole-string check.
func (m Message) Text() string { return m.text }

// Slot is one bound slot: the name its template gives it and its value as
// the message holds it (a PathList without the paths it left out).
type Slot struct {
	Name  string
	Value slot.Value
}

// template is one fixed message form: an id and the text, with a {name}
// placeholder for each slot.
type template struct {
	id     string
	format string
}

// templateKey selects a template by pattern and binding struct type, so a
// card's degraded form is its own binding struct and its own row.
type templateKey struct {
	pattern pattern.ID
	binding reflect.Type
}

// Version is the renderer's version, written on every steer line as
// renderer_version (tech 4.2.3; its example writes 1). A suppressed line
// keeps its slots and this version and is re-rendered on read (RES-32), so
// a change to a row's text is a version change.
const Version = 1

// RegressionLocationBinding is regression-location's slots (archive
// 40.4.2): the gate, how long ago it last exited 0, its exit on this run, and
// the files written between the two runs. 2.3's producer fills it.
type RegressionLocationBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Exit    slot.Exit     `slot:"exit"`
	Paths   slot.PathList `slot:"paths"`
}

// RegressionLocationDegradedBinding is regression-location's no-paths form
// (card 2.3 R7): the full row's first sentence and the count of writes
// recorded between the two runs, for writes the record names no path for
// (record scope v4) or whose every named path the deny list or the entropy
// test left out (slot.ErrNoPathsLeft).
type RegressionLocationDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Exit    slot.Exit    `slot:"exit"`
	Count   slot.Count   `slot:"count"`
}

// RegressionLocationOutputBinding is regression-location's slots for a run
// whose outcome was read from its output because a pipe hid its exit status
// (D229): the gate, how long ago it last passed, and the files written
// between the two runs. No exit slot: the pipe's own status is not the run's.
type RegressionLocationOutputBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Paths   slot.PathList `slot:"paths"`
}

// RegressionLocationOutputDegradedBinding is the output form's no-paths row.
type RegressionLocationOutputDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Count   slot.Count   `slot:"count"`
}

// StaleEvidenceOutputBinding is stale-evidence's slots where the green was
// read from the run's output (D229).
type StaleEvidenceOutputBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Paths   slot.PathList `slot:"paths"`
}

// StaleEvidenceOutputDegradedBinding is the output form's no-paths row.
type StaleEvidenceOutputDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Count   slot.Count   `slot:"count"`
}

// StaleEvidenceBinding is stale-evidence's slots (card STALE-NUDGE1): the
// gate, how long ago its green run exited 0, and the files this call wrote
// after it. Only this call's files: a write another call declared beside it
// is never put in this call's sentence (INV-2).
type StaleEvidenceBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Paths   slot.PathList `slot:"paths"`
}

// StaleEvidenceDegradedBinding is stale-evidence's no-paths form, for a
// call whose every named path the deny list or the entropy test left out
// (slot.ErrNoPathsLeft): the count of the files it wrote.
type StaleEvidenceDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Count   slot.Count   `slot:"count"`
}

// IntegrityStoreTouchedBinding is integrity/store-touched's user line
// (archive 40.4.7; card P5B, 5.7): the path under .claude/rashomon the call
// wrote, project-relative.
type IntegrityStoreTouchedBinding struct {
	Path slot.Path `slot:"path"`
}

// IntegrityStoreTouchedUnnamedBinding is store-touched's no-path form, for a
// path the fact or render slot refuses (its charset, the deny list, the
// entropy test).
type IntegrityStoreTouchedUnnamedBinding struct{}

// IntegrityStoreRootTouchedBinding is store-touched's line for a write under
// rashomon's store root rather than .claude/rashomon (D173): the path
// relative to that root, since a path slot is never absolute.
type IntegrityStoreRootTouchedBinding struct {
	Path slot.Path `slot:"path"`
}

// IntegrityStoreRootTouchedUnnamedBinding is that line's no-path form.
type IntegrityStoreRootTouchedUnnamedBinding struct{}

// IntegrityStoreRootNamedBinding is the store root's line for a call whose
// input named the root on a record that withholds its paths (D173b): a read
// there cannot be told from a write, so the line says named.
type IntegrityStoreRootNamedBinding struct{}

// IntegrityTestRemovedBinding, IntegrityGateWeakenedBinding and
// IntegrityGateConfigEditedBinding are the other three one-way doors' lines
// (PF-IV3). Their entries carry no path, so the lines name the kind only;
// none has a producer on a real session yet (D139, IV-PRODUCERS1).
type IntegrityTestRemovedBinding struct{}

type IntegrityGateWeakenedBinding struct{}

type IntegrityGateConfigEditedBinding struct{}

// LearnedFactBinding is learned-fact's note at a prompt (LEARN-DELIVER1): an
// approved lesson's id and its text, which renders in quotes.
type LearnedFactBinding struct {
	Lesson slot.LessonID `slot:"lesson"`
	Text   slot.Approved `slot:"text"`
}

// LearnedFactUncheckedBinding and LearnedFactOversizeBinding are
// learned-fact's two user flags for a ledger the prompt path could not read:
// its files fail their signature check, or one is over the size the prompt
// path reads. One struct each, since a row is keyed by its binding's type.
type LearnedFactUncheckedBinding struct{}

type LearnedFactOversizeBinding struct{}

// TaskPreflightBinding is task-preflight's later-prompt note (internal/intent).
type TaskPreflightBinding struct {
	Paths    slot.PathSet  `slot:"paths"`
	Rules    slot.RuleRefs `slot:"rules"`
	Gates    slot.GateRefs `slot:"gates"`
	Source   slot.Enum     `slot:"source"`
	Evidence slot.EnumList `slot:"evidence"`
	Unknown  slot.EnumList `slot:"unknown"`
}

// TaskPreflightFirstBinding is task-preflight's first-prompt note.
type TaskPreflightFirstBinding struct {
	Instructions slot.PathSet  `slot:"instructions"`
	Manifests    slot.PathSet  `slot:"manifests"`
	GoDirs       slot.Count    `slot:"go_dirs"`
	Paths        slot.PathSet  `slot:"paths"`
	Rules        slot.RuleRefs `slot:"rules"`
	Gates        slot.GateRefs `slot:"gates"`
	Source       slot.Enum     `slot:"source"`
	Evidence     slot.EnumList `slot:"evidence"`
	Unknown      slot.EnumList `slot:"unknown"`
}

// TaskPreflightLessonBinding is the later-prompt note with an approved lesson
// as its last line, one whose text names a path the request named
// (LEARN-RIDE1): the lesson arrives on the prompt it is about.
type TaskPreflightLessonBinding struct {
	Paths    slot.PathSet  `slot:"paths"`
	Rules    slot.RuleRefs `slot:"rules"`
	Gates    slot.GateRefs `slot:"gates"`
	Source   slot.Enum     `slot:"source"`
	Evidence slot.EnumList `slot:"evidence"`
	Unknown  slot.EnumList `slot:"unknown"`
	Lesson   slot.LessonID `slot:"lesson"`
	Text     slot.Approved `slot:"text"`
}

// TaskPreflightLessonKnownBinding is TaskPreflightLessonBinding without the
// unknowns line, the note's first line to go when the lesson's does not fit.
type TaskPreflightLessonKnownBinding struct {
	Paths    slot.PathSet  `slot:"paths"`
	Rules    slot.RuleRefs `slot:"rules"`
	Gates    slot.GateRefs `slot:"gates"`
	Source   slot.Enum     `slot:"source"`
	Evidence slot.EnumList `slot:"evidence"`
	Lesson   slot.LessonID `slot:"lesson"`
	Text     slot.Approved `slot:"text"`
}

// PlatformRuleCommandBinding is platform-rule/command's line (archive
// 40.4.6): the program the call ran and the id of the forbid_command rule
// its line matched, the id without its "spec:" prefix.
type PlatformRuleCommandBinding struct {
	Program slot.Program `slot:"program"`
	Rule    slot.RuleID  `slot:"rule_id"`
}

// PlatformRuleCommandApprovedBinding is that line and the form a trusted
// rule records instead, its alternative (archive 20.2.8: the one rule text
// that may reach the model, and only through the Approved slot).
type PlatformRuleCommandApprovedBinding struct {
	Program  slot.Program  `slot:"program"`
	Rule     slot.RuleID   `slot:"rule_id"`
	Approved slot.Approved `slot:"approved"`
}

// PlatformRuleCommandUnnamedBinding is the command line for a program or a
// rule id its slot refuses.
type PlatformRuleCommandUnnamedBinding struct{}

// IntegrityFrozenPathBinding is integrity/frozen-path's user line: the path
// the call wrote, project-relative, and the id of the protect_path rule
// whose paths match it.
type IntegrityFrozenPathBinding struct {
	Path slot.Path   `slot:"path"`
	Rule slot.RuleID `slot:"rule_id"`
}

// IntegrityFrozenPathUnnamedBinding is the frozen-path line for a path or a
// rule id its slot refuses.
type IntegrityFrozenPathUnnamedBinding struct{}

// OrderingRuleThenBinding is ordering-rule's line: the program of the call
// that ran before the earlier step an order rule requires, and the id of the
// rule.
type OrderingRuleThenBinding struct {
	Program slot.Program `slot:"program"`
	Rule    slot.RuleID  `slot:"rule_id"`
}

// OrderingRuleThenUnnamedBinding is that line for a program or a rule id its
// slot refuses.
type OrderingRuleThenUnnamedBinding struct{}

// templates is the closed template table. Each pattern's PR adds its rows;
// New refuses a binding that has none.
//
// regression-location/v1 is archive 40.4.2's model text with "gate" before
// the gate's quote: Gate renders inside a quote after a word that names it
// (P07N-11), so the archive's bare {gate} opening cannot render as written.
// regression-location-degraded/v1 keeps that first sentence and states the
// count, so the note still names the gate (card 2.3 R7). The other-agents
// sentence is its own binding struct and row, which comes with
// subagent-claim's producer (D62). The integrity rows are archive 40.4.7's
// user lines for PF-IV3's four one-way doors (card P5B, 5.7); user-only,
// so no model text exists for them. stale-evidence/v1 states the green and
// the write after it and stops there: the card's "Run it again before
// claiming done" is an order, which STR-14 refuses, and the last sentence
// says the same as a fact. learned-fact/v1 is a lesson a person approved at a
// terminal, its text in quotes (LEARN-DELIVER1, L7); its two flags say why a
// ledger gave none.
// The platform-rule rows state which rule a command matched, never its why;
// the frozen-path rows are user lines, as every integrity row is. Each has a
// row without its named slots for a value a slot refuses.
func templates() map[templateKey]template {
	templatesOnce.Do(func() {
		templatesOnce.v = map[templateKey]template{
			{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationBinding{})}: {
				id: "regression-location/v1",
				format: "rashomon record: gate {gate} exited 0 earlier in this session ({minutes} min ago) and exited {exit} on this run. " +
					"Files written between the two runs: {paths}.",
			},
			{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationDegradedBinding{})}: {
				id: "regression-location-degraded/v1",
				format: "rashomon record: gate {gate} exited 0 earlier in this session ({minutes} min ago) and exited {exit} on this run. " +
					"{count} file writes were recorded between the two runs.",
			},
			{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationOutputBinding{})}: {
				id: "regression-location-output/v1",
				format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago) and its output reports a failure on this run " +
					"(read from its output; the pipe hid the exit status). Files written between the two runs: {paths}.",
			},
			{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationOutputDegradedBinding{})}: {
				id: "regression-location-output-degraded/v1",
				format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago) and its output reports a failure on this run " +
					"(read from its output; the pipe hid the exit status). {count} file writes were recorded between the two runs.",
			},
			{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceOutputBinding{})}: {
				id: "stale-evidence-output/v1",
				format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago), read from its output as the pipe hid the exit status; " +
					"this call then wrote {paths}. No later run of the gate is recorded.",
			},
			{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceOutputDegradedBinding{})}: {
				id: "stale-evidence-output-degraded/v1",
				format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago), read from its output as the pipe hid the exit status; " +
					"this call then wrote {count} files. No later run of the gate is recorded.",
			},
			{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceBinding{})}: {
				id: "stale-evidence/v1",
				format: "rashomon record: gate {gate} exited 0 earlier in this session ({minutes} min ago); this call then wrote {paths}. " +
					"No later run of the gate is recorded.",
			},
			{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceDegradedBinding{})}: {
				id: "stale-evidence-degraded/v1",
				format: "rashomon record: gate {gate} exited 0 earlier in this session ({minutes} min ago); this call then wrote {count} files. " +
					"No later run of the gate is recorded.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityStoreTouchedBinding{})}: {
				id:     "integrity-store-touched/v1",
				format: "rashomon record: this call wrote {path}, under .claude/rashomon, which holds the spec and the task file rashomon reads.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityStoreTouchedUnnamedBinding{})}: {
				id:     "integrity-store-touched-unnamed/v1",
				format: "rashomon record: this call wrote a file under .claude/rashomon, which holds the spec and the task file rashomon reads.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootTouchedBinding{})}: {
				id: "integrity-store-root-touched/v1",
				format: "rashomon record: this call wrote {path} in rashomon's store, which holds the trust bindings; " +
					"no repository spec counts as trusted for the rest of this session.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootTouchedUnnamedBinding{})}: {
				id: "integrity-store-root-touched-unnamed/v1",
				format: "rashomon record: this call wrote a file in rashomon's store, which holds the trust bindings; " +
					"no repository spec counts as trusted for the rest of this session.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootNamedBinding{})}: {
				id: "integrity-store-root-named/v1",
				format: "rashomon record: this call named rashomon's store, which holds the trust bindings; " +
					"no repository spec counts as trusted for the rest of this session.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityTestRemovedBinding{})}: {
				id:     "integrity-test-deleted/v1",
				format: "rashomon record: this edit removed a test after a gate run in this session exited non-zero.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityGateWeakenedBinding{})}: {
				id:     "integrity-gate-weakened/v1",
				format: "rashomon record: this call ran a gate with a flag or variable that skips its checks.",
			},
			{pattern.Integrity, reflect.TypeOf(IntegrityGateConfigEditedBinding{})}: {
				id:     "integrity-gate-config-edited/v1",
				format: "rashomon record: this edit changed a CI or test-configuration file.",
			},
		}
		maps.Copy(templatesOnce.v, templatesLater())
		maps.Copy(templatesOnce.v, templatesRide())
	})
	return templatesOnce.v
}

// templatesLater is the rows templates adds to its own map: the lesson,
// task-preflight, platform-rule and frozen-path rows, split out so each function stays inside Law 10 as the
// rows of several cards accumulate.
func templatesLater() map[templateKey]template {
	return map[templateKey]template{
		{pattern.LearnedFact, reflect.TypeOf(LearnedFactBinding{})}: {
			id:     "learned-fact/v1",
			format: "rashomon record: lesson {lesson}, approved for this project: {text}.",
		},
		{pattern.LearnedFact, reflect.TypeOf(LearnedFactUncheckedBinding{})}: {
			id: "learned-fact-unchecked/v1",
			format: "rashomon record: this project's lesson files fail their signature check, " +
				"so no lesson was given to the agent; rashomon learn lists them.",
		},
		{pattern.LearnedFact, reflect.TypeOf(LearnedFactOversizeBinding{})}: {
			id: "learned-fact-oversize/v1",
			format: "rashomon record: this project's lesson files are larger than the 4 MiB the prompt path reads, " +
				"so no lesson was given to the agent; rashomon learn lists them.",
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightBinding{})}: {
			id:     "task-preflight/v1",
			format: preflightHead + preflightLines,
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightFirstBinding{})}: {
			id: "task-preflight-first/v1",
			format: "rashomon record: project brief and preflight for this request from the repository; the request text is not kept.\n" +
				"project files: {instructions}; manifests: {manifests}; directories with Go files: {go_dirs}\n" + preflightLines,
		},
		{pattern.PlatformRule, reflect.TypeOf(PlatformRuleCommandBinding{})}: {
			id:     "platform-rule-command/v1",
			format: "rashomon record: this {program} call matched spec rule {rule_id}.",
		},
		{pattern.PlatformRule, reflect.TypeOf(PlatformRuleCommandApprovedBinding{})}: {
			id:     "platform-rule-command-approved/v1",
			format: "rashomon record: this {program} call matched spec rule {rule_id}. The spec records the approved form: {approved}.",
		},
		{pattern.PlatformRule, reflect.TypeOf(PlatformRuleCommandUnnamedBinding{})}: {
			id:     "platform-rule-command-unnamed/v1",
			format: "rashomon record: this call matched a spec rule.",
		},
		{pattern.OrderingRule, reflect.TypeOf(OrderingRuleThenBinding{})}: {
			id:     "ordering-rule-then/v1",
			format: "rashomon record: this {program} call ran before the earlier step spec rule {rule_id} requires.",
		},
		{pattern.OrderingRule, reflect.TypeOf(OrderingRuleThenUnnamedBinding{})}: {
			id:     "ordering-rule-then-unnamed/v1",
			format: "rashomon record: this call ran before the earlier step a spec rule requires.",
		},
		{pattern.Integrity, reflect.TypeOf(IntegrityFrozenPathBinding{})}: {
			id:     "integrity-frozen-path/v1",
			format: "rashomon record: this call wrote {path}, which spec rule {rule_id} protects.",
		},
		{pattern.Integrity, reflect.TypeOf(IntegrityFrozenPathUnnamedBinding{})}: {
			id:     "integrity-frozen-path-unnamed/v1",
			format: "rashomon record: this call wrote a file that a spec rule protects.",
		},
	}
}

// preflightLines are the four lines both task-preflight rows end with: what
// the request named, the spec rules over it, the gates and the evidence a
// done claim needs (preflightKnown), and what the preflight could not read.
const preflightLines = preflightKnown + "\nunknown: {unknown}"

// preflightKnown is preflightLines without the unknowns line, the line a
// note that carries a lesson drops first to fit its cap (LEARN-RIDE1).
const preflightKnown = "paths named: {paths}\n" +
	"spec rules that apply: {rules}\n" +
	"gates before done: {gates}; evidence for a done claim ({source}): {evidence}"

// preflightHead is the later prompts' task-preflight header line.
const preflightHead = "rashomon record: preflight for this request from the repository; the request text is not kept.\n"

// lessonRide is the line a lesson takes at the end of a preflight note: the
// words of its own learned-fact row, its text in quotes.
const lessonRide = "\nlesson {lesson}, approved for this project: {text}"

// templatesRide is the two task-preflight rows that carry a lesson as their
// last line (LEARN-RIDE1): the later prompts' note, with its unknowns line
// and without it. The first prompt's six lines are the cap, so it has none.
func templatesRide() map[templateKey]template {
	return map[templateKey]template{
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightLessonBinding{})}: {
			id:     "task-preflight-lesson/v1",
			format: preflightHead + preflightLines + lessonRide,
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightLessonKnownBinding{})}: {
			id:     "task-preflight-lesson-known/v1",
			format: preflightHead + preflightKnown + lessonRide,
		},
	}
}

// templatesOnce holds what templates returns, built on its first call and
// never at init (RES-2).
var templatesOnce struct {
	sync.Once
	v map[templateKey]template
}

var (
	// ErrUnknownPattern is returned by New for an id outside the frozen set.
	ErrUnknownPattern = errors.New("render: unknown pattern")
	// ErrNoTemplate is returned by New when the table has no row for the
	// pattern and the binding's type.
	ErrNoTemplate = errors.New("render: no template for this pattern and binding")
	// ErrBinding is returned by New for a binding that is not a struct of
	// exported slot-type fields, each with a unique `slot:"name"` tag.
	ErrBinding = errors.New("render: binding is not a struct of tagged slot fields")
	// ErrTemplate is returned by New when a template and its binding
	// disagree: a placeholder with no slot, a slot with no placeholder, or a
	// brace outside a placeholder.
	ErrTemplate = errors.New("render: template and binding disagree")
)

// New fills p's template for binding's type, runs every slot check (a
// listed path that fails the deny list or the entropy test is left out of
// the list instead) and the whole-string check -- ValidateBriefing for
// the two multi-line notes, continuity-briefing and task-preflight, Validate
// for every other pattern -- and returns the Message, or the zero Message
// and an error.
func New(p pattern.ID, binding any) (Message, error) {
	return newMessage(templates(), p, binding)
}

func newMessage(table map[templateKey]template, p pattern.ID, binding any) (Message, error) {
	if !pattern.Valid(p) {
		return Message{}, fmt.Errorf("%w: %q", ErrUnknownPattern, p)
	}
	tmpl, ok := table[templateKey{pattern: p, binding: reflect.TypeOf(binding)}]
	if !ok {
		return Message{}, fmt.Errorf("%w: %q, %T", ErrNoTemplate, p, binding)
	}
	slots, err := bindSlots(binding)
	if err != nil {
		return Message{}, err
	}
	text, err := fill(tmpl.format, slots)
	if err != nil {
		return Message{}, err
	}
	check := Validate
	if p == pattern.ContinuityBriefing || p == pattern.TaskPreflight {
		check = ValidateBriefing
	}
	if err := check(text); err != nil {
		return Message{}, err
	}
	return Message{pattern: p, template: tmpl.id, slots: slots, text: text}, nil
}

// slotPkgPath is the import path every binding field's type must come from.
var slotPkgPath = reflect.TypeOf(slot.Path("")).PkgPath()

// bindSlots checks every field of binding, a struct of slot types with a
// `slot:"name"` tag each, and returns the bound values in field order.
func bindSlots(binding any) ([]Slot, error) {
	v := reflect.ValueOf(binding)
	if v.Kind() != reflect.Struct {
		return nil, ErrBinding
	}
	t := v.Type()
	slots := make([]Slot, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Tag.Get("slot")
		if !f.IsExported() || !validSlotName(name) || hasSlot(slots, name) ||
			f.Type.PkgPath() != slotPkgPath || f.Type.Kind() == reflect.Interface {
			return nil, fmt.Errorf("%w: field %s", ErrBinding, f.Name)
		}
		val, ok := v.Field(i).Interface().(slot.Value)
		if !ok {
			return nil, fmt.Errorf("%w: field %s is not a slot value", ErrBinding, f.Name)
		}
		bound, err := slot.Bind(val)
		if err != nil {
			return nil, fmt.Errorf("render: slot %s: %w", name, err)
		}
		slots = append(slots, Slot{Name: name, Value: bound})
	}
	return slots, nil
}

// validSlotName reports a lower_snake_case name of 1 to 32 bytes, starting
// with a letter.
func validSlotName(name string) bool {
	if name == "" || len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func hasSlot(slots []Slot, name string) bool {
	return slices.ContainsFunc(slots, func(s Slot) bool { return s.Name == name })
}

// fill replaces each {name} in format with the named slot's text. Every
// placeholder must name a bound slot, every bound slot must be used, and a
// brace outside a placeholder is refused. Slot text is written, never
// scanned again, so a value cannot add a placeholder.
func fill(format string, slots []Slot) (string, error) {
	var b strings.Builder
	used := make(map[string]bool, len(slots))
	for i := 0; i < len(format); {
		switch c := format[i]; c {
		case '{':
			end := strings.IndexAny(format[i+1:], "{}")
			if end < 0 || format[i+1+end] != '}' {
				return "", fmt.Errorf("%w: an unclosed placeholder", ErrTemplate)
			}
			name := format[i+1 : i+1+end]
			at := slices.IndexFunc(slots, func(s Slot) bool { return s.Name == name })
			if at < 0 {
				return "", fmt.Errorf("%w: placeholder %q has no slot", ErrTemplate, name)
			}
			b.WriteString(slots[at].Value.Text())
			used[name] = true
			i += end + 2
		case '}':
			return "", fmt.Errorf("%w: a stray '}'", ErrTemplate)
		default:
			b.WriteByte(c)
			i++
		}
	}
	if len(used) != len(slots) {
		return "", fmt.Errorf("%w: a bound slot has no placeholder", ErrTemplate)
	}
	return b.String(), nil
}
