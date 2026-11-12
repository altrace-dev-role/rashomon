package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Event is one record line or steer line, decoded to its place in the
// session's merged order and the fields a fold rule reads, nothing else: a
// line's words (model_text, user_text) and every field no rule reads are
// never decoded, so they cannot reach a frontier (TestEvent_HoldsNoWords).
type Event struct {
	// Seq is a record's seq, or a steer line's after_seq: the last record
	// seq the line's decision saw.
	Seq int64
	// Steer is true for a steer line. At one seq a record sorts first,
	// because the line came after the record its after_seq names.
	Steer bool
	// AtUnixMS and ID are a steer line's clock and id, the order among the
	// lines at one seq. A record leaves them zero.
	AtUnixMS int64
	ID       string
	// Type is the record's type, or steer.LineType.
	Type string
	// State, Key and Kind are a steer line's state (steer.LineState), its
	// finding's dedupe key and its kind, which the Delivered rule reads. A
	// record leaves them empty.
	State string
	Key   string
	Kind  string
	// PromptID is a steer line's prompt_id, an id the Intents rule reads;
	// a line with no prompt (a briefing: prompt_id null) leaves it empty.
	PromptID string
	// Wrote are the project-relative paths a declaration names as written,
	// which the Touched rule reads (wrote). Every other record and every
	// steer line leave it nil.
	Wrote []string
	// WroteWithheld is true for a declaration whose write path the record
	// withheld (file_path_withheld or written_paths_withheld set: no_consent
	// below record scope v5, oversize, switched_off), which TouchedWithheld
	// counts. A withheld cwd or project alone is no write.
	WroteWithheld bool
	// WroteOutside is true for a declaration that names a path as written
	// outside the project (inProject false), which TouchedOutside counts;
	// the declaration's paths inside the project are still in Wrote.
	WroteOutside bool
}

// Before reports whether e sorts before o in the merged order (seq, record
// before line, at_unix_ms, id). Record seqs are unique (nextSeq allocates
// them under the records lock) and so are line ids, so the order is total.
func (e Event) Before(o Event) bool {
	if e.Seq != o.Seq {
		return e.Seq < o.Seq
	}
	if e.Steer != o.Steer {
		return !e.Steer
	}
	if e.AtUnixMS != o.AtUnixMS {
		return e.AtUnixMS < o.AtUnixMS
	}
	return e.ID < o.ID
}

var errUnplaced = errors.New("session: a line the merge cannot place")

// RecordEvent decodes one records.ndjson line. Every record with a type and
// a seq is an event; a rule that has no use for its type passes it by. Only
// a declaration's paths are writes: an instructions record carries a
// file_path too, the instructions file it loaded.
func RecordEvent(line []byte) (ev Event, ok bool, err error) {
	var h struct {
		Type         string   `json:"type"`
		Seq          *int64   `json:"seq"`
		FilePath     *string  `json:"file_path"`
		WrittenPaths []string `json:"written_paths"`
		Project      *string  `json:"project"`
		CWD          *string  `json:"cwd"`
		// FileWithheld and WrittenWithheld are the two write paths'
		// withheld reasons; WroteWithheld reads only whether one is set.
		FileWithheld    *string `json:"file_path_withheld"`
		WrittenWithheld *string `json:"written_paths_withheld"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		return Event{}, false, fmt.Errorf("session: record line: %w", err)
	}
	if h.Type == "" || h.Seq == nil {
		return Event{}, false, fmt.Errorf("%w: record without type or seq", errUnplaced)
	}
	ev = Event{Seq: *h.Seq, Type: h.Type}
	if h.Type == store.TypeDeclaration {
		ev.Wrote, ev.WroteOutside = wrote(h.FilePath, h.WrittenPaths, h.Project, h.CWD)
		ev.WroteWithheld = h.FileWithheld != nil || h.WrittenWithheld != nil
	}
	return ev, true, nil
}

// wrote is the paths a declaration names as written, in the project's
// terms, as verify reads a write: file_path, which only a file-changing
// tool's declaration carries (shape.DeclaredPath: Edit, Write, MultiEdit,
// NotebookEdit; never Read), then each written_paths entry (a shell line's),
// each through inProject: a path in the project is in paths, and one
// outside it is dropped and sets outside. A withheld path is null and names
// nothing, inside or out.
func wrote(file *string, shell []string, project, cwd *string) (paths []string, outside bool) {
	add := func(p string) {
		rel, ok := inProject(p, project, cwd)
		if !ok {
			outside = true
			return
		}
		paths = append(paths, rel)
	}
	if file != nil {
		add(*file)
	}
	for _, p := range shell {
		add(p)
	}
	return paths, outside
}

// inProject is p relative to the project and whether it lies in it, by
// verify's two rules composed as its rule check composes them
// (rules_raise.go: projectPath(relativise(p, project, cwd))): separators
// slashed; the project prefix stripped, else the cwd prefix; cleaned; and
// outside when still absolute, "..", climbing out or drive-lettered. It is
// a copy because both rules are unexported in verify and this lane's file
// list is closed; relativise's leading "./" loop is left out because Clean
// removes the same prefix. Pure over its strings, as verify's are: no
// symlink is resolved, so a path written through an alias of the project
// (/tmp for /private/tmp) is outside here as it is there.
//
// The copy's sources, at 20a64198 (verify is untouched on this branch):
// internal/verify/classify.go (blob 88d9c2cbf2bf) relativise, under,
// samePrefix and isLetter, which cutDir, sameDir and isASCIILetter copy;
// internal/verify/rules.go (blob f5a820ddd857) projectPath. It is a debt:
// the extraction into one package both read waits for the M2 merge (ruling
// 11:37). Until then TestInProject_DecidesAsVerify holds the copy's
// decision to verify.RelevantUnder's on a fixed table.
func inProject(p string, project, cwd *string) (string, bool) {
	p = strings.ReplaceAll(p, `\`, "/")
	for _, dir := range [2]*string{project, cwd} {
		if dir == nil {
			continue
		}
		if rel, ok := cutDir(p, strings.ReplaceAll(*dir, `\`, "/")); ok {
			p = rel
			break
		}
	}
	p = path.Clean(p)
	if path.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") || len(p) > 1 && p[1] == ':' {
		return "", false
	}
	return p, true
}

// cutDir is p below dir, without dir and the separator, and whether it is
// (verify's under). A dir of "" or "/" alone is no prefix: it would make
// every absolute path relative.
func cutDir(p, dir string) (string, bool) {
	dir = strings.TrimRight(dir, "/")
	if dir == "" || len(p) <= len(dir) || p[len(dir)] != '/' || !sameDir(p[:len(dir)], dir) {
		return p, false
	}
	return p[len(dir)+1:], true
}

// sameDir compares two directory strings exactly, except that a leading
// drive letter (C: against c:) compares without case (verify's samePrefix).
func sameDir(a, b string) bool {
	if len(a) >= 2 && a[1] == ':' && b[1] == ':' && isASCIILetter(a[0]) && isASCIILetter(b[0]) {
		return a[0]|0x20 == b[0]|0x20 && a[2:] == b[2:]
	}
	return a == b
}

func isASCIILetter(c byte) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' }

// SteerEvent decodes one steer.ndjson line. Only a steer.LineType line is an
// event: a steer_evidence line (steer.EvidenceLineType) carries no after_seq
// and feeds no frontier field, so ok is false and err nil; its bytes still
// move the steer cursor.
func SteerEvent(line []byte) (ev Event, ok bool, err error) {
	var h struct {
		Type     string  `json:"type"`
		ID       string  `json:"id"`
		AtUnixMS int64   `json:"at_unix_ms"`
		AfterSeq *int64  `json:"after_seq"`
		State    string  `json:"state"`
		Key      string  `json:"key"`
		Kind     string  `json:"kind"`
		PromptID *string `json:"prompt_id"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		return Event{}, false, fmt.Errorf("session: steer line: %w", err)
	}
	if h.Type != steer.LineType {
		return Event{}, false, nil
	}
	if h.AfterSeq == nil || h.ID == "" {
		return Event{}, false, fmt.Errorf("%w: steer line without after_seq or id", errUnplaced)
	}
	ev = Event{Seq: *h.AfterSeq, Steer: true, AtUnixMS: h.AtUnixMS, ID: h.ID, Type: h.Type, State: h.State, Key: h.Key, Kind: h.Kind}
	if h.PromptID != nil {
		ev.PromptID = *h.PromptID
	}
	return ev, true, nil
}

