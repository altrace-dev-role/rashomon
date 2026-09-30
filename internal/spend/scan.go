package spend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// maxLine bounds one transcript line, as report's reader bounds it: lines
// carry whole messages and can be large, and the bound is against a corrupted
// file, not a real one. A line longer than this ends that file's read (the
// scanner cannot step over it) and the file is counted as unreadable rather
// than silently truncated.
const maxLine = 64 << 20

// usageMarker is a byte test run before any decoding. Only a line that carries
// an API response's usage can contribute, and on a real transcript most lines
// do not (attachments, tool results, bookkeeping); skipping them without a
// parse is what keeps a multi-gigabyte history affordable.
var usageMarker = []byte(`"usage"`)

// userMarker is the same byte test for a subagent transcript's user lines,
// whose promptId keys the responses after them to a turn (readFile). Every
// user line carries `"type":"user"`, so a line without the bytes is not one.
var userMarker = []byte(`"user"`)

// The decoded shape of one transcript line. THIS IS THE WHOLE USAGE READ
// PATH -- every total, breakdown and heuristic in the document comes from it
// -- and it is stated as narrowly as the recorder's own rules are:
// message.id, message.model, message.stop_reason, message.usage's token
// counts and speed, each usage.iterations entry's counts, type and model, and
// the line's timestamp, sessionId and isSidechain -- and, for a
// subagent transcript's user lines, type, isMeta and promptId, the key that
// ties the responses after them to a turn. There is no
// field for message.content -- or for anything else -- so encoding/json skips
// those bytes without materialising them: on this path the text of a
// conversation is never a value in this process, not even briefly, which is a
// stronger property than "read and then discarded".
// TestContentHasNoFieldToLandIn holds this shape.
//
// The one other read is the silent-failure line's, and it is not this one:
// for a recorded turn with a failed call, Join has report.FinalAssistantTexts
// decode, in memory, the text blocks of every assistant line tied to that
// turn -- no other block's contents -- and keep only the last, to take the
// digest's verdict. That is message content, read under report's render-time
// rule -- reduced to the verdict, never written or output -- and it departs
// from the design's "never message.content", which did not account for this
// line needing the summary it is judged against. The same pass reads the
// message ids of the turn's responses (its spend) and, on a user line with no
// promptId, the content block TYPES only, never their text.
//
// cwd, which the design lists for a project name, is deliberately not read:
// nothing here renders a project, and a directory name is a path, which the
// privacy rule keeps out of every output this program has.
type line struct {
	Type        string  `json:"type"`
	Timestamp   string  `json:"timestamp"`
	SessionID   string  `json:"sessionId"`
	IsSidechain bool    `json:"isSidechain"`
	IsMeta      bool    `json:"isMeta"`
	PromptID    string  `json:"promptId"`
	Message     message `json:"message"`
}

type message struct {
	ID         string  `json:"id"`
	Model      string  `json:"model"`
	StopReason *string `json:"stop_reason"`
	Usage      *usage  `json:"usage"`
}

// tokens is the counting part of a usage object, shared by the top level and
// by each entry of iterations[].
type tokens struct {
	Input         int64          `json:"input_tokens"`
	Output        int64          `json:"output_tokens"`
	CacheRead     int64          `json:"cache_read_input_tokens"`
	CacheCreation int64          `json:"cache_creation_input_tokens"`
	CacheSplit    *cacheCreation `json:"cache_creation"`
}

type cacheCreation struct {
	Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
}

type usage struct {
	tokens
	// Speed is "fast" on a fast-mode response, which bills at a premium the
	// table does not hold (FastMode): a closed word, counted, never printed.
	Speed string `json:"speed"`
	// Iterations is one entry per attempt. The API documents it as the per-
	// attempt source of truth, with the top-level counts covering only the
	// attempt that produced the returned message.
	Iterations []iteration `json:"iterations"`
}

