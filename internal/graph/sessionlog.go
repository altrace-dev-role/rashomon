package graph

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"unicode/utf8"
)

// The session log (ruling 15(a), W-M2.11) is one JSONL file per repository
// in the install store, in the format of the code index: a Header, then one
// Record per line. A hook appends lines; nothing rewrites the file. It is
// never .rashomon/graph.jsonl, which `graph index` regenerates (15(e)).
// LoadSessionLog reads it into the Store that holds the code index, so
// there is one Store at read time and no fold.

// Caps, in bytes (ruling 15: a command line cut at the existing cap, output
// tails at a fixed cap, a runner's output whole, under _cut markers).
const (
	// SessionCommandCap equals store.MaxCommandBytes (pinned by a test).
	SessionCommandCap = 65536
	// SessionOutputTail is how much of a tool's output, from its end, a
	// call keeps.
	SessionOutputTail = 8192
	// SessionRunnerCap bounds a runner's output kept whole; a line must
	// stay under store.MaxRecordBytes.
	SessionRunnerCap = 262144
	SessionTextCap   = 65536 // a prompt or an edit's diff
	SessionNoteCap   = 8192  // the words of a note
	SessionQuoteCap  = 4096  // a claim's quoted sentence
)

// ErrNoSessionConsent is Append's refusal when the log was opened without
// session consent (the install's consent covers scope_version 6).
var ErrNoSessionConsent = errors.New("graph: session content needs consent at scope_version 6")

// SessionLog appends session records for one repository. It holds no file
// open: each Append opens, writes once and closes, so a hook killed between
// calls loses nothing already written.
type SessionLog struct {
	// Path is the log file.
	Path    string
	repo    string
	rev     Rev
	allowed bool
}

// SessionLogPath is <storeRoot>/graph/<16 hex of sha256(rid)>.jsonl: a rid
// holds slashes and may hold dots, so it is hashed, never joined.
func SessionLogPath(storeRoot, rid string) string {
	sum := sha256.Sum256([]byte(rid))
	return filepath.Join(storeRoot, "graph", hex.EncodeToString(sum[:8])+".jsonl")
}

// OpenSessionLog is the log for repository rid under storeRoot. Every record
// it builds is at rev (the code index's commit, so a reader at that commit
// sees both). allowed is store.SessionContentMarker(root), read by the
// caller: false makes Append refuse and write nothing. It does no I/O.
func OpenSessionLog(storeRoot, rid string, rev Rev, allowed bool) *SessionLog {
	return &SessionLog{Path: SessionLogPath(storeRoot, rid), repo: rid, rev: rev, allowed: allowed}
}

// Append writes recs as lines in one write(2) on an O_APPEND handle, the
// header first when the file is new, a line feed first when a killed
// append left a part line. It does not fsync: a kill of the process cannot
// lose a completed write. ErrNoSessionConsent when the log is not allowed.
func (l *SessionLog) Append(recs ...Record) error {
	if !l.allowed {
		return ErrNoSessionConsent
	}
	if len(recs) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err = l.prefix(f, enc, &buf)
	for i := 0; err == nil && i < len(recs); i++ {
		err = enc.Encode(recs[i])
	}
	if err == nil {
		_, err = f.Write(buf.Bytes())
	}
	return errors.Join(err, f.Close())
}

// prefix puts the header (empty file) or a repairing line feed (part last
// line) into buf.
func (l *SessionLog) prefix(f *os.File, enc *json.Encoder, buf *bytes.Buffer) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return enc.Encode(Header{Schema: Schema, Repo: l.repo, Rev: HeaderRev{Commit: l.rev.Commit}, Producers: map[string]string{"session": "session/v1"}})
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return err
	}
	if last[0] != '\n' {
		buf.WriteByte('\n')
	}
	return nil
}

// LoadSessionLog reads a session log into s: the header (schema checked),
// then every record. It skips a repeated header and ANY line that does not
// parse, and returns how many it skipped. A killed append leaves a part
// line, and the next Append starts a new line after it (prefix), so the part
// line becomes a middle line of the file; failing on it, as Load does, would
// fail every later load.
func LoadSessionLog(s *Store, r io.Reader) (Header, int, error) {
	h, skipped, _, err := LoadSessionLogGroups(s, r)
	return h, skipped, err
}

// LoadSessionLogGroups is LoadSessionLog that also returns the complete
// overlay groups the log holds (overlaylog.go). A record at an overlay
// revision is never asserted into s here: it is held until its group's
// marker closes it, and a group is applied by ReplayOverlays for one session.
func LoadSessionLogGroups(s *Store, r io.Reader) (Header, int, []OverlayGroup, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	var h Header
	var oc overlayCollector
	first, skipped := true, 0
	for {
		line, rerr := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var l struct {
				Header
				Record
			}
			uerr := json.Unmarshal(line, &l)
			switch {
			case first:
				if uerr != nil || l.Schema != Schema {
					return h, skipped, nil, fmt.Errorf("graph: session log header: schema %d, want %d (%v)", l.Schema, Schema, uerr)
				}
				h, first = l.Header, false
			case uerr != nil:
				skipped++
				oc.reset()
			case l.Schema != 0:
				oc.reset()
			case oc.take(l.Record):
			case l.Node != nil:
				s.AssertNode(*l.Node)
			case l.Edge != nil:
				s.AssertEdge(*l.Edge)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return h, skipped, oc.groups, nil
			}
			return h, skipped, oc.groups, rerr
		}
	}
}