// Step applies one event, taken in the merged order, to f. It is pure: no
// I/O, no clock and no graph, so what it writes depends on f and ev alone,
// and a fold over a prefix then the tail equals a fold over the whole.
func Step(f *Frontier, ev Event) {
	if ev.PromptID != "" {
		intent(f, ev)
	}
	if ev.State == string(steer.LineStateDelivered) { // only SteerEvent sets State
		deliver(f, ev)
	}
	for _, p := range ev.Wrote {
		touch(f, p)
	}
	if ev.WroteWithheld {
		f.TouchedWithheld++
	}
	if ev.WroteOutside {
		f.TouchedOutside++
	}
}

// touch adds the file a call wrote to Touched as its graph.FileID, kept
// sorted and unique at every step, so Touched is a set: the same writes in
// any order fold to the same list (D1: files, not units; see Frontier).
func touch(f *Frontier, rel string) {
	id := string(graph.FileID(rel))
	i := sort.SearchStrings(f.Touched, id)
	if i < len(f.Touched) && f.Touched[i] == id {
		return
	}
	f.Touched = append(f.Touched, "")
	copy(f.Touched[i+1:], f.Touched[i:])
	f.Touched[i] = id
}

// deliver keeps a delivered steer line as a LineRef at its after_seq. Only
// state delivered counts: suppressed, superseded, stale, expired and
// unconfirmed lines never reached the model as a delivery. One LineRef per
// line id (canon's order needs it): a line seen again keeps its first ref.
// Unit stays empty: only a graph-kind line names one, and at this commit
// no writer emits a graph-kind steer line (M1's brief is kind "", its
// graph lines inside its slots).
func deliver(f *Frontier, ev Event) {
	for _, r := range f.Delivered {
		if r.ID == ev.ID {
			return
		}
	}
	f.Delivered = append(f.Delivered, LineRef{ID: ev.ID, Key: ev.Key, Kind: ev.Kind, Seq: ev.Seq})
}

// intent keeps each prompt's first steer line, in any state, as an
// IntentRef (D2). n is the prompt's order of first appearance in the merged
// order, counted from 1 as the graph's prompt nodes are, so ID is that
// node's id; Digest is the prompt id, because no line carries a prompt
// digest; StatedAt is the line's after_seq; Status is open (v1 closes no
// intent). A prompt that wrote no steer line has no IntentRef and shifts
// every later n: the record holds no prompt line to count from.
func intent(f *Frontier, ev Event) {
	for _, r := range f.Intents {
		if r.Digest == ev.PromptID {
			return
		}
	}
	n := len(f.Intents) + 1
	f.Intents = append(f.Intents, IntentRef{ID: string(graph.PromptNodeID(f.Session, n)), Digest: ev.PromptID, StatedAt: ev.Seq, Status: ReqOpen})
}
