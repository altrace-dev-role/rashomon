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
// omitted count under "more", and every other type as the JSON string of
// its raw value. Never Text(): a repository-named slot's text carries the
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
		case slot.FactValue:
			err = putJSON(put, s.Name, v.Value)
		default:
			err = fmt.Errorf("%w: %q holds %T", errSlot, s.Name, s.Value)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// putJSON writes v's JSON encoding under name.
func putJSON(put func(string, json.RawMessage) error, name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return put(name, raw)
}
