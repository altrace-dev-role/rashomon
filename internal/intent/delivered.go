package intent

import (
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// Delivered is what this session's task-preflight lines already gave.
type Delivered struct {
	Paths, Rules, Gates, Evidence map[string]bool
	BriefGiven                    bool
}

const (
	dkKey = iota
	dkPattern
	dkSlots
	dkState
	dkCount
)

// lineKeys is the one KeySet a steer line is read with (RES-5's pattern: no
// decode of a whole line).
func lineKeys() *jsonscan.KeySet {
	lineKeysOnce.Do(func() { lineKeysOnce.v = jsonscan.NewKeySet("key", "pattern", "slots", "state") })
	return lineKeysOnce.v
}

// lineKeysOnce holds what lineKeys returns, built on its first call and never
// at init (RES-2).
var lineKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// ReadDelivered reads the delivered set from a run's steer lines: every
// task-preflight line in state delivered, whatever its delivery (a held-out
// note was still given), read through jsonscan as RES-5 asks. The set is
// what the lines' slots named; a line that does not read is passed over.
func ReadDelivered(lines [][]byte) Delivered {
	d := Delivered{Paths: map[string]bool{}, Rules: map[string]bool{}, Gates: map[string]bool{}, Evidence: map[string]bool{}}
	var out [dkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, lineKeys(), out[:]) != nil ||
			text(b, out[dkPattern]) != string(pattern.TaskPreflight) || text(b, out[dkState]) != string(steer.LineStateDelivered) {
			continue
		}
		if text(b, out[dkKey]) == keyBrief {
			d.BriefGiven = true
		}
		slots := out[dkSlots]
		mark(d.Paths, b, slots, "paths", "items")
		mark(d.Rules, b, slots, "rules", "ids")
		mark(d.Gates, b, slots, "gates", "gates")
		mark(d.Evidence, b, slots, "evidence", "")
	}
	return d
}

// mark sets each string of the array at slots.name, or at slots.name.field
// when field is not empty.
func mark(set map[string]bool, b []byte, slots jsonscan.Span, name, field string) {
	sp, err := jsonscan.Lookup(b, slots, name)
	if err == nil && field != "" && sp.Kind == jsonscan.Object {
		sp, err = jsonscan.Lookup(b, sp, field)
	}
	if err != nil || sp.Kind != jsonscan.Array {
		return
	}
	end := int(sp.Off + sp.Len - 1)
	for i := int(sp.Off) + 1; i < end; {
		switch b[i] {
		case ' ', '\t', '\n', '\r', ',':
			i++
			continue
		case '"':
		default:
			return
		}
		j := i + 1
		for j < end && b[j] != '"' {
			if b[j] == '\\' {
				j++
			}
			j++
		}
		if j >= end {
			return
		}
		if s := text(b, jsonscan.Span{Off: int32(i), Len: int32(j + 1 - i), Kind: jsonscan.String}); s != "" {
			set[s] = true
		}
		i = j + 1
	}
}

// text is a string value's text, "" for any other kind or a bad escape.
func text(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}
