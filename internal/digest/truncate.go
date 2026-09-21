package digest

import (
	"encoding/json"
	"sort"
)

// truncate enforces CapBytes on the WHOLE marshalled document, not per
// field. WithoutExecution, Unterminated and Dropped grow with the turn; ByTool
// and ByLabel grow with the number of distinct tools and labels it touched --
// a turn wide in several of these at once could exceed the total while every
// field stayed inside a cap of its own, which is why there is one ceiling and
// not five.
//
// The five are cut in this FIXED order, each carrying how much of it was
// omitted:
//
//  1. Dropped, 2. Unterminated -- the rarest on a healthy turn, and already
//     empty on most of them; cutting them costs the least.
//  2. WithoutExecution -- a real inventory (denied, failed, or unrecorded),
//     more central than the first two but still a list of ids rather than
//     the turn's headline counts.
//  3. ByLabel, 5. ByTool -- the low-cardinality inventories a reader wants
//     intact most: what this turn touched, in one word each. Cut last.
//
// Each field is cut in HALF of what remains, repeatedly, rather than one
// entry at a time: re-marshalling the whole document to check size is O(size)
// per attempt, and cutting one entry at a time against a turn with thousands
// of them would be O(n) attempts where O(log n) suffices.
func truncate(d *Digest) {
	if !oversize(d) {
		return
	}
	d.Truncated = true

	steps := []func() bool{
		func() bool { return trimStrings(&d.Declarations.Dropped, &d.Declarations.DroppedOmitted) },
		func() bool { return trimStrings(&d.Declarations.Unterminated, &d.Declarations.UnterminatedOmitted) },
		func() bool {
			return trimUnexecuted(&d.Declarations.WithoutExecution, &d.Declarations.WithoutExecutionOmitted)
		},
		func() bool { return trimIntMap(&d.Declarations.ByLabel, &d.Declarations.ByLabelOmitted) },
		func() bool { return trimIntMap(&d.Declarations.ByTool, &d.Declarations.ByToolOmitted) },
	}
	for _, step := range steps {
		for oversize(d) {
			if !step() {
				break // this field is now empty; move to the next one
			}
		}
		if !oversize(d) {
			return
		}
	}
	// Every growable field is now empty and the document is still over cap.
	// Nothing left in it grows with the turn -- session_id, prompt_id,
	// install_id and the fixed-shape counts and flags are all bounded by the
	// store's own schema -- so this is believed unreachable against a real
	// store. Accepting the residual is safer than a sixth field silently
	// deciding it too may be cut.
}

func oversize(d *Digest) bool {
	b, err := json.Marshal(d)
	// A marshal error here would be this program's own bug, not the store's
	// content -- Digest holds no channel, func or cyclic value. Treated as
	// "fits", so that bug surfaces as encoding/json's own error at the real
	// call site in cmd/rashomon rather than as a silently mangled truncation.
	return err == nil && len(b) > CapBytes
}

// trimStrings halves *s from the tail, returning false once it is empty.
func trimStrings(s *[]string, omitted *int) bool {
	if len(*s) == 0 {
		return false
	}
	cut := (len(*s) + 1) / 2
	*omitted += cut
	*s = (*s)[:len(*s)-cut]
	return true
}

func trimUnexecuted(s *[]Unexecuted, omitted *int) bool {
	if len(*s) == 0 {
		return false
	}
	cut := (len(*s) + 1) / 2
	*omitted += cut
	*s = (*s)[:len(*s)-cut]
	return true
}

// trimIntMap halves *m by KEY, sorted first so which half is cut is
// deterministic rather than dependent on map iteration order.
func trimIntMap(m *map[string]int, omitted *int) bool {
	if len(*m) == 0 {
		return false
	}
	keys := make([]string, 0, len(*m))
	for k := range *m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cut := (len(keys) + 1) / 2
	for _, k := range keys[len(keys)-cut:] {
		delete(*m, k)
	}
	*omitted += cut
	return true
}
