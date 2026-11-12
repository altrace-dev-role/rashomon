package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/events"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
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
	// Stated are the requirements a steer line's graph lines state (slots
	// line1..line5, graphSlot.stated), in slot order. The Reqs rule reads
	// them on a delivered line (require). A record leaves it nil.
	Stated []StatedReq
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
	// Ledger is true for a ledger line (ledgerType), whose rows and cut
	// counts the Obligations rule copies (ledger). Every other record and
	// every steer line leave all four empty.
	Ledger             bool
	Obligations        []ObligationRef
	ObligationsOmitted int
	UnitRowsCut        int
	// Boundary is true for a session record whose SessionStart source is
	// compact or resume, which sets LastBoundary (Step). Startup and clear
	// are not boundaries (Frontier.LastBoundary). No writer exists before
	// W-M4.3 and W-M4.4.
	Boundary bool
	// Checkpoint is true for a Stop's verify checkpoint record (type verify,
	// point stop), which StopsWithoutVerdict holds until a ledger line's
	// verdict names it.
	Checkpoint bool
	// Verdict is true for a ledger line that carried done_flagged, unmet and
	// checkpoint_seq, all three, which DoneFlagged, Unmet and CheckpointSeq
	// copy. A line without one adds no VerdictRef, never false or 0, and its
	// rows still apply.
	Verdict       bool
	DoneFlagged   bool
	Unmet         int
	CheckpointSeq int64
}

// ledgerType is the record type of a ledger line, the verify ledger at a
// point (D3). No writer exists before the M2 merge (ledger condition 7) and
// no store constant names the type yet, so the fold names it as the
// contract does (11:34, msg 71635d33); the writer's constant replaces this
// one when it lands.
const ledgerType = "ledger"

// stopPoint is the point of a Stop's verify checkpoint record
// (verify.PointStop). The fold imports no verify outside its tests, so it
// names the point, and TestCheckpoint_PointIsVerifys ties the two.
const stopPoint = "stop"

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
// file_path too, the instructions file it loaded. Only a session record's
// source can be a boundary.
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
		// Obligations are a ledger line's rows by verify.Obligation's own
		// keys, not ObligationRef's, so the frontier's names never decide
		// what a ledger line reads. Scope, source and evidence are not read.
		Obligations []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Unit  string `json:"unit"`
			State string `json:"state"`
		} `json:"obligations"`
		ObligationsOmitted int `json:"obligations_omitted"`
		UnitRowsCut        int `json:"unit_rows_cut"`
		// Source is a session record's SessionStart source.
		Source *string `json:"source"`
		// Point is a verify record's point kind.
		Point *string `json:"point"`
		// DoneFlagged, Unmet and CheckpointSeq are a ledger line's verdict,
		// each a pointer so an absent or null key never reads as false or 0.
		DoneFlagged   *bool  `json:"done_flagged"`
		Unmet         *int   `json:"unmet"`
		CheckpointSeq *int64 `json:"checkpoint_seq"`
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
	if h.Type == ledgerType {
		ev.Ledger = true
		ev.Obligations = make([]ObligationRef, 0, len(h.Obligations))
		for _, o := range h.Obligations {
			ev.Obligations = append(ev.Obligations, ObligationRef{ID: o.ID, Kind: o.Kind, Unit: o.Unit, State: o.State})
		}
		ev.ObligationsOmitted, ev.UnitRowsCut = h.ObligationsOmitted, h.UnitRowsCut
		ev.Verdict = h.DoneFlagged != nil && h.Unmet != nil && h.CheckpointSeq != nil
		if ev.Verdict {
			ev.DoneFlagged, ev.Unmet, ev.CheckpointSeq = *h.DoneFlagged, *h.Unmet, *h.CheckpointSeq
		}
	}
	if h.Type == store.TypeSession && h.Source != nil {
		ev.Boundary = *h.Source == events.Compact || *h.Source == events.Resume
	}
	if h.Type == store.TypeVerify && h.Point != nil && *h.Point == stopPoint {
		ev.Checkpoint = true
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

// graphSlot is the part of a graph line on a steer line (steer's
// graphLineJSON, one per slot line1..line5) a Req is stated from: its kind,
// a code line's role word, a knowledge line's id, and its subject as the
// path, or in the record form (record scope v4, slot.GraphLine.Record) the
// path's keyed digest and no path.
type graphSlot struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Role   string `json:"role"`
	ID     string `json:"id"`
}

// codeKind is a code graph line's kind. slot.GraphLine.Kind returns it as
// a literal; no package exports it.
const codeKind = "code"

// StatedReq is one requirement a graph line states: Pred, a compliance
// predicate's name (steer.Compliance), and Arg, its subject as the line
// holds it.
type StatedReq struct {
	Pred string
	Arg  string
}