// iteration is one usage.iterations entry: an attempt's counts, its kind and
// the model that ran it. The refusals-and-fallback page documents both
// fields on every entry, and bills each attempt "at the rates of the model
// that ran it".
type iteration struct {
	tokens
	// Type is "message" for an attempt by the model asked, or by a hop that
	// declined, and "fallback_message" for the fallback model that served.
	// Read as a closed word (iterationType), never printed as read.
	Type string `json:"type"`
	// Model is the model that ran the attempt. It prices the attempt, and it
	// is printed only through displayModel's closed-shape rule, as
	// message.model is.
	Model string `json:"model"`
}

// Iteration types, as iterationType reads them.
const (
	IterMessage  = "message"
	IterFallback = "fallback_message"
	IterOther    = "other"
)

// iterationType is an entry's type as a closed word: the two the API
// documents, and "other" for anything else.
func iterationType(s string) string {
	switch s {
	case IterMessage, IterFallback:
		return s
	}
	return IterOther
}

// maxTokens bounds one count of one response. The largest context window a
// model in the table has is a few million tokens, so a count past a hundred
// million is not a response's; and the bound keeps every price this package
// computes (tokens x nanodollars per token, summed) far inside an int64.
const maxTokens = 100_000_000

// plausible reports whether every count in a usage -- top level, TTL split
// and each iteration -- is at least zero and at most maxTokens.
func (u *usage) plausible() bool {
	ok := func(t tokens) bool {
		n := []int64{t.Input, t.Output, t.CacheRead, t.CacheCreation}
		if t.CacheSplit != nil {
			n = append(n, t.CacheSplit.Ephemeral5m, t.CacheSplit.Ephemeral1h)
		}
		for _, v := range n {
			if v < 0 || v > maxTokens {
				return false
			}
		}
		return true
	}
	if !ok(u.tokens) {
		return false
	}
	for _, it := range u.Iterations {
		if !ok(it.tokens) {
			return false
		}
	}
	return true
}

// carriesTokens reports whether any count in the usage, top level or an
// iteration's, is non-zero -- or is not a count at all (plausible).
func (u *usage) carriesTokens() bool {
	if !u.plausible() {
		return true
	}
	if u.split().Total() > 0 {
		return true
	}
	for _, it := range u.Iterations {
		if it.split().Total() > 0 {
			return true
		}
	}
	return false
}

// Tokens is one response's token counts by kind, with the cache writes
// split by TTL because the two TTLs are priced differently.
type Tokens struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
}

// Total is every token of every kind.
func (t Tokens) Total() int64 {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite5m + t.CacheWrite1h
}

func (t *Tokens) add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite5m += o.CacheWrite5m
	t.CacheWrite1h += o.CacheWrite1h
}

// split turns a usage object's counts into Tokens.
//
// cache_creation_input_tokens is the total write; cache_creation carries its
// TTL split. Whatever part of the total the split does not account for --
// all of it, on a response that carries no split -- is priced at the 5-minute
// rate, because 5 minutes is the API's TTL when a request names none. That is
// the API's own default, not a guess between two numbers, and it is also the
// lower of the two rates, so an unsplit write can only be under-, never over-
// stated.
func (t tokens) split() Tokens {
	out := Tokens{Input: t.Input, Output: t.Output, CacheRead: t.CacheRead}
	if t.CacheSplit != nil {
		out.CacheWrite5m = t.CacheSplit.Ephemeral5m
		out.CacheWrite1h = t.CacheSplit.Ephemeral1h
	}
	if rest := t.CacheCreation - out.CacheWrite5m - out.CacheWrite1h; rest > 0 {
		out.CacheWrite5m += rest
	}
	return out
}

