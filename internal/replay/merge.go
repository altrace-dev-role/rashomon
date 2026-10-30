package replay

import (
	"bytes"
	"errors"
	"io"
	"iter"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// This file builds one session's single timeline from all the evidence the
// session left (docs/autosteer-prescan.md, §6.2). A session is its Claude Code
// sessionId, and its evidence can be:
//
//   - the main transcript;
//   - a resumed session, as another file or as appended history, which may
//     repeat the turns before it;
//   - a copy: the same session under two project directories, or a backup;
//   - subagent files, <sid>/subagents/agent-<id>.jsonl, and sidechain lines
//     inside the main file in older versions.
//
// WalkSession reads them all and yields the events of the session as the live
// recorder would have received them, once each, in one order. Everything B2a and
// B2b guarantee of one file holds of the merged timeline: an event's payload is
// built by the same code (Walk), events are ordered by when they happened, a
// point sees exactly the events before it, and a gap taints what follows.
//
// # Two passes
//
// The first pass reads every file for its calls (history.Read) and builds the
// session's index: for each tool_use_id, a digest of the call's input and of its
// result, the Agent calls and the agents their results name, and the type each
// subagent's lines are attributed to. It decides, before anything is yielded,
// which calls have copies that disagree and which subagents have a type. The
// second pass walks each file (Walk) and merges the streams.
//
// # Merging
//
// Each file is a stream of events in its own order, which the merge never
// disturbs. The streams are merged by time: the next event yielded is the
// earliest head among the streams, ranked by the timestamp of the line it was
// taken from (an event with none takes its stream's previous one), then, for
// the same instant, by class (the main agent's declarations and prompts, then a
// subagent's events, then the main agent's executions and turn ends, so a
// subagent's result comes before the Agent call that returned it), then by the
// file's place in the order of OrderFiles. A subagent's event is never placed
// before the parent's Agent call was made. The rule is a function of the files'
// bytes alone, so the same session in any directories gives the same timeline.
//
// # Copies
//
// An event is a copy when its call (tool_use_id) was already yielded as the
// same kind of event, or when it is a prompt or a turn end with the same
// payload and the same time as one already yielded. A copy is dropped and
// counted; it contributes nothing. It is dropped whichever file it is in: the
// earlier in the merge is kept, and that is the same event.
//
// If two copies of one tool_use_id disagree, on the tool, the input or the
// result, the call cannot be trusted and neither is: its events are Unsupported
// (CopiesDisagree), so they are Gaps, and the timeline is tainted from the first
// of them on. A copy that has no result where another has one is not a
// disagreement: the file was cut there, and the one that has it is the more
// complete. A prompt or a turn end that two files give differently is not
// detected as a disagreement (their time is the key); the calls are.
//
// # Subagents
//
// The type of a subagent's calls is recovered through one link and no other
// (revision 7, ruled). The subagent's lines carry a top-level agentId. In the
// parent transcript, exactly one Agent or Task call is answered by a line whose
// toolUseResult.agentId is that id, and that call's input.subagent_type is the
// type, byte for byte what the hook payloads' agent_type is. Nothing else is a
// source: not the text of a message (some results end with an `agentId: ...`
// trailer), not a file name, not the agent-<id>.meta.json beside the file,
// which is paired only by name. The lines' own attributionAgent is read as a
// cross-check, never as a source: if any line of the subagent names a type
// other than the one the link gave, the subagent's calls stay Unsupported.
//
// A subagent whose id no Agent call claims, or that two different calls claim
// (or one call claims along with another id), or whose claimant is not an Agent
// or Task call with a subagent_type, or whose claimant's copies disagree, keeps
// SidechainAgentType: its calls are Unsupported, Gaps, and taint what follows.
// Two copies of the one call claiming the same agent are one parent, not two.
// The state of each subagent's link is in MergeSummary.Agents.
//
// A subagent's own lines open no turn, so the prompt and the permission mode of
// its payloads are those of the turn the parent made the Agent call in.
//
// # Taint
//
// Taint is recomputed over the merged order: it is the earliest gap, over all
// files, among the events yielded so far. A file's unreadable line is a gap at
// the place it falls in its own stream, and the merge cannot say what instant
// that is, so it takes effect as early as it could have: right after the last
// event of that file before it (or from the start of the merge, when the file's
// first event is already behind one). A file that ends in an unreadable line
// taints from the end of its events.
//
// # Bounds
//
// The merge holds a digest per tool_use_id of the session, at most MaxCallIDs
// of them (20,000, §6.7). A call past the cap cannot be recognised if a later
// file repeats it, so it is Unsupported (SessionIDCap) and the summary says the
// cap was hit. At most MaxSessionFiles files are read for one session, each
// streaming one event ahead, and the files past it are not read and taint the
// whole timeline. Nothing is held beyond one event per file, the digests, and
// the turn keys (capped the same way).

// Bounds of the merge.
const (
	// DefaultMaxCallIDs is MergeOptions.MaxCallIDs when zero: the §6.7 cap on
	// the tool_use_id digests held per session.
	DefaultMaxCallIDs = 20000
	// MaxSessionFiles is how many files of one session are read.
	MaxSessionFiles = 128

	maxClaimants    = 4
	maxAgents       = 4096
	maxAttributions = 4
	maxAgentDepth   = 4
)

// Source is one file of a session. Open is called when the file is read, and
// the file is closed when that read is done.
type Source struct {
	// Name is the caller's name for the file, for its own reporting.
	Name string
	// Main: the file is the session's own (OrderFiles' first group), not a
	// subagent's. It decides how the final-message budget is shared.
	Main bool
	Open func() (io.ReadSeekCloser, error)
}

// MergeOptions configure WalkSession.
type MergeOptions struct {
	// Options are Walk's, for every file. Agents is set by WalkSession.
	Options
	// MaxCallIDs is the most tool_use_id digests held; zero is
	// DefaultMaxCallIDs.
	MaxCallIDs int
}

func (o MergeOptions) maxIDs() int {
	if o.MaxCallIDs <= 0 {
		return DefaultMaxCallIDs
	}
	return o.MaxCallIDs
}

// Place is a position in one file of a session's files.
type Place struct {
	File int
	Pos  history.Pos
}

// IsZero reports no place.
func (p Place) IsZero() bool { return p.Pos.IsZero() }

// LinkStatus is how the link of one subagent to its parent's Agent call stands.
type LinkStatus string

const (
	// LinkOK: exactly one Agent or Task call claims the agent, names its type,
	// and no line of the agent contradicts it.
	LinkOK LinkStatus = "linked"
	// LinkNoParent: no Agent or Task call is answered by a line naming the
	// agent (the parent was never read, or its result line could not be).
	LinkNoParent LinkStatus = "no_parent"
	// LinkAmbiguous: two different calls claim the agent, or the one that does
	// also claims another.
	LinkAmbiguous LinkStatus = "ambiguous_parent"
	// LinkParentConflict: the claiming call's copies disagree.
	LinkParentConflict LinkStatus = "parent_copies_disagree"
	// LinkNoType: the claiming call's input names no subagent_type.
	LinkNoType LinkStatus = "no_subagent_type"
	// LinkAttributionMismatch: a line of the agent names a type other than the
	// link's (attributionAgent).
	LinkAttributionMismatch LinkStatus = "attribution_mismatch"
	// LinkNoTurn: the claim stands but the parent turn is not known (the Agent
	// call is before any prompt), so no prompt can be named.
	LinkNoTurn LinkStatus = "no_turn"
)

// AgentLink is the link of one subagent seen in the session's lines.
type AgentLink struct {
	AgentID string
	// Type is the type the link gave, "" unless Status is LinkOK or LinkNoTurn.
	Type   string
	Status LinkStatus
	// Calls is how many distinct calls carry the agentId.
	Calls int
}

// FileSummary is what Walk saw of one file.
type FileSummary struct {
	Name    string
	Main    bool
	Summary Summary
}

// MergeSummary is what WalkSession saw besides the events.
type MergeSummary struct {
	Files  []FileSummary
	Agents []AgentLink
	// Events were yielded. Copies were dropped as the same event again,
	// OtherSession for belonging to another session of a file that names
	// several.
	Events, Copies, OtherSession int
	// CallIDs is how many tool_use_ids were tracked; CapHit: a call or a turn
	// came past MaxCallIDs; Untracked is how many calls did.
	CallIDs   int
	CapHit    bool
	Untracked int
	// OrderGaps counts the events whose place among the files is not
	// established (the reasons ResultBeforeUse, OrderAmbiguous, OrderRegression
	// and SubagentOutsideCall name); each taints from where it is found.
	OrderGaps int
	// Conflicts is how many calls have copies that disagree.
	Conflicts int
	// FilesDropped were not read, past MaxSessionFiles.
	FilesDropped int
	// TaintFrom is where evidence became incomplete, across the whole merged
	// timeline; zero if it never did. TaintReason is the first gap's reason, or
	// "unreadable_line", or "session_file_cap".
	TaintFrom   Place
	TaintReason string
	Gaps        int
	Unreadable  int
	// Missing is how many calls were declared and never answered in any file
	// (a denial is not one).
	Missing int
	// Complete: the timeline ends at a turn end the transcript establishes, no
	// call is unanswered, and nothing is outstanding: the session reads as
	// finished.
	Complete bool
}

// SessionTimeline is a merged session held whole, for tests and small
// sessions.
type SessionTimeline struct {
	Events []Event
	MergeSummary
}

// BuildSession is WalkSession holding every event.
func BuildSession(sessionID string, files []Source, opt MergeOptions) (SessionTimeline, error) {
	var tl SessionTimeline
	sum, err := WalkSession(sessionID, files, opt, func(e Event) error {
		tl.Events = append(tl.Events, e)
		return nil
	})
	tl.MergeSummary = sum
	return tl, err
}

// stream is one file's events, pulled one at a time.
type stream struct {
	i    int
	src  Source
	next func() (Event, bool)
	stop func()

	head Event
	has  bool
	// kt and kc are the head's merge key: its time and its class.
	kt time.Time
	kc int
	// viol is a reason the head's place among the other files is not
	// established (a Reason), "" for none.
	viol Reason

	prevKt    time.Time
	lastTaint history.Pos // the last event consumed, its file-local Taint
	firstGap  history.Pos
	consumed  int
	lineTaint bool // the stream's unreadable-line taint has been applied
	sum       Summary
	err       error
	exhausted bool
	tailTaint bool
}

type turnKey struct {
	kind Kind
	at   int64
	sum  digest16
}

type merger struct {
	sessionID string
	opt       MergeOptions
	ix        *index
	agents    map[string]Agent
	streams   []*stream
	turns     map[turnKey]bool
	sum       MergeSummary
	first     Place
	reason    string
	last      *Event
}

// WalkSession yields the events of sessionID's merged timeline, from files in
// the order OrderFiles gave (a position is the file's index here and a line and
// block in it), and returns what it saw besides. yield's error ends the walk;
// ErrStop ends it without one. An error from a file is returned; every event
// before it has been yielded.
func WalkSession(sessionID string, files []Source, opt MergeOptions, yield func(Event) error) (MergeSummary, error) {
	var sum MergeSummary
	if len(files) > MaxSessionFiles {
		sum.FilesDropped = len(files) - MaxSessionFiles
		files = files[:MaxSessionFiles]
	}
	ix := &index{cap: opt.maxIDs(), ids: map[digest16]*callState{}, claims: map[string][]digest16{}, agents: map[string]*agentSeen{}}
	lim := opt.Options.limits()
	for i, f := range files {
		rc, err := f.Open()
		if err != nil {
			return sum, err
		}
		_, err = history.Read(rc, lim, func(c history.Call) {
			if c.SessionID == sessionID {
				ix.add(i, &c)
			}
		})
		rc.Close()
		if err != nil {
			return sum, err
		}
	}
	links, claimant := ix.resolve()
	agents, err := agentTurns(links, claimant, files, opt.Options)
	if err != nil {
		return sum, err
	}
	for i := range links {
		if links[i].Status == LinkOK && agents[links[i].AgentID].Turn == nil {
			// The type is known but the parent turn is not: the calls cannot
			// be given a prompt. They are Unsupported, by their own reason.
			links[i].Status = LinkNoTurn
		}
	}
	sum.Agents = links
	sum.CallIDs, sum.CapHit, sum.Untracked = len(ix.ids), ix.capHit, ix.untracked
	for _, st := range ix.ids {
		if st.conflict {
			sum.Conflicts++
		}
	}

	m := &merger{sessionID: sessionID, opt: opt, ix: ix, agents: agents, turns: map[turnKey]bool{}, sum: sum}
	if sum.FilesDropped > 0 {
		m.first, m.reason = Place{Pos: history.Pos{Line: 1}}, "session_file_cap"
	}
	return m.run(files, yield)
}

func (m *merger) run(files []Source, yield func(Event) error) (MergeSummary, error) {
	mains := 0
	for _, f := range files {
		if f.Main {
			mains++
		}
	}
	o := m.opt.Options
	o.Agents = m.agents
	o.FinalBudget = o.finalBudget() / max(mains, 1)
	for i, f := range files {
		s := &stream{i: i, src: f}
		seq := func(emit func(Event) bool) {
			rc, err := s.src.Open()
			if err != nil {
				s.err = err
				return
			}
			defer rc.Close()
			s.sum, s.err = Walk(rc, o, func(e Event) error {
				if !emit(e) {
					return ErrStop
				}
				return nil
			})
		}
		s.next, s.stop = iter.Pull(seq)
		m.streams = append(m.streams, s)
		defer s.stop()
	}
	for _, s := range m.streams {
		m.advance(s)
	}
	var walkErr error
	for walkErr == nil {
		var best *stream
		for _, s := range m.streams {
			if s.has && (best == nil || s.before(best)) {
				best = s
			}
		}
		if best == nil {
			break
		}
		e := best.head
		if o := m.tied(best); o != nil {
			m.sum.OrderGaps++
			m.taint(Place{File: best.i, Pos: e.Pos}, string(OrderAmbiguous))
		}
		best.consumed++
		best.lastTaint = e.Taint
		if e.Gap != "" && best.firstGap.IsZero() {
			best.firstGap = e.Pos
		}
		best.prevKt = best.kt
		walkErr = m.process(best, e, yield)
		if walkErr == nil {
			m.advance(best)
		}
	}
	for _, s := range m.streams {
		if s.err != nil && walkErr == nil && !errors.Is(s.err, ErrStop) {
			walkErr = s.err
		}
	}
	return m.finish(), walkErr
}

// tied returns another stream whose head carries the same time as best's and
// is a different event that nothing orders: the one order the merge picked
// could as well have been the other. Two copies of one event tie as a matter of
// course and are not it; a subagent's event and the Agent call that started it
// or returned it are ordered by the call. Nil when there is none.
func (m *merger) tied(best *stream) *stream {
	for _, o := range m.streams {
		if o == best || !o.has || !o.kt.Equal(best.kt) || o.head.BoundaryUnknown || best.head.BoundaryUnknown || sameEvent(&best.head, &o.head) || m.causal(&best.head, &o.head) {
			continue
		}
		return o
	}
	return nil
}

// sameEvent: the two are copies of one event.
func sameEvent(a, b *Event) bool {
	if a.SessionID != b.SessionID {
		return false
	}
	switch isCallKind(a.Kind) {
	case true:
		return isCallKind(b.Kind) && a.ToolUseID == b.ToolUseID && (a.Kind == PreToolUse) == (b.Kind == PreToolUse)
	}
	return a.Kind == b.Kind && len(a.Payload) > 0 && bytes.Equal(a.Payload, b.Payload) && a.At.Equal(b.At)
}

func isCallKind(k Kind) bool { return k == PreToolUse || k == PostToolUse || k == PostToolUseFailure }

// causal: one of the events is a subagent's and the other is the Agent call
// that started it or returned it, whose order is the call's.
func (m *merger) causal(a, b *Event) bool {
	for _, p := range [][2]*Event{{a, b}, {b, a}} {
		if p[0].AgentID != "" && isCallKind(p[1].Kind) && p[1].ToolUseID == m.agents[p[0].AgentID].CallID {
			return true
		}
	}
	return false
}

// before orders two stream heads: by time, then class, then file.
func (s *stream) before(t *stream) bool {
	if !s.kt.Equal(t.kt) {
		return s.kt.Before(t.kt)
	}
	if s.kc != t.kc {
		return s.kc < t.kc
	}
	return s.i < t.i
}

// advance pulls a stream's next event and keys it, applying the stream's
// unreadable-line taint at the moment its predecessor is behind us.
func (m *merger) advance(s *stream) {
	e, ok := s.next()
	if !ok {
		s.has, s.exhausted = false, true
		// A file that ends in an unreadable line: nothing in it after its last
		// event is known.
		if s.err == nil && !s.tailTaint && (s.consumed == 0 || s.lastTaint.IsZero()) {
			if from := s.sum.TaintFrom; !from.IsZero() {
				m.taint(Place{File: s.i, Pos: from}, "unreadable_line")
			}
		}
		s.tailTaint = true
		return
	}
	s.head, s.has = e, true
	// An unreadable line before this event, and not behind a gap this stream
	// already had: it is a gap from as early as it could have been.
	gapPos := s.firstGap
	if gapPos.IsZero() && e.Gap != "" {
		gapPos = e.Pos
	}
	if !e.Taint.IsZero() && (gapPos.IsZero() || e.Taint.Before(gapPos)) && !s.lineTaint {
		s.lineTaint = true
		m.taint(Place{File: s.i, Pos: e.Taint}, "unreadable_line")
	}
	raw := e.At
	t := raw
	if t.IsZero() {
		t = s.prevKt
	}
	s.viol, s.kc = "", 0
	switch {
	case e.AgentID != "":
		s.kc = 1
		a := m.agents[e.AgentID]
		if !raw.IsZero() && (!a.NotBefore.IsZero() && raw.Before(a.NotBefore) || !a.NotAfter.IsZero() && raw.After(a.NotAfter)) {
			s.viol = SubagentOutsideCall
		}
		if !a.NotBefore.IsZero() && t.Before(a.NotBefore) {
			t = a.NotBefore
		}
	case e.Kind == PostToolUse || e.Kind == PostToolUseFailure || e.Kind == Stop:
		s.kc = 2
	}
	// A file's own order is the order of its lines. Its times only ever move
	// forward in the merge; where they went backward, its place among the other
	// files is not known.
	if len(m.streams) > 1 && !raw.IsZero() && raw.Before(s.prevKt) && s.viol == "" {
		s.viol = OrderRegression
	}
	if t.Before(s.prevKt) {
		t = s.prevKt
	}
	s.kt = t
}

// taint sets the merged timeline's first gap, if it has none.
func (m *merger) taint(p Place, why string) {
	if m.first.IsZero() {
		m.first, m.reason = p, why
	}
}

// process places one event of the merged timeline: it is dropped for a copy or
// another session, marked for a disagreement or the cap, tainted, and yielded.
func (m *merger) process(s *stream, e Event, yield func(Event) error) error {
	if s.viol != "" {
		m.sum.OrderGaps++
		m.taint(Place{File: s.i, Pos: e.Pos}, string(s.viol))
	}
	if e.SessionID != "" && e.SessionID != m.sessionID {
		m.sum.OtherSession++
		return nil
	}
	switch e.Kind {
	case PreToolUse, PostToolUse, PostToolUseFailure:
		if m.call(&e) {
			return nil
		}
	default:
		if m.turn(&e) {
			return nil
		}
	}
	if e.Gap != "" {
		m.sum.Gaps++
		m.taint(Place{File: s.i, Pos: e.Pos}, string(e.Gap))
	}
	e.File = s.i
	e.Taint, e.TaintFile = m.first.Pos, m.first.File
	m.sum.Events++
	m.last = &e
	err := yield(e)
	if errors.Is(err, ErrStop) {
		return ErrStop
	}
	return err
}

// call handles a call's event: whether it is a copy, and whether its call is
// one the merge can vouch for.
func (m *merger) call(e *Event) (drop bool) {
	if e.SessionID == "" {
		return false
	}
	st := m.ix.ids[hash16([]byte(e.ToolUseID))]
	if st == nil {
		unsupport(e, SessionIDCap)
		return false
	}
	switch e.Kind {
	case PreToolUse:
		if st.pre {
			m.sum.Copies++
			return true
		}
		st.pre, st.denied = true, e.Denied
	default:
		if st.post {
			m.sum.Copies++
			return true
		}
		st.post = true
		if !st.pre {
			// Merged before the call's own use: the files disagree on which
			// came first, and the recorder would see an execution with no
			// declaration.
			unsupport(e, ResultBeforeUse)
		}
	}
	if st.conflict {
		unsupport(e, CopiesDisagree)
	}
	return false
}

// turn handles a prompt or a turn end: a copy of one already yielded is
// dropped.
func (m *merger) turn(e *Event) (drop bool) {
	if !e.Supported() || e.BoundaryUnknown || e.At.IsZero() {
		return false
	}
	k := turnKey{e.Kind, e.At.UnixNano(), hash16(e.Payload)}
	if m.turns[k] {
		m.sum.Copies++
		return true
	}
	if len(m.turns) >= m.ix.cap {
		m.sum.CapHit = true
		return false
	}
	m.turns[k] = true
	return false
}

// unsupport makes an event Unsupported for r as well: it is not fed, and it is
// a Gap if it was not one already.
func unsupport(e *Event, r Reason) {
	e.Unsupported = append(e.Unsupported, r)
	e.Payload = nil
	if e.Gap == "" {
		e.Gap = r
	}
}

func (m *merger) finish() MergeSummary {
	sum := m.sum
	sum.TaintFrom, sum.TaintReason = m.first, m.reason
	for _, s := range m.streams {
		sum.Files = append(sum.Files, FileSummary{Name: s.src.Name, Main: s.src.Main, Summary: s.sum})
		sum.Unreadable += s.sum.Unreadable
	}
	for _, st := range m.ix.ids {
		if st.pre && !st.post && !st.denied {
			sum.Missing++
		}
	}
	sum.Complete = m.last != nil && m.last.Kind == Stop && !m.last.BoundaryUnknown && sum.Missing == 0
	return sum
}
