package verify

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// The platform rows (card PL-WIRE1; PF-PL8, PF-PG5; WED-SLICE rows 1 and 2).
// The Stop's caller reads the pgraph log for the session's written Go files
// (internal/platform, through StopInput.Platform: that package imports this
// one) and the ledger owes one platform_impact row per package they belong
// to: the package's test gate, the command a trusted spec declares or the
// project's own files name (GATE-INFER1; never one read from a manifest's
// presence alone, which is a convention, not the project's word), run after
// the package's last write. A row is
//   - met, citing the write and the run, where the last run of the gate's
//     runner after that write covers the package (coverOf) and is green;
//   - unmet, citing both, where that run is red;
//   - open, citing the write, where no run of the runner after the write
//     covers the package and none might: none ran, or the only ones ran in
//     a directory above the package's module;
//   - unknown, with no evidence, where the record is incomplete, no gate
//     stands, the log holds no such package or there is no log (PF-PL11,
//     INV-2), a run of the runner after the write might have covered it with
//     words the record cannot show (platform_cover.go), or the covering
//     run's outcome is not known (masked, backgrounded, unread); and
//   - one unknown row with no family where the log could not be read whole.
//
// A package with a written .go file whose content differs from the digest
// the log's sync recorded has stale facts (PF-PL6, first half): its family
// carries no hash, since the log's facts are not about the code as it is
// now. Its row is still owed and judged: what it owes rests on the written
// path, Go's own package rule and the record's runs, all read now, not on
// the stale facts. A re-sync gives the hash back.

// SourcePlatform is the Source of every platform_impact row.
const SourcePlatform = "platform"

// FactsState is what the pgraph log says of one package's facts.
type FactsState string

const (
	// FactsKnown: the log holds the package and no written file of it
	// differs from the digest its sync recorded; Hash is its family hash.
	FactsKnown FactsState = "known"
	// FactsStale: the log holds the package, and a written .go file of it
	// differs from its recorded digest, is not among them, or cannot be
	// read, or the digests cannot be; no Hash.
	FactsStale FactsState = "stale"
	// FactsUnknown: there is no log, or it holds no such package (one
	// created after the sync, or built only under another configuration).
	FactsUnknown FactsState = "unknown"
)

// PlatformFamily is one Go package the session's writes touched, as
// internal/platform read it. Package is its import path (Go's rule, D132:
// the nearest go.mod's module path joined with the directory); Dir and
// ModuleDir are the package's directory and that go.mod's, project-relative
// and slash-separated, "." for the project root; Hash is the hex SHA-256 of
// its live facts, set only where Facts is FactsKnown.
type PlatformFamily struct {
	Package, Dir, ModuleDir string
	Facts                   FactsState
	Hash                    string
}

// Family is the string a row's id digests: the package and its hash, or the
// package and the word for why there is none, so a package whose facts
// change owes a new row and a stale one never shares an id with a fresh one.
func (f PlatformFamily) Family() string {
	if f.Facts == FactsKnown {
		return f.Package + "@" + f.Hash
	}
	return f.Package + "@" + string(f.Facts)
}

// PlatformRead is what internal/platform read for a Stop: one family per
// package, sorted by Package, never nil; Unreadable where a log is there and
// could not be read whole (an unknown header, over its cap, a malformed
// record, past the deadline), which owes one unknown row and no family, never
// a partial answer.
type PlatformRead struct {
	Families   []PlatformFamily
	Unreadable bool
}

