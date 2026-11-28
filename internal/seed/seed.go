// Package seed counts the corrections a person made to their coding agent,
// read through the trace adapter from the Claude Code transcripts already on
// their disk (card SEED1;
// PRD PF-AU1, PF-AU2, PF-HD8 and PF-HD17's read path). It reads, folds and
// renders, and writes one file when asked, the kept before count (WriteKept,
// card SEED1-KEEP); it has no transport. The brief reads that file at session
// start (ReadKept), so a hook links it; it holds no table that code builds at
// start-up (its id arrays are static data), so linking it costs a hook
// process nothing until the brief reads.
package seed

import (
	"cmp"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/trace"
	"github.com/altrace-dev-role/rashomon/internal/trace/claudecode"
)

// Summary is a fold's counts: the sessions, projects and tool calls read,
// one count per kind of correction, and every signature's recurrence.
type Summary struct {
	Sessions, Projects, Calls int

	Interrupted, Rejected, Blocked, PushedBack, FailedAgain int

	// Stale counts transcripts last written before the window, and Unread
	// the lines, files and folders that could not be read: the counts above
	// leave both out, and the card says so rather than letting them read as
	// nothing. The caller that discovered the files sets Stale; Read adds
	// the lines it stepped over to Unread.
	Stale, Unread int

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
type Tally struct {
	// Until, when set, leaves out every line not stamped before it: the
	// install time, for the before count (Kept).
	Until time.Time
	// Key, when set, names the project a transcript belongs to from the
	// first working directory it records (baseline.Key): Kept's projects.
	Key func(cwd string) string

	sessions, projects map[string]struct{}
	calls, unread      int
	kinds              [numKinds]int
	rows               map[sig]*recurrence
	per                map[string]*Tally
}

type sig struct {
	k kind
	c class
}

type recurrence struct {
	times    int
	sessions map[string]struct{}
}

// New is an empty Tally.
func New() *Tally {
	return &Tally{sessions: map[string]struct{}{}, projects: map[string]struct{}{}, rows: map[sig]*recurrence{}}
}

// file is one transcript's state: the calls it has seen, by id, the turn
// since the person last spoke, and its project's tally once Key has named it.
type file struct {
	t       *Tally
	pt      *Tally
	keyed   bool
	skip    bool
	project string
	sub     bool
	calls   map[string]call
	turn    turn
}

type call struct {
	name string
	c    class
}

// turn is what the agent did since the person's last line: whether it did
// anything, whether its last block was a question, its last call, and the
// calls that failed and have not been tried again.
type turn struct {
	acted, question, hasCall bool
	last                     class
	pending                  []call
}

// Read folds one transcript: project names the projects/ folder it sits in,
// sub whether it is a subagent's. A line that does not parse is passed over;
// one longer than report.TranscriptMaxLine is stepped over and counted in
// Unread, as the report's reader counts one.
func (t *Tally) Read(r io.Reader, project string, sub bool) error {
	f := &file{t: t, project: project, sub: sub, calls: map[string]call{}}
	skipped, err := claudecode.Read(r, f.step)
	t.unread += skipped
	return err
}

// step folds one step. An Entry opens a record: one stamped at or after
// Until leaves out every step of its record, and the first record with a
// working directory names the project when Key is set.
func (f *file) step(s trace.Step) {
	if s.Kind == trace.Entry {
		f.skip = !f.t.Until.IsZero() && !(!s.At.IsZero() && s.At.Before(f.t.Until))
		if !f.skip && f.t.Key != nil && !f.keyed && s.Cwd != "" {
			f.pt, f.keyed = f.t.project(f.t.Key(s.Cwd)), true
		}
		return
	}
	if f.skip {
		return
	}
	switch s.Kind {
	case trace.Message:
		f.t.saw(s.Session, f.project)
		if f.pt != nil {
			f.pt.saw(s.Session, f.project)
		}
		f.turn.acted = true
	case trace.Call:
		f.called(s.Call, s.Session)
	case trace.Text:
		if t := strings.TrimSpace(s.Text); t != "" {
			f.turn.question = strings.HasSuffix(t, "?")
		}
	case trace.Result:
		f.result(&s)
	case trace.Said:
		f.said(&s)
	}
}

// project is key's tally, made on first use; nil for "".
func (t *Tally) project(key string) *Tally {
	if key == "" {
		return nil
	}
	if t.per == nil {
		t.per = map[string]*Tally{}
	}
	p := t.per[key]
	if p == nil {
		p = New()
		t.per[key] = p
	}
	return p
}

func (f *file) called(c *trace.CallStep, session string) {
	k := call{c.Tool, classOf(c)}
	f.calls[c.ID] = k
	f.t.calls++
	if f.pt != nil {
		f.pt.calls++
	}
	f.turn.last, f.turn.hasCall, f.turn.question = k.c, true, false
	f.retried(c.Tool, session)
}

// retried counts each failure of the turn that this call to the same tool
// tries again, once.
func (f *file) retried(name, session string) {
	kept := f.turn.pending[:0]
	for _, p := range f.turn.pending {
		if p.name == name {
			f.add(failedAgain, p.c, session)
			continue
		}
		kept = append(kept, p)
	}
	f.turn.pending = kept
}

// result reads one tool result: a rejection is the person's act and ends the
// turn; a rule's refusal is counted; a tool error of a known kind waits for
// the agent to try the same tool again.
func (f *file) result(s *trace.Step) {
	r := s.Result
	if !r.Failed {
		return
	}
	c, ok := f.calls[r.CallID]
	if !ok {
		c = call{c: cOther}
	}
	switch r.Denied {
	case trace.DeniedByPerson:
		f.add(rejected, c.c, s.Session)
		f.turn = turn{}
	case trace.DeniedByRule:
		f.add(blocked, c.c, s.Session)
	case trace.DeniedNone:
		if e, ok := errorClass(r.Error); ok {
			f.turn.pending = append(f.turn.pending, call{c.name, e})
		}
	}
}

// said reads a line in the person's slot and ends the turn. Only a main
// session's lines count: a subagent's transcript echoes the interrupt that
// stopped its parent.
func (f *file) said(s *trace.Step) {
	text := strings.TrimSpace(s.Text)
	if text == "" {
		return
	}
	if !f.sub && !s.Sidechain && f.turn.acted && s.Origin != trace.OriginMachine {
		last := cReply
		if f.turn.hasCall {
			last = f.turn.last
		}
		switch {
		case s.Signal == trace.SignalInterrupt:
			f.add(interrupted, last, s.Session)
		case s.Signal == trace.SignalInterruptForTool:
			// The rejection before it was the act.
		case pushback(text) && !(f.turn.question && startsNo(text)):
			f.add(pushedBack, last, s.Session)
		}
	}
	f.turn = turn{}
}

func (f *file) add(k kind, c class, session string) {
	f.t.add(k, c, session)
	if f.pt != nil {
		f.pt.add(k, c, session)
	}
}

func (t *Tally) saw(session, project string) {
	if session != "" {
		t.sessions[session] = struct{}{}
	}
	t.projects[project] = struct{}{}
}

func (t *Tally) add(k kind, c class, session string) {
	t.kinds[k]++
	s := sig{k, c}
	r := t.rows[s]
	if r == nil {
		r = &recurrence{sessions: map[string]struct{}{}}
		t.rows[s] = r
	}
	r.times++
	if session != "" {
		r.sessions[session] = struct{}{}
	}
}

// Summary is the fold so far.
func (t *Tally) Summary() Summary {
	s := Summary{
		Sessions: len(t.sessions), Projects: len(t.projects), Calls: t.calls, Unread: t.unread,
		Interrupted: t.kinds[interrupted], Rejected: t.kinds[rejected], Blocked: t.kinds[blocked],
		PushedBack: t.kinds[pushedBack], FailedAgain: t.kinds[failedAgain],
	}
	for g, r := range t.rows {
		s.Rows = append(s.Rows, Row{rowText(g.k, g.c), r.times, len(r.sessions)})
	}
	slices.SortFunc(s.Rows, func(a, b Row) int {
		return cmp.Or(cmp.Compare(b.Sessions, a.Sessions), cmp.Compare(b.Times, a.Times), strings.Compare(a.Text, b.Text))
	})
	return s
}
