package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The unit-brief file (W-M2.3 hook half, card m2-unitbrief-hook1 D2, D3).
// The prompt hook writes each unit's brief, as UnitBrief bound it, to
// graph.UnitBriefFileName in the session's unit-brief directory; the post
// hook of the first edit in that unit reads it back and renders it through
// engine.Rebind, the same bind and deliver that rendered it. The file holds
// the bound facts and the counts bind left, never rendered text, since text
// cannot become a Message again; every string in it is one bind already
// screened and cut, so it holds no free text and needs no _cut marker.
const (
	unitBriefVersion = 1
	// maxUnitBriefBytes caps the file: the writer refuses a brief that
	// encodes over it and the reader reads at most one byte more.
	// TestUnitBrief_MaxShapedBriefIsUnderTheCap pins the largest brief
	// deliver keeps under it.
	maxUnitBriefBytes = 8 << 10
)

// unitBriefFile is the file's closed shape at version 1. Rev is a commit:
// graphBrief reads the index at its commit, never at an overlay. Prompt is
// the prompt id the brief was written under, never the prompt's words:
// UnitBrief takes none.
type unitBriefFile struct {
	V      int             `json:"v"`
	Unit   graph.ID        `json:"unit"`
	First  graph.ID        `json:"first"`
	Tier   graph.Tier      `json:"tier"`
	Rev    string          `json:"rev"`
	Prompt string          `json:"prompt"`
	Lines  []unitBriefLine `json:"lines"`
}

// unitBriefLine is one kept line: its key, its one fact under the field of
// its kind, and the counts bind left beyond the items it lists
// (slot.GraphLine.More). The field that is set is the line's kind, so no
// separate kind word can disagree with it.
type unitBriefLine struct {
	Key       graph.ID            `json:"key"`
	Code      *slot.CodeFact      `json:"code,omitempty"`
	Table     *slot.TableFact     `json:"table,omitempty"`
	Knowledge *slot.KnowledgeFact `json:"knowledge,omitempty"`
	Unit      *slot.UnitFact      `json:"unit,omitempty"`
	NotKnown  *slot.NotKnownFact  `json:"not_known,omitempty"`
	More      slot.Counts         `json:"more"`
}

// storedUnitBrief is a unit-brief file read back: the brief Rebind renders
// from its lines, and the unit, first row, tier and prompt id it was
// written under. The brief's Prompt and Tokens stay empty.
type storedUnitBrief struct {
	engine.Brief
	Unit, First graph.ID
	Tier        graph.Tier
	PromptID    string
}

// unitBriefLineOf is l's stored form.
func unitBriefLineOf(l engine.Line) unitBriefLine {
	out := unitBriefLine{Key: l.Key, More: l.Slot.More()}
	if f, ok := l.Slot.Code(); ok {
		out.Code = &f
	}
	if f, ok := l.Slot.Table(); ok {
		out.Table = &f
	}
	if f, ok := l.Slot.Knowledge(); ok {
		out.Knowledge = &f
	}
	if f, ok := l.Slot.Unit(); ok {
		out.Unit = &f
	}
	if f, ok := l.Slot.NotKnown(); ok {
		out.NotKnown = &f
	}
	return out
}

// slot is the line its fact builds, through the fact's own constructor,
// and how many facts the stored line holds; the line stands only when that
// is one. The counts are the caller's to restore.
func (l unitBriefLine) slot() (slot.GraphLine, int) {
	var out slot.GraphLine
	n := 0
	if l.Code != nil {
		out, n = slot.NewCodeLine(*l.Code), n+1
	}
	if l.Table != nil {
		out, n = slot.NewTableLine(*l.Table), n+1
	}
	if l.Knowledge != nil {
		out, n = slot.NewKnowledgeLine(*l.Knowledge), n+1
	}
	if l.Unit != nil {
		out, n = slot.NewUnitLine(*l.Unit), n+1
	}
	if l.NotKnown != nil {
		out, n = slot.NewNotKnownLine(*l.NotKnown), n+1
	}
	return out, n
}

