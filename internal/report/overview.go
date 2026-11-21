package report

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// OverviewOption sets one choice of the overview render.
type OverviewOption func(*overviewOptions)

type overviewOptions struct{ color bool }

// WithOverviewColor paints the overview with ANSI colors. The caller decides
// when (cmd/rashomon's useColor); a render never looks at its writer, so a
// piped run gets no escape byte unless a caller asks for one.
func WithOverviewColor() OverviewOption { return func(o *overviewOptions) { o.color = true } }

// Overview writes the one-session view of rep (card RPT-1): a header, a box
// with the recording and the last Stop, what Rashomon did, what needs
// attention, the project's knowledge, the session's activity and where the
// full evidence is. It shows the first session of rep that is not the calls
// without a session id; the caller builds rep for that one session.
//
// Every sentence rests on a field the report computes, and a part of the
// record this report did not read says so: it never reads as none or zero.
func Overview(w io.Writer, rep *Report, opts ...OverviewOption) error {
	var o overviewOptions
	for _, opt := range opts {
		opt(&o)
	}
	s := overviewSession(rep)
	if s == nil {
		return errors.New("report: no session to show")
	}
	v := &ovView{color: o.color}
	rec := ovRecordingOf(s)
	v.header(rep, s)
	v.box(s, rec)
	v.whatDid(s, rec.complete)
	v.attention(s)
	v.knowledge(s)
	v.activity(s)
	v.evidence(rep, s)
	_, err := w.Write(v.b.Bytes())
	return err
}

// overviewSession is the session the overview shows: the first one that is
// a session, or nil.
func overviewSession(rep *Report) *Session {
	if rep == nil {
		return nil
	}
	for i := range rep.Sessions {
		if rep.Sessions[i].SessionID != store.UnattributedSession {
			return &rep.Sessions[i]
		}
	}
	return nil
}

// ovView is one overview being written: the text and whether to paint it.
type ovView struct {
	b     bytes.Buffer
	color bool
}

// The overview's width and its SGR codes: green for a check mark and its
// heading, amber for a bang, its heading and the item numbers, blue for next
// steps and evidence commands, dim for section heads and labels.
const (
	ovWidth = 100
	ovGreen = "32"
	ovAmber = "33"
	ovBlue  = "34"
	ovDim   = "2"
)

// paint wraps s in one SGR code when color is on; padding is computed on the
// plain text before, so a painted line keeps its columns.
func (v *ovView) paint(sgr, s string) string {
	if !v.color || sgr == "" || s == "" {
		return s
	}
	return "\x1b[" + sgr + "m" + s + "\x1b[0m"
}

// markColor is the color of a mark: green for a check, amber for a bang,
// none for a question mark or a dash.
func markColor(mark string) string {
	switch mark {
	case "✓":
		return ovGreen
	case "!":
		return ovAmber
	}
	return ""
}

func (v *ovView) line(s string) {
	v.b.WriteString(s)
	v.b.WriteByte('\n')
}

func (v *ovView) blank() { v.b.WriteByte('\n') }

// section is a head between blank lines.
func (v *ovView) section(title string) {
	v.blank()
	v.line(v.paint(ovDim, title))
	v.blank()
}

// para is s wrapped at the overview's width.
func (v *ovView) para(s string) {
	for _, l := range ovWrap(s, ovWidth) {
		v.line(l)
	}
}

// header is the session's short id and the time the report was generated,
// then the project, the calls, the prompts and the subagents.
func (v *ovView) header(rep *Report, s *Session) {
	left := "rashomon · session " + ovShort(s.SessionID)
	right := "generated " + time.UnixMilli(rep.GeneratedAtUnixMS).UTC().Format("2006-01-02 15:04") + " UTC"
	v.line(left + ovPad(max(ovWidth-ovLen(left)-ovLen(right), 1)) + right)
	var parts []string
	if k := s.Knowledge; k != nil && k.Project != "" {
		parts = append(parts, "project "+printable(k.Project))
	}
	n, p := s.Declarations.Recorded, len(s.Chains.Prompts)
	calls := fmt.Sprintf("%d tool call%s across %d prompt%s", n, plural(n), p, plural(p))
	if u := len(s.Chains.Unattributed); u > 0 {
		calls += fmt.Sprintf(", %d under no prompt", u)
	}
	parts = append(parts, calls)
	switch sub := len(s.Subagents); sub {
	case 0:
		parts = append(parts, "no subagents")
	default:
		parts = append(parts, fmt.Sprintf("%d subagent%s", sub, plural(sub)))
	}
	v.line(strings.Join(parts, " · "))
}

// ovRecording is the box's first part: its mark, heading and sentence, and
// whether every clause of the complete state held.
type ovRecording struct {
	mark, head, body string
	complete         bool
}

