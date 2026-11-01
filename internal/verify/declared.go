package verify

import (
	"cmp"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The words a declared gate's row carries (snapshot.go: Family "declared",
// Source declared | ci_commands).
const (
	familyDeclared   = "declared"
	sourceDeclared   = "declared"
	sourceCICommands = "ci_commands"
)

// The gate an InferredGate makes (card GATE-INFER1): one closed id, so no
// word of the project's files reaches the record, and SourceInferred as its
// Source and its trust.
const (
	InferredGateID = "inferred:test"
	SourceInferred = "inferred"
)

// declaredGate is one gate a trusted or managed spec declares, as the
// evaluator matches it to runs (card DECLARED-GATES1). runners holds the
// runner keys of its argv and aliases matched at their runner (shape.RunnerKey:
// the runner's row name, so a run's arguments never unbind it, or a launcher's
// exact words; GATE-BIND1), words those argv's exact-words runner digests,
// which say whether a bound run was the gate's own words, and lines the
// whole-line digests of those matched exactly or naming no runner (G4, G5).
// location is the CWDDigest a run must carry: the project root joined with
// the gate's Dir, the root itself where Dir is empty (D158); "" where no root
// is known, which matches no run (G7).
type declaredGate struct {
	id, kind, source, trust string
	required                bool
	runners, words, lines   map[string]bool
	location                string
}

// declaredGates is sp's gates whose source is trusted or managed, each argv
// and alias digested as a Bash line under key, the install key the hook
// digested every run with. A nil spec or an empty key declares nothing: no
// line digests as the hook's did. A gate whose Source is outside Sources (a
// hand-built Compiled) is untrusted (G1). Cost: one Derive per argv and alias.
func declaredGates(sp *spec.Compiled, key []byte, root string) []declaredGate {
	if sp == nil || len(key) == 0 {
		return nil
	}
	var out []declaredGate
	for _, d := range sp.Gates {
		if d.Source < 0 || d.Source >= len(sp.Sources) {
			continue
		}
		tr := sp.Sources[d.Source].Trust
		if tr != spec.Trusted && tr != spec.Managed {
			continue
		}
		g := declaredGate{id: d.ID, kind: d.Gate.Kind, source: sourceDeclared, trust: string(tr),
			required: d.Gate.Required == nil || *d.Gate.Required, runners: map[string]bool{}, words: map[string]bool{}, lines: map[string]bool{}}
		if strings.HasPrefix(d.ID, "ci:") {
			g.source = sourceCICommands
		}
		if root != "" {
			g.location = shape.CWDDigest(key, filepath.Join(root, d.Gate.Dir))
		}
		for _, argv := range append([][]string{d.Gate.Argv}, d.Gate.Aliases...) {
			if len(argv) == 0 {
				continue
			}
			s := lineShape(argv, key)
			if k, ok := shape.RunnerKey(nameOf(s.RunnerName), s.RunnerDigest); d.Gate.Match != "exact" && ok {
				g.runners[k] = true
				g.words[*s.RunnerDigest] = true
			} else {
				g.lines[s.Digest] = true
			}
		}
		out = append(out, g)
	}
	return out
}

// DeclaresRequired reports whether sp declares a required gate from a trusted
// or managed source, as declaredGates reads one, with no digest built (D222).
func DeclaresRequired(sp *spec.Compiled) bool {
	if sp == nil {
		return false
	}
	for _, d := range sp.Gates {
		if d.Source < 0 || d.Source >= len(sp.Sources) {
			continue
		}
		if tr := sp.Sources[d.Source].Trust; (tr == spec.Trusted || tr == spec.Managed) && (d.Gate.Required == nil || *d.Gate.Required) {
			return true
		}
	}
	return false
}

// lineShape is the shape of the Bash line argv's words make, joined by one
// space, each quoted as a shell reads it back to the same word (shellWord).
func lineShape(argv []string, key []byte) shape.Shape {
	words := make([]string, len(argv))
	for i, w := range argv {
		words[i] = shellWord(w)
	}
	input, _ := json.Marshal(struct {
		Command string `json:"command"`
	}{strings.Join(words, " ")})
	return shape.Derive("Bash", input, key)
}

// shellWord is w as it stands where every byte is one no shell treats
// specially, else in single quotes, each quote in it closed, escaped and
// reopened. A run matches a quoted word only where its line quoted it the
// same way.
func shellWord(w string) string {
	plain := w != ""
	for i := 0; i < len(w) && plain; i++ {
		c := w[i]
		plain = 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("%+,-./:=@_", c) >= 0
	}
	if plain {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

// lineRuns is every Bash call with a picked execution, read as it ran
// (newRun), in record order: what a declared gate matches, a line no runner
// names as well as classify's gate runs (G4). A rewritten line ran from a
// location the record does not hold, as classify reads it, so no declared
// gate takes it.
func lineRuns(run *store.Run, picked map[string]*store.Execution) []lineRun {
	var out []lineRun
	for i := range run.Declarations {
		d := &run.Declarations[i]
		e := picked[d.ToolUseID]
		if e == nil || d.ToolName != "Bash" {
			continue
		}
		masked, runner, _, _ := asRan(d, e)
		g := newRun(d, e, masked, runner)
		if e.ExecutedDigest != "" && e.ExecutedDigest != d.Shape.Digest {
			g.Location = ""
		}
		out = append(out, lineRun{gateRun: g, digest: cmp.Or(e.ExecutedDigest, d.Shape.Digest)})
	}
	return out
}

// lineRun is a candidate run for a declared gate: the run, and its line's
// shape digest as it ran, the execution's where it saw a tool_input, else
// the declaration's, which a gate matched exactly reads. The digest lives
// here, built only where a trusted spec declares gates, not on gateRun,
// which every evaluation builds for every gate run (D209: a field there grew
// every run by a string and every runs slice with it, as D173d's raw did).
type lineRun struct {
	gateRun
	digest string
}

// matches: the run is where the gate binds it, never at another location,
// and its runner key (shape.RunnerKey, the same function the gate's argv went
// through) is one of the gate's runners, its runner's words are the gate's
// (the rule before GATE-BIND1, and the one a run recorded with no
// runner_name, before call schema 8, can meet), or its whole line is one of
// the gate's lines.
func (g *declaredGate) matches(r *lineRun) bool {
	if g.location == "" || r.Location != g.location {
		return false
	}
	k, ok := shape.RunnerKey(shape.RunnerNameOf(r.RunnerID), r.Runner)
	return ok && (g.runners[k] || g.words[*r.Runner]) || r.digest != "" && g.lines[r.digest]
}

// nameOf is a shape's RunnerName, "" for none.
func nameOf(name *string) string {
	if name == nil {
		return ""
	}
	return *name
}

// state is the gate's row from its matched runs at the point, in Seq order:
// the learned table's (stateOf), or Absent with none, under the gate's own
// words. Its last run is marked OtherArgs where that run was bound at its
// runner with words none of the gate's argv has, so no line says the gate
// passed of a run that was not the gate's own (GATE-BIND1, D-c).
func (g *declaredGate) state(fr []gateRun, writes []write, unnamed []int64, at int64, tree map[string]*string) GateState {
	var s GateState
	if len(fr) > 0 {
		s = stateOf(familyKey{runner: g.id}, g.kind, fr, writes, unnamed, at, tree)
	} else {
		s = absentState(writes, unnamed, at)
	}
	s.Gate, s.Kind, s.Family, s.Source, s.Trust, s.Required = g.id, g.kind, familyDeclared, g.source, g.trust, g.required
	if s.LastRun != nil {
		for i := range fr {
			if r := &fr[i]; r.Seq == s.LastRun.Seq && r.Runner != nil && len(g.words) > 0 && !g.words[*r.Runner] {
				s.LastRun.OtherArgs = true
			}
		}
	}
	return s
}

// absentState is a declared gate's row with no run at the point (G8):
// Absent, never_run_after_write where a relevant named write is at or before
// at, every one counted as after a green that never was, else empty_run.
func absentState(writes []write, unnamed []int64, at int64) GateState {
	var counted []write
	for _, w := range writes {
		if w.Path != nil && Relevant(w.Class) && w.Seq <= at {
			counted = append(counted, w)
		}
	}
	g := GateState{State: Absent, Reason: EmptyRun, WritesSinceGreen: len(counted), LastWrite: lastWrite(counted)}
	if len(counted) > 0 {
		g.Reason = NeverRunAfterWrite
	}
	g.WritersSinceGreen, g.WritersOmitted = writers(counted)
	for _, s := range unnamed {
		if s <= at {
			g.UnknownTarget++
		}
	}
	return g
}

// withDeclared is gates, the learned rows at the point, with each declared
// gate's row joined (G2) and every learned row whose runs at the point some
// declared gate matched, all of them, dropped (absorption): a family with one
// unmatched run keeps its row. cands is lineRuns', runs classify's; a run
// counts at the point when it landed at or before at, as in gateStates, and
// a gate's runs are read in Seq order, so the result depends on the order of
// no input; it is sorted by Gate then Family. Cost: O(G x R) over the
// candidates, and a sort of each gate's runs.
func withDeclared(gates []GateState, dgs []declaredGate, cands []lineRun, runs []gateRun, writes []write, unnamed []int64, at int64, tree map[string]*string) []GateState {
	matched := map[int64]bool{}
	for i := range dgs {
		var fr []gateRun
		for j := range cands {
			if s, _ := landed(cands[j].gateRun); s <= at && dgs[i].matches(&cands[j]) {
				fr = append(fr, cands[j].gateRun)
				matched[cands[j].Seq] = true
			}
		}
		slices.SortFunc(fr, func(a, b gateRun) int { return cmp.Compare(a.Seq, b.Seq) })
		gates = append(gates, dgs[i].state(fr, writes, unnamed, at, tree))
	}
	open := map[string]bool{}
	for _, r := range runs {
		if k, ok := r.family(); ok && !matched[r.Seq] {
			if s, _ := landed(r); s <= at {
				open[k.runner+"@"+k.location] = true
			}
		}
	}
	gates = slices.DeleteFunc(gates, func(g GateState) bool { return g.Source == sourceLearned && !open[g.Family] })
	slices.SortFunc(gates, func(a, b GateState) int {
		return cmp.Or(cmp.Compare(a.Gate, b.Gate), cmp.Compare(a.Family, b.Family))
	})
	return gates
}