// stated is the Req g states, if any (D6 (b), ruling 12:52): a unit line
// states opened_unit, a code line in role reuse called_symbol and in role
// consumer edited_or_verified, each on its path or, at record scope v4,
// its keyed digest; a decision line states paths_untouched on its
// knowledge id (dec:D145), which the record form keeps. Each is the first
// predicate steer.Compliance names for the kind. A code line with no role,
// a table, a not-known line, any other knowledge kind and a line with no
// subject state none.
//
// The fold marks only opened_unit (openedUnit). called_symbol,
// edited_or_verified and paths_untouched stay open: a status changes only
// by a record line a later card defines (ruling 12:52 (1)), and no Stop
// resolves them, since the Stop's predicates (verify.Preds, W-M3.4) are
// four other names (ruling 13:11 (5)). called_symbol's Arg is the code
// line's path, not its symbol, because the record holds no symbol id: a
// card that needs the symbol adds it to slot.Record under its own
// disclosure ruling (debt (r), engine lane), and the fold never marks a
// Req whose symbol the record cannot name (ruling 12:52 (3)).
func (g graphSlot) stated() (StatedReq, bool) {
	arg := g.Path
	if arg == "" {
		arg = g.Digest
	}
	var k steer.Kind
	switch {
	case g.Kind == string(steer.KindUnit):
		k = steer.KindUnit
	case g.Kind == codeKind && g.Role == slot.RoleReuse:
		k = steer.KindReuse
	case g.Kind == codeKind && g.Role == slot.RoleConsumer:
		k = steer.KindConsumer
	case g.Kind == string(steer.KindDecision):
		k, arg = steer.KindDecision, g.ID
	default:
		return StatedReq{}, false
	}
	if arg == "" {
		return StatedReq{}, false
	}
	preds, _ := steer.Compliance(k)
	return StatedReq{Pred: preds[0], Arg: arg}, true
}

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
		// Slots are read only at the five graph line keys, each an object
		// slot.GraphLine wrote; a pattern's other slot keys are not read.
		Slots struct {
			Line1 *graphSlot `json:"line1"`
			Line2 *graphSlot `json:"line2"`
			Line3 *graphSlot `json:"line3"`
			Line4 *graphSlot `json:"line4"`
			Line5 *graphSlot `json:"line5"`
		} `json:"slots"`
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
	for _, g := range [...]*graphSlot{h.Slots.Line1, h.Slots.Line2, h.Slots.Line3, h.Slots.Line4, h.Slots.Line5} {
		if g == nil {
			continue
		}
		if r, ok := g.stated(); ok {
			ev.Stated = append(ev.Stated, r)
		}
	}
	return ev, true, nil
}

// Step applies one event, taken in the merged order, to f. It is pure: no
// I/O, no clock and no graph, so what it writes depends on f and ev alone,
// and a fold over a prefix then the tail equals a fold over the whole. It
// writes no element a Frontier copied before it can see: it appends, or
// rebuilds a list it changes in place (touch, openedUnit, verdict), so the
// copy keeps every field as it was (TestStep_LeavesCopies). Stepping both
// copies is not supported, since they share arrays past their lengths;
// fork one with ParseFrontier(MarshalFrontier(f)).
func Step(f *Frontier, ev Event) {
	if ev.PromptID != "" {
		intent(f, ev)
	}
	if ev.State == string(steer.LineStateDelivered) { // only SteerEvent sets State
		deliver(f, ev)
		for _, r := range ev.Stated {
			require(f, r.Pred, r.Arg, ev.Seq)
		}
	}
	for _, p := range ev.Wrote {
		touch(f, p)
	}
	openedUnit(f, ev)
	if ev.WroteWithheld {
		f.TouchedWithheld++
	}
	if ev.WroteOutside {
		f.TouchedOutside++
	}
	if ev.Ledger {
		ledger(f, ev)
	}
	if ev.Boundary {
		f.LastBoundary = ev.Seq
	}
	if ev.Checkpoint {
		f.StopsWithoutVerdict = append(f.StopsWithoutVerdict, ev.Seq)
	}
	if ev.Verdict {
		verdict(f, ev)
	}
}

