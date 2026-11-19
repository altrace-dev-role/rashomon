package steer

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"math"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// budgetChunk is how much of the steer log one read takes, newest first.
// A prompt's lines are expected under 64 KiB, so one read is the usual cost.
const budgetChunk = 64 << 10

// budgetKeys are the eleven keys the scan reads from a line, in one walk
// that allocates nothing per value (jsonscan.Top); RES-5 bans a decode on
// the hook path.
func budgetKeys() *jsonscan.KeySet {
	budgetKeysOnce.Do(func() {
		budgetKeysOnce.v = jsonscan.NewKeySet("session_id", "prompt_id", "at_unix_ms", "state", "level", "pattern", "kind", "key", "type", "suppressed_by", "after_seq")
	})
	return budgetKeysOnce.v
}

// budgetKeysOnce holds what budgetKeys returns, built on its first call and
// never at init (RES-2).
var budgetKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// budgetKeys' slots, in the order given above.
const (
	bkSession = iota
	bkPrompt
	bkAt
	bkState
	bkLevel
	bkPattern
	bkKind
	bkKey
	bkType
	bkSuppressedBy
	bkAfterSeq
	bkCount
)

// findingKey is the per-key budget's unit (tech 4.2.5: one note per
// (session_id, prompt_id, pattern, key)), with the kind, since a pattern's
// kinds are different claims.
type findingKey struct{ pattern, kind, key string }

// counts is what the current prompt has already delivered: notes at nudge
// or stop, flags at flag, the keys of its delivered notes, and the notes the
// window reads. A line arm off withheld counts as delivered, so the off arm's
// budget runs as the steer arm's would.
type counts struct {
	notes, flags int
	keys         map[findingKey]bool
	// window holds the prompt's notes with the largest after_seq, largest
	// first, kept of them; the briefing is not among them (tech 4.2.5).
	window [maxNotes]recent
	kept   int
}

// recent is one delivered note as the window reads it.
type recent struct {
	seq     int64
	pattern string
}

// keep adds a delivered note to the window's state, which holds the three
// largest after_seq values and not the last three written: the calls of one
// reply run in parallel, so a later seq can be written first
// (NOTE-BUDGET). Three are enough: the notes above any threshold are a prefix of
// the largest-first order, so up to three of them are exactly the first
// ones kept, and past three the window is full either way.
func (c *counts) keep(seq int64, pat string) {
	i := c.kept
	if i == len(c.window) {
		if seq <= c.window[i-1].seq {
			return
		}
		i--
	} else {
		c.kept++
	}
	for ; i > 0 && c.window[i-1].seq < seq; i-- {
		c.window[i] = c.window[i-1]
	}
	c.window[i] = recent{seq, pat}
}

// spent is the window a note at after_seq s would join: n, the prompt's
// delivered notes with after_seq above s - windowSeqs, with no upper bound,
// and repeat, whether one of them is of pattern pat (NOTE-BUDGET).
// No upper bound, because a note written earlier can carry a later seq: take
// any window of windowSeqs seqs and the note written last in it, at s; every
// other note of that window is above s - windowSeqs, so the router counted
// them all before it let that one through.
func (c *counts) spent(s int64, pat string) (n int, repeat bool) {
	for _, r := range c.window[:c.kept] {
		if r.seq > s-windowSeqs {
			n++
			repeat = repeat || r.pattern == pat
		}
	}
	return n, repeat
}

// seqOf is a line's after_seq: the digits of a JSON number, read with no
// allocation. Anything else -- a fraction, a sign, a string, null, absent,
// a value past int64 -- is math.MaxInt64, inside every window, so a note
// whose seq cannot be read counts as spent (tech 4.2.5).
func seqOf(b []byte, sp jsonscan.Span) int64 {
	if sp.Kind != jsonscan.Number {
		return math.MaxInt64
	}
	raw := sp.Raw(b)
	if len(raw) == 0 {
		return math.MaxInt64
	}
	var n int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return math.MaxInt64
		}
		d := int64(c - '0')
		if n > (math.MaxInt64-d)/10 {
			return math.MaxInt64
		}
		n = n*10 + d
	}
	return n
}

// errNoSize: the log's reader cannot say how long the log is.
var errNoSize = errors.New("steer: the steer log's reader has no size")

