// Package seed counts the corrections a person made to their coding agent,
// read from the Claude Code transcripts already on their disk (card SEED1;
// PRD PF-AU1, PF-AU2, PF-HD8 and PF-HD17's read path). It reads, folds and
// renders; it opens no store, writes nothing and has no transport. Nothing on
// a hook path imports it.
package seed

import "io"

// Summary is a fold's counts: the sessions, projects and tool calls read,
// one count per kind of correction, and every signature's recurrence.
type Summary struct {
	Sessions, Projects, Calls int

	Interrupted, Rejected, Blocked, PushedBack, FailedAgain int

	// Rows is every signature, most sessions first, then most times, then
	// by text. Its text is built from this package's constants only.
	Rows []Row
}

// Row is one failure signature and its recurrence.
type Row struct {
	Text            string
	Times, Sessions int
}

// Tally folds transcripts one at a time.
type Tally struct{}

// New is an empty Tally.
func New() *Tally { return &Tally{} }

// Read folds one transcript: project names the projects/ folder it sits in,
// sub whether it is a subagent's.
func (t *Tally) Read(r io.Reader, project string, sub bool) error { return nil }

// Summary is the fold so far.
func (t *Tally) Summary() Summary { return Summary{} }

// Card renders s over a window of days.
func Card(s Summary, days int) string { return "" }

// Vocabulary is every word Card can print but a number.
func Vocabulary() []string { return nil }

func pushback(text string) bool { return false }
