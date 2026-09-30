package spend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// The decoded shape of one transcript line. THIS IS THE WHOLE USAGE READ
// PATH -- every total, breakdown and heuristic in the document comes from it
// -- and it is stated as narrowly as the recorder's own rules are:
// message.id, message.model, message.stop_reason, message.usage's token
// counts, and the line's timestamp, sessionId and isSidechain. There is no
// field for message.content -- or for anything else -- so encoding/json skips
// those bytes without materialising them: on this path the text of a
// conversation is never a value in this process, not even briefly, which is a
// stronger property than "read and then discarded".
// TestContentHasNoFieldToLandIn holds this shape.
//
// The one other read is the silent-failure line's, and it is not this one:
// for a recorded turn with a failed call, Join has report.FinalAssistantTexts
// read that turn's final assistant message, in memory, to take the digest's
// verdict. That is message content, read under report's render-time rule --
// reduced to the verdict, never written or output -- and it departs from the
// design's "never message.content", which did not account for this line
// needing the summary it is judged against.
//
// cwd, which the design lists for a project name, is deliberately not read:
// nothing here renders a project, and a directory name is a path, which the
// privacy rule keeps out of every output this program has.
type line struct {
	Timestamp   string  `json:"timestamp"`
	SessionID   string  `json:"sessionId"`
	IsSidechain bool    `json:"isSidechain"`
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
	// Iterations is one entry per attempt. The API documents it as the per-
	// attempt source of truth, with the top-level counts covering only the
	// attempt that produced the returned message.
	Iterations []tokens `json:"iterations"`
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
	// StartMS is the earliest timestamp among the response's lines: the
	// moment its first content block was written, which is the closest a
	// transcript comes to when the request was made.
	StartMS int64
	Tokens  Tokens
	// ExtraAttempts counts iterations beyond the one that produced the
	// message, and ExtraTokens is their tokens. They come from the same line
	// the counts do; see keep.
	ExtraAttempts int
	ExtraTokens   Tokens

	file int // index into Scan.Files, the file this response was first seen in

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
// honest nothing.
func Discover(configDir string, modifiedSince time.Time) ([]TranscriptFile, error) {
	root := filepath.Join(configDir, "projects")
	projects, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TranscriptFile
	fresh := func(p string) bool {
		if modifiedSince.IsZero() {
			return true
		}
		info, err := os.Stat(p)
		return err == nil && !info.ModTime().Before(modifiedSince)
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(root, p.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			switch {
			case !e.IsDir() && strings.HasSuffix(name, ".jsonl"):
				path := filepath.Join(dir, name)
				if fresh(path) {
					out = append(out, TranscriptFile{Path: path, Session: strings.TrimSuffix(name, ".jsonl"), Main: path})
				}
			case e.IsDir():
				subs, err := subagentFiles(filepath.Join(dir, name, "subagents"))
				if err != nil {
					return nil, err
				}
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
	return out, nil
}

// subagentFiles lists every agent-*.jsonl under dir, at any depth. A missing
// directory is the common case, a session with no subagents.
func subagentFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
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
// cannot be counted without risking the 2x error; it is skipped.
func Read(files []TranscriptFile) (*Scan, error) {
	sc := &Scan{Files: files}
	byID := map[string]*Response{}
	for i, f := range files {
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

	s := bufio.NewScanner(fh)
	s.Buffer(make([]byte, 0, 256*1024), maxLine)
	for s.Scan() {
		raw := s.Bytes()
		if !bytes.Contains(raw, usageMarker) {
			continue
		}
		var l line
		if json.Unmarshal(raw, &l) != nil || l.Message.Usage == nil {
			continue
		}
		sc.UsageLines++
		if l.Message.ID == "" {
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
			StartMS:    startMS,
			Tokens:     l.Message.Usage.split(),
			file:       idx,
			complete:   stop != "",
		}
		if cand.SessionID == "" {
			cand.SessionID = f.Session
		}
		cand.ExtraAttempts, cand.ExtraTokens = extraAttempts(l.Message.Usage.Iterations)
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
// sighting; the counts come from the most complete line; the start time is
// the earliest any line carries.
func keep(prev, cand *Response) {
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
	prev.complete = cand.complete
	prev.ExtraAttempts, prev.ExtraTokens = cand.ExtraAttempts, cand.ExtraTokens
	if prev.Model == "" {
		prev.Model = cand.Model
	}
}

// extraAttempts counts the attempts in iterations[] other than the one that
// produced the message, and sums their tokens.
//
// The producing attempt is taken to be the LAST entry: the top-level usage
// covers "only the attempt that produced the returned message", and an attempt
// that was declined and retried necessarily came before the retry. Its tokens
// are already in the top-level counts, so counting it again here would be the
// very double-count the dedupe exists to prevent.
func extraAttempts(its []tokens) (int, Tokens) {
	if len(its) <= 1 {
		return 0, Tokens{}
	}
	var sum Tokens
	for _, it := range its[:len(its)-1] {
		sum.add(it.split())
	}
	return len(its) - 1, sum
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