// countBudget counts the current prompt's delivered lines from the steer
// log, read through locked, the handle that holds the steer lock (card 2.2
// R6). It scans from the tail, newest line first, in budgetChunk reads, and
// stops at the first line of another session or prompt: a prompt's lines
// are contiguous in an append-only log, so the scan costs the current
// prompt's lines, not the log's.
//
// One exception, and its bound. A foreign batch at the tail -- lines of one
// append, which share one at_unix_ms and one prompt -- is skipped before the
// current prompt's lines are read. A foreign line can land after this
// prompt's lines only if a process of the previous prompt is still alive
// after this prompt's first routed call, which comes after a model turn: the
// previous prompt's PostToolUse processes ended before its Stop, and the
// supervisor's deadline bounds every hook, so the one process that can
// straddle is the previous Stop's recap, one append. A second straddling
// append would hide this prompt's lines behind it and undercount them; that
// residual is accepted for v0.
//
// A torn or damaged line is skipped, as ReadSteer skips it. Unconfirmed
// lines count in nothing (tech 4.2.3), and a subagent's lines of this prompt
// count (4.2.5: "subagents included").
func countBudget(locked io.ReaderAt, ctx *Context) (counts, error) {
	size, err := readerSize(locked)
	if err != nil {
		return counts{}, err
	}
	s := budgetScan{ctx: ctx}
	// work holds one read and, after it, the carry: the start of the line
	// the later read began inside, kept in its own buffer so the next read
	// cannot overwrite it.
	var work, carry []byte
	for end := size; end > 0 && !s.done; {
		start := max(end-budgetChunk, 0)
		n := int(end - start)
		if cap(work) < n+len(carry) {
			work = make([]byte, 0, n+len(carry))
		}
		work = work[:n+len(carry)]
		if _, err := locked.ReadAt(work[:n], start); err != nil {
			return counts{}, err
		}
		copy(work[n:], carry)
		// Every segment after the first is a whole line: it starts after a
		// line feed this read holds and ends at one, or at the carry's end,
		// which is one. The first starts in an earlier read, unless this read
		// reached the log's start.
		first := bytes.IndexByte(work, '\n')
		if first < 0 && start > 0 {
			carry = append(carry[:0], work...)
			end = start
			continue
		}
		whole := first
		if start == 0 {
			whole = -1
		}
		for i := len(work); i > whole && !s.done; {
			j := bytes.LastIndexByte(work[:i], '\n')
			s.line(work[j+1 : i])
			i = j
		}
		carry = append(carry[:0], work[:max(first, 0)]...)
		end = start
	}
	return s.c, nil
}

// readerSize is the log's length: a *os.File's from Stat, a bytes.Reader's
// from Size.
func readerSize(r io.ReaderAt) (int64, error) {
	switch v := r.(type) {
	case interface{ Size() int64 }:
		return v.Size(), nil
	case interface{ Stat() (fs.FileInfo, error) }:
		fi, err := v.Stat()
		if err != nil {
			return 0, err
		}
		return fi.Size(), nil
	}
	return 0, errNoSize
}

// budgetScan is countBudget's state across lines, newest first.
type budgetScan struct {
	ctx  *Context
	out  [bkCount]jsonscan.Span
	c    counts
	done bool
	// own: a line of the current prompt has been read, so the next foreign
	// line is the prompt's start.
	own bool
	// batch, batchAt and batchPrompt: the foreign batch being skipped at
	// the tail, by its raw at_unix_ms and prompt_id.
	batch                bool
	batchAt, batchPrompt []byte
	scratch              []byte
}

// line reads one line, newest first.
func (s *budgetScan) line(b []byte) {
	if len(b) == 0 || jsonscan.Top(b, budgetKeys(), s.out[:]) != nil {
		return
	}
	// A channel-evidence line (steer_evidence, evidence.go) is the reader's
	// record of a note it saw, not a note: it counts in nothing and, being
	// written later than the note it names, is no prompt's start, so it cannot
	// end the scan or open a foreign batch.
	if s.is(b, s.out[bkType], EvidenceLineType) {
		return
	}
	if !s.ours(b) {
		at, prompt := s.out[bkAt].Raw(b), s.out[bkPrompt].Raw(b)
		switch {
		case s.own:
			s.done = true
		case !s.batch:
			s.batch = true
			s.batchAt = append(s.batchAt[:0], at...)
			s.batchPrompt = append(s.batchPrompt[:0], prompt...)
		case !bytes.Equal(at, s.batchAt) || !bytes.Equal(prompt, s.batchPrompt):
			s.done = true
		}
		return
	}
	s.own = true
	if !s.is(b, s.out[bkState], string(LineStateDelivered)) && !s.is(b, s.out[bkSuppressedBy], string(SuppressedByArmOff)) {
		return
	}
	switch {
	case s.is(b, s.out[bkLevel], LevelFlag.String()):
		s.c.flags++
	case s.is(b, s.out[bkLevel], LevelNudge.String()), s.is(b, s.out[bkLevel], LevelStop.String()):
		// The briefing is outside the prompt budget (tech 4.2.5): given at a
		// resumed session's first prompt (D-c), it charges none of the
		// prompt's notes nor any window, and its key still answers a second
		// one.
		k := findingKey{s.text(b, s.out[bkPattern]), s.text(b, s.out[bkKind]), s.text(b, s.out[bkKey])}
		if k.pattern != string(pattern.ContinuityBriefing) {
			s.c.notes++
			s.c.keep(seqOf(b, s.out[bkAfterSeq]), k.pattern)
		}
		if s.c.keys == nil {
			s.c.keys = make(map[findingKey]bool, 3)
		}
		s.c.keys[k] = true
	}
}

// ours reports a line of the context's session and prompt; a null or
// absent prompt_id is the nil prompt's.
func (s *budgetScan) ours(b []byte) bool {
	if !s.is(b, s.out[bkSession], s.ctx.SessionID) {
		return false
	}
	p := s.out[bkPrompt]
	if s.ctx.PromptID == nil {
		return p.Kind == jsonscan.Null || p.Kind == jsonscan.Absent
	}
	return s.is(b, p, *s.ctx.PromptID)
}

// is reports a string span whose unescaped value is want: the writer
// escapes & < > (json.Marshal), so raw bytes are never compared.
func (s *budgetScan) is(b []byte, sp jsonscan.Span, want string) bool {
	if sp.Kind != jsonscan.String {
		return false
	}
	if cap(s.scratch) < len(want)+1 {
		s.scratch = make([]byte, 0, max(len(want)+1, 64))
	}
	got, truncated, err := jsonscan.Head(b, sp, len(want)+1, s.scratch[:0])
	return err == nil && !truncated && string(got) == want
}

// text is a string span's value, "" for any other kind.
func (s *budgetScan) text(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	v, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return v
}
