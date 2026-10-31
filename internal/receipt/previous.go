package receipt

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The previous session (card REPORT1b): the project's newest checkpoint that
// is not the current session's, found by the scan Latest makes under a query
// that can skip a run, bound the record's time and cap the work. It feeds the
// one line SessionStart prints beside the brief and the report's baseline.

// TailBytes is the first read of a run's records file on the startup path:
// the last 64 KiB, which holds eight checkpoints at store-side
// verify.CheckpointMaxBytes (8 KiB).
const TailBytes = 64 << 10

// Query bounds one scan. A zero field is no bound (and no skip).
type Query struct {
	// Project is baseline.Key of the project, which a checkpoint's
	// project_root holds. A record of any other project is passed over.
	Project string
	// Skip is the session id of a run to pass over by name: the current one.
	Skip string
	// Before, when set, is the exclusive upper bound on a record's
	// recorded_at_unix_ms: the baseline of a session is the project's newest
	// checkpoint before its first. With Before set a run is read through
	// every checkpoint, not its tail.
	Before int64
	// MaxRuns is how many runs are read at most, MaxBytes how many bytes in
	// all. The stat of every run is not counted by either.
	MaxRuns  int
	MaxBytes int64
}

// Gate is a gate as a checkpoint kept it.
type Gate struct {
	Gate     string
	Required bool
	State    string
}

// Last is the checkpoint the scan found, as much of it as the startup
// line and the report's fold read.
type Last struct {
	SessionID        string
	ProjectRoot      string
	RecordedAtUnixMS int64
	PointSeq         int64
	RecordGreen      bool
	// NotGreen is the required gates whose state is not green; NotGreenAtLeast
	// is set when the checkpoint cut gates (GatesOmitted) and kept none
	// green, so that non-green ones may have been cut with them.
	NotGreen            int
	NotGreenAtLeast     bool
	IntegrityUnresolved bool
	NoRequired          bool
	// SpecTrust and SpecDigest are the spec's trust and compiled digest at
	// the checkpoint, "" for none.
	SpecTrust, SpecDigest string
	Gates                 []Gate
	GatesOmitted          int
}

// Scan says how the walk ended: Found when a checkpoint was found, and when
// not, Bound is "" when every run was read, "runs" when the run bound ended
// it and "bytes" when the byte bound did.
type Scan struct {
	Found bool
	Bound string
	// ReadBytes is how many bytes of records files the walk read.
	ReadBytes int64
}

// Bounds of a Scan.
const (
	BoundRuns  = "runs"
	BoundBytes = "bytes"
)

// decodeHeadT is what a checkpoint line is read as: the head of one record
// the store wrote, at most verify.CheckpointMaxBytes, a closed shape.
type decodeHeadT struct {
	SessionID        string  `json:"session_id"`
	ProjectRoot      string  `json:"project_root"`
	RecordedAt       int64   `json:"recorded_at_unix_ms"`
	PointSeq         int64   `json:"point_seq"`
	SpecDigest       *string `json:"spec_digest"`
	SpecTrust        *string `json:"spec_trust"`
	RecordGreen      bool    `json:"record_green"`
	GatesOmitted     int     `json:"gates_omitted"`
	IntegrityOmitted int     `json:"integrity_omitted"`
	Gates            []struct {
		Gate     string `json:"gate"`
		Required bool   `json:"required"`
		State    string `json:"state"`
	} `json:"gates"`
	Integrity struct {
		Unresolved []json.RawMessage `json:"unresolved"`
	} `json:"integrity"`
}

// decodeHead is the one decode this package makes, allowed by name in
// RES-5 (internal/hookio/res5_test.go): rashomon's own checkpoint line,
// capped by the store, read on the startup path and by the report.
func decodeHead(line []byte) (Last, bool) {
	var h decodeHeadT
	if json.Unmarshal(line, &h) != nil {
		return Last{}, false
	}
	p := Last{SessionID: h.SessionID, ProjectRoot: h.ProjectRoot, RecordedAtUnixMS: h.RecordedAt, PointSeq: h.PointSeq,
		RecordGreen: h.RecordGreen, GatesOmitted: h.GatesOmitted,
		IntegrityUnresolved: len(h.Integrity.Unresolved) > 0 || h.IntegrityOmitted > 0}
	if h.SpecTrust != nil {
		p.SpecTrust = *h.SpecTrust
	}
	if h.SpecDigest != nil {
		p.SpecDigest = *h.SpecDigest
	}
	required, green := 0, 0
	for _, g := range h.Gates {
		p.Gates = append(p.Gates, Gate{g.Gate, g.Required, g.State})
		if g.State == "green" {
			green++
		}
		if g.Required {
			required++
			if g.State != "green" {
				p.NotGreen++
			}
		}
	}
	p.NotGreenAtLeast = h.GatesOmitted > 0 && green == 0
	p.NoRequired = required == 0 && h.GatesOmitted == 0
	return p, true
}

// Decode is a checkpoint line, as the store wrote it, read as a Last.
func Decode(line []byte) (Last, bool) { return decodeHead(line) }

// Previous is the newest checkpoint of q.Project among each run's, by
// recorded_at, under q's bounds and skip, with how the walk ended. The order
// and the stop rule are Latest's: runs newest written first, and the walk
// stops at a run last written before the best record so far, so the usual
// answer is the newest run's last checkpoint, found in one read.
func Previous(st *store.Store, q Query) (Last, Scan, error) {
	_, p, sc, err := scan(st, q)
	return p, sc, err
}

