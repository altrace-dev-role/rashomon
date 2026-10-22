package install

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// OnlyEntriesRemoved reports whether after is before with installID's
// entries taken out and nothing else changed. It is what detach's
// "every other byte is as you and other tools left it" claims, and it holds
// only when both checks below do: every byte of after is a byte of before,
// in order, so nothing was added, restyled or moved; and what went is
// installID's entries and nothing else.
func OnlyEntriesRemoved(before, after []byte, installID string) bool {
	return onlyTakesOut(before, after) && onlyOursGone(before, after, installID)
}

// onlyTakesOut reports whether after can be made from before by deleting
// bytes and nothing else: every byte of after appears in before, in order.
// It is what "every other byte is as you left it" means for a detach, whose
// only change is taking our entries out.
func onlyTakesOut(before, after []byte) bool {
	i := 0
	for _, c := range after {
		for i < len(before) && before[i] != c {
			i++
		}
		if i == len(before) {
			return false
		}
		i++
	}
	return true
}

// onlyOursGone reports whether after holds exactly what before holds once
// installID's entries are taken out of it, compared as decoded JSON: an
// emptied event key goes with its last entry, and an emptied hooks object
// with its last event, as detach removes them.
func onlyOursGone(before, after []byte, installID string) bool {
	var b, a any
	if decodeJSON(before, &b) != nil || decodeJSON(after, &a) != nil {
		return false
	}
	doc, ok := b.(map[string]any)
	if !ok {
		return false
	}
	if hooks, ok := doc["hooks"].(map[string]any); ok {
		dropped := false
		for event, v := range hooks {
			groups, ok := v.([]any)
			if !ok {
				continue
			}
			var keep []any
			for _, g := range groups {
				raw, err := json.Marshal(g)
				if err != nil || Owner(raw, event) != installID {
					keep = append(keep, g)
				}
			}
			switch {
			case len(keep) == len(groups):
			case len(keep) == 0:
				delete(hooks, event)
				dropped = true
			default:
				hooks[event] = keep
			}
		}
		if dropped && len(hooks) == 0 {
			delete(doc, "hooks")
		}
	}
	return reflect.DeepEqual(doc, a)
}

// decodeJSON decodes with numbers kept as written, so 1.0 and 1 differ.
func decodeJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(v)
}
