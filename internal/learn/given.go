package learn

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// Given is one knowledge record a session's graph brief delivered, with
// the paths its lines named, the record's own line and each code line it
// applies to, each as the line holds it: its keyed digest at record scope
// v4, where the record form withholds the path, and the path itself at v5
// (card LABEL-WRITER1). A writer compares what the line holds with the
// same form of what the session wrote.
type Given struct {
	ID    string
	Paths []string
}

// GivenGraph is the knowledge the session's delivered task-preflight graph
// lines gave, in first-seen order: a line the agent saw (state delivered),
// not one an arm withheld, since a label judges what the agent was told.
// Lines are located with jsonscan, never decoded (RES-5).
func GivenGraph(lines [][]byte) []Given {
	var out []Given
	at := map[string]int{}
	add := func(id, path string) {
		if id == "" {
			return
		}
		i, ok := at[id]
		if !ok {
			i = len(out)
			at[id] = i
			out = append(out, Given{ID: id})
		}
		if path != "" {
			out[i].Paths = append(out[i].Paths, path)
		}
	}
	var sp [dkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, deliveredKeys(), sp[:]) != nil || spanText(b, sp[dkPattern]) != string(pattern.TaskPreflight) ||
			spanText(b, sp[dkState]) != string(steer.LineStateDelivered) {
			continue
		}
		for _, name := range graphLineSlots {
			line, err := jsonscan.Lookup(b, sp[dkSlots], name)
			if err != nil || line.Kind != jsonscan.Object {
				continue
			}
			path := ""
			for _, field := range [...]string{"digest", "path"} {
				if v, err := jsonscan.Lookup(b, line, field); err == nil && path == "" {
					path = spanText(b, v)
				}
			}
			if id, err := jsonscan.Lookup(b, line, "id"); err == nil {
				add(spanText(b, id), path)
			}
			if arr, err := jsonscan.Lookup(b, line, "lessons"); err == nil && arr.Kind == jsonscan.Array {
				_ = jsonscan.Elements(b, arr, func(el jsonscan.Span) bool {
					add(spanText(b, el), path)
					return true
				})
			}
		}
	}
	return out
}

// Written is what a session wrote: Keys holds each path in both forms a
// line holds one in, the path and its keyed digest, so a session labels
// under either record scope; Paths counts the paths themselves.
type Written struct {
	Keys  map[string]bool
	Paths int
}

// Judge is the label a session's writes give g, and the sentence the row
// keeps: confirmed when a path the record's lines named was written after
// it was given, refuted when the session wrote files and none of them,
// unresolved when it wrote none or the lines named no path.
func Judge(g Given, w Written) (steer.Label, string) {
	if w.Paths == 0 {
		return steer.LabelUnresolved, "the session wrote no file"
	}
	if len(g.Paths) == 0 {
		return steer.LabelUnresolved, "the lines named no path"
	}
	for _, d := range g.Paths {
		if w.Keys[d] {
			return steer.LabelConfirmed, "the session wrote a path the record's line named"
		}
	}
	return steer.LabelRefuted, "the session wrote " + strconv.Itoa(w.Paths) + " path(s) and none the record's lines named"
}

// FoldID is a graph record's id as the ledger holds it: the ledger's id
// charset has no upper case (idForm), so les:INC-212 is the ledger's
// les:inc-212, folded at the ledger's door and nowhere else.
func FoldID(id string) string { return strings.ToLower(id) }
