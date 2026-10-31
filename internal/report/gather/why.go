package gather

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MinWhyPrefix is the shortest id prefix `why` takes: six characters of a
// line's 32, so a prefix a person can type names one line in a session.
const MinWhyPrefix = 6

// Why is why the steer line of sessionID whose id starts with prefix fired
// (card REPORT1b). It reads and writes nothing else: the store is opened
// without a ledger and no report is built, so asking why changes no file.
// A prefix is at least MinWhyPrefix characters of [0-9a-z-]; none matching,
// or more than one line, is an error that names what was found, never a
// guess. The line is decoded whole, its evidence rows come from the run's
// timeline, and the gate it names is read as the session's last checkpoint
// stored it (LastStop), not derived again.
func Why(st *store.Store, sessionID, prefix string) (report.WhyView, error) {
	if err := checkPrefix(prefix); err != nil {
		return report.WhyView{}, err
	}
	lines, _, err := st.ReadSteer(sessionID)
	if err != nil {
		return report.WhyView{}, err
	}
	raw, err := pickLine(lines, prefix, sessionID)
	if err != nil {
		return report.WhyView{}, err
	}
	var l steer.Line
	if err := json.Unmarshal(raw, &l); err != nil {
		return report.WhyView{}, fmt.Errorf("the line does not read: %w", err)
	}
	v := report.WhyView{Prefix: prefix, SessionID: sessionID, LineID: l.ID, Project: l.Project, Pattern: l.Pattern, Title: l.Pattern,
		Kind: l.Kind, Key: l.Key, Event: l.Event, Level: l.Level, AtMS: l.AtUnixMS, AfterSeq: l.AfterSeq,
		State: string(l.State), TextKept: l.State != steer.LineStateSuppressed, Slots: slotsOf(l.Slots), Evidence: []report.TimelineCall{}}
	if l.PromptID != nil {
		v.PromptID = *l.PromptID
	}
	if l.Delivery != nil {
		v.Delivery = string(*l.Delivery)
	}
	if l.SuppressedBy != nil {
		v.SuppressedBy = string(*l.SuppressedBy)
	}
	if v.TextKept {
		v.ModelText, v.UserText = l.ModelText, l.UserText
	}
	v.Source = sourceOf(l)
	if l.Facts.OutcomeFrom != nil && *l.Facts.OutcomeFrom == report.OutcomeFromOutput {
		v.OutcomeFrom = report.OutcomeFromOutput
	}
	seqs := l.EvidenceSeqs
	if len(seqs) == 0 && l.AfterSeq > 0 {
		seqs = []int64{l.AfterSeq}
	}
	if run, err := st.ReadRunDirForReport(st.DirName(sessionID)); err == nil && len(seqs) > 0 {
		v.Evidence = report.TimelineRows(run, seqs)
	}
	v.Resolution = resolutionOf(st, sessionID, v.Source)
	return v, nil
}

// checkPrefix is K6's rule: at least MinWhyPrefix characters, each of
// 0-9, a-z or "-" (a line id is lowercase hex, and a test's may carry "-").
func checkPrefix(p string) error {
	if len(p) < MinWhyPrefix {
		return fmt.Errorf("an id prefix is at least %d characters; %q is %d", MinWhyPrefix, p, len(p))
	}
	for i := 0; i < len(p); i++ {
		if c := p[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c == '-') {
			return fmt.Errorf("an id prefix is lowercase letters, digits and \"-\" only; %q is not", p)
		}
	}
	return nil
}

var idKeys struct {
	sync.Once
	v *jsonscan.KeySet
}