// scan is the walk Latest and PreviousOf share. line is the best
// checkpoint's bytes as stored.
func scan(st *store.Store, q Query) (line []byte, best Last, sc Scan, err error) {
	runs, err := byWritten(st)
	if err != nil {
		return nil, Last{}, Scan{}, err
	}
	skip := ""
	if q.Skip != "" {
		skip = st.DirName(q.Skip)
	}
	var spent int64
	read := 0
	for _, r := range runs {
		if r.name == skip {
			continue
		}
		if sc.Found && r.at < best.RecordedAtUnixMS {
			break
		}
		if q.MaxRuns > 0 && read >= q.MaxRuns {
			sc.Bound = BoundRuns
			break
		}
		if q.MaxBytes > 0 && spent >= q.MaxBytes {
			sc.Bound = BoundBytes
			break
		}
		read++
		var l []byte
		var capped bool
		if q.Before > 0 {
			l, spent, err = beforeIn(st, r, q, spent)
		} else {
			l, spent, capped, err = tailIn(st, r, q, spent)
		}
		if err != nil {
			return nil, Last{}, Scan{}, err
		}
		if l != nil {
			if p, ok := decodeHead(l); ok && p.ProjectRoot == q.Project && (!sc.Found || p.RecordedAtUnixMS > best.RecordedAtUnixMS) {
				line, best, sc.Found = l, p, true
			}
		}
		if capped && !sc.Found {
			sc.Bound = BoundBytes
			break
		}
	}
	if sc.Found {
		sc.Bound = ""
	}
	sc.ReadBytes = spent
	return line, best, sc, nil
}

// tailIn is run r's last checkpoint on the startup path: the last TailBytes
// first and, only when that holds none and the file is longer, the last of
// what the byte budget has left. capped is true when the file was not read
// to its start and the budget was what stopped it.
func tailIn(st *store.Store, r written, q Query, spent int64) (line []byte, now int64, capped bool, err error) {
	first := int64(TailBytes)
	if q.MaxBytes > 0 && first > q.MaxBytes-spent {
		first = q.MaxBytes - spent
	}
	line, ok, n, err := st.LatestRecordTail(r.name, store.TypeVerify, first)
	spent += n
	if err != nil || ok || r.size <= first {
		return line, spent, false, err
	}
	// No checkpoint in the end of a longer file.
	left := r.size
	if q.MaxBytes > 0 {
		left = q.MaxBytes - spent
	}
	if left <= first {
		return nil, spent, true, nil
	}
	line, ok, n, err = st.LatestRecordTail(r.name, store.TypeVerify, left)
	spent += n
	if err != nil || ok {
		return line, spent, false, err
	}
	return nil, spent, r.size > left, nil
}

// beforeIn is the newest checkpoint of run r recorded before q.Before, read
// through the whole file; its size counts against the byte budget.
func beforeIn(st *store.Store, r written, q Query, spent int64) (line []byte, now int64, err error) {
	var at int64
	_, err = st.EachRecordOfKind(r.name, store.TypeVerify, func(l []byte) bool {
		p, ok := decodeHead(l)
		if ok && p.ProjectRoot == q.Project && p.RecordedAtUnixMS < q.Before && (line == nil || p.RecordedAtUnixMS >= at) {
			line, at = l, p.RecordedAtUnixMS
		}
		return true
	})
	return line, spent + r.size, err
}

// PreviousLine is the startup line for what the scan found, at most 80
// columns with its two-space indent and its "last session: " lead. A found
// checkpoint gives its time and one reason it was or was not green, then
// "; " and pointer, the report command the user can run; a reason that
// would not fit is left out, never the pointer. Nothing found gives the bound
// that ended the walk and no pointer, since there is no session to show.
func PreviousLine(p Last, s Scan, pointer string) string {
	const lead = "last session: "
	if !s.Found {
		switch s.Bound {
		case BoundRuns:
			return lead + "none in the " + strconv.Itoa(MaxRuns) + " most recent sessions on this machine"
		case BoundBytes:
			return lead + "none within the " + strconv.Itoa(MaxBytes>>20) + " MiB read bound on this machine"
		}
		return lead + "none recorded for this project"
	}
	at := time.UnixMilli(p.RecordedAtUnixMS).Format("01-02 15:04")
	line := lead + at + ", " + reason(p) + "; " + pointer
	if len("  "+line) > 80 {
		line = lead + at + "; " + pointer
	}
	return line
}

// The startup path's bounds (card REPORT1b K5): 16 runs and 8 MiB in all.
const (
	MaxRuns  = 16
	MaxBytes = 8 << 20
)

// reason is the one reason the line carries, in precedence order.
func reason(p Last) string {
	switch {
	case p.RecordGreen:
		return "record green"
	case p.NotGreen > 0:
		s := fmt.Sprintf("%d gates not green", p.NotGreen)
		if p.NotGreen == 1 {
			s = "1 gate not green"
		}
		if p.NotGreenAtLeast {
			s = "at least " + s
		}
		return s
	case p.IntegrityUnresolved:
		return "integrity unresolved"
	case p.NoRequired:
		return "no required gate"
	}
	return "not green"
}