// CallFields is what a call record keeps. Command must be the masked text.
// Exit is nil when the exit status is unknown, which is not zero.
type CallFields struct {
	Tool, Command, CWD, Path, Diff, Output string
	Exit                                   *int
	DurationMS                             int64
	// Runner keeps Output whole up to SessionRunnerCap instead of its tail.
	Runner bool
}

func (l *SessionLog) node(id ID, kind string, attrs map[string]string) Record {
	return Record{Node: &Node{ID: id, Kind: kind, Attrs: attrs, Tier: Observed, Conf: 1, Status: Asserted, Rev: l.rev,
		Prov: Prov{Extractor: "session", Version: "v1"}}}
}

// Edge is an edge record between any two ids.
func (l *SessionLog) Edge(src ID, pred Pred, dst ID) Record {
	return Record{Edge: &Edge{Src: src, Pred: pred, Dst: dst, Tier: Observed, Conf: 1, Status: Asserted, Rev: l.rev,
		Prov: Prov{Extractor: "session", Version: "v1"}}}
}

// Session is the session node.
func (l *SessionLog) Session(sid string) Record {
	return l.node(SessionNodeID(sid), KindSession, map[string]string{"session_id": sid})
}

// Prompt is the nth prompt, its text kept up to SessionTextCap.
func (l *SessionLog) Prompt(sid string, n int, text string) Record {
	a := map[string]string{"n": strconv.Itoa(n)}
	put(a, "text", text, SessionTextCap)
	return l.node(PromptNodeID(sid, n), KindPrompt, a)
}

// Call is a tool call: the command (cut at SessionCommandCap), cwd, exit
// status, duration, the output tail or a runner's output whole, and for an
// edit the diff. An empty field is not written.
func (l *SessionLog) Call(sid, toolUseID string, c CallFields) Record {
	a := map[string]string{"tool": c.Tool}
	put(a, "command", c.Command, SessionCommandCap)
	put(a, "cwd", c.CWD, 4096)
	put(a, "path", c.Path, 4096)
	put(a, "diff", c.Diff, SessionTextCap)
	if c.Runner {
		put(a, "output", c.Output, SessionRunnerCap)
	} else if out, cut := TailText(c.Output, SessionOutputTail); out != "" {
		a["output"] = out
		if cut != "" {
			a["output_cut"] = cut
		}
	}
	if c.Exit != nil {
		a["exit"] = strconv.Itoa(*c.Exit)
	}
	if c.DurationMS > 0 {
		a["duration_ms"] = strconv.FormatInt(c.DurationMS, 10)
	}
	return l.node(CallNodeID(sid, toolUseID), KindCall, a)
}

// Claim is a claim's quoted sentence, its class and verdict.
func (l *SessionLog) Claim(sid string, n int, quote, class, verdict string) Record {
	a := map[string]string{"class": class, "verdict": verdict}
	put(a, "quote", quote, SessionQuoteCap)
	return l.node(ClaimNodeID(sid, n), KindClaim, a)
}

// Note is a note as delivered: its event and its words.
func (l *SessionLog) Note(sid string, n int, event, words string) Record {
	a := map[string]string{"event": event}
	put(a, "words", words, SessionNoteCap)
	return l.node(NoteNodeID(sid, n), KindNote, a)
}

// Verdict is a verify verdict: the gates and their states.
func (l *SessionLog) Verdict(sid string, n int, gates, states string) Record {
	return l.node(VerdictNodeID(sid, n), KindVerdict, map[string]string{"gates": gates, "states": states})
}

// put sets a[name] to s cut to max, and a[name+"_cut"] when it was cut; an
// empty s writes nothing.
func put(a map[string]string, name, s string, max int) {
	if s == "" {
		return
	}
	out, cut := CutText(s, max)
	a[name] = out
	if cut != "" {
		a[name+"_cut"] = cut
	}
}

// CutText keeps the head and the tail of s when it is over max bytes, on
// rune boundaries, and says "<offset>:<removed bytes>" (the store's Cut) or
// "" when nothing was cut.
func CutText(s string, max int) (string, string) {
	if len(s) <= max {
		return s, ""
	}
	h := max / 2
	for h > 0 && !utf8.RuneStart(s[h]) {
		h--
	}
	t := len(s) - (max - max/2)
	for t < len(s) && !utf8.RuneStart(s[t]) {
		t++
	}
	return s[:h] + s[t:], strconv.Itoa(h) + ":" + strconv.Itoa(t-h)
}

// TailText keeps the last max bytes of s, on a rune boundary; the cut is
// "0:<removed>".
func TailText(s string, max int) (string, string) {
	if len(s) <= max {
		return s, ""
	}
	t := len(s) - max
	for t < len(s) && !utf8.RuneStart(s[t]) {
		t++
	}
	return s[t:], "0:" + strconv.Itoa(t)
}