// pickLine is the one line whose id starts with prefix. A line seen twice
// counts once. None is an error, and so are two or more, naming each
// candidate by the first characters of its id.
func pickLine(lines [][]byte, prefix, sessionID string) ([]byte, error) {
	idKeys.Do(func() { idKeys.v = jsonscan.NewKeySet("id") })
	byID := map[string][]byte{}
	var out [1]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, idKeys.v, out[:]) != nil || out[0].Kind != jsonscan.String {
			continue
		}
		id, err := jsonscan.DecodeString(b, out[0])
		if err != nil || !strings.HasPrefix(id, prefix) {
			continue
		}
		if _, seen := byID[id]; !seen {
			byID[id] = b
		}
	}
	switch len(byID) {
	case 0:
		return nil, fmt.Errorf("no steer line of session %s has an id starting %q", sessionID, prefix)
	case 1:
		for _, b := range byID {
			return b, nil
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id[:min(len(id), len(prefix)+4)])
	}
	sort.Strings(ids)
	return nil, fmt.Errorf("%d steer lines of session %s start %q (%s); give more of the id", len(ids), sessionID, prefix, strings.Join(ids, ", "))
}

// slotsOf is a line's slots as text in name order: a string as itself, a
// number as written, a list or object as "(list)". Only a slot whose name is
// in report.WhySlotShown (or "path") keeps its string: any other string is
// left out and only its length kept, whether the line was delivered or not.
func slotsOf(m map[string]json.RawMessage) []report.WhySlot {
	out := make([]report.WhySlot, 0, len(m))
	for name, raw := range m {
		v := slotText(raw)
		if !report.WhySlotShown(name) && name != "path" && isString(raw) {
			out = append(out, report.WhySlot{Name: name, Bytes: len(v)})
			continue
		}
		out = append(out, report.WhySlot{Name: name, Value: v})
	}
	slices.SortFunc(out, func(a, b report.WhySlot) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func isString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil
}

func slotText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return "(list)"
}

// sourceOf is where the line's content came from, read from its slots and
// key and never from Facts, whose source members no producer writes (only
// Facts.OutcomeFrom is written, and WhyView carries it apart). A learned-fact
// line is the lesson its key names, or the ledger flag; a line with a rules
// slot names its rule ids (PREFLIGHT1's RuleRefs); a platform-rule line names
// the one rule its rule_id slot holds; one with a gate slot names the gate.
func sourceOf(l steer.Line) report.WhySource {
	if l.Pattern == "learned-fact" {
		if l.Key == learn.UncheckedKey {
			return report.WhySource{Kind: "ledger_unchecked"}
		}
		return report.WhySource{Kind: "lesson", Detail: l.Key}
	}
	if raw, ok := l.Slots["rules"]; ok {
		var r struct {
			IDs []string `json:"ids"`
		}
		if json.Unmarshal(raw, &r) == nil && len(r.IDs) > 0 {
			return report.WhySource{Kind: "rules", Detail: strings.Join(r.IDs, ", ")}
		}
	}
	if l.Pattern == "platform-rule" {
		var id string
		if json.Unmarshal(l.Slots["rule_id"], &id) == nil && id != "" {
			return report.WhySource{Kind: "rules", Detail: id}
		}
	}
	if raw, ok := l.Slots["gate"]; ok {
		var g string
		if json.Unmarshal(raw, &g) == nil && g != "" {
			return report.WhySource{Kind: "gate", Detail: g}
		}
	}
	return report.WhySource{Kind: "none"}
}

// resolutionOf is the gate a gate-sourced line names, as the session's last
// checkpoint stored it: matched by the gate's name or its kind, as the
// report's findings match a gate word. Nil for any other source, a session
// with no checkpoint or one that holds no such gate.
func resolutionOf(st *store.Store, sessionID string, src report.WhySource) *report.WhyResolution {
	if src.Kind != "gate" {
		return nil
	}
	cp, err := LastStop(st, sessionID)
	if err != nil || cp == nil {
		return nil
	}
	for _, g := range cp.Gates {
		if g.Gate == src.Detail || g.Kind == src.Detail {
			return &report.WhyResolution{Gate: g.Gate, State: string(g.State), LastGreenSeq: g.LastGreenSeq, PointSeq: cp.PointSeq}
		}
	}
	return nil
}
