package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// This file is the first thing the prescan does with a transcript file, before
// any session is built from it: it reads the file once, for what says which
// session or sessions it belongs to, what role it plays in each, when it
// begins, and what its bytes are. From that alone a set of files is grouped
// into sessions (a session is its Claude Code sessionId), and the files of
// each are put in an order by a rule that depends on nothing but their
// contents.
//
// # The file order
//
// OrderFiles ranks the files of one session by this rule, and no other:
//
//  1. The session's own files come before its subagents'. A file is the
//     session's own for sessionId S when it holds at least one line of S that
//     is a user or assistant line and not a sidechain one (the transcript the
//     session wrote, or a resumed or copied one); a file whose lines of S are
//     all sidechain lines is a subagent's file (<sid>/subagents/agent-<id>.jsonl).
//  2. Within each group, by the earliest timestamp on any line of the file,
//     earliest first; a file with no readable timestamp comes last.
//  3. Then by the SHA-256 of the file's bytes.
//
// The rule reads nothing the file system says: not the path, not the
// modification time, not the order a directory listed them in. The same bytes
// in other directories, copied, restored or listed differently, give the same
// order. Two files with equal digests are one file: the later is not a second
// source, it is dropped (OrderFiles returns it as a duplicate), so a backup
// copy costs one digest and nothing else.
//
// The rule is a convention, not a claim about which file is "right". What it
// settles is the file index that positions carry (Event.File) and, where two
// copies of one call agree, which copy's position is kept. Where they disagree
// no copy is trusted, whatever the order.

// MaxFileSessions bounds how many distinct sessionIds Inspect keeps for one
// file; a file naming more is flagged (FileMeta.SessionsOverflow) and the
// rest are not attributed. A transcript names one, or a few where a resume
// copied an earlier session's lines into the new file.
const MaxFileSessions = 64

// FileMeta is what Inspect read of one file.
type FileMeta struct {
	// Digest is the hex SHA-256 of the file's bytes, whole.
	Digest string
	Size   int64
	Lines  int
	// Unreadable counts lines that were too long or did not decode.
	Unreadable int
	// Sessions are the sessionIds the file's lines name, by id, at most
	// MaxFileSessions of them. SessionsOverflow: it named more.
	Sessions         []FileSession
	SessionsOverflow bool
	// First is the earliest timestamp on any line of the file, zero when none
	// parses.
	First time.Time
}

// FileSession is one session a file belongs to.
type FileSession struct {
	ID string
	// Main: the file holds a user or assistant line of this session that is
	// not a sidechain one. A file that is not Main for a session is a
	// subagent's.
	Main bool
}

// Has reports whether the file names sessionID, and whether it is the
// session's own file there.
func (m FileMeta) Has(sessionID string) (named, main bool) {
	i := sort.Search(len(m.Sessions), func(i int) bool { return m.Sessions[i].ID >= sessionID })
	if i < len(m.Sessions) && m.Sessions[i].ID == sessionID {
		return true, m.Sessions[i].Main
	}
	return false, false
}

type inspectHead struct {
	SessionID   string `json:"sessionId"`
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
}

// Inspect reads r to its end once and reports what it needs of the file. A
// line that cannot be read is counted and stepped over, as everywhere.
func Inspect(r io.Reader) (FileMeta, error) {
	h := sha256.New()
	cr := &countingReader{r: io.TeeReader(r, h)}
	sc := linescan.New(cr, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	var m FileMeta
	main := map[string]bool{}
	for sc.Scan() {
		m.Lines++
		if sc.TooLong() {
			m.Unreadable++
			continue
		}
		var hd inspectHead
		switch report.DecodeHeader(sc.Bytes(), &hd) {
		case report.LineBlank:
			continue
		case report.LineUndecodable, report.LineUndecodableSidechain:
			m.Unreadable++
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, hd.Timestamp); err == nil {
			if t = t.UTC(); m.First.IsZero() || t.Before(m.First) {
				m.First = t
			}
		}
		if hd.SessionID == "" {
			continue
		}
		isMain, seen := main[hd.SessionID]
		if !seen && len(main) >= MaxFileSessions {
			m.SessionsOverflow = true
			continue
		}
		own := !hd.IsSidechain && (hd.Type == "user" || hd.Type == "assistant")
		main[hd.SessionID] = isMain || own
	}
	if err := sc.Err(); err != nil {
		return FileMeta{}, err
	}
	m.Size = cr.n
	m.Digest = hex.EncodeToString(h.Sum(nil))
	for id, own := range main {
		m.Sessions = append(m.Sessions, FileSession{ID: id, Main: own})
	}
	sort.Slice(m.Sessions, func(i, j int) bool { return m.Sessions[i].ID < m.Sessions[j].ID })
	return m, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// OrderFiles is the files of sessionID in the order of the rule above:
// order holds their indexes into metas, first file first. duplicates holds
// the indexes of files dropped for being byte for byte another file's.
func OrderFiles(metas []FileMeta, sessionID string) (order, duplicates []int) {
	type ranked struct {
		i    int
		main bool
	}
	var rs []ranked
	for i, m := range metas {
		if named, main := m.Has(sessionID); named {
			rs = append(rs, ranked{i, main})
		}
	}
	sort.SliceStable(rs, func(a, b int) bool {
		x, y := rs[a], rs[b]
		if x.main != y.main {
			return x.main
		}
		fx, fy := metas[x.i].First, metas[y.i].First
		if fx.IsZero() != fy.IsZero() {
			return fy.IsZero()
		}
		if !fx.Equal(fy) {
			return fx.Before(fy)
		}
		return metas[x.i].Digest < metas[y.i].Digest
	})
	seen := map[string]bool{}
	for _, r := range rs {
		if d := metas[r.i].Digest; seen[d] {
			duplicates = append(duplicates, r.i)
			continue
		} else {
			seen[d] = true
		}
		order = append(order, r.i)
	}
	return order, duplicates
}

// Contexts are a file's turn contexts: for any line, the prompt and
// permission mode of the turn it falls in.
type Contexts struct{ sc *scan }

// TurnContext is one turn's context for a line.
type TurnContext struct {
	PromptID, Mode string
	// ModeChangeLine is the line of the first permission-mode line inside the
	// turn, 0 for none.
	ModeChangeLine int
}

// ReadContexts reads src for its turns, as the first pass of Walk does.
func ReadContexts(src io.Reader, opt Options) (*Contexts, error) {
	sc, err := scanTurns(src, opt)
	if err != nil {
		return nil, err
	}
	return &Contexts{sc}, nil
}

// At is the context of the turn a line falls in, false before the file's first
// prompt.
func (c *Contexts) At(line int) (TurnContext, bool) {
	t := c.sc.at(line)
	if t == nil {
		return TurnContext{}, false
	}
	return TurnContext{PromptID: t.promptID, Mode: t.mode, ModeChangeLine: t.permChange}, true
}
