package steer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// errSlot is slotsOf's refusal: a slot it has no row for, a nil value, or
// two slots that would write one key. The line is then render_refused,
// never written with a slot missing or guessed.
var errSlot = errors.New("steer: a slot the line cannot record")

// slotsOf is a Message's slots as a steer line records them (tech 4.2.3's
// example: {"gate":"go test","exit":1,"paths":[...],"more":0,"minutes":6}):
// the bound values, from which a suppressed line is re-rendered on read
// (RES-32). The zero Message has none.
func slotsOf(m render.Message) (map[string]json.RawMessage, error) {
	return slotsFrom(m.Slots())
}

// slotsFrom is slotsOf's table, one row per concrete slot type, an allow
// list: counts, exit codes and minutes as JSON numbers, dollars as a number
// with two decimals, a path list as its paths under the slot's name and its
// omitted count under "more", the preflight's lists as one object each
// (preflightJSON), and every other type as the JSON string of its raw value. Never Text(): a repository-named slot's text carries the
// quotes it renders inside (P07N-11), and the example records "go test",
// not "'go test'" (card 2.2 R10, A2). Each value is the bound one, so a path
// the renderer left out is in "more", never in "paths".
func slotsFrom(slots []render.Slot) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(slots)+1)
	put := func(name string, raw json.RawMessage) error {
		if _, dup := out[name]; dup {
			return fmt.Errorf("%w: two slots write %q", errSlot, name)
		}
		out[name] = raw
		return nil
	}
	for _, s := range slots {
		var err error
		switch v := s.Value.(type) {
		case slot.Count:
			err = put(s.Name, json.RawMessage(strconv.Itoa(int(v))))
		case slot.Exit:
			err = put(s.Name, json.RawMessage(strconv.Itoa(int(v))))
		case slot.Minutes:
			err = put(s.Name, json.RawMessage(strconv.Itoa(int(v))))
		case slot.USD:
			err = put(s.Name, json.RawMessage(strconv.FormatFloat(float64(v), 'f', 2, 64)))
		case slot.PathList:
			paths := v.Paths()
			names := make([]string, len(paths))
			for i, p := range paths {
				names[i] = string(p)
			}
			err = putJSON(put, s.Name, names)
			if err == nil {
				err = put("more", json.RawMessage(strconv.Itoa(v.More())))
			}
		case slot.Gate:
			err = putJSON(put, s.Name, string(v))
		case slot.Program:
			err = putJSON(put, s.Name, string(v))
		case slot.Path:
			err = putJSON(put, s.Name, string(v))
		case slot.AgentType:
			err = putJSON(put, s.Name, string(v))
		case slot.Approved:
			err = putJSON(put, s.Name, string(v))
		case slot.Mechanism:
			err = putJSON(put, s.Name, string(v))
		case slot.LessonID:
			err = putJSON(put, s.Name, string(v))
		case slot.TaskID:
			err = putJSON(put, s.Name, string(v))
		case slot.RuleID:
			err = putJSON(put, s.Name, string(v))
		case slot.Model:
			err = putJSON(put, s.Name, string(v))
		case slot.Date:
			err = putJSON(put, s.Name, string(v))
		case slot.Enum:
			err = putJSON(put, s.Name, v.Text())
		case slot.Said:
			err = saidJSON(put, s.Name, v)
		case slot.FactValue:
			err = putJSON(put, s.Name, v.Value)
		case slot.PathSet, slot.RuleRefs, slot.GateRefs, slot.EnumList, slot.SymbolFacts:
			err = putJSON(put, s.Name, preflightJSON(v))
		default:
			err = fmt.Errorf("%w: %q holds %T", errSlot, s.Name, s.Value)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// saidJSON is a quote of the person's words as its line records it: the
// record form's keyed digests and byte lengths (COMPACT-SLOT1, D160). The
// words are refused, so a finding that rendered them without a Record is
// render_refused and its line never holds them.
func saidJSON(put func(string, json.RawMessage) error, name string, v slot.Said) error {
	if !v.Keyed() {
		return fmt.Errorf("%w: %q holds the person's words, not their keyed digest", errSlot, name)
	}
	return putJSON(put, name, saidLine{Bytes: v.Bytes(), Digest: v.Digest(), RestatesBytes: v.RestatesBytes(),
		RestatesDigest: v.RestatesDigest()})
}

// saidLine is saidJSON's form, keys in sorted order as the preflight
// lists' are.
type saidLine struct {
	Bytes          int    `json:"bytes"`
	Digest         string `json:"digest"`
	RestatesBytes  int    `json:"restates_bytes"`
	RestatesDigest string `json:"restates_digest"`
}

// putJSON writes v's JSON encoding under name.
func putJSON(put func(string, json.RawMessage) error, name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return put(name, raw)
}

// The preflight lists' line forms: flat arrays of raw values beside their
// counts, keys in sorted order, so the delivered-set reader scans them with
// jsonscan and never decodes a line (RES-5).
type (
	pathSetJSON struct {
		Earlier int      `json:"earlier"`
		Items   []string `json:"items"`
		More    int      `json:"more"`
	}
	ruleRefsJSON struct {
		Earlier  int      `json:"earlier"`
		IDs      []string `json:"ids"`
		More     int      `json:"more"`
		Trust    []string `json:"trust"`
		Why      []string `json:"why"`
		Withheld int      `json:"withheld"`
	}
	gateRefsJSON struct {
		Gates   []string `json:"gates"`
		Learned int      `json:"learned"`
		More    int      `json:"more"`
		Trust   []string `json:"trust"`
	}
	// symbolFactsJSON is the symbols line's form: the record's (Counted,
	// record scope v4) holds the count in more and no name or directory.
	symbolFactsJSON struct {
		Callers  []int    `json:"callers"`
		Dirs     []string `json:"dirs"`
		More     int      `json:"more"`
		Names    []string `json:"names"`
		Packages []int    `json:"packages"`
	}
)

// preflightJSON is one preflight list as its line records it.
func preflightJSON(v slot.Value) any {
	switch v := v.(type) {
	case slot.PathSet:
		out := pathSetJSON{Earlier: v.Earlier(), Items: []string{}, More: v.More()}
		for _, p := range v.Paths() {
			out.Items = append(out.Items, string(p))
		}
		out.Items = append(out.Items, v.Digests()...)
		return out
	case slot.RuleRefs:
		out := ruleRefsJSON{Earlier: v.Earlier(), IDs: []string{}, More: v.More(), Trust: []string{}, Why: []string{}, Withheld: v.Withheld()}
		for _, r := range v.Items() {
			out.IDs, out.Trust, out.Why = append(out.IDs, r.ID), append(out.Trust, r.Trust.Text()), append(out.Why, string(r.Why))
		}
		return out
	case slot.GateRefs:
		out := gateRefsJSON{Gates: []string{}, Learned: v.Learned(), More: v.More(), Trust: []string{}}
		for _, g := range v.Items() {
			out.Gates, out.Trust = append(out.Gates, string(g.Words)), append(out.Trust, g.Trust.Text())
		}
		return out
	case slot.EnumList:
		return append([]string{}, v.Values()...)
	case slot.SymbolFacts:
		out := symbolFactsJSON{Callers: []int{}, Dirs: []string{}, More: v.More(), Names: []string{}, Packages: []int{}}
		for _, f := range v.Items() {
			out.Names, out.Dirs = append(out.Names, f.Name), append(out.Dirs, string(f.Dir))
			out.Callers, out.Packages = append(out.Callers, f.Callers), append(out.Packages, f.Packages)
		}
		return out
	}
	return nil
}
