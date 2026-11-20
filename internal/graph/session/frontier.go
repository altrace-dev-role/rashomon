package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// FrontierSchema is the frontier's own schema, written first as "schema".
const FrontierSchema = 1

// foldVersion is the version of the rules Step applies, written as
// "fold_version" in every frontier it folds: with ledgerVersion and Through it
// is a saved frontier's key, and a frontier of another version is a miss
// (ParseFrontier, Catch), folded again in full and never continued. A change
// to what Step writes for any input bumps it, and
// TestFold_GoldenTiesFoldVersionSynthetic fails on a change that does not:
// its golden is the frontier its events fold to under this version. The
// limit is the golden's inputs: a Step change no golden input exercises
// passes unbumped.
const foldVersion = 1

// ledgerVersion is the version of the ledger line the fold reads (ledgerType;
// RecordEvent's ledger keys, ledger, verdict), written as "ledger_version":
// it moves with what an obligation id names (verify's ObligationID), since a
// change there makes the ids a saved frontier holds another ledger's. A
// ledger row's req key needs no bump: the ledger line carries it from its
// first version.
const ledgerVersion = 1

// Mark is an event's place in the merged order (Event.Before): its seq,
// whether it is a steer line, and a line's clock and id. Frontier.Through
// holds the last applied event's. The zero Mark is an empty fold's and sorts
// before every event a log holds: record seqs start at 1, and a record sorts
// before a line at its seq, so only a line at after_seq 0 shares its seq, and
// a line sorts after the zero Mark's record place.
type Mark struct {
	Seq      int64  `json:"seq"`
	Steer    bool   `json:"steer"`
	AtUnixMS int64  `json:"at_unix_ms"`
	ID       string `json:"id"`
}

// ReqStatus is where a requirement stands in the session's history. open
// goes to met_once at evidence after StatedAt, or to superseded when a later
// prompt's delivered line moves past it (supersede). superseded goes to
// met_once at evidence as open does, or back to open only when a delivered
// line states the same Pred and Arg again, never by evidence, a prompt alone
// or time (require). met_once is final. dropped is written only by an
// explicit drop line, which v1 has no writer for. met_once is history and
// named for it: satisfied at least once after StatedAt, MetAt the first such
// seq, so a later red run or a removed call never undoes it. Whether a
// requirement holds now is the Stop's verdict (its ledger row, bound by
// checkpoint_seq as VerdictRef is), which the fold copies and never
// computes; a reader never renders met_once as met, holds or verified.
// superseded is not met: a list or count of unfinished requirements
// includes it, marked.
type ReqStatus string

const (
	ReqOpen       ReqStatus = "open"
	ReqMetOnce    ReqStatus = "met_once"
	ReqDropped    ReqStatus = "dropped"
	ReqSuperseded ReqStatus = "superseded"
)

