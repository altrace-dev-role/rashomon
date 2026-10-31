package verify

import (
	"errors"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// SteerView is what the steer step reads from VERIFY at one call (card 2.3
// R1): the candidates, each with its rendered Message (the zero Message when
// the render was refused, which Route logs as render_refused); the project
// root the record holds, "" where it holds none (record scope v4); how many
// facts the mapper refused, a contract break between producer and mapper that
// is counted, never passed on; and the spec judged against, the snapshot's
// (card 3.1 (c)).
type SteerView struct {
	Findings    []steer.Finding
	ProjectRoot string
	Dropped     int
	Spec        *SpecInfo
}

// SteerAt evaluates run at pt once and returns the steer view of the call
// whose tool_use_id is toolUseID, raised at event. The id is the capped form
// both records store (store.CapID), so a call whose payload id is over the
// cap still matches its own run. Evaluate and EvaluateWithObligations are
// untouched: this is a third view of the same classification. key is the
// install key the gates sp declares are digested under (evaluate). Its facts
// are regression-location's, stale-evidence's and the one-way doors this
// call opened (integrityRaises, card P5B). stale-evidence is raised at a
// PostToolUse only, under an index that has established a family (D164):
// the call's declaration seq, which its writes carry, is looked up only
// then.
func SteerAt(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, key []byte, pt Point, now time.Time, event, toolUseID string) SteerView {
	var c classified
	snap := evaluate(run, idx, sp, key, pt, now, &c)
	facts := regressionLocations(snap, c.runs, c.writes, pt, event, toolUseID)
	var decl int64
	if c.learned && event == string(steer.EventPostToolUse) {
		decl = declSeqAt(run, toolUseID, pt.Seq)
	}
	facts = append(facts, staleEvidence(snap, c.runs, c.writes, decl, pt, event)...)
	facts = append(facts, integrityRaises(run, snap, c.runs, event, toolUseID)...)
	found, dropped := mapFacts(run, facts, pt, decl)
	return SteerView{Findings: found, ProjectRoot: snap.ProjectRoot, Dropped: dropped, Spec: snap.Spec}
}

// mapFacts maps each fact to its steer candidate through ToSteer and renders
// its Message from the record: an integrity fact by renderIntegrity, a
// stale-evidence fact by renderStale against this call's declaration at
// decl, and any other gate fact by renderRegression. A fact ToSteer refuses
// is counted in dropped.
func mapFacts(run *store.Run, facts []Finding, pt Point, decl int64) (found []steer.Finding, dropped int) {
	if len(facts) == 0 {
		return nil, 0
	}
	agent := ""
	if pt.AgentID != nil {
		agent = *pt.AgentID
	}
	times := declTimes{run: run}
	for _, f := range facts {
		sf, err := ToSteer(f, pt, agent)
		if err != nil {
			dropped++
			continue
		}
		switch {
		case f.Integrity != nil:
			sf.Message = renderIntegrity(f)
		case f.Pattern == pattern.StaleEvidence:
			sf.Message = renderStale(&times, f, decl)
		default:
			sf.Message = renderRegression(run, &times, f)
		}
		found = append(found, sf)
	}
	return found, dropped
}

// regressionLocations is regression-location's producer (card 2.3 R6): for
// each gate, in snapshot order, whose last run is this call, red by its exit
// code (not 126 or 127: the gate did not run) after an earlier green, the
// relevant writes strictly between the two runs. Strict bounds, because a
// run line's own redirect shares the run's seq. A red with nothing written
// between is not a located regression. writes are classify's, in seq order.
func regressionLocations(snap Snapshot, runs []gateRun, writes []write, pt Point, event, toolUseID string) []Finding {
	var out []Finding
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if !regressedHere(g, toolUseID) {
			continue
		}
		green, last := g.LastGreen.Seq, g.LastRun.Seq
		fact := GateFact{GateID: SlotID(g.Gate), State: g.State, Reason: g.Reason,
			LastGreenSeq: &green, LastRunSeq: &last, PartialRuns: g.PartialRunsSince,
			OutcomeFrom: OutcomeFrom(g.LastRun.OutcomeFrom.String())}
		if g.LastRun.ExitCode != nil && g.LastRun.OutcomeFrom == SourceExit {
			exit := *g.LastRun.ExitCode
			fact.ExitCode = &exit
		}
		if p := Program(programOf(runs, last)); p.Valid() {
			fact.Program = p
		}
		between(&fact, writes, pt.AgentID)
		if fact.WritesSince == 0 {
			continue
		}
		prompt := ""
		if pt.PromptID != nil {
			prompt = *pt.PromptID
		}
		out = append(out, Finding{Pattern: pattern.RegressionLocation, Class: Absence, Event: event,
			DedupeKey: string(pattern.RegressionLocation) + "|" + g.Gate + "|" + prompt, Gate: &fact})
	}
	return out
}

