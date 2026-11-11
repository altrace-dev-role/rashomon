package verify

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The platform rows (card PL-WIRE1; PF-PL8, PF-PG5; WED-SLICE rows 1 and 2).
// `rashomon report --ledger`, which a person runs, replays the session's last
// Stop with StopInput.Platform set: internal/platform reads the pgraph log at
// the project root for the session's written Go files (that package imports
// this one), and the ledger owes one platform_impact row per package they
// belong to. The Stop hook sets it too (PW1b-ON-230 step 2), and its line
// names the open and unmet rows (recap's platformLines); StopFailure and the
// catch-up leave it nil.
//
// A row owes the package's `go test` (S4): a required test gate a trusted
// spec declares, or the inferred one where the project's own files name it,
// whose runner is go test (GATE-INFER1; never a manifest's convention, never
// a make or npm gate). It is
//   - met, citing the write and the run, where the last go test run after the
//     package's last write (writeTouches) covers the package (coverOf), is
//     green, no unnamed write came after it, nor a call that may have
//     written its files and did not end ok (mayHaveTouched), and the
//     package's written files read as they did at the run (bind);
//   - unmet, citing both, where that run is red and named the package alone:
//     a red run over many packages says only that one of them failed;
//   - open, citing the write, where no go test run after the write covers it
//     and none might;
//   - unknown, with no evidence, where there is no log, no go test gate, no
//     test file in the package, a run after the write that might have tested
//     it with words the record cannot show, a write the record cannot name
//     or a call that may have written and did not end ok after the cover,
//     or a covering run whose outcome is not known; and
//   - one unknown row with no family where the log could not be read whole,
//     and one more for the packages past the read's cap.
//
// The verdict does not rest on the log's facts: a package the log holds, one
// whose files changed since the sync (stale, its hash withheld) and one the
// log does not hold (new) are judged alike. The log's facts name the family,
// so a package whose facts change owes a new row.

// SourcePlatform is the Source of every platform_impact row.
const SourcePlatform = "platform"

// LogState is what the platform read found of the pgraph log.
type LogState string

const (
	// LogNone: no log at the project root, so no package is judged.
	LogNone LogState = "none"
	// LogRead: the log was read whole.
	LogRead LogState = "read"
	// LogUnreadable: a log is there and could not be read whole (an unknown
	// header, over its byte cap, a malformed record, not a regular file).
	LogUnreadable LogState = "unreadable"
	// LogTimedOut: the read passed internal/platform's crash guard.
	LogTimedOut LogState = "timed_out"
)

// FactsState is what the pgraph log says of one package's facts.
type FactsState string

const (
	// FactsKnown: the log holds the package and every file of it the sync's
	// digests list, and every .go file of it the session wrote, reads as the
	// sync read it; Hash is its family hash.
	FactsKnown FactsState = "known"
	// FactsStale: the log holds the package and a file of it reads otherwise,
	// is not listed, cannot be read, or lies past the hash cap, or the
	// digests cannot be read; no Hash.
	FactsStale FactsState = "stale"
	// FactsNew: the log was read and holds no such package (one created
	// after the sync, or built only under another configuration).
	FactsNew FactsState = "new"
	// FactsUnknown: there is no log.
	FactsUnknown FactsState = "unknown"
)