// Response is one API response, counted once however many lines it was
// written on.
type Response struct {
	ID        string
	Model     string
	SessionID string
	// Subagent is true for a response read from a subagents/ transcript, or
	// from a line the main transcript itself marks isSidechain.
	Subagent   bool
	StopReason string
	// Fast is true when the response ran in fast mode (usage.speed "fast").
	Fast bool
	// StartMS is the earliest timestamp among the response's lines: the
	// moment its first content block was written, which is the closest a
	// transcript comes to when the request was made.
	StartMS int64
	Tokens  Tokens
	// Attempts are the iterations other than the one that produced the
	// message (extraAttempts). They come from the same line the counts do;
	// see keep.
	Attempts []Attempt
	// Fallback is true when a fallback model served the response: its
	// producing entry is a "fallback_message". Requested is then the model
	// the first entry names when that entry is a "message" -- the model asked
	// -- and "" when there is none: a sticky-routed turn went straight to the
	// fallback, and the transcript does not say which model was asked. As
	// read; displayModel prints it.
	Fallback  bool
	Requested string

	file int // index into Scan.Files, the file this response was first seen in

	// files is every file the response was seen in, first sighting first. A
	// resumed or forked conversation carries earlier responses into a second
	// transcript, and which of the two sorts first is only a path order: the
	// coverage rule reads every transcript holding a response (Join), so that
	// order cannot decide whether it was recorded.
	files []int

	// prompt is, for a response first seen in a subagents/ transcript, the
	// promptId of the user line before it in that file: the turn it was
	// spent in (Join). "" when no keyed user line precedes it.
	prompt string

	// complete is true when the kept line carried a stop_reason; see keep.
	complete bool
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
	// are negative or implausibly large (plausible), or a usage with tokens
	// on a line with no message.id to deduplicate by. Such a line used to be
	// dropped without a word, or -- a negative count -- priced as negative
	// dollars subtracted from the total.
	Unparsed int
	// Stale counts the transcripts Discover skipped as last written before
	// the window (Found.Stale).
	Stale int
	// UnreadableDirs counts the folders Discover could not list
	// (Found.UnreadableDirs).
	UnreadableDirs int
}

// Found is what Discover found: the transcripts to read, and a count of what
// it passed over, so "nothing in the window" and "nothing at all" are two
// different answers.
type Found struct {
	Files []TranscriptFile
	// Stale counts transcripts not read because they were last written
	// before the window. Without it, a machine whose every transcript is old
	// printed "no Claude Code transcripts were found" -- false: they were
	// found, and none was written in the window.
	Stale int
	// UnreadableDirs counts folders under projects/ that could not be listed
	// (a permission, a symlink that loops or dangles). One such folder used
	// to abort the whole command; it is now passed over, counted and said,
	// because the transcripts it holds are spend this reader did not count.
	UnreadableDirs int
}

// TranscriptFile is one transcript on disk.
type TranscriptFile struct {
	Path     string
	Subagent bool
	// Session is the session id the file's location implies: the file's base
	// name for a main transcript, the <session> directory for a subagent's.
	// A line's own sessionId wins over it; this is the fallback for a line
	// that carries none.
	Session string
	// Main is the main transcript this file's conversation is keyed by: the
	// file itself for a main transcript, and projects/<project>/<session>.jsonl
	// for a subagent's -- whether or not that file exists or was read. It is
	// what the silent-failure line calls a transcript when it says which
	// were recorded (Join).
	Main string
}