// checkUnitBriefHeader is whether unit and first name the brief whose first
// keyed line is firstKey: both global ids of one repository, first that
// line's, and tier one of the graph's tiers. The finding the post hook
// builds names first as its source, so a header that names another row is
// refused rather than cited.
func checkUnitBriefHeader(unit, first graph.ID, tier graph.Tier, firstKey graph.ID) error {
	urid, _, ok := graph.SplitGlobal(unit)
	if !ok {
		return fmt.Errorf("unit brief: unit %q is not a global id", unit)
	}
	frid, local, ok := graph.SplitGlobal(first)
	if !ok || frid != urid {
		return fmt.Errorf("unit brief: first row %q is not of the unit's repository", first)
	}
	if local != firstKey {
		return fmt.Errorf("unit brief: first row %q is not the brief's first line %q", first, firstKey)
	}
	switch tier {
	case graph.Proven, graph.Algorithmic, graph.Declared, graph.Observed, graph.Inferred:
		return nil
	}
	return fmt.Errorf("unit brief: tier %q", tier)
}

// encodeUnitBrief is b's file: its kept lines (b.Lines[:b.Kept]) in order
// under the header. It refuses, with no bytes, what the reader could not
// render back as b: an empty brief; a brief at an overlay revision, which
// the reader would take for a commit; a brief with seeds or a
// neighborhood, which the template's binding carries and Rebind sets
// neither of (a prompt brief); a header that does not name b; a line with
// no fact; and a file over maxUnitBriefBytes.
func encodeUnitBrief(b engine.Brief, unit, first graph.ID, tier graph.Tier, promptID string) ([]byte, error) {
	switch {
	case b.Empty():
		return nil, errors.New("unit brief: empty")
	case b.Rev.Kind != graph.RevKindCommit:
		return nil, fmt.Errorf("unit brief: rev %s is not a commit", b.Rev)
	case len(b.Seeds) != 0 || b.Neighborhood != 0:
		return nil, errors.New("unit brief: a brief with seeds or a neighborhood")
	}
	if err := checkUnitBriefHeader(unit, first, tier, b.FirstKey()); err != nil {
		return nil, err
	}
	f := unitBriefFile{V: unitBriefVersion, Unit: unit, First: first, Tier: tier, Rev: b.Rev.Commit, Prompt: promptID}
	for i, l := range b.Lines[:b.Kept] {
		sl := unitBriefLineOf(l)
		if _, n := sl.slot(); n != 1 {
			return nil, fmt.Errorf("unit brief: line %d holds %d facts", i+1, n)
		}
		f.Lines = append(f.Lines, sl)
	}
	data, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	if len(data) > maxUnitBriefBytes {
		return nil, fmt.Errorf("unit brief: %d B, over the cap %d", len(data), maxUnitBriefBytes)
	}
	return data, nil
}

// decodeUnitBrief is the brief data holds, rendered by engine.Rebind. A
// file over the cap, of another version, with a field the shape does not
// name, with anything after its one object, with no line, with a line of
// no fact or two, with a line that bind refuses, changes, or that deliver
// no longer renders, or with a header that does not name the brief, is an
// error: a miss, never a different brief. It is RES-5's named exception for this file (res5Allowed):
// rashomon's own file under the store root, capped, in a closed shape.
func decodeUnitBrief(data []byte) (storedUnitBrief, error) {
	if len(data) > maxUnitBriefBytes {
		return storedUnitBrief{}, fmt.Errorf("unit brief: %d B, over the cap %d", len(data), maxUnitBriefBytes)
	}
	var f unitBriefFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return storedUnitBrief{}, fmt.Errorf("unit brief: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return storedUnitBrief{}, errors.New("unit brief: data after the file's object")
	}
	if f.V != unitBriefVersion {
		return storedUnitBrief{}, fmt.Errorf("unit brief: version %d", f.V)
	}
	if len(f.Lines) == 0 {
		return storedUnitBrief{}, errors.New("unit brief: no line")
	}
	lines := make([]engine.Line, 0, len(f.Lines))
	for i, sl := range f.Lines {
		s, n := sl.slot()
		if n != 1 {
			return storedUnitBrief{}, fmt.Errorf("unit brief: line %d holds %d facts", i+1, n)
		}
		lines = append(lines, engine.Line{Key: sl.Key, Slot: s.WithMore(sl.More)})
	}
	b := engine.Rebind(graph.RevCommit(f.Rev), lines)
	if b.Kept != len(f.Lines) {
		return storedUnitBrief{}, fmt.Errorf("unit brief: %d of %d lines render", b.Kept, len(f.Lines))
	}
	// A stored line is bind's own output, and bind is idempotent on it, so
	// it rebinds to itself. bind keeps what passes a check and counts or
	// blanks the rest rather than refuse the line, so a line it changed was
	// not one it wrote: a file another writer made, or one a tightened
	// screen now reads differently.
	for i, l := range b.Lines {
		if !reflect.DeepEqual(unitBriefLineOf(l), f.Lines[i]) {
			return storedUnitBrief{}, fmt.Errorf("unit brief: line %d is not as bind leaves it", i+1)
		}
	}
	if err := checkUnitBriefHeader(f.Unit, f.First, f.Tier, b.FirstKey()); err != nil {
		return storedUnitBrief{}, err
	}
	return storedUnitBrief{Brief: b, Unit: f.Unit, First: f.First, Tier: f.Tier, PromptID: f.Prompt}, nil
}