// ovRecordingOf is the recording's state. The check mark holds only when
// coverage is verified with no reason, at least one transcript was read, no
// read transcript lists a call missing from the store, missing from the
// transcript or executed but unrecorded, and no gap is recorded. The lists
// are read here as well as the reasons, so a record whose reasons lag its
// lists still reads incomplete. Verified with no transcript read is a
// question mark; anything else is the bang with the reasons in words.
func ovRecordingOf(s *Session) ovRecording {
	reasons := slices.Clone(s.Coverage.Reasons)
	add := func(code string, on bool) {
		if on && !slices.Contains(reasons, code) {
			reasons = append(reasons, code)
		}
	}
	var read []Transcript
	var mismatch, unexecuted bool
	for _, t := range s.Transcripts {
		if t.Status != TranscriptRead {
			continue
		}
		read = append(read, t)
		mismatch = mismatch || len(t.MissingFromStore) > 0 || len(t.MissingFromTranscript) > 0
		unexecuted = unexecuted || len(t.ExecutedButUnrecorded) > 0
	}
	add(ReasonTranscriptMismatch, mismatch)
	add(ReasonExecutionMismatch, unexecuted)
	add(ReasonGap, len(s.Gaps) > 0)
	switch n := s.Declarations.Recorded; {
	case s.Coverage.State != store.StateVerified || len(reasons) > 0:
		return ovRecording{mark: "!", head: "RECORDING INCOMPLETE", body: "The record is not verified complete" + ovReasons(reasons)}
	case len(read) == 0:
		return ovRecording{mark: "?", head: "RECORDING NOT CHECKED AGAINST A TRANSCRIPT",
			body: fmt.Sprintf("%d tool call%s %s recorded. No transcript was read, so the record was not compared with one.", n, plural(n), wasWere(n))}
	}
	return ovRecording{mark: "✓", head: "RECORDING COMPLETE", body: ovComplete(read), complete: true}
}

// ovReasons is the reasons in words, joined by "; ", or a bare full stop. An
// unknown code prints as it is. The long view's pointer to lines this view
// does not print ("; see the two `missing from` lines below") is cut.
func ovReasons(codes []string) string {
	if len(codes) == 0 {
		return "."
	}
	texts := make([]string, len(codes))
	for i, c := range codes {
		t, ok := reasonText[c]
		if !ok {
			texts[i] = printable(c)
			continue
		}
		if j := strings.Index(t, "; see "); j >= 0 {
			t = t[:j]
		}
		texts[i] = t
	}
	return ": " + strings.Join(texts, "; ") + "."
}

// ovComplete is the complete state's sentence. Its count is the ids of every
// read transcript less those refused before any hook ran (in neither missing
// list, report.go); "with their results" only when every read transcript
// counted its results and they are no fewer than its ids.
func ovComplete(read []Transcript) string {
	ids, results, refused, counted := 0, 0, 0, true
	for _, t := range read {
		if t.IDsInTranscript != nil {
			ids += *t.IDsInTranscript
		}
		if t.ResultsInTranscript == nil {
			counted = false
		} else {
			results += *t.ResultsInTranscript
		}
		refused += len(t.RefusedBeforeHooks)
	}
	n := ids - refused
	where := "in the session transcript"
	if refused > 0 {
		where = "that reached a hook"
	}
	s := fmt.Sprintf("All %d tool calls %s were recorded", n, where)
	if n == 1 {
		s = fmt.Sprintf("The 1 tool call %s was recorded", where)
	}
	switch {
	case counted && results >= ids && n == 1:
		s += " with its result"
	case counted && results >= ids:
		s += " with their results"
	default:
		s += ", and every result in the transcript has its execution recorded"
	}
	if refused > 0 {
		s += fmt.Sprintf("; %d more %s refused before any hook ran", refused, wasWere(refused))
	}
	return s + ". No recording gaps were detected."
}

// ovStopPart is the box's second part, the last Stop. The verified row rests
// on verify's TurnSummary (evaluate.go, first match wins): the turn's gates
// known and the worst of them green, where an edit after a green run leaves
// a gate stale; no defect; the turn wrote; no advisory cap; no unnamed write
// after the earliest green run; and the record green. So its heading says
// the gates were green after the last edit, and no more.
func ovStopPart(a *Autosteer) ovRecording {
	switch {
	case a == nil:
		return ovRecording{mark: "?", head: "LAST STOP NOT READ", body: "The last Stop was not read here, so no gate state is shown."}
	case a.Stop == nil:
		return ovRecording{mark: "?", head: "NO STOP RECORDED", body: "No Stop was recorded for this session, so no gate state was read."}
	case a.Stop.Row == "verified": // verify.RowVerified; report does not import verify
		return ovRecording{mark: "✓", head: "GATES GREEN AFTER THE LAST EDIT", body: ovStopAt(a.Stop) + " Gates: " + storyGates(a.Gates) + "."}
	case len(a.Gates) == 0:
		return ovRecording{mark: "!", head: "TASK COMPLETION NOT VERIFIED",
			body: "Rashomon cannot confirm the final work: no verification gate (a test, build or lint command whose result settles a check) is known for this project."}
	}
	return ovRecording{mark: "!", head: "TASK COMPLETION NOT VERIFIED", body: ovStopAt(a.Stop) + " Gates: " + storyGates(a.Gates) + "."}
}

