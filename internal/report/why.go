package report

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// WhyView is why one steer line fired (card REPORT1b, `rashomon why`): the
// line as the steer log kept it, the evidence rows its seqs name, where its
// content came from, and how the gate it names stood at the session's last
// Stop. It is read from records and re-derives nothing: the resolution is the
// stored checkpoint's own gate, not an evaluation of today's index, spec and
// tree.
type WhyView struct {
	// Prefix is the id prefix as typed, SessionID the session it was found
	// in and LineID the line's whole id.
	Prefix    string `json:"prefix"`
	SessionID string `json:"session_id"`
	LineID    string `json:"line_id"`
	Project   string `json:"project"`
	// Pattern is the pattern's id and Title its name in the report ("note" is
	// never used: a pattern with no title is named by its id).
	Pattern  string `json:"pattern"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	Event    string `json:"event"`
	Level    string `json:"level"`
	PromptID string `json:"prompt_id"`
	AtMS     int64  `json:"at_unix_ms"`
	AfterSeq int64  `json:"after_seq"`
	// State, Delivery and SuppressedBy are the line's own words. TextKept is
	// false for a line whose text the log does not keep (a suppressed one
	// keeps its slots only, RES-32), and ModelText and UserText are then "".
	State        string `json:"state"`
	Delivery     string `json:"delivery"`
	SuppressedBy string `json:"suppressed_by"`
	TextKept     bool   `json:"text_kept"`
	ModelText    string `json:"model_text"`
	UserText     string `json:"user_text"`
	// Slots are the values the line's message bound, in name order, a list or
	// object as "(list)".
	Slots []WhySlot `json:"slots"`
	// Evidence is the record rows the line cites: its evidence seqs, else
	// the seq it was decided after.
	Evidence []TimelineCall `json:"evidence"`
	Source   WhySource      `json:"source"`
	// OutcomeFrom is the closed word OutcomeFromOutput where the run the
	// line rests on had its outcome read from its output (the pipe hid the
	// exit status), "" on every other line.
	OutcomeFrom string `json:"outcome_from"`
	// Resolution is the named gate as the session's last checkpoint stored
	// it, null when the line names no gate or the checkpoint holds none.
	Resolution *WhyResolution `json:"resolution"`
	Redacted   bool           `json:"redacted"`
}

// WhySlot is one slot's name and value as text. A slot whose name is not in
// the closed set WhySlotShown names never carries its text: Value is "" and
// Bytes is the length of what was left out (a learned-fact line keeps its
// lesson text in slot "text", a free-text slot a share must not carry).
type WhySlot struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Bytes int    `json:"bytes,omitempty"`
}

// WhySlotShown reports whether slot name's value may be printed as it is.
// "path" is separate: it prints as is, and --redact digests it.
//
// The cases are the closed set of slot names whose value `why` prints: each is
// a charset-checked, closed-vocabulary slot of internal/steer/render (gate,
// minutes, exit, count, go_dirs, lesson id, source enum, the evidence and
// unknown enum lists and the "rules" and "gates" ref lists) plus the program
// name. Every other name is hidden, so a slot a later pattern adds is hidden
// until it is listed here. A platform-rule line's "rule_id" is one such: it
// prints as a byte count, and its id is named only by the line's source. A switch, not a map: a map literal is built at package init, in
// every hook call (RES-2's init budget).
func WhySlotShown(name string) bool {
	switch name {
	case "gate", "program", "exit", "minutes", "count", "go_dirs", "lesson", "source", "rules", "gates",
		"evidence", "unknown":
		return true
	}
	return false
}

// OutcomeFromOutput is the one OutcomeFrom word `why` carries: the run's
// outcome was read from its output.
const OutcomeFromOutput = "output_summary"

// WhySource is what the line's content came from: Kind is gate, rules,
// lesson, ledger_unchecked or none, Detail the name or names.
type WhySource struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// WhyResolution is a gate at the session's last checkpoint: its state, the
// seq of its last green run (null with none) and the checkpoint's point seq.
type WhyResolution struct {
	Gate         string `json:"gate"`
	State        string `json:"state"`
	LastGreenSeq *int64 `json:"last_green_seq"`
	PointSeq     int64  `json:"point_seq"`
}

// TimelineRows are the timeline rows of run whose declaration seq is in
// seqs, in seq order: buildTimeline with no denied set, for a caller that
// cites a few calls (`rashomon why`). A seq with no declaration has no row.
func TimelineRows(run *store.Run, seqs []int64) []TimelineCall {
	rows := []TimelineCall{}
	for _, c := range buildTimeline(run, nil).Calls {
		if c.Seq != nil && slices.Contains(seqs, *c.Seq) {
			rows = append(rows, c)
		}
	}
	return rows
}

// RedactWhy is v for sharing: the project root becomes a keyed digest, both
// texts are dropped, and a slot that holds a path is digested (a list is
// already "(list)"), and every slot outside the closed WhySlotShown set is
// blanked down to its byte count. Ids, seqs, gate names, states and times
// stay.
func RedactWhy(v *WhyView, key []byte) *WhyView {
	if v == nil {
		return nil
	}
	out := *v
	out.Redacted = true
	out.Project = redactHost(v.Project, key)
	out.ModelText, out.UserText = "", ""
	out.Slots = make([]WhySlot, 0, len(v.Slots))
	for _, s := range v.Slots {
		switch {
		case s.Name == "path":
			s.Value = redactHost(s.Value, key)
		case !WhySlotShown(s.Name):
			if s.Bytes == 0 {
				s.Bytes = len(s.Value)
			}
			s.Value = ""
		}
		out.Slots = append(out.Slots, s)
	}
	out.Evidence = append([]TimelineCall{}, v.Evidence...)
	if v.Resolution != nil {
		r := *v.Resolution
		if r.LastGreenSeq != nil {
			g := *r.LastGreenSeq
			r.LastGreenSeq = &g
		}
		out.Resolution = &r
	}
	return &out
}