// regressedHere is the candidate condition: the gate is red (not with a write
// declared after the red, D67), it was green earlier, and its last run is
// this call, and the red is one of two kinds. By a failed exit, with an exit
// code other than 126 and 127 -- not by an output line alone (D61). Or by the
// run's output, where the run's outcome was read from it because a pipe hid
// the exit status (outcome_from output_summary, D229): readsOutput let in
// only summary_fail, or a fail line for a runner whose table has no summary
// form, so a FAIL substring a pipe happened to keep is never one. The run's
// exit status is then the pipe's own and no code is named.
func regressedHere(g *GateState, toolUseID string) bool {
	if g.State != Red || g.LastGreen == nil || g.LastRun == nil || g.LastRun.ToolUseID != toolUseID {
		return false
	}
	switch g.Reason {
	case FailedExit:
		if g.LastRun.ExitCode == nil {
			return false
		}
		code := *g.LastRun.ExitCode
		return code != 126 && code != 127
	case FailedOutput:
		return g.LastRun.OutcomeFrom == SourceOutput
	}
	return false
}

// staleEvidence is stale-evidence's producer (card STALE-NUDGE1): for each
// gate, in snapshot order, that staleHere names, the fact of its green run
// and of this call's writes after it, declared at decl. decl is 0 where the
// call raises none: not a PostToolUse, no established family, or no
// declaration of the call in the record.
func staleEvidence(snap Snapshot, runs []gateRun, writes []write, decl int64, pt Point, event string) []Finding {
	if decl == 0 {
		return nil
	}
	var out []Finding
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if !staleHere(g, writes, decl) {
			continue
		}
		green := g.LastGreen.Seq
		fact := GateFact{GateID: SlotID(g.Gate), State: g.State, Reason: g.Reason,
			LastGreenSeq: &green, LastRunSeq: &green, PartialRuns: g.PartialRunsSince,
			OutcomeFrom: OutcomeFrom(g.LastGreen.OutcomeFrom.String())}
		if p := Program(programOf(runs, green)); p.Valid() {
			fact.Program = p
		}
		own(&fact, writes, decl)
		prompt := ""
		if pt.PromptID != nil {
			prompt = *pt.PromptID
		}
		out = append(out, Finding{Pattern: pattern.StaleEvidence, Class: Absence, Event: event,
			DedupeKey: string(pattern.StaleEvidence) + "|" + g.Gate + "|" + prompt, Gate: &fact})
	}
	return out
}

// staleHere is the candidate condition: a required gate, stale by writes
// after its green, whose first counted write after that green is this
// call's, declared at decl. The first, not the last (LastWrite): the call
// that left the green behind raises it, and the writes after it are quiet.
// A write carries its declaration's seq (appendWrites), never the point's,
// which is the execution's, so the call is matched by its declaration. A
// moved tree is no reason here: a per-call point carries no Tree.
func staleHere(g *GateState, writes []write, decl int64) bool {
	if !g.Required || g.State != Stale || g.Reason != WritesAfterGreen || g.LastGreen == nil {
		return false
	}
	lo := g.LastGreen.Seq
	for i := sort.Search(len(writes), func(i int) bool { return writes[i].Seq > lo }); i < len(writes); i++ {
		if w := &writes[i]; w.Path != nil && Relevant(w.Class) {
			return w.Seq == decl
		}
	}
	return false
}