// Frontier is a session's state folded from its record and steer log: a
// pure function of them, so deleting a saved one changes no answer, only its
// cost. Every string in it is an id, a digest or a closed name, never prompt
// or file words (TestFrontier_WordsByReference). It holds no map, so its
// JSON is its canonical order (MarshalFrontier).
//
// What it holds and leaves out, each for a reason: no Repo and no Commit,
// because no record line carries a repo id or the session's start commit,
// and a field the record cannot give would make the frontier more than a
// function of its logs; no Drift yet, which a later fold_version adds;
// ObligationsAt, because obligations carry over from one Stop to the next
// and a reader must know which ledger line they are from; ObligationsOmitted
// and UnitRowsCut, the rows the ledger line cut, so a short list is never
// read as a complete one; Touched holds files, not units, for the same
// reason (its doc); TouchedWithheld and TouchedOutside, the writes Touched
// cannot place in the project, counted for the same reason;
// StopsWithoutVerdict, the Stop checkpoints no verdict names; VerdictRef's
// DoneFlagged and Unmet, named for what the writer decides (VerdictRef's
// doc); a ReqState keyed Pred:Arg, not by its line's key, because one steer
// line's slots hold up to five graph lines; and ReqState's LastStatedAt,
// which supersede reads: a Catch from a saved frontier must know it, and no
// other field holds it. FoldVersion, LedgerVersion and Through are the saved
// frontier's key: the versions live here and nowhere else, the file's
// envelope holding only its schema and cursors, and Through is the last
// applied event's place in the merged order, not a seq bound, because a
// steer line's after_seq can name a seq a later line has passed.
type Frontier struct {
	Schema int `json:"schema"`
	// FoldVersion and LedgerVersion are foldVersion and ledgerVersion as the
	// frontier was folded, and Through is the place of the last event it
	// applied, Mark{} for none: the three are its key. A frontier is continued
	// only under its own versions and only by events after Through (Catch);
	// they follow Schema so a file opens with its validity key.
	FoldVersion   int    `json:"fold_version"`
	LedgerVersion int    `json:"ledger_version"`
	Through       Mark   `json:"through"`
	Session       string `json:"session"`
	Continues     string `json:"continues"`
	// Intents are the session's prompts by reference: graph.PromptNodeID
	// and the prompt digest.
	Intents []IntentRef `json:"intents"`
	// Reqs are the requirements the session's delivered lines stated, one
	// per key Pred:Arg, the first kept, StatedAt the stating line's
	// after_seq (require), each from one graph line (graphSlot.stated):
	// opened_unit, called_symbol and edited_or_verified on a path as the
	// line holds it, its keyed digest at record scope v4, and
	// paths_untouched on a decision id. The fold marks only opened_unit,
	// met_once at a later write under the unit (openedUnit); it never marks
	// the rest, and no Stop resolves them. A later prompt's line supersedes
	// an earlier Req of a predicate it states on another argument, and a
	// line stating a superseded Req again reopens it (supersede, require).
	// Open is "not seen met", never "not done" or "unmet": the record names
	// no read, and a write it withheld or placed outside the project
	// (TouchedWithheld, TouchedOutside) meets nothing. The ID is opaque: a
	// reader takes Pred and Arg from their fields and never splits the ID,
	// since an Arg may hold a colon.
	Reqs      []ReqState    `json:"reqs"`
	Decisions []DecisionRef `json:"decisions"`
	// Touched are file ids: the graph.FileID of each project-relative path
	// the session's calls named as written, sorted and unique, so one log
	// always folds to one list. Written only, never a read: the record names
	// a path only for a file-changing call (shape.DeclaredPath) or a shell
	// line's written_paths. Files, not units: a unit id needs the language's
	// resolver, and pylite's units come from the index, not the record, so a
	// unit here would make the frontier more than a function of its logs. The
	// unit form and the global form are the reader's, made at read time:
	// lang.Registry maps a file to its unit, and the repo id makes the global
	// id (graph.GlobalID).
	Touched []string `json:"touched"`
	// TouchedWithheld counts write declarations, not files: each declaration
	// whose write path the record withheld counts once (Event.WroteWithheld),
	// whatever the reason (no_consent below record scope v5, oversize,
	// switched_off). Below v5 the gate nulls a shell line's written_paths
	// whole and keys it no_consent only when the line named a path
	// (store.GateDeclaration), so a Bash line that named three paths counts
	// 1, and one that named none holds no key and counts 0. When it is above
	// zero Touched is not every file the calls named, and nothing built from
	// it may say every. Touched is never every file written in any case: a
	// shell line that may write names no path when its written_paths list is
	// incomplete (verify's unnamed writes).
	TouchedWithheld int `json:"touched_withheld"`
	// TouchedOutside counts write declarations, not files, that named a
	// path outside the project: still absolute after the project and cwd
	// prefixes are cut, climbing out, or on another drive (inProject). A
	// declaration with paths on both sides adds its inside ones to Touched
	// and counts once here. No symlink is resolved, so a path written
	// through an alias of the project (/tmp for /private/tmp) is outside: a
	// session launched by an alias has an empty Touched and every write
	// here. Both counts at zero still do not make an empty Touched "wrote
	// nothing": a shell line that may write names no path when its list is
	// incomplete, so a reader says at most "named no written file".
	TouchedOutside int `json:"touched_outside"`
	// Obligations are copied from the last ledger line, as of its seq
	// (ObligationsAt); the fold never computes one and never cuts one.
	// Each row's state is verify's as written (open, met, unmet, unknown):
	// unknown never reads unmet, which would claim a failure nobody saw.
	// ObligationsOmitted is the ledger line's own cut count, rows its writer
	// left off the line; UnitRowsCut is the unit rows verify never made past
	// its cap of 32 per Stop (verify.MaxTurnUnitRows), 0 when the line has
	// none. Two
	// counts, not one, since the causes differ. When either is above zero
	// the list is not every obligation, and nothing built from it may say
	// every.
	Obligations        []ObligationRef `json:"obligations"`
	ObligationsAt      int64           `json:"obligations_at"`
	ObligationsOmitted int             `json:"obligations_omitted"`
	UnitRowsCut        int             `json:"unit_rows_cut"`
	Delivered          []LineRef       `json:"delivered"`
	Verdicts           []VerdictRef    `json:"verdicts"`
	// StopsWithoutVerdict are the seqs of the Stop checkpoints the record
	// holds that Verdicts has no entry for: no ledger line followed (a kill,
	// a lock timeout), or its line carried no verdict keys (an older line).
	// The Stop was judged; its verdict was not logged, and nothing may read
	// it as not done. A list, not a count: a ledger line binds
	// by seq to a checkpoint read before it, and a Catch continuing a saved
	// frontier must still know those checkpoints; the count is its length.
	// Each checkpoint is in Verdicts or here, never both, so len(Verdicts) +
	// len(StopsWithoutVerdict) is the record's Stop checkpoints. A Stop a
	// Catch reads between its two appends is here until its line lands.
	StopsWithoutVerdict []int64 `json:"stops_without_verdict"`
	// LastBoundary is the seq of the last session record whose SessionStart
	// source is compact or resume, where the session's context was rebuilt;
	// 0 for none. Startup is no boundary, and neither is clear: clear starts
	// a new run, and that run's Continues carries the link.
	LastBoundary int64 `json:"last_boundary"`
}