// readUnitBrief is the file at path decoded, reading at most one byte over
// the cap, so an oversized file is refused without being read whole. A
// missing, unreadable, over-cap, torn or stale file is an error the caller
// counts as a miss.
func readUnitBrief(path string) (storedUnitBrief, error) {
	f, err := os.Open(path)
	if err != nil {
		return storedUnitBrief{}, err
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxUnitBriefBytes+1))
	if err != nil {
		return storedUnitBrief{}, err
	}
	return decodeUnitBrief(data)
}

// writeUnitBrief writes data to path, 0600, through a temporary file in
// path's directory renamed over it, making the directories 0700, so a
// reader finds the whole file or the one before it. It does not sync,
// unlike writeShareFile: the file is a cache whose torn or lost form the
// reader refuses as a miss, so the prompt hook writing up to eight of them
// pays no fsync for a durability nothing reads.
func writeUnitBrief(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, graph.UnitBriefTempPattern)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name()) //nolint:errcheck // best effort cleanup
	}
	return err
}

// maxUnitBriefs caps the unit briefs one prompt stores (D1).
const maxUnitBriefs = 8

// briefUnits is the units b's keyed lines lie in (unitOf), each once, in
// line order, at most maxUnitBriefs. Every line counts, kept or cut at the
// brief's cap, since each was selected for the prompt; a keyless line (the
// not-known line) names no unit.
func briefUnits(s *graph.Store, rev graph.Rev, b engine.Brief) []graph.ID {
	var units []graph.ID
	for _, l := range b.Lines {
		if len(units) == maxUnitBriefs {
			break
		}
		if l.Key == "" {
			continue
		}
		if u := unitOf(s, rev, l.Key); u != "" && !slices.Contains(units, u) {
			units = append(units, u)
		}
	}
	return units
}

// writeUnitBriefs stores, for each unit of the prompt brief b (briefUnits),
// the brief engine.UnitBrief gives the first write in it, for the post hook
// of that write to read (D1, D4): under the session's unit-brief directory
// by graph.UnitBriefFileName of the index root and the unit. Only when the
// store holds the session-content marker, the post read's own gate, so an
// unconsented prompt pays one marker read and no UnitBrief. An empty unit
// brief writes no file, and a later prompt's brief for a unit replaces the
// file. A write that fails is left: the post counts the absent file as a
// miss.
func writeUnitBriefs(storeRoot, sessionID, promptID, root, repo string, s *graph.Store, rev graph.Rev, b engine.Brief, opt engine.Options) {
	if storeRoot == "" || !store.SessionContentMarker(storeRoot) {
		return
	}
	dir := graph.UnitBriefDir(storeRoot, sessionID)
	for _, u := range briefUnits(s, rev, b) {
		ub := engine.UnitBrief(s, rev, u, b, opt)
		first := ub.FirstKey()
		if ub.Empty() || first == "" {
			continue
		}
		n, _ := s.Node(first, rev)
		data, err := encodeUnitBrief(ub, graph.GlobalID(repo, u), graph.GlobalID(repo, first), n.Tier, promptID)
		if err != nil {
			continue
		}
		_ = writeUnitBrief(filepath.Join(dir, graph.UnitBriefFileName(root, u)), data)
	}
}