// verdict copies a ledger line's verdict as the VerdictRef of the checkpoint
// its checkpoint_seq names, never computing one (rulings 12:03, 12:12). The
// line binds by that seq, never by adjacency: a record can land between a
// Stop's two appends. It binds only to a stop checkpoint read before it and
// still in StopsWithoutVerdict, which the checkpoint then leaves. A line
// naming none (no checkpoint at that seq, one not yet read, or one already
// judged) adds no verdict, so of two lines naming one checkpoint the first
// is kept; its rows apply either way (ledger). A checkpoint with no ledger
// line gets none: obligations carry over to the next Stop (ledger condition
// 4), a verdict belongs to one Stop. The list is cloned before the cut, so a
// Frontier copied before the Step keeps its own.
func verdict(f *Frontier, ev Event) {
	i := slices.Index(f.StopsWithoutVerdict, ev.CheckpointSeq)
	if i < 0 {
		return
	}
	f.StopsWithoutVerdict = slices.Delete(slices.Clone(f.StopsWithoutVerdict), i, i+1)
	f.Verdicts = append(f.Verdicts, VerdictRef{Seq: ev.CheckpointSeq, DoneFlagged: ev.DoneFlagged, Unmet: ev.Unmet})
}

// ledger makes a ledger line's rows the frontier's obligations, as of the
// line's seq (ObligationsAt), with its two cut counts: the last ledger line
// is the session's ledger, and the fold copies it, never computing a row
// (D3). A row is keyed by its id alone, one ref per id with the first kept,
// since canon orders the rows by id. State is copied as the line wrote it,
// so open, met, unmet and unknown stay four states and unknown never reads
// unmet (INV-2); scope is opaque and not read. A record that is no ledger
// line never reaches here, so it leaves the obligations as they were
// (ledger condition 4).
func ledger(f *Frontier, ev Event) {
	obs := make([]ObligationRef, 0, len(ev.Obligations))
	seen := make(map[string]bool, len(ev.Obligations))
	for _, o := range ev.Obligations {
		if seen[o.ID] {
			continue
		}
		seen[o.ID] = true
		obs = append(obs, o)
	}
	f.Obligations = obs
	f.ObligationsAt = ev.Seq
	f.ObligationsOmitted = ev.ObligationsOmitted
	f.UnitRowsCut = ev.UnitRowsCut
}

// touch adds the file a call wrote to Touched as its graph.FileID, kept
// sorted and unique at every step, so Touched is a set: the same writes in
// any order fold to the same list (D1: files, not units; see Frontier). An
// insert builds a new list, so a copy taken before keeps its own.
func touch(f *Frontier, rel string) {
	id := string(graph.FileID(rel))
	i := sort.SearchStrings(f.Touched, id)
	if i < len(f.Touched) && f.Touched[i] == id {
		return
	}
	f.Touched = slices.Concat(f.Touched[:i], []string{id}, f.Touched[i:])
}

// require states a Req on a delivered line's subject at the line's
// after_seq (D6): one ReqState per key pred:arg, the first kept, so a line
// delivered again, or a second line naming the subject, leaves StatedAt as
// it was. The key is not the line's: one steer line's slots hold up to five
// graph lines (Frontier).
func require(f *Frontier, pred, arg string, seq int64) {
	id := pred + ":" + arg
	for _, r := range f.Reqs {
		if r.ID == id {
			return
		}
	}
	f.Reqs = append(f.Reqs, ReqState{ID: id, Pred: pred, Arg: arg, Status: ReqOpen, StatedAt: seq})
}

// openedUnit marks met_once each open opened_unit Req whose unit a write of ev
// is in, the path itself or under it (PLAN 1489: a Touch under the unit),
// at ev's seq: the first such write, and met_once never goes back. A Req exists
// only after its line, by the merged order (at one seq a record sorts
// before a line), so a line is judged by the writes after it, as
// steer.Compliance judges a line followed, and a write before it meets
// nothing. Both sides are paths as the record holds them: the slot path
// relative to the hook's working directory when the line was written
// (relTo, cmd/rashomon/graph.go), and a written path relative to the
// project (inProject). They agree when the hook ran at the project root, as
// it did for 4205 of the 4280 fixture declarations that name both; the
// steer line names no cwd, so a line written from a subdirectory is read
// as if from the root, and a write to the same path there meets its Req (a
// lookalike the record cannot tell apart). A path that climbs, ".", or a
// keyed digest
// (record scope v4) matches no written path, so its Req stays open: open is
// "not seen met", never "unmet" (ruling 12:52). The list is cloned before
// its first change, so a copy taken before keeps its own.
func openedUnit(f *Frontier, ev Event) {
	cloned := false
	for i, r := range f.Reqs {
		if r.Pred != steer.PredOpenedUnit || r.Status != ReqOpen || !wroteUnder(ev.Wrote, r.Arg) {
			continue
		}
		if !cloned {
			f.Reqs, cloned = slices.Clone(f.Reqs), true
		}
		f.Reqs[i].Status, f.Reqs[i].MetAt = ReqMetOnce, ev.Seq
	}
}

// wroteUnder reports whether a path in wrote is dir or lies under it.
func wroteUnder(wrote []string, dir string) bool {
	return slices.ContainsFunc(wrote, func(p string) bool { return p == dir || strings.HasPrefix(p, dir+"/") })
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
