package directive

import "github.com/altrace-dev-role/rashomon/internal/steer"

// MaxLines is the most quotes the briefing holds under its header.
const MaxLines = 5

// Briefing is the continuity briefing at the compaction b. Red: none.
func Briefing(ds []Directive, b Boundary, digest func(string) string, afterSeq int64) (steer.Finding, bool) {
	return steer.Finding{}, false
}
