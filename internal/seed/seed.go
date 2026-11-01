// Package seed counts the corrections a person made to their coding agent,
// read from the Claude Code transcripts already on their disk (card SEED1;
// PRD PF-AU1, PF-AU2, PF-HD8 and PF-HD17's read path). It reads, folds and
// renders; it opens no store, writes nothing and has no transport. Nothing on
// a hook path imports it, and it holds no package-level table, so linking it
// into the binary costs a hook process nothing at start-up.
package seed

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
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
	sessions, projects map[string]struct{}
	calls, unread      int
	kinds              [numKinds]int
	rows               map[sig]*recurrence
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

// line is the part of a transcript line the fold reads.
type line struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	// DenialKind is Claude Code's own word for a refused call (2.1.280):
	// "user-rejected" when the person said no at the prompt,
	// "permission-rule" when a hook or a settings rule refused it. A line
	// without it is read from the result's text (report.UserRejected).
	DenialKind string `json:"toolDenialKind"`
	Message    struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
	Text      string          `json:"text"`
}

// file is one transcript's state: the calls it has seen, by id, and the
// turn since the person last spoke.
type file struct {
	t       *Tally
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
	sc := linescan.New(r, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	for sc.Scan() {
		b := sc.Bytes()
		// Only user and assistant lines carry what is counted; progress,
		// snapshot and summary lines, most of a transcript's bytes, are
		// passed over before any decode.
		if !bytes.Contains(b, []byte(`"type":"user"`)) && !bytes.Contains(b, []byte(`"type":"assistant"`)) {
			continue
		}
		var l line
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		switch l.Type {
		case "assistant":
			f.assistant(&l)
		case "user":
			f.user(&l)
		}
	}
	t.unread += sc.Skipped()
	return sc.Err()
}

func (f *file) assistant(l *line) {
	var blocks []block
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	if l.SessionID != "" {
		f.t.sessions[l.SessionID] = struct{}{}
	}
	f.t.projects[f.project] = struct{}{}
	f.turn.acted = true
	for i := range blocks {
		b := &blocks[i]
		switch b.Type {
		case "tool_use":
			c := call{b.Name, classOf(b.Name, b.Input)}
			f.calls[b.ID] = c
			f.t.calls++
			f.turn.last, f.turn.hasCall, f.turn.question = c.c, true, false
			f.retried(b.Name, l.SessionID)
		case "text":
			if s := strings.TrimSpace(b.Text); s != "" {
				f.turn.question = strings.HasSuffix(s, "?")
			}
		}
	}
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

func (f *file) user(l *line) {
	if l.IsMeta {
		return
	}
	c := bytes.TrimSpace(l.Message.Content)
	if len(c) > 0 && c[0] == '"' {
		var s string
		if json.Unmarshal(c, &s) == nil {
			f.said(l, s)
		}
		return
	}
	var blocks []block
	if json.Unmarshal(c, &blocks) != nil {
		return
	}
	for i := range blocks {
		if blocks[i].Type == "tool_result" {
			f.result(l, &blocks[i])
		}
	}
	for i := range blocks {
		if blocks[i].Type == "text" {
			f.said(l, blocks[i].Text)
			return
		}
	}
}

// result reads one tool_result: a rejection is the person's act and ends the
// turn; a rule's refusal is counted; a tool error of a known kind waits for
// the agent to try the same tool again.
func (f *file) result(l *line, b *block) {
	if !b.IsError {
		return
	}
	c, ok := f.calls[b.ToolUseID]
	if !ok {
		c = call{c: cOther}
	}
	switch l.DenialKind {
	case "user-rejected":
		f.add(rejected, c.c, l.SessionID)
		f.turn = turn{}
		return
	case "permission-rule":
		f.add(blocked, c.c, l.SessionID)
		return
	case "":
		// A version before the field: the text says which, below.
	default:
		// A kind this reader does not know is not counted.
		return
	}
	text, denied, _ := report.ResultClass(true, b.Content)
	switch {
	case denied && report.UserRejected(text):
		f.add(rejected, c.c, l.SessionID)
		f.turn = turn{}
	case denied:
	default:
		if e, ok := errorClass(text); ok {
			f.turn.pending = append(f.turn.pending, call{c.name, e})
		}
	}
}

// said reads a line the person typed (or a marker Claude Code writes as
// theirs) and ends the turn. Only a main session's lines count: a
// subagent's transcript echoes the interrupt that stopped its parent.
func (f *file) said(l *line, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if !f.sub && !l.IsSidechain && f.turn.acted && !strings.HasPrefix(text, "<") {
		last := cReply
		if f.turn.hasCall {
			last = f.turn.last
		}
		switch {
		case text == interruptMark:
			f.add(interrupted, last, l.SessionID)
		case strings.HasPrefix(text, "[Request interrupted"):
			// "... for tool use": the rejection before it was the act.
		case pushback(text) && !(f.turn.question && startsNo(text)):
			f.add(pushedBack, last, l.SessionID)
		}
	}
	f.turn = turn{}
}

// interruptMark is the line Claude Code writes when the person stops the
// agent mid-turn.
const interruptMark = "[Request interrupted by user]"

func (f *file) add(k kind, c class, session string) {
	f.t.kinds[k]++
	s := sig{k, c}
	r := f.t.rows[s]
	if r == nil {
		r = &recurrence{sessions: map[string]struct{}{}}
		f.t.rows[s] = r
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
