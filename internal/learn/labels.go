package learn

import "bytes"

// labelIndex is labels.ndjson by lesson: the sessions that have labeled it
// and its last row, so a writer labels a session once and the listing names
// the latest basis (card LABEL-WRITER1).
type labelIndex struct {
	sessions map[string]map[string]bool
	last     map[string]Label
}

// indexLabels reads labels.ndjson as tallies does; a line that does not
// parse is not a row.
func indexLabels(b []byte) labelIndex {
	var x labelIndex
	for line := range bytes.SplitSeq(b, []byte("\n")) {
		var lb Label
		if decode(line, &lb) != nil || lb.LessonID == "" {
			continue
		}
		x.add(lb)
	}
	return x
}

func (x *labelIndex) add(lb Label) {
	if x.sessions == nil {
		x.sessions, x.last = map[string]map[string]bool{}, map[string]Label{}
	}
	if x.sessions[lb.LessonID] == nil {
		x.sessions[lb.LessonID] = map[string]bool{}
	}
	x.sessions[lb.LessonID][lb.SessionID] = true
	x.last[lb.LessonID] = lb
}

// Labeled is whether session has a label row for lesson id.
func (l *Ledger) Labeled(id, session string) bool { return l.labels.sessions[id][session] }

// Last is lesson id's last label row in file order, and whether it has one.
func (l *Ledger) Last(id string) (Label, bool) {
	lb, ok := l.labels.last[id]
	return lb, ok
}