// Discover lists the transcripts under a Claude Code configuration
// directory: projects/*/*.jsonl, and every agent-*.jsonl at any depth under
// projects/*/<session>/subagents/.
//
// At any depth, for the reason report's subagentTranscripts gives: a
// workflow's subagents write <session>/subagents/workflows/<run>/agent-*.jsonl,
// and a one-level glob misses them. modifiedSince skips a file whose
// modification time is older than the window: every line of a file was
// written at or before its mtime, so such a file cannot hold a response inside
// the window. The zero time reads everything.
//
// A missing projects directory is not an error: it is a machine where Claude
// Code has recorded nothing, and the answer to "what did it spend" is then an
// honest nothing. An unreadable projects directory is, since nothing at all
// could be counted. Below it, a folder that cannot be listed is counted in
// UnreadableDirs and passed over, and a folder reached through a symlink is
// read like any other (entryKind): a DirEntry reports a symlink as neither a
// file nor a directory, and a projects folder linked in from elsewhere was
// skipped without a word -- its spend an unflagged $0.
func Discover(configDir string, modifiedSince time.Time) (*Found, error) {
	found := &Found{}
	root := filepath.Join(configDir, "projects")
	projects, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return found, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TranscriptFile
	fresh := func(p string) bool {
		if modifiedSince.IsZero() {
			return true
		}
		info, err := statFile(p)
		if err != nil {
			// A file that vanished since the folder was listed holds
			// nothing. One that cannot be stat'ed (a folder that can be
			// listed but not searched) is not known to be old, so it is
			// kept: Read then counts it as unreadable. Returning false here
			// dropped its transcripts with no count and no note.
			return !errors.Is(err, fs.ErrNotExist)
		}
		if info.ModTime().Before(modifiedSince) {
			found.Stale++
			return false
		}
		return true
	}
	for _, p := range projects {
		dir := filepath.Join(root, p.Name())
		kind := entryKind(dir, p)
		if kind == kindUnreadable {
			found.UnreadableDirs++
		}
		if kind != kindDir {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			found.UnreadableDirs++
			continue
		}
		for _, e := range entries {
			name := e.Name()
			path := filepath.Join(dir, name)
			switch kind := entryKind(path, e); {
			case kind == kindFile && strings.HasSuffix(name, ".jsonl"):
				if fresh(path) {
					out = append(out, TranscriptFile{Path: path, Session: strings.TrimSuffix(name, ".jsonl"), Main: path})
				}
			case kind == kindUnreadable:
				found.UnreadableDirs++
			case kind == kindDir:
				subs, unreadable := subagentFiles(filepath.Join(path, "subagents"))
				found.UnreadableDirs += unreadable
				for _, s := range subs {
					if fresh(s) {
						out = append(out, TranscriptFile{Path: s, Subagent: true, Session: name,
							Main: filepath.Join(dir, name+".jsonl")})
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	found.Files = out
	return found, nil
}

// statFile is os.Stat, as a variable so a test can fail it the way a folder
// that can be listed but not searched fails it -- which root, as tests often
// run, never sees.
var statFile = os.Stat

// Kinds of directory entry, symlinks followed.
const (
	kindOther = iota
	kindFile
	kindDir
	kindUnreadable
)

// entryKind is what a directory entry is once a symlink is followed: a
// regular file, a directory, something else, or -- a symlink whose target
// cannot be reached (it loops, dangles or is not permitted) --
// unreadable. A dangling link is unreadable too: it names something this
// reader was meant to find and could not, which is not the same as nothing.
func entryKind(path string, e fs.DirEntry) int {
	t := e.Type()
	if t&fs.ModeSymlink != 0 {
		info, err := os.Stat(path)
		if err != nil {
			return kindUnreadable
		}
		t = info.Mode().Type()
	}
	switch {
	case t.IsDir():
		return kindDir
	case t.IsRegular():
		return kindFile
	}
	return kindOther
}

// subagentFiles lists every agent-*.jsonl under dir, at any depth, and counts
// the folders under it that could not be read. A missing directory is the
// common case, a session with no subagents. A folder that cannot be read is
// skipped and counted, so the rest of the walk -- and of spend -- goes on.
func subagentFiles(dir string) ([]string, int) {
	var out []string
	unreadable := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			unreadable++
			if d != nil && !d.IsDir() {
				return nil
			}
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	return out, unreadable
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
	sc := &Scan{Files: found.Files, Stale: found.Stale, UnreadableDirs: found.UnreadableDirs}
	byID := map[string]*Response{}
	for i, f := range found.Files {
		if err := readFile(sc, byID, i, f); err != nil {
			sc.Unreadable++
		}
	}
	for _, r := range sc.Responses {
		if r.StartMS == 0 {
			sc.Undated++
		}
	}
	return sc, nil
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
	// response after it is left to no turn -- a floor, never a guess. Only the
	// line's header is decoded; its content has no field to land in.
	var prompt string
	s := bufio.NewScanner(fh)
	s.Buffer(make([]byte, 0, 256*1024), maxLine)
	for s.Scan() {
		raw := s.Bytes()
		usageLine := bytes.Contains(raw, usageMarker)
		if !usageLine && !(f.Subagent && bytes.Contains(raw, userMarker)) {
			continue
		}
		var l line
		if json.Unmarshal(raw, &l) != nil {
			// A usage line that does not decode is counted. A subagent user
			// line that does not decode is no usage to count, but it may be
			// a prompt this reader cannot key, so it ends the tie.
			if usageLine {
				sc.Unparsed++
			} else {
				prompt = ""
			}
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
		stop := ""
		if l.Message.StopReason != nil {
			stop = *l.Message.StopReason
		}
		cand := &Response{
			ID:         l.Message.ID,
			Model:      l.Message.Model,
			SessionID:  l.SessionID,
			Subagent:   f.Subagent || l.IsSidechain,
			StopReason: stop,
			Fast:       l.Message.Usage.Speed == "fast",
			StartMS:    startMS,
			Tokens:     l.Message.Usage.split(),
			file:       idx,
			files:      []int{idx},
			complete:   stop != "",
		}
		if f.Subagent {
			cand.prompt = prompt
		}
		if cand.SessionID == "" {
			cand.SessionID = f.Session
		}
		cand.Attempts = extraAttempts(l.Message.Usage.Iterations)
		cand.Fallback, cand.Requested = route(l.Message.Usage.Iterations)
		if !dated {
			cand.StartMS = 0
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

// keep folds a later line of an already-seen response into the one kept.
// Identity (which file, which session, main or subagent) stays with the first
// sighting; the file is added to the files it was seen in; the counts come
// from the most complete line; the start time is the earliest any line
// carries.
func keep(prev, cand *Response) {
	if !slices.Contains(prev.files, cand.file) {
		prev.files = append(prev.files, cand.file)
	}
	if cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS < prev.StartMS) {
		prev.StartMS = cand.StartMS
	}
	better := (cand.complete && !prev.complete) ||
		(cand.complete == prev.complete && cand.Tokens.Output > prev.Tokens.Output)
	if !better {
		return
	}
	prev.Tokens = cand.Tokens
	prev.StopReason = cand.StopReason
	prev.Fast = cand.Fast
	prev.complete = cand.complete
	prev.Attempts, prev.Fallback, prev.Requested = cand.Attempts, cand.Fallback, cand.Requested
	if prev.Model == "" {
		prev.Model = cand.Model
	}
}

// Attempt is one iteration other than the one that produced the message.
type Attempt struct {
	Type   string // iterationType's closed word
	Model  string // as read; displayModel prints it
	Tokens Tokens
}

// extraAttempts is the attempts in iterations[] other than the one that
// produced the message, each with its type, model and tokens.
//
// The producing attempt is taken to be the LAST entry: the top-level usage
// covers "only the attempt that produced the returned message", an attempt
// that was declined and retried necessarily came before the retry, and the
// page's example ends with the "fallback_message" entry that served. Its
// tokens are already in the top-level counts, so counting it again here would
// be the very double-count the dedupe exists to prevent.
func extraAttempts(its []iteration) []Attempt {
	if len(its) <= 1 {
		return nil
	}
	out := make([]Attempt, 0, len(its)-1)
	for _, it := range its[:len(its)-1] {
		out = append(out, Attempt{Type: iterationType(it.Type), Model: it.Model, Tokens: it.split()})
	}
	return out
}

// route reads which model served a response and which was asked. A
// fallback served it when its producing (last) entry is a
// "fallback_message"; the model asked is the first entry's when that entry is
// a "message". A sticky-routed turn has no such entry -- the page: identify
// it "by the fallback_message entry ..., the absence of a message entry for
// the requested model, and the response's model field" -- so it is served by
// a fallback with the model asked unknown.
func route(its []iteration) (bool, string) {
	if len(its) == 0 || iterationType(its[len(its)-1].Type) != IterFallback {
		return false, ""
	}
	if first := its[0]; len(its) > 1 && iterationType(first.Type) == IterMessage {
		return true, first.Model
	}
	return true, ""
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