// ovStopAt is what the last Stop printed, with one full stop.
func ovStopAt(st *AutosteerStop) string {
	if s := strings.TrimSuffix(strings.TrimSpace(printable(st.Sentence)), "."); s != "" {
		return fmt.Sprintf("At the last Stop (call %d): %s.", st.Seq, s)
	}
	return fmt.Sprintf("The last Stop (call %d) printed no state line.", st.Seq)
}

// box is, after a blank line, the recording, the last Stop and, only when the
// recording is complete, what a complete recording does not show.
func (v *ovView) box(s *Session, rec ovRecording) {
	parts := []ovRecording{rec, ovStopPart(s.Autosteer)}
	if rec.complete {
		parts = append(parts, ovRecording{body: "A complete recording shows the calls the agent made. It does not show that the work is correct."})
	}
	inner := ovWidth - 4
	v.blank()
	v.line("┌" + strings.Repeat("─", ovWidth-2) + "┐")
	for i, p := range parts {
		if i > 0 {
			v.boxLine("", "")
		}
		if p.head != "" {
			h := p.mark + " " + p.head
			v.boxLine(v.paint(markColor(p.mark), h), h)
		}
		for _, l := range ovWrap(p.body, inner) {
			v.boxLine(l, l)
		}
	}
	v.line("└" + strings.Repeat("─", ovWidth-2) + "┘")
}

// boxLine is one line inside the box, padded on its plain text.
func (v *ovView) boxLine(painted, plain string) {
	v.line("│ " + painted + ovPad(ovWidth-4-ovLen(plain)) + " │")
}

// whatDid is WHAT RASHOMON DID: the lead, then the four rows when the last
// Stop's record was read. With no record or no Stop, the lists are not read
// (autosteerOf returns them empty), so no row prints a count.
func (v *ovView) whatDid(s *Session, complete bool) {
	v.section("WHAT RASHOMON DID")
	start := "Recorded the whole session"
	if !complete {
		n := s.Declarations.Recorded
		start = fmt.Sprintf("Recorded %d call%s", n, plural(n))
	}
	a := s.Autosteer
	switch {
	case a == nil:
		v.para(start + "; AutoSteer's record of it was not read here.")
		return
	case a.Stop == nil:
		v.para(start + "; no Stop was recorded, so AutoSteer's record of it was not read.")
		return
	}
	f := ovCountFindings(a.Findings)
	notes := ovNotes(a.Delivered)
	if len(a.Delivered) == 0 {
		notes = "no notes"
	}
	v.para(start + ", and AutoSteer sent the agent " + notes + "." + f.lead())
	v.blank()
	if len(a.Delivered) == 0 {
		v.row("Guidance delivered", "–", "None sent.")
	} else {
		paras := []string{notes + "."}
		if seqs := ovNoteSeqs(a.Delivered); seqs != "" {
			paras = append(paras, "Calls "+seqs)
		}
		v.row("Guidance delivered", "✓", paras...)
		v.row("Agent action observed", "–",
			fmt.Sprintf("Not recorded. This report has no trace of how the agent responded to the note%s.", plural(len(a.Delivered))))
	}
	mark, outcome := f.outcome()
	v.row("Outcome verified", mark, outcome)
	v.row("Improvement demonstrated", "?", ovImprovement(a.Mistakes))
	if len(a.Delivered) > 0 {
		v.blank()
		v.para("Rashomon was active, but the available evidence does not establish whether its guidance improved the final result.")
	}
}

// row is one row of WHAT RASHOMON DID: the label padded to 27 columns, the
// mark, two spaces, and each paragraph wrapped at 70 under column 30.
func (v *ovView) row(label, mark string, paras ...string) {
	var ls []string
	for _, p := range paras {
		ls = append(ls, ovWrap(p, ovWidth-30)...)
	}
	for i, l := range ls {
		if i == 0 {
			v.line(v.paint(ovDim, label) + ovPad(27-ovLen(label)) + v.paint(markColor(mark), mark) + "  " + l)
			continue
		}
		v.line(ovPad(30) + l)
	}
}
