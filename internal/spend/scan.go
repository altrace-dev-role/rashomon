package spend

import (
	"bytes"
	"math"
	"os"
	"slices"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// Response is one API response, counted once however many lines it was
// written on.
type Response struct {
	ID    string
	Model string
	// Subagent is true for a response read from a subagents/ transcript, or
	// from a line the main transcript itself marks isSidechain.
	Subagent   bool
	StopReason string
	// Category is a refusal's stop_details.category (refusalCategory), and
	// "" for a response that did not end in a refusal.
	Category string
	// Fast is true when the response ran in fast mode (usage.speed "fast").
	Fast bool
	// StartMS is the earliest timestamp among the response's lines: the
	// moment its first content block was written, which is the closest a
	// transcript comes to when the request was made.
	StartMS int64
	Tokens  Tokens
	// Attempts are the tokens of each iteration other than the one that
	// produced the message (extraAttempts). They come from the same line the
	// counts do; see keep.
	Attempts []Tokens
	// Fallback is true when a fallback chain ran: the producing entry is a
	// "fallback_message". Model is then that entry's model when it names one
	// (route). The chain served the response unless it ended in a refusal --
	// every model declined.
	Fallback bool

	file int // index into Scan.Files, the file this response was first seen in

	// files is every file the response was seen in, first sighting first,
	// each with the session id its own line carries. A resumed or branched
	// conversation (/branch, --fork-session) carries earlier responses into a
	// second transcript, and which of the two sorts first is only a path
	// order: the coverage rule reads every transcript holding a response
	// (Join), and the sessions a shared response belongs to are chosen by
	// ownerSessions, so that order decides neither.
	files []sighting

	// owners is the session or sessions the response belongs to
	// (ownerSessions), sorted: its cost is in each one's per-session row.
	owners []string

	// prompt is, for a response first seen in a subagents/ transcript, the
	// promptId of the user line before it in that file: the turn it was
	// spent in (Join). "" when no keyed user line precedes it.
	prompt string

	// complete is true when the kept line carried a stop_reason; see keep.
	complete bool

	// requestID is the line's requestId when it has the closed shape
	// (requestIDShaped), "" otherwise.
	requestID string

	// costUnknown marks a refusal before any output (output_tokens 0) with
	// usage: whether it was billed depends on its category, so Read marks
	// it, and Build shows its tokens with the cost unknown, out of the total.
	costUnknown bool
}

// sighting is one file a response was seen in, with the session id the
// response's line in that file carries (or the file's own, lacking one).
type sighting struct {
	idx     int
	session string
}

// Scan is everything read from the transcripts: one Response per message.id.
type Scan struct {
	Responses []*Response
	// Files is every transcript read, main and subagent, in sorted order.
	Files []TranscriptFile
	// UsageLines counts the lines that carried a usage object, before
	// deduplication: set beside len(Responses), it is the measurement that
	// makes the dedupe rule checkable on any machine.
	UsageLines int
	// Unreadable counts transcript files that could not be read to the end.
	// Named, never dropped: a file this reader could not finish is spend it
	// did not count, and saying nothing would present a partial sum as whole.
	Unreadable int
	// Undated counts responses with no parseable timestamp. They cannot be
	// placed in or out of a window, so they are excluded and counted.
	Undated int
	// Unparsed counts lines that may carry usage and could not be counted:
	// a line that does not decode into the usage shape (a token count
	// written as a string, a timestamp as a number), a usage whose counts
	// are negative or implausibly large (plausible), a usage with tokens
	// on a line with no message.id to deduplicate by, or a line too long to
	// read at all (report.TranscriptMaxLine). Such a line used to be
	// dropped without a word, or -- a negative count -- priced as negative
	// dollars subtracted from the total.
	Unparsed int
	// Stale counts the transcripts Discover skipped as last written before
	// the window (Found.Stale).
	Stale int
	// UnreadableDirs counts the folders Discover could not list
	// (Found.UnreadableDirs).
	UnreadableDirs int

	// firstMS is, per file, the timestamp of its first dated line, 0 when no
	// line carries one (ownerSessions).
	firstMS []int64

	// partial is, per file, whether some response in it may be missing from
	// Responses: the file was not read to the end, or it holds a usage line
	// that could not be counted (Unparsed). Either may be the response that
	// made a call, so Join takes no such transcript for one watched with no
	// call made (mayHaveCalled).
	partial []bool
}

// Read reads the usage of every response in files, counting each message.id
// ONCE.
//
// THE DEDUPE IS THE MOST IMPORTANT CORRECTNESS RULE IN THIS PACKAGE. Claude
// Code writes one API response as several lines -- one per content block --
// and each line carries the response's usage. Measured on a real transcript
// (Claude Code 2.1.285): 277 usage lines, 129 distinct message ids; summing
// lines overstates spend 2.1x. So a response is keyed by message.id across
// every file, and a line whose id was already seen adds nothing.
//
// Which of a response's lines is KEPT matters too, and it is not "the first".
// A subagent transcript measured on the same version writes a streaming line
// first -- output_tokens 4, stop_reason null, no iterations -- and the
// completed line (output_tokens 107, stop_reason set) after it. Keeping the
// first would undercount that response's output 27x. So the kept line is the
// most complete one: a line with a stop_reason beats one without, and between
// two of the same kind the one with more output tokens wins. Identical lines
// -- the common case -- tie, and either is the same answer.
//
// A line with no message.id cannot be deduplicated against anything, so it
// cannot be counted without risking the 2x error; it is skipped, and counted
// in Unparsed when it carries tokens. It used to be dropped without a word:
// an id-less line carrying $20 of input rendered "est. <$0.01" and no note.
func Read(found *Found) (*Scan, error) {
	sc := &Scan{Files: found.Files, Stale: found.Stale, UnreadableDirs: found.UnreadableDirs,
		firstMS: make([]int64, len(found.Files)), partial: make([]bool, len(found.Files))}
	byID := map[string]*Response{}
	for i, f := range found.Files {
		unparsed := sc.Unparsed
		if err := readFile(sc, byID, i, f); err != nil {
			sc.Unreadable++
			sc.partial[i] = true
		}
		if sc.Unparsed > unparsed {
			sc.partial[i] = true
		}
	}
	sc.foldRefusalMessages()
	for _, r := range sc.Responses {
		if r.StartMS == 0 {
			sc.Undated++
		}
		r.owners = sc.ownerSessions(r)
		if r.StopReason == "refusal" && r.Tokens.Output == 0 && r.Tokens.Total() > 0 {
			r.costUnknown = true
		}
	}
	return sc, nil
}

// foldRefusalMessages folds each zero-usage refusal line into the response it
// reports, when there is one.
//
// After a refusal with no fallback, Claude Code writes a separate zero-usage
// "<synthetic>" line with stop_reason "refusal", the response's
// stop_details and the same requestId as the real response -- after a
// mid-stream refusal too, not only before any output. Every zero-usage
// refusal line was read as a pre-output refusal, so a mid-stream refusal was
// counted twice: priced, and again as "written without usage". So a
// zero-usage refusal line whose requestId names a response with usage in the
// same file is that response's: the response is marked a refusal of its
// category if its own lines did not say so, and the line is dropped. Only an
// unmatched one is a refusal without usage (Refusals.WithoutUsage).
func (sc *Scan) foldRefusalMessages() {
	type key struct {
		idx int
		req string
	}
	billed := map[key]*Response{}
	for _, r := range sc.Responses {
		if r.requestID == "" || r.Tokens.Total() == 0 && len(r.Attempts) == 0 {
			continue
		}
		for _, f := range r.files {
			billed[key{f.idx, r.requestID}] = r
		}
	}
	out := sc.Responses[:0]
	for _, r := range sc.Responses {
		if r.StopReason == "refusal" && r.requestID != "" && r.Tokens.Total() == 0 && len(r.Attempts) == 0 {
			if into := billed[key{r.file, r.requestID}]; into != nil {
				if into.StopReason != "refusal" {
					into.StopReason, into.Category = "refusal", r.Category
				}
				continue
			}
		}
		out = append(out, r)
	}
	sc.Responses = out
}

// ownerSessions is the session or sessions a response belongs to: for a
// response seen in one file, that file's line's session; for one a resumed or
// branched conversation carries into a second transcript, the session of
// the file whose first dated line is earliest -- the conversation it was
// first written in -- and an undated file last. Never path order, and never
// session-id order: /branch keeps the original timestamps on every copied
// line, so the two files start at the same moment, and breaking that tie by
// session id gave the shared responses to the copy whenever its random id
// sorted first, with the original's session gone from the rows. On a tie
// the response belongs to each tied session, so either may be the original.
func (sc *Scan) ownerSessions(r *Response) []string {
	key := func(s sighting) int64 {
		if ms := sc.firstMS[s.idx]; ms != 0 {
			return ms
		}
		return math.MaxInt64
	}
	first := key(r.files[0])
	for _, s := range r.files[1:] {
		first = min(first, key(s))
	}
	var out []string
	for _, s := range r.files {
		if key(s) == first && !slices.Contains(out, s.session) {
			out = append(out, s.session)
		}
	}
	sort.Strings(out)
	return out
}

func readFile(sc *Scan, byID map[string]*Response, idx int, f TranscriptFile) error {
	fh, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer fh.Close() //nolint:errcheck // read-only

	// A SUBAGENT'S SPEND IS KEYED BY ITS OWN TRANSCRIPT'S PROMPTID. The hooks
	// record a subagent's calls with the parent turn's prompt_id but the MAIN
	// transcript as transcript_path -- measured on every subagent declaration
	// in a real store -- so no record names a subagent file, and joining one
	// through the record never counted a subagent at all. The subagent file
	// itself carries the key: measured on 36 real subagent transcripts, every
	// user line (the task it was handed, each tool result, a later message
	// sent to it) carries a promptId, and the parent turn's recorded prompt_id
	// is among them. So a subagent response belongs to the promptId of the
	// user line before it, as a main-agent response does in the main
	// transcript (report.FinalAssistantTexts). A user line with none, other
	// than an injected meta line, ends the tie: none was measured, and the
	// response after it is left to no turn -- a floor, never a guess. So does a
	// line that cannot be decoded, a usage line included: it may be a prompt
	// this reader cannot key. A byte test for "user" before decoding missed a
	// user line cut short before its type, and an undecodable usage line kept
	// the tie, so the next prompt's spend landed on this one. Every non-blank
	// line of a subagent transcript is decoded (report.DecodeHeader), the
	// header only; its content has no field to land in.
	var prompt string
	s := linescan.New(fh, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	// The file's first dated line dates it (ownerSessions): a main
	// transcript's lines are decoded, header only, until one carries a
	// timestamp, and then only those that may carry usage.
	fileDated := false
	for s.Scan() {
		if s.TooLong() {
			// A line too long to read (report.TranscriptMaxLine) may have
			// carried usage, so it is counted as one that could not be,
			// and, like any line that does not decode, it ends a subagent
			// transcript's tie. The lines after it are read.
			prompt = ""
			sc.Unparsed++
			continue
		}
		raw := s.Bytes()
		mayCarry := mayCarryUsage(raw)
		if !mayCarry && !f.Subagent && fileDated {
			continue
		}
		var l line
		switch report.DecodeHeader(raw, &l) {
		case report.LineBlank:
			continue
		case report.LineUndecodable, report.LineUndecodableSidechain:
			// A usage line that does not decode is counted, by the exact
			// marker, searched for only here: mayCarryUsage is wider on
			// purpose, and a line that decodes needs no search at all. Any
			// line that does not decode may be a prompt this reader cannot
			// key, so it ends a subagent transcript's tie.
			if bytes.Contains(raw, usageMarker) {
				sc.Unparsed++
			}
			prompt = ""
			continue
		}
		if !fileDated {
			if ms, ok := parseTimestamp(l.Timestamp); ok {
				sc.firstMS[idx], fileDated = ms, true
			}
		}
		if !mayCarry && !f.Subagent {
			continue
		}
		if f.Subagent && l.Type == "user" {
			if l.PromptID != "" {
				prompt = l.PromptID
			} else if !l.IsMeta {
				prompt = ""
			}
			continue
		}
		if l.Message.Usage == nil {
			continue
		}
		if l.Message.ID == "" {
			// No id to deduplicate against, so not counted -- and, when it
			// carries tokens, counted as a line that could not be, so the
			// note says so rather than the total silently leaving it out.
			if l.Message.Usage.carriesTokens() {
				sc.Unparsed++
			}
			continue
		}
		sc.UsageLines++
		if !l.Message.Usage.plausible() {
			sc.Unparsed++
			continue
		}
		startMS, dated := parseTimestamp(l.Timestamp)
		if !dated {
			startMS = 0
		}
		cand := newResponse(&l, f, idx, startMS)
		if f.Subagent {
			cand.prompt = prompt
		}

		prev, seen := byID[cand.ID]
		if !seen {
			byID[cand.ID] = cand
			sc.Responses = append(sc.Responses, cand)
			continue
		}
		keep(prev, cand)
	}
	return s.Err()
}

// newResponse is one usage line's response, as readFile keys and keeps it:
// its id, model, tokens and stop, the file and session it was seen in, its
// request id when that has the closed shape of one, its refusal category,
// its extra attempts, and the model that served it when a fallback did.
// startMS is the line's own instant, zero when it carries none.
func newResponse(l *line, f TranscriptFile, idx int, startMS int64) *Response {
	stop := ""
	if l.Message.StopReason != nil {
		stop = *l.Message.StopReason
	}
	cand := &Response{
		ID:         l.Message.ID,
		Model:      l.Message.Model,
		Subagent:   f.Subagent || l.IsSidechain,
		StopReason: stop,
		Fast:       l.Message.Usage.Speed == "fast",
		StartMS:    startMS,
		Tokens:     l.Message.Usage.split(),
		file:       idx,
		complete:   stop != "",
	}
	session := l.SessionID
	if session == "" {
		session = f.Session
	}
	cand.files = []sighting{{idx, session}}
	if requestIDShaped(l.RequestID) {
		cand.requestID = l.RequestID
	}
	if stop == "refusal" {
		cand.Category = refusalCategory(l.Message.StopDetails)
	}
	cand.Attempts = extraAttempts(l.Message.Usage.Iterations)
	var served string
	cand.Fallback, served = route(l.Message.Usage.Iterations)
	if served != "" {
		cand.Model = served
	}
	return cand
}

// keep folds a later line of an already-seen response into the one kept.
// Identity (which file, main or subagent) stays with the first sighting; a
// new file is added to the sightings, with the session its own line carries
// (Read then picks the response's sessions, ownerSessions); the counts come
// from the most complete line; the start time is the earliest any line
// carries.
func keep(prev, cand *Response) {
	if !slices.ContainsFunc(prev.files, func(s sighting) bool { return s.idx == cand.file }) {
		prev.files = append(prev.files, cand.files[0])
	}
	if cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS < prev.StartMS) {
		prev.StartMS = cand.StartMS
	}
	if prev.requestID == "" {
		prev.requestID = cand.requestID
	}
	better := (cand.complete && !prev.complete) ||
		(cand.complete == prev.complete && cand.Tokens.Output > prev.Tokens.Output)
	if !better {
		return
	}
	prev.Tokens = cand.Tokens
	prev.StopReason = cand.StopReason
	prev.Category = cand.Category
	prev.Fast = cand.Fast
	prev.complete = cand.complete
	prev.Attempts, prev.Fallback = cand.Attempts, cand.Fallback
	// The served model replaces the one a streamed line named (route), but
	// a line that names no model never replaces one.
	if cand.Model != "" && (prev.Model == "" || cand.Fallback) {
		prev.Model = cand.Model
	}
}

// extraAttempts is the tokens of each attempt in iterations[] other than the
// one that produced the message.
//
// The producing attempt is taken to be the LAST entry: the top-level usage
// covers "only the attempt that produced the returned message", an attempt
// that was declined and retried necessarily came before the retry, and the
// page's example ends with the "fallback_message" entry that served. Its
// tokens are already in the top-level counts, so counting it again here would
// be the very double-count the dedupe exists to prevent.
func extraAttempts(its []iteration) []Tokens {
	if len(its) <= 1 {
		return nil
	}
	out := make([]Tokens, 0, len(its)-1)
	for _, it := range its[:len(its)-1] {
		out = append(out, it.split())
	}
	return out
}

// route reads whether a fallback chain ran, and which model served it. A
// chain ran when the producing (last) entry is a "fallback_message", and the
// model that ran that attempt is the entry's own: for a mid-output fallback
// the page says message_start "already named the requested model, so read
// the serving model from ... the fallback_message entry", so message.model
// can name the model that declined, and is used only when the entry names
// none (served "").
func route(its []iteration) (fallback bool, served string) {
	if len(its) == 0 || its[len(its)-1].Type != iterFallback {
		return false, ""
	}
	return true, its[len(its)-1].Model
}

// parseTimestamp reads Claude Code's RFC 3339 timestamp to Unix ms.
func parseTimestamp(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, false
	}
	return t.UnixMilli(), true
}
