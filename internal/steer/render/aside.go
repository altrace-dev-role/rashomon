package render

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TOKEN-BUDGET1: a note's model text carries only what the record proves.
// A template may set an aside, the person's lines, filled from the same
// slots and never sent to the model, and may be sparse: a line that holds a
// list (slot.Lister) and whose every list names nothing is left out, and a
// message whose model lines are all left out has no model text, so only its
// aside prints. One template and one slot set make both texts, so a steer
// line's slots re-render either.

// AsidePrefix opens a note's aside where it prints, after the note's mirror
// in the one systemMessage, or alone when the agent was sent nothing.
const AsidePrefix = "※ rashomon, not sent to the agent: "

// ErrNothing is returned by New for a sparse message every line of which,
// model's and person's, was left out.
var ErrNothing = errors.New("render: the message has no line to print")

// Aside is the person's lines of the message, "" for a template with none.
func (m Message) Aside() string { return m.aside }

// fillSplit fills t's format, the model's lines, and its aside, the
// person's, from one slot set: every placeholder names a bound slot and
// every bound slot is used by one of the two.
func fillSplit(t template, slots []Slot) (text, aside string, err error) {
	used := make(map[string]bool, len(slots))
	if text, err = fillLines(t.format, slots, used, t.sparse, true); err != nil {
		return "", "", err
	}
	if aside, err = fillLines(t.aside, slots, used, t.sparse, false); err != nil {
		return "", "", err
	}
	if len(used) != len(slots) {
		return "", "", fmt.Errorf("%w: a bound slot has no placeholder", ErrTemplate)
	}
	return text, aside, nil
}

// fillLines fills format line by line. With sparse set, a line whose lists
// all name nothing is left out; with head set, the first line is the
// format's header, kept, and a format left with its header alone fills to "".
func fillLines(format string, slots []Slot, used map[string]bool, sparse, head bool) (string, error) {
	if format == "" {
		return "", nil
	}
	var kept []string
	for i, line := range strings.Split(format, "\n") {
		var b strings.Builder
		names, err := fillInto(&b, line, slots, used)
		if err != nil {
			return "", err
		}
		if sparse && !(head && i == 0) && listsNothing(names, slots) {
			continue
		}
		kept = append(kept, b.String())
	}
	if sparse && head && len(kept) == 1 {
		return "", nil
	}
	return strings.Join(kept, "\n"), nil
}

// listsNothing reports a line that holds a list and whose every list names
// nothing; a line with no list is never left out.
func listsNothing(names []string, slots []Slot) bool {
	lists := false
	for _, n := range names {
		at := slices.IndexFunc(slots, func(s Slot) bool { return s.Name == n })
		if l, ok := slots[at].Value.(slot.Lister); ok {
			if l.Lists() {
				return false
			}
			lists = true
		}
	}
	return lists
}

// validateSplit is the whole-string check for a message with an aside or
// sparse lines: the model text, when there is one, passes check, and the
// two together fit MaxBriefingMessageBytes, the cap the full note met when
// it went to the model whole, so intent's ladder steps down as before. The
// aside gets no line check of its own: its slots are enums, counts and
// charset-checked paths, whose texts passed the line rules as the model's
// lines before TOKEN-BUDGET1.
func validateSplit(text, aside string, check func(string) error) error {
	switch {
	case text == "" && aside == "":
		return ErrNothing
	case len(text)+len(aside) > MaxBriefingMessageBytes:
		return fmt.Errorf("render: note and aside are %d bytes, want at most %d", len(text)+len(aside), MaxBriefingMessageBytes)
	case text != "":
		return check(text)
	}
	return nil
}