// platformPaths is every relevant named write's path at or before at that is
// a Go file under the project (relativise left it relative), once, sorted:
// what read, StopInput.Platform, is given; none when read is nil, so a Stop
// with no reader allocates nothing here.
func platformPaths(read func(string, []string) PlatformRead, writes []write, at int64) []string {
	if read == nil {
		return nil
	}
	var out []string
	for _, w := range writes {
		if w.Seq <= at && isGoWrite(w) {
			out = append(out, *w.Path)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// isGoWrite: a relevant named write of a .go file the project holds.
func isGoWrite(w write) bool {
	return w.Path != nil && Relevant(w.Class) && strings.HasSuffix(*w.Path, ".go") &&
		!path.IsAbs(*w.Path) && *w.Path != ".." && !strings.HasPrefix(*w.Path, "../")
}

// platformObligations is the ledger's platform rows at snap's point over
// read, in its order; key is the install key the hook digested every run's
// location and runner words under. An incomplete record reaches here with no
// family: evaluate classifies no write of one, so platformPaths gives none.
func platformObligations(snap Snapshot, c *classified, key []byte, read PlatformRead) []Obligation {
	scope := cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession)
	if read.Unreadable {
		return []Obligation{platformRow(scope, "", ObligationUnknown)}
	}
	if len(read.Families) == 0 {
		return nil
	}
	p := platformJudge{snap: &snap, c: c, key: key, runners: testRunners(c)}
	out := make([]Obligation, 0, len(read.Families))
	for _, f := range read.Families {
		state, ev := ObligationUnknown, []int64{}
		if f.Facts != FactsUnknown && len(p.runners) > 0 {
			state, ev = p.judge(f)
		}
		o := platformRow(scope, f.Family(), state)
		o.Evidence = ev
		out = append(out, o)
	}
	return out
}

func platformRow(scope, family string, state ObligationState) Obligation {
	return Obligation{ID: ObligationID(scope, PlatformImpact, family), Scope: scope, Kind: PlatformImpact,
		Source: SourcePlatform, State: state, Evidence: []int64{}}
}

// testRunners is the runner keys (shape.RunnerKey) of every required test
// gate evaluate joined: a trusted or managed spec's, and the inferred one
// where the project's own files name it (InferredGate.Declared), never where
// it stands for a manifest's convention.
func testRunners(c *classified) map[string]bool {
	out := map[string]bool{}
	for _, g := range c.gates {
		if g.kind != "test" || !g.required || g.id == InferredGateID && !c.inferredDeclared {
			continue
		}
		for k := range g.runners {
			out[k] = true
		}
	}
	return out
}

// platformJudge judges families' rows against the record. digests holds
// candidateLines' runner digests by directory and family, derived once.
type platformJudge struct {
	snap    *Snapshot
	c       *classified
	key     []byte
	runners map[string]bool
	digests map[string]map[string]bool
}

// judge is f's state and evidence: of the runs of a test gate's runner
// declared after f's last write and landed by the point, the last that
// covers f decides, unless a later one might have covered it (a red run of
// part of f's tests after a green whole one is not a pass), which makes the
// row unknown, as one that might cover it does where none covers; no such
// run leaves it open.
func (p *platformJudge) judge(f PlatformFamily) (ObligationState, []int64) {
	at := p.snap.Point.Seq
	var last int64
	for _, w := range p.c.writes {
		if w.Seq <= at && isGoWrite(w) && path.Dir(*w.Path) == f.Dir {
			last = max(last, w.Seq)
		}
	}
	if last == 0 {
		return ObligationUnknown, []int64{}
	}
	var cover *gateRun
	maybe := false
	for i := range p.c.runs {
		r := &p.c.runs[i]
		if s, _ := landed(*r); r.Seq <= last || s > at {
			continue
		}
		if k, ok := shape.RunnerKey(shape.RunnerNameOf(r.RunnerID), r.Runner); !ok || !p.runners[k] {
			continue
		}
		switch p.coverOf(r, f) {
		case triYes:
			cover, maybe = r, false
		case triMaybe:
			maybe = true
		}
	}
	switch {
	case maybe:
		return ObligationUnknown, []int64{}
	case cover != nil && green(cover):
		return ObligationMet, []int64{last, cover.Seq}
	case cover != nil && !cover.Backgrounded && !masked(cover) && !emptyRun(cover) && failed(cover):
		return ObligationUnmet, []int64{last, cover.Seq}
	case cover != nil:
		return ObligationUnknown, []int64{}
	}
	return ObligationOpen, []int64{last}
}
