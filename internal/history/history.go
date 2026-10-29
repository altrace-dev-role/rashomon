// Package history reads a Claude Code transcript as the sequence of tool
// calls it records: each tool_use joined to its tool_result by id, with the
// call's outcome and a grade saying how far that outcome can be trusted
// (docs/autosteer-prescan.md, B1).
//
// It is the reader half of the prescan. The replay half derives each call
// with the hook's own code (hook.Locate, shape, gateout), so nothing here
// decides what a gate is: this package reports what the transcript says
// happened, and says plainly when it cannot tell.
//
// Nothing read here is stored. A call's input and its output head and tail
// live in memory until the caller's emit returns; the prior the scan writes
// holds digests and counts only.
package history

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// Grade is how far a call's outcome can be trusted. Only Verified calls count
// toward seeding; the report shows all four.
type Grade uint8

const (
	// Verified: the tool_use was joined to its tool_result by id, its outcome
	// is known, its cwd is known, and its transcript version is supported.
	Verified Grade = iota
	// Partial: the outcome is known, but something the replay needs is not
	// (Reason names it).
	Partial
	// Unsupported: a transcript version this reader has no fixture for, or
	// two results for one call that disagree. Read for coverage only.
	Unsupported
	// Missing: no result was read for the call. The session was cut off
	// mid-call, the file ends early, or the call was evicted (Limits).
	Missing
)

var gradeNames = [...]string{"verified", "partial", "unsupported", "missing"}

func (g Grade) String() string {
	if int(g) < len(gradeNames) {
		return gradeNames[g]
	}
	return "grade(" + strconv.Itoa(int(g)) + ")"
}

// Outcome is what the transcript says the call did.
type Outcome uint8

const (
	// OutcomeNone: no result was read (Missing).
	OutcomeNone Outcome = iota
	// OK: a result that is not an error.
	OK
	// Failed: an error result opening with "Exit code N", N not 124, 126 or
	// 127.
	Failed
	// Timeout: exit code 124. Its own class, of unknown cause: a timeout can
	// be a code regression as easily as a slow machine.
	Timeout
	// NotRun: exit code 126 or 127. Its own class, of unknown cause: a change
	// can break permissions or remove a tool.
	NotRun
	// ToolError: an error result with no exit code, such as an Edit whose
	// old_string was not found, or a Bash error worded otherwise.
	ToolError
	// Denied: refused before it ran (report.IsDenial). Never a failure.
	Denied
	// Interrupted: the result says the call was interrupted.
	Interrupted
)

var outcomeNames = [...]string{"none", "ok", "failed", "timeout", "not_run", "tool_error", "denied", "interrupted"}

func (o Outcome) String() string {
	if int(o) < len(outcomeNames) {
		return outcomeNames[o]
	}
	return "outcome(" + strconv.Itoa(int(o)) + ")"
}

// Reason is why a call is not Verified: the first cause found, in the order
// grade applies them. ReasonNone on a Verified call.
type Reason uint8

const (
	ReasonNone Reason = iota
	// NoVersion: the tool_use line names no version.
	NoVersion
	// UnsupportedVersion: a version Supported does not list.
	UnsupportedVersion
	// ResultConflict: two results for one call that disagree on its outcome.
	ResultConflict
	// NoResult: the file ended with the call unanswered.
	NoResult
	// Evicted: the call waited past Limits and was given up on.
	Evicted
	// NoCWD: the tool_use line names no working directory.
	NoCWD
	// LineSkipped: a line between the call and its result could not be read
	// (too long, or not JSON), so what happened between them is not known.
	LineSkipped
	// NoExitCode: a Bash error whose text names no exit code.
	NoExitCode
	// InputTooLarge: the input was over Limits.MaxInputBytes and not kept.
	InputTooLarge
	// OverCap: the call came after Limits.MaxCallsPerFile, where duplicate
	// ids are no longer detected.
	OverCap
)

var reasonNames = [...]string{"", "no_version", "unsupported_version", "result_conflict", "no_result",
	"evicted", "no_cwd", "line_skipped", "no_exit_code", "input_too_large", "over_cap"}

func (r Reason) String() string {
	if int(r) < len(reasonNames) {
		return reasonNames[r]
	}
	return "reason(" + strconv.Itoa(int(r)) + ")"
}

// supported are the Claude Code versions this reader has a real transcript
// fixture for (test/fixtures/claude-code/<version>). A version is added only
// with its fixture: a transcript shape is Claude Code's, not this program's,
// and a version nobody has read is graded Unsupported rather than guessed at.
var supported = map[string]bool{
	"2.1.280": true,
	"2.1.312": true,
}

// Supported reports whether v is a transcript version this reader has a
// fixture for.
func Supported(v string) bool { return supported[v] }