// declSeqAt is the seq of the call's declaration, the last one at or before
// at whose id is toolUseID (both records store the capped id), else 0.
func declSeqAt(run *store.Run, toolUseID string, at int64) int64 {
	var seq int64
	for i := range run.Declarations {
		if d := &run.Declarations[i]; d.ToolUseID == toolUseID && d.Seq <= at && d.Seq > seq {
			seq = d.Seq
		}
	}
	return seq
}

// own fills f's write counts from this call's relevant writes, those
// declared at decl: the distinct named paths, first first, at most MaxPaths,
// and PathsOmitted the rest or those outside the slot's charset. A write
// another call declared beside this one, by this agent or another, is not
// this call's and is never named in its sentence (INV-2).
func own(f *GateFact, writes []write, decl int64) {
	seen := map[PathSlot]bool{}
	for i := sort.Search(len(writes), func(i int) bool { return writes[i].Seq >= decl }); i < len(writes) && writes[i].Seq == decl; i++ {
		w := &writes[i]
		if w.Path == nil || !Relevant(w.Class) {
			continue
		}
		f.WritesSince++
		p := PathSlot(*w.Path)
		if seen[p] {
			continue
		}
		seen[p] = true
		if p.Valid() && len(f.Paths) < MaxPaths {
			f.Paths = append(f.Paths, p)
		} else {
			f.PathsOmitted++
		}
	}
}

// between fills f's write counts from the relevant writes strictly between
// its two runs: a binary search for the first past the green, a scan to the
// red. Paths are the distinct named paths, first write first, at most
// MaxPaths; PathsOmitted the distinct ones past that or outside the slot's
// charset; OtherAgents the writes by an agent other than the point's.
func between(f *GateFact, writes []write, agent *string) {
	lo, hi := *f.LastGreenSeq, *f.LastRunSeq
	seen := map[PathSlot]bool{}
	for i := sort.Search(len(writes), func(i int) bool { return writes[i].Seq > lo }); i < len(writes) && writes[i].Seq < hi; i++ {
		w := &writes[i]
		if !Relevant(w.Class) {
			continue
		}
		f.WritesSince++
		if !sameAgent(w.AgentID, agent) {
			f.OtherAgents++
		}
		if w.Path == nil {
			continue
		}
		p := PathSlot(*w.Path)
		if seen[p] {
			continue
		}
		seen[p] = true
		if p.Valid() && len(f.Paths) < MaxPaths {
			f.Paths = append(f.Paths, p)
		} else {
			f.PathsOmitted++
		}
	}
}

// programOf is the program of the run declared at seq: the shape's base
// name of argv[0], "go" for `go test ./...`.
func programOf(runs []gateRun, seq int64) string {
	for i := range runs {
		if runs[i].Seq == seq {
			return runs[i].Program
		}
	}
	return ""
}

// declTimes maps a declaration's seq to its recorded time, built from the
// run once, on the first finding that needs it.
type declTimes struct {
	run *store.Run
	at  map[int64]int64
}

func (d *declTimes) of(seq int64) (int64, bool) {
	if d.at == nil {
		d.at = make(map[int64]int64, len(d.run.Declarations))
		for _, x := range d.run.Declarations {
			d.at[x.Seq] = x.RecordedAtMS
		}
	}
	ms, ok := d.at[seq]
	return ms, ok && ms > 0
}

