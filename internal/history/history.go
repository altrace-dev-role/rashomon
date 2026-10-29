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
	"bytes"
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

// Pos is a place in a transcript file: the 1-based Line and the index of the
// block within that line's content array (Block, from 0). Lines are counted
// as Read counts them, every one of them: a blank line, one that did not
// parse and one that was too long each take a number, so a position names the
// same place whatever was readable around it.
//
// Positions are what the replay orders events by. A call's use and its result
// are different places, and calls overlap (parallel tool uses, a result that
// arrives after the next prompt), so the order calls are emitted in is the
// order of their Seq, not of their results; see Call.UsePos and Call.ResultPos.
type Pos struct{ Line, Block int }

// IsZero reports the zero Pos, which is no place: lines count from 1.
func (p Pos) IsZero() bool { return p == Pos{} }

// Before reports whether p is earlier in the file than q.
func (p Pos) Before(q Pos) bool {
	return p.Line < q.Line || p.Line == q.Line && p.Block < q.Block
}

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
	Line int
	// UsePos is where the tool_use block is. Its Line is Line.
	UsePos Pos
	// ResultPos is where the tool_result block that answered the call is, the
	// first one read when two agree. It is the zero Pos when no result was
	// read (Missing).
	//
	// Calls are emitted in Seq order, which is use order; the two Pos are the
	// order the file itself records. A later call can be answered before an
	// earlier one, and either result can fall on either side of a write or a
	// turn's end, which Seq cannot say.
	ResultPos Pos
	Timestamp string // as written; parsing it is the caller's
	// CWD is the working directory the tool_use line names: where the call
	// was made, and what a PreToolUse payload carries.
	CWD     string
	Version string
	// ResultCWD is the working directory the tool_result line names: where the
	// shell ended up, and what a PostToolUse payload carries. It differs from
	// CWD after `cd sub && ...`. Empty when no result was read, when the line
	// names none, and when it was over maxHeaderBytes.
	ResultCWD string
	// ResultPromptID is the promptId the tool_result line names, with the same
	// three empties as ResultCWD.
	ResultPromptID string
	// ResultTimestamp is the tool_result line's timestamp as written, the
	// moment a PostToolUse would have fired (Timestamp is the tool_use line's,
	// the moment of the PreToolUse). Empty with the same three empties as
	// ResultCWD. Parsing it is the caller's.
	ResultTimestamp string
	// ResultObject: the result line held exactly one tool_result and its
	// toolUseResult was an object. Only then do ResultBackgroundTaskID and
	// ResultBackgroundedByUser say anything: on a line with several results
	// toolUseResult describes none of them in particular, and a string
	// toolUseResult (a failure's) is the error again, with no keys to read.
	ResultObject bool
	// ResultBackgroundTaskID and ResultBackgroundedByUser are whether that
	// object carries backgroundTaskId or backgroundedByUser with a value
	// other than null or false: the two presence bits the live recorder
	// reads off a Bash call's tool_response (hook.backgroundedCall). On every
	// fixture toolUseResult equals the hook's tool_response key for key; the
	// values are never kept.
	ResultBackgroundTaskID, ResultBackgroundedByUser bool
	ToolName                                         string
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
	// OutputHead and OutputTail are the first and last bytes of a result's
	// text: a Bash result's always, for the output classifier, and any other
	// tool's only when it is an error, which is the text a PostToolUseFailure
	// payload carries as its error. In memory only.
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

// maxHeaderBytes bounds the three strings read off a call's result line, its
// cwd, its promptId and its timestamp. A path or an id is a few hundred bytes at most; one
// over this is not kept, and the call reads as if its line named none. They
// are held for as long as the call waits, so they count against
// Limits.MaxQueuedBytes as the output head and tail do.
const maxHeaderBytes = 4 << 10

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
	PromptID    string `json:"promptId"`
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
		at := Pos{Line: n, Block: i}
		switch {
		case b.Type == "tool_use" && b.ID != "":
			rd.use(&h, b, at)
		case b.Type == "tool_result" && b.ToolUseID != "":
			// toolUseResult describes the line's result, so it is read only
			// when the line holds exactly one.
			var tur toolUseResult
			if results == 1 {
				tur = readToolUseResult(h.ToolUseResult)
			}
			rd.result(&h, b, at, tur)
		}
	}
	rd.flush()
}

func (rd *reader) use(h *header, b *block, at Pos) {
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
		SessionID: h.SessionID, Sidechain: h.IsSidechain, ToolUseID: b.ID, Seq: rd.seq, Line: at.Line, UsePos: at,
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

func (rd *reader) result(h *header, b *block, at Pos, tur toolUseResult) {
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
	out, code := outcome(b.IsError, first, tur.interrupted)
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
	p.ResultPos = at
	if len(h.CWD) <= maxHeaderBytes {
		p.ResultCWD = h.CWD
	}
	if len(h.PromptID) <= maxHeaderBytes {
		p.ResultPromptID = h.PromptID
	}
	if len(h.Timestamp) <= maxHeaderBytes {
		p.ResultTimestamp = h.Timestamp
	}
	p.ResultObject = tur.object
	p.ResultBackgroundTaskID, p.ResultBackgroundedByUser = tur.backgroundTaskID, tur.backgroundedByUser
	if rd.unread > p.unread && p.Reason == ReasonNone {
		p.Reason = LineSkipped
	}
	held := len(p.ResultCWD) + len(p.ResultPromptID)
	held += len(p.ResultTimestamp)
	if p.ToolName == "Bash" || b.IsError {
		p.OutputHead, p.OutputTail = headTail(parts, rd.lim.OutputHeadBytes, rd.lim.OutputTailBytes)
		held += len(p.OutputHead) + len(p.OutputTail)
	}
	p.bytes += held
	rd.queued += held
	rd.evict()
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

// toolUseResult is what the reader keeps of a result line's toolUseResult:
// whether it was an object at all, and three of its keys. Nothing else of it
// is read, and none of the values is kept.
type toolUseResult struct {
	object                               bool
	interrupted                          bool
	backgroundTaskID, backgroundedByUser bool
}

// readToolUseResult reads toolUseResult.interrupted, which Bash results carry,
// and the presence of backgroundTaskId and backgroundedByUser, which a
// backgrounded Bash call's carries. A toolUseResult that is a string or absent
// says nothing, and is not an object: a value that is not set (null, false)
// is no presence.
func readToolUseResult(raw json.RawMessage) toolUseResult {
	var r struct {
		Interrupted        bool            `json:"interrupted"`
		BackgroundTaskID   json.RawMessage `json:"backgroundTaskId"`
		BackgroundedByUser json.RawMessage `json:"backgroundedByUser"`
	}
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &r) != nil {
		return toolUseResult{}
	}
	return toolUseResult{object: true, interrupted: r.Interrupted,
		backgroundTaskID: set(r.BackgroundTaskID), backgroundedByUser: set(r.BackgroundedByUser)}
}

// set reports a JSON value present and neither null nor false, which is the
// bit hookio.Payload.Has and jsonscan.Span.Set read.
func set(v json.RawMessage) bool {
	s := bytes.TrimSpace(v)
	return len(s) > 0 && string(s) != "null" && string(s) != "false"
}