// Call is one tool call as the transcript records it.
type Call struct {
	SessionID string
	// Sidechain is set for a subagent's line; AgentID is its agentId, which
	// may be empty on a sidechain line that names none.
	Sidechain bool
	AgentID   string
	ToolUseID string
	// Seq is the call's place among the file's tool_use blocks, from 0. Calls
	// are emitted in Seq order.
	Seq int
	// Line is the 1-based line of the tool_use.
	Line      int
	Timestamp string // as written; parsing it is the caller's
	CWD       string
	Version   string
	ToolName  string
	// ToolInput is the input as written, nil when it was over
	// Limits.MaxInputBytes.
	ToolInput json.RawMessage
	// Backgrounded: a Bash call run with run_in_background. Its result is the
	// launch, not the command's outcome.
	Backgrounded bool
	Outcome      Outcome
	// ExitCode is the code an error result names, nil when it names none.
	ExitCode *int
	Grade    Grade
	Reason   Reason
	// OutputHead and OutputTail are the first and last bytes of a Bash
	// result's text, for the output classifier. In memory only.
	OutputHead, OutputTail []byte
}

// Limits bound what Read holds in memory at once.
type Limits struct {
	// MaxOutstanding is how many calls may wait to be emitted. Calls are
	// emitted in order, so one unanswered call holds back every later one;
	// past this many, the oldest is emitted as Missing (Evicted).
	MaxOutstanding int
	// MaxQueuedBytes bounds the inputs and outputs held by waiting calls, by
	// the same eviction.
	MaxQueuedBytes int
	// MaxInputBytes is the largest input kept; a larger one is dropped and
	// the call graded Partial (InputTooLarge).
	MaxInputBytes int
	// OutputHeadBytes and OutputTailBytes are how much of a Bash result's
	// text is kept for the output classifier.
	OutputHeadBytes, OutputTailBytes int
	// MaxCallsPerFile bounds the set of ids used to find duplicates; calls
	// past it are graded Partial (OverCap).
	MaxCallsPerFile int
}

// DefaultLimits are the bounds the prescan reads with (§6.7).
var DefaultLimits = Limits{
	MaxOutstanding:  1024,
	MaxQueuedBytes:  16 << 20,
	MaxInputBytes:   1 << 20,
	OutputHeadBytes: 8 << 10,
	OutputTailBytes: 8 << 10,
	MaxCallsPerFile: 20000,
}

// maxVersions is how many distinct versions Stats names; the rest are
// counted in OtherVersions.
const maxVersions = 16

// Stats are what one Read saw, for the scan's coverage report.
type Stats struct {
	Lines, Blank, Undecodable, TooLong int
	// Uses are the tool_use blocks read, Results the tool_result blocks.
	Uses, Results int
	// DuplicateUses are tool_use blocks whose id was already read;
	// DuplicateResults agree with a result already read; OrphanResults
	// answer no call read in this file.
	DuplicateUses, DuplicateResults, OrphanResults int
	// Grades counts emitted calls by grade, Outcomes by outcome.
	Grades   [4]int
	Outcomes [8]int
	// Versions counts tool_use lines by version, for up to maxVersions
	// versions; OtherVersions counts the lines of any further ones.
	Versions      map[string]int
	OtherVersions int
}

// pending is a call read but not yet emitted.
type pending struct {
	Call
	answered bool
	unread   int // unreadLines when the call was read
	bytes    int // what it holds against MaxQueuedBytes
}

type reader struct {
	lim    Limits
	emit   func(Call)
	st     Stats
	queue  []*pending
	byID   map[string]*pending
	seen   map[string]bool
	queued int // bytes held by the queue
	seq    int
	unread int // lines not read so far: too long or not JSON
}