// IntentRef is one prompt by reference. Its Status is open and only open in
// v1: no rule closes, meets or supersedes an intent.
type IntentRef struct {
	ID       string    `json:"id"`
	Digest   string    `json:"digest"`
	StatedAt int64     `json:"stated_at"`
	Status   ReqStatus `json:"status"`
}

// ReqState is a requirement by its predicate's name and argument, so a
// frontier is printed, compared and re-delivered without the store.
// StatedAt is the after_seq of the first delivered line that named it, kept
// for good; LastStatedAt is the last such line's, which supersede reads;
// MetAt is the seq of the first evidence after StatedAt, 0 until met_once.
// SupersededBy is the IntentRef.ID of the prompt whose line superseded it,
// set exactly while Status is superseded: a line that states it again, or
// evidence that marks it met_once, empties it, and after a reopen the next
// supersede sets it anew.
type ReqState struct {
	ID           string    `json:"id"`
	Pred         string    `json:"pred"`
	Arg          string    `json:"arg"`
	Status       ReqStatus `json:"status"`
	StatedAt     int64     `json:"stated_at"`
	LastStatedAt int64     `json:"last_stated_at"`
	MetAt        int64     `json:"met_at"`
	SupersededBy string    `json:"superseded_by"`
}

// DecisionRef is a delivered decision line: the ids it applies to and what
// it asks of them.
type DecisionRef struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
}

// ObligationRef is one ledger row as the ledger line carried it.
type ObligationRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Unit  string `json:"unit"`
	State string `json:"state"`
}

// LineRef is a steer line the session was delivered.
type LineRef struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Unit string `json:"unit"`
	Seq  int64  `json:"seq"`
}

// VerdictRef is a Stop's verdict, by the seq of its checkpoint record: the
// verify record at point stop, whose point_seq, the last call record it
// judged, stays on that record. Both values are the ledger line's, copied,
// never computed: its writer evaluates each once.
// DoneFlagged is recap.DoneFlag's second return at that Stop, the decision
// the next prompt's note is given: true when the flag fired (a strict claim
// the record does not back, or a strict done claim with an expected kind
// unmet); false when it did not, which includes a Stop with no claim, so
// false is never read as done or verified. Unmet counts the Stop's ledger
// rows of scope session in state unmet, before any cut the line applies;
// open and unknown rows are not counted, since neither is a failure seen.
// The names say what the writer decided, not "done" and "owed": a false
// DoneFlagged is not done, and Unmet is not every row still owed.
type VerdictRef struct {
	Seq         int64 `json:"seq"`
	DoneFlagged bool  `json:"done_flagged"`
	Unmet       int   `json:"unmet"`
}