// PlatformFamily is one Go package the session's writes touched, as
// internal/platform read it. Package is its import path (Go's rule, D132:
// the nearest go.mod's module path joined with the directory); Dir and
// ModuleDir are the package's directory and that go.mod's, project-relative
// and slash-separated, "." for the project root; Hash is the hex SHA-256 of
// its live facts, set only where Facts is FactsKnown. Tests says a _test.go
// file is in Dir now: a package with none is never met, since a run that
// names it tests nothing of it (N2).
type PlatformFamily struct {
	Package, Dir, ModuleDir string
	Facts                   FactsState
	Hash                    string
	Tests                   bool
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

// PlatformMore is the family of the one unknown row that stands for the
// packages past the read's cap (PlatformRead.More).
const PlatformMore = "more"

// PlatformRead is what internal/platform read: the log's state, one family
// per package up to its cap, sorted by Package, never nil (none where the
// log was unreadable or timed out), and how many packages the cap left out.
type PlatformRead struct {
	Log      LogState
	Families []PlatformFamily
	More     int
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
	return w.Path != nil && Relevant(w.Class) && strings.HasSuffix(*w.Path, ".go") && inProject(*w.Path)
}

// inProject: a path relativise left project-relative.
func inProject(p string) bool {
	return !path.IsAbs(p) && p != ".." && !strings.HasPrefix(p, "../")
}

// writeTouches: a relevant named write that can change f's tests' outcome:
// one under f's directory (its files, its testdata, the packages below it),
// or its module's go.mod or go.sum (M2).
func writeTouches(w write, f PlatformFamily) bool {
	if w.Path == nil || !Relevant(w.Class) || !inProject(*w.Path) {
		return false
	}
	p := *w.Path
	return within(path.Dir(p), f.Dir) || p == path.Join(f.ModuleDir, "go.mod") || p == path.Join(f.ModuleDir, "go.sum")
}

// platformObligations is the ledger's platform rows at snap's point over
// read, in its order; key is the install key the hook digested every run's
// line and location under, run the record their whole-line digests are read
// from. An incomplete record reaches here with no family: evaluate
// classifies no write of one, so platformPaths gives none.
func platformObligations(snap Snapshot, c *classified, run *store.Run, key []byte, read PlatformRead) []Obligation {
	scope := cmp.Or(obligationScopes()[snap.Point.Kind], ScopeSession)
	if read.Log == LogUnreadable || read.Log == LogTimedOut {
		return []Obligation{platformRow(scope, "", ObligationUnknown)}
	}
	p := platformJudge{snap: &snap, c: c, run: run, key: key}
	gate := goTestGate(c)
	out := make([]Obligation, 0, len(read.Families)+1)
	for _, f := range read.Families {
		state, ev := ObligationUnknown, []int64{}
		if f.Facts != FactsUnknown && gate && f.Tests {
			state, ev = p.judge(f)
		}
		o := platformRow(scope, f.Family(), state)
		o.Evidence = ev
		out = append(out, o)
	}
	if read.More > 0 {
		out = append(out, platformRow(scope, PlatformMore, ObligationUnknown))
	}
	return out
}

func platformRow(scope, family string, state ObligationState) Obligation {
	return Obligation{ID: ObligationID(scope, PlatformImpact, family), Scope: scope, Kind: PlatformImpact,
		Source: SourcePlatform, State: state, Evidence: []int64{}}
}

// goTestKey is go test's runner key (shape.RunnerKey): its row name, which no
// argument enters.
func goTestKey() string {
	k, _ := shape.RunnerKey("go test", new(string))
	return k
}

// goTestGate reports a required test gate evaluate joined whose runner is go
// test: a trusted or managed spec's, or the inferred one where the project's
// own files name it (InferredGate.Declared), never one standing for a
// manifest's convention (S4).
func goTestGate(c *classified) bool {
	k := goTestKey()
	for _, g := range c.gates {
		if g.kind == "test" && g.required && (g.id != InferredGateID || c.inferredDeclared) && g.runners[k] {
			return true
		}
	}
	return false
}

// platformJudge judges families' rows against the record. locs caches each
// directory's location digest and lines each directory's covering lines'
// digests, so neither is derived twice.
type platformJudge struct {
	snap  *Snapshot
	c     *classified
	run   *store.Run
	key   []byte
	decls map[int64]*store.Declaration
	locs  map[string]string
	lines map[string]map[string]cover
}

// judge is f's state and evidence. Of the gate runs declared after f's last
// write (writeTouches) and landed by the point, each go test run either
// covers f, alone or with other packages, or might have; a run with no
// runner key (a runner a cd ran past the fold, two hidden runners), or one
// whose key is its words' digest (a launcher, make or npm run, or a record
// before runner names), or a test run of another runner (make test, which
// may run go test), might have run f's tests (M1); a build or other run of
// a named runner (go build, go vet) is passed over. The last cover decides
// unless a run that might have covered f came after it.
func (p *platformJudge) judge(f PlatformFamily) (ObligationState, []int64) {
	at := p.snap.Point.Seq
	var last int64
	var touched []write
	for _, w := range p.c.writes {
		if w.Seq <= at && writeTouches(w, f) {
			last = max(last, w.Seq)
			touched = append(touched, w)
		}
	}
	if last == 0 {
		return ObligationUnknown, []int64{}
	}
	var cov *gateRun
	alone, maybe := false, false
	goTest := goTestKey()
	for i := range p.c.runs {
		r := &p.c.runs[i]
		if s, _ := landed(*r); r.Seq <= last || s > at {
			continue
		}
		k, ok := shape.RunnerKey(shape.RunnerNameOf(r.RunnerID), r.Runner)
		switch {
		case ok && k == goTest:
			switch c := p.coverOf(r, f); c {
			case coverAlone, coverMany:
				cov, alone, maybe = r, c == coverAlone, false
			case coverMaybe:
				maybe = true
			}
		case !ok || !strings.HasPrefix(k, "\x00") || r.Kind == kindTest:
			maybe = true
		}
	}
	switch {
	case maybe:
		return ObligationUnknown, []int64{}
	case cov == nil:
		return ObligationOpen, []int64{last}
	case slices.ContainsFunc(p.c.unnamed, func(s int64) bool { return s > cov.Seq && s <= at }) ||
		mayHaveTouched(p.c.held, f, cov.Seq, at):
		return ObligationUnknown, []int64{}
	}
	// touched is f's writes alone (writeTouches), so no gate root narrows
	// them: "" covers every one (covers, STALE-COVER1).
	if b := bind(touched, p.c.unnamed, "", cov.Seq, at, p.snap.Point.Tree); b == boundOwn || b == boundForeign {
		return ObligationUnknown, []int64{}
	}
	switch {
	case green(cov):
		return ObligationMet, []int64{last, cov.Seq}
	case alone && !cov.Backgrounded && !masked(cov) && !emptyRun(cov) && failed(cov):
		return ObligationUnmet, []int64{last, cov.Seq}
	}
	return ObligationUnknown, []int64{}
}

// mayHaveTouched reports a held write declared in (from, at] that may have
// written f's files: a call that failed and may have written, or has no
// recorded ending, naming one of them (writeTouches) or no path. A failed
// file tool did not apply and is passed over. Such a call after the cover
// is no edit the row names, and none it can rule out: the row is unknown,
// as for a write the record cannot name. The cost: a failed sed that wrote
// nothing, or an Edit refused after the run, turns a met row unknown.
func mayHaveTouched(held []heldWrite, f PlatformFamily, from, at int64) bool {
	return slices.ContainsFunc(held, func(w heldWrite) bool {
		return w.Seq > from && w.Seq <= at && w.End != EndFailedEdit && (w.Path == nil || writeTouches(w.write, f))
	})
}

// declaration is the declaration at seq, indexed once per judge.
func (p *platformJudge) declaration(seq int64) *store.Declaration {
	if p.decls == nil {
		p.decls = make(map[int64]*store.Declaration, len(p.run.Declarations))
		for i := range p.run.Declarations {
			p.decls[p.run.Declarations[i].Seq] = &p.run.Declarations[i]
		}
	}
	return p.decls[seq]
}