// Read reads one transcript file from r and calls emit once per tool_use,
// in the order the tool_use blocks appear. A call is emitted once its result
// is read, or as Missing when the file ends or the call is evicted. emit must
// not keep the Call's byte slices past its return.
//
// A line that does not parse, or is longer than report.TranscriptMaxLine, is
// counted and stepped over, as the report's readers do. An error is r's;
// every call read before it has been emitted.
func Read(r io.Reader, lim Limits, emit func(Call)) (Stats, error) {
	rd := &reader{lim: lim, emit: emit, byID: map[string]*pending{}, seen: map[string]bool{},
		st: Stats{Versions: map[string]int{}}}
	sc := linescan.New(r, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	line := 0
	for sc.Scan() {
		line++
		rd.st.Lines++
		if sc.TooLong() {
			rd.st.TooLong++
			rd.unread++
			continue
		}
		rd.line(sc.Bytes(), line)
	}
	for len(rd.queue) > 0 {
		rd.pop(NoResult)
	}
	return rd.st, sc.Err()
}

// header is the part of a transcript line the reader decodes.
type header struct {
	SessionID   string `json:"sessionId"`
	CWD         string `json:"cwd"`
	Version     string `json:"version"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	AgentID     string `json:"agentId"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

func (rd *reader) line(raw []byte, n int) {
	var h header
	switch report.DecodeHeader(raw, &h) {
	case report.LineBlank:
		rd.st.Blank++
		return
	case report.LineUndecodable, report.LineUndecodableSidechain:
		rd.st.Undecodable++
		rd.unread++
		return
	}
	if len(h.Message.Content) == 0 {
		return
	}
	var blocks []block
	if json.Unmarshal(h.Message.Content, &blocks) != nil {
		return // a plain string content: a prompt or a reply, no calls
	}
	results := 0
	for i := range blocks {
		if blocks[i].Type == "tool_result" {
			results++
		}
	}
	for i := range blocks {
		b := &blocks[i]
		switch {
		case b.Type == "tool_use" && b.ID != "":
			rd.use(&h, b, n)
		case b.Type == "tool_result" && b.ToolUseID != "":
			// toolUseResult describes the line's result, so it is read only
			// when the line holds exactly one.
			interrupted := results == 1 && interruptedResult(h.ToolUseResult)
			rd.result(b, interrupted)
		}
	}
	rd.flush()
}

func (rd *reader) use(h *header, b *block, n int) {
	rd.st.Uses++
	overCap := len(rd.seen) >= rd.lim.MaxCallsPerFile
	if rd.seen[b.ID] || rd.byID[b.ID] != nil {
		rd.st.DuplicateUses++
		return
	}
	if !overCap {
		rd.seen[b.ID] = true
	}
	if h.Version != "" {
		if _, ok := rd.st.Versions[h.Version]; ok || len(rd.st.Versions) < maxVersions {
			rd.st.Versions[h.Version]++
		} else {
			rd.st.OtherVersions++
		}
	}
	p := &pending{Call: Call{
		SessionID: h.SessionID, Sidechain: h.IsSidechain, ToolUseID: b.ID, Seq: rd.seq, Line: n,
		Timestamp: h.Timestamp, CWD: h.CWD, Version: h.Version, ToolName: b.Name,
	}, unread: rd.unread}
	rd.seq++
	if h.IsSidechain {
		p.AgentID = h.AgentID
	}
	if len(b.Input) <= rd.lim.MaxInputBytes {
		p.ToolInput = b.Input
		p.bytes = len(b.Input)
	} else {
		p.Reason = InputTooLarge
	}
	if overCap {
		p.Reason = OverCap
	}
	p.Backgrounded = b.Name == "Bash" && runInBackground(b.Input)
	// byID holds only waiting calls, so it is bounded by the queue whether
	// or not the call is past the cap.
	rd.queue = append(rd.queue, p)
	rd.byID[b.ID] = p
	rd.queued += p.bytes
	rd.evict()
}

func (rd *reader) result(b *block, interrupted bool) {
	rd.st.Results++
	p := rd.byID[b.ToolUseID]
	if p == nil {
		if rd.seen[b.ToolUseID] {
			rd.st.DuplicateResults++ // its call was already emitted
		} else {
			rd.st.OrphanResults++
		}
		return
	}
	parts := textParts(b.Content)
	first := ""
	if len(parts) > 0 {
		first = parts[0]
	}
	out, code := outcome(b.IsError, first, interrupted)
	if p.answered {
		if out != p.Outcome || !sameCode(code, p.ExitCode) {
			p.Reason = ResultConflict
		} else {
			rd.st.DuplicateResults++
		}
		return
	}
	p.answered = true
	p.Outcome, p.ExitCode = out, code
	if rd.unread > p.unread && p.Reason == ReasonNone {
		p.Reason = LineSkipped
	}
	if p.ToolName == "Bash" {
		p.OutputHead, p.OutputTail = headTail(parts, rd.lim.OutputHeadBytes, rd.lim.OutputTailBytes)
		n := len(p.OutputHead) + len(p.OutputTail)
		p.bytes += n
		rd.queued += n
		rd.evict()
	}
}

// evict emits from the front of the queue until it is within Limits. An
// unanswered call emitted this way is Missing (Evicted).
func (rd *reader) evict() {
	for len(rd.queue) > rd.lim.MaxOutstanding || rd.queued > rd.lim.MaxQueuedBytes && len(rd.queue) > 1 {
		rd.pop(Evicted)
	}
}

// flush emits the answered calls at the front of the queue.
func (rd *reader) flush() {
	for len(rd.queue) > 0 && rd.queue[0].answered {
		rd.pop(ReasonNone)
	}
}

// pop emits the front of the queue. An unanswered call is emitted as Missing,
// for why: NoResult at the end of the file, Evicted by Limits.
func (rd *reader) pop(why Reason) {
	p := rd.queue[0]
	rd.queue[0] = nil
	rd.queue = rd.queue[1:]
	rd.queued -= p.bytes
	if rd.byID[p.ToolUseID] == p {
		delete(rd.byID, p.ToolUseID)
	}
	if !p.answered {
		p.Outcome, p.ExitCode = OutcomeNone, nil
		p.Reason = why
	}
	c := p.Call
	c.Grade, c.Reason = grade(p)
	rd.st.Grades[c.Grade]++
	rd.st.Outcomes[c.Outcome]++
	rd.emit(c)
}

// grade is the call's grade and the reason for it: Unsupported first, then
// Missing, then Partial.
func grade(p *pending) (Grade, Reason) {
	switch {
	case p.Version == "":
		return Unsupported, NoVersion
	case !Supported(p.Version):
		return Unsupported, UnsupportedVersion
	case p.Reason == ResultConflict:
		return Unsupported, ResultConflict
	case !p.answered:
		return Missing, p.Reason
	case p.Reason != ReasonNone:
		return Partial, p.Reason
	case p.CWD == "":
		return Partial, NoCWD
	case p.ToolName == "Bash" && p.Outcome == ToolError:
		return Partial, NoExitCode
	}
	return Verified, ReasonNone
}

// outcome reads a result: a denial first, as the report reads one; then an
// interruption; then success; then the exit code an error names.
func outcome(isError bool, text string, interrupted bool) (Outcome, *int) {
	switch {
	case report.IsDenial(isError, text):
		return Denied, nil
	case interrupted:
		return Interrupted, nil
	case !isError:
		return OK, nil
	}
	code, ok := exitCode(text)
	if !ok {
		return ToolError, nil
	}
	switch code {
	case 124:
		return Timeout, &code
	case 126, 127:
		return NotRun, &code
	}
	return Failed, &code
}

const exitPrefix = "Exit code "

// exitCode reads the code from an error text opening "Exit code N", anchored
// at the start: a command whose output quotes the phrase further in did not
// fail with that code. N is an optional minus and one to nine digits; any
// more cannot be an exit status and the text names none.
func exitCode(text string) (int, bool) {
	rest, ok := strings.CutPrefix(text, exitPrefix)
	if !ok {
		return 0, false
	}
	end := 0
	if end < len(rest) && rest[end] == '-' {
		end++
	}
	digits := end
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if n := end - digits; n == 0 || n > 9 {
		return 0, false
	}
	code, err := strconv.Atoi(rest[:end])
	return code, err == nil
}

func sameCode(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// textParts are the texts of a tool_result's content, which is a plain string
// in some results and an array of blocks in others.
func textParts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return []string{plain}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Type == "text" {
			out = append(out, b.Text)
		}
	}
	return out
}

// headTail returns the first h and last t bytes of the parts joined with
// newlines, without building the join: a result can be as long as a
// transcript line. When the whole text fits in h, the tail is empty.
func headTail(parts []string, h, t int) (head, tail []byte) {
	total := 0
	for i, p := range parts {
		if i > 0 {
			total++
		}
		total += len(p)
	}
	head = make([]byte, 0, min(h, total))
	for i, p := range parts {
		if i > 0 {
			head = appendCapped(head, "\n", h)
		}
		head = appendCapped(head, p, h)
		if len(head) == h {
			break
		}
	}
	if total <= h {
		return head, nil
	}
	want := min(t, total-h)
	tail = make([]byte, want)
	at := want
	for i := len(parts) - 1; i >= 0 && at > 0; i-- {
		p := parts[i]
		n := min(len(p), at)
		at -= n
		copy(tail[at:], p[len(p)-n:])
		if i > 0 && at > 0 {
			at--
			tail[at] = '\n'
		}
	}
	return head, tail
}

func appendCapped(b []byte, s string, max int) []byte {
	return append(b, s[:min(len(s), max-len(b))]...)
}

// runInBackground reports whether a Bash input asks for a background run.
func runInBackground(input json.RawMessage) bool {
	var in struct {
		RunInBackground bool `json:"run_in_background"`
	}
	return json.Unmarshal(input, &in) == nil && in.RunInBackground
}

// interruptedResult reads toolUseResult.interrupted, which Bash results
// carry. A toolUseResult that is a string or absent says nothing.
func interruptedResult(raw json.RawMessage) bool {
	var r struct {
		Interrupted bool `json:"interrupted"`
	}
	return len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &r) == nil && r.Interrupted
}