func newFrontier(sessionID string) Frontier {
	return Frontier{Schema: FrontierSchema, FoldVersion: foldVersion, LedgerVersion: ledgerVersion, Session: sessionID}
}

// canon returns f with every slice in its canonical order and every nil
// slice empty, so one state has one encoding. It copies; f is not changed.
// The order is total only while every sort key is unique, which the fold
// keeps: one ReqState per Pred:Arg, one VerdictRef per checkpoint seq, one
// LineRef per steer line id. Two elements equal on the key and different
// elsewhere would let two input orders encode differently.
func canon(f Frontier) Frontier {
	g := f
	g.Intents = append([]IntentRef{}, f.Intents...)
	sort.Slice(g.Intents, func(i, j int) bool {
		a, b := g.Intents[i], g.Intents[j]
		return a.StatedAt < b.StatedAt || a.StatedAt == b.StatedAt && a.ID < b.ID
	})
	g.Reqs = append([]ReqState{}, f.Reqs...)
	sort.Slice(g.Reqs, func(i, j int) bool {
		a, b := g.Reqs[i], g.Reqs[j]
		return a.StatedAt < b.StatedAt || a.StatedAt == b.StatedAt && a.ID < b.ID
	})
	g.Decisions = make([]DecisionRef, len(f.Decisions))
	for i, d := range f.Decisions {
		ids := append([]string{}, d.IDs...)
		sort.Strings(ids)
		g.Decisions[i] = DecisionRef{IDs: ids, Action: d.Action}
	}
	sort.Slice(g.Decisions, func(i, j int) bool {
		a, b := strings.Join(g.Decisions[i].IDs, "\x00"), strings.Join(g.Decisions[j].IDs, "\x00")
		return a < b || a == b && g.Decisions[i].Action < g.Decisions[j].Action
	})
	g.Touched = append([]string{}, f.Touched...)
	sort.Strings(g.Touched)
	g.Obligations = append([]ObligationRef{}, f.Obligations...)
	sort.Slice(g.Obligations, func(i, j int) bool { return g.Obligations[i].ID < g.Obligations[j].ID })
	g.Delivered = append([]LineRef{}, f.Delivered...)
	sort.Slice(g.Delivered, func(i, j int) bool {
		a, b := g.Delivered[i], g.Delivered[j]
		return a.Seq < b.Seq || a.Seq == b.Seq && a.ID < b.ID
	})
	g.Verdicts = append([]VerdictRef{}, f.Verdicts...)
	sort.Slice(g.Verdicts, func(i, j int) bool { return g.Verdicts[i].Seq < g.Verdicts[j].Seq })
	g.StopsWithoutVerdict = append([]int64{}, f.StopsWithoutVerdict...)
	sort.Slice(g.StopsWithoutVerdict, func(i, j int) bool { return g.StopsWithoutVerdict[i] < g.StopsWithoutVerdict[j] })
	return g
}

// MarshalFrontier encodes f in its canonical order: two orders of one state
// give the same bytes.
func MarshalFrontier(f Frontier) ([]byte, error) {
	return json.Marshal(canon(f))
}

// ParseFrontier decodes a frontier of FrontierSchema, foldVersion and
// ledgerVersion. Any other schema or version, an unknown field or trailing
// data is an error, which a cache reader takes as a miss.
func ParseFrontier(b []byte) (Frontier, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f Frontier
	if err := dec.Decode(&f); err != nil {
		return Frontier{}, err
	}
	// More would answer false at a stray } or ], which ends no container
	// here; Token reports io.EOF only when nothing but space is left.
	if _, err := dec.Token(); err != io.EOF {
		return Frontier{}, fmt.Errorf("frontier: data after the object")
	}
	if f.Schema != FrontierSchema {
		return Frontier{}, fmt.Errorf("frontier: schema %d, want %d", f.Schema, FrontierSchema)
	}
	if f.FoldVersion != foldVersion || f.LedgerVersion != ledgerVersion {
		return Frontier{}, fmt.Errorf("frontier: fold_version %d, ledger_version %d, want %d and %d", f.FoldVersion, f.LedgerVersion, foldVersion, ledgerVersion)
	}
	return canon(f), nil
}