// renderRegression is the finding's note (card 2.3 R7), or the zero Message
// when the record cannot back one: a run with no recorded time or no exit
// code, or a span or an exit outside its slot's range (an honest refusal,
// never a clamped number). The full row lists the named paths, with "and k
// more" counting the named paths it does not list and the writes that name
// no path (D85, the slot's own count). When there are no paths to list, or
// the renderer left every one out, the degraded row states the count of
// writes instead.
func renderRegression(run *store.Run, times *declTimes, f Finding) render.Message {
	g := f.Gate
	green, okGreen := times.of(*g.LastGreenSeq)
	red, okRed := times.of(*g.LastRunSeq)
	fromOutput := g.OutcomeFrom == OutcomeFromOutput
	if !okGreen || !okRed || g.ExitCode == nil && !fromOutput {
		return render.Message{}
	}
	minutes := slot.Minutes(max(0, (red-green)/60_000))
	word := slot.Gate(g.Program)
	if g.Program == "" {
		word = slot.Gate(kindOfGateID(string(g.GateID)))
	}
	var exit slot.Exit
	if g.ExitCode != nil {
		exit = slot.Exit(*g.ExitCode)
	}
	if len(g.Paths) > 0 {
		paths := make([]slot.Path, len(g.Paths))
		for i, p := range g.Paths {
			paths[i] = slot.Path(p)
		}
		unlisted := g.PathsOmitted + unnamedBetween(run.Declarations, *g.LastGreenSeq, *g.LastRunSeq)
		var m render.Message
		var err error
		if fromOutput {
			// The run's outcome is its output's: the note says so (D229).
			m, err = render.New(pattern.RegressionLocation, render.RegressionLocationOutputBinding{
				Gate: word, Minutes: minutes, Paths: slot.NewPathList(paths, unlisted)})
		} else {
			m, err = render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
				Gate: word, Minutes: minutes, Exit: exit, Paths: slot.NewPathList(paths, unlisted)})
		}
		if !errors.Is(err, slot.ErrNoPathsLeft) {
			return m
		}
	}
	if fromOutput {
		m, _ := render.New(pattern.RegressionLocation, render.RegressionLocationOutputDegradedBinding{
			Gate: word, Minutes: minutes, Count: slot.Count(g.WritesSince)})
		return m
	}
	m, _ := render.New(pattern.RegressionLocation, render.RegressionLocationDegradedBinding{
		Gate: word, Minutes: minutes, Exit: exit, Count: slot.Count(g.WritesSince)})
	return m
}

// renderStale is stale-evidence's note, or the zero Message when the record
// cannot back one: the green run or this call, declared at decl, with no
// recorded time, or a span outside its slot's range. The full row lists this
// call's named paths; when the renderer left every one out, the degraded row
// counts them.
func renderStale(times *declTimes, f Finding, decl int64) render.Message {
	g := f.Gate
	green, okGreen := times.of(*g.LastGreenSeq)
	wrote, okWrote := times.of(decl)
	if !okGreen || !okWrote {
		return render.Message{}
	}
	minutes := slot.Minutes(max(0, (wrote-green)/60_000))
	word := slot.Gate(g.Program)
	if g.Program == "" {
		word = slot.Gate(kindOfGateID(string(g.GateID)))
	}
	fromOutput := g.OutcomeFrom == OutcomeFromOutput
	if len(g.Paths) > 0 {
		paths := make([]slot.Path, len(g.Paths))
		for i, p := range g.Paths {
			paths[i] = slot.Path(p)
		}
		var m render.Message
		var err error
		if fromOutput {
			// The green is the run's output's: the note says so (D229).
			m, err = render.New(pattern.StaleEvidence, render.StaleEvidenceOutputBinding{
				Gate: word, Minutes: minutes, Paths: slot.NewPathList(paths, g.PathsOmitted)})
		} else {
			m, err = render.New(pattern.StaleEvidence, render.StaleEvidenceBinding{
				Gate: word, Minutes: minutes, Paths: slot.NewPathList(paths, g.PathsOmitted)})
		}
		if !errors.Is(err, slot.ErrNoPathsLeft) {
			return m
		}
	}
	if fromOutput {
		m, _ := render.New(pattern.StaleEvidence, render.StaleEvidenceOutputDegradedBinding{
			Gate: word, Minutes: minutes, Count: slot.Count(g.WritesSince)})
		return m
	}
	m, _ := render.New(pattern.StaleEvidence, render.StaleEvidenceDegradedBinding{
		Gate: word, Minutes: minutes, Count: slot.Count(g.WritesSince)})
	return m
}

// unnamedBetween counts the writes declared strictly between two seqs that
// name no path, read through appendWrites, the classifier's own reading of a
// declaration. GateFact's fields are 2.2.9's (TestVER54c) and PathsOmitted
// counts named paths only, so the rest of "and k more" comes from the
// record. A write with no path has no class, which is relevant.
func unnamedBetween(decls []store.Declaration, lo, hi int64) int {
	n := 0
	var ws []write
	for i := range decls {
		if decls[i].Seq <= lo || decls[i].Seq >= hi {
			continue
		}
		ws, _ = appendWrites(ws[:0], nil, &decls[i])
		for _, w := range ws {
			if w.Path == nil {
				n++
			}
		}
	}
	return n
}
