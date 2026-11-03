// Package directive is the seam between the person's standing words in a
// Claude Code session's transcript and the continuity briefing that says them
// again after a compaction (DIRECTIVE-KEEP-2026-11-02). Read, the detection
// and pairing, is row DIRECTIVE-KEEP1's; LastBoundary and Briefing, the
// moment and the slot, are COMPACT-SLOT1's. The transcript is the log and the
// briefing a projection of it: the words are read from the transcript when a
// briefing quotes them and are never kept (D160). No model, no network, and
// nothing written.
package directive

// The kinds of directive Read returns.
const (
	KindDirective  = "directive"
	KindCorrection = "correction"
)

// Directive is one standing directive of the person's in a transcript.
type Directive struct {
	// Kind is KindDirective, or KindCorrection for words that restate an
	// earlier directive.
	Kind string
	// Said is the person's words, verbatim: the clause that states the
	// directive, a substring of one typed prompt, at most slot.MaxSaidBytes
	// (240) bytes, never a paraphrase.
	Said string
	// Line is the number of the transcript line the words are on, counted
	// from 1, as LastBoundary counts.
	Line int
	// Restates is the Line of the earlier directive a correction restates,
	// 0 when it restates none.
	Restates int
}

// Read is row DIRECTIVE-KEEP1's contract: every directive that stands in the
// transcript at path, in line order, each from a typed prompt of the
// person's (a user record that is not isMeta, not a peer's message, not a
// scheduled prompt and not a tool result), with each correction's Restates
// naming the directive it says again. A transcript over capBytes is not read.
// It reads through jsonscan and decodes nothing (RES-5), and keeps nothing.
// Until the row's body lands it returns none, and the briefing says nothing.
func Read(transcriptPath string, capBytes int64) ([]Directive, error) {
	return nil, nil
}
