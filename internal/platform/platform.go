// Package platform turns a pgraph fact log into the obligations a run's
// writes owe (PRD 1.5, PF-PL8): the Go packages whose files the run wrote,
// each a family the evaluator can hold a gate against.
//
// Boundary. pgraph is a separate module that the rashomon module never
// imports (pgraph/doc.go); the crossing is a file in the format pgraph's
// (*Log).Encode writes, versioned by its header line. This package reads it
// with its own decoder, the header and the record grammar alone, never through
// pgraph.DecodeLog. A header it does not know, a log over its cap, a malformed
// line or a decode past Deadline is unknown, never a guess (INV-14).
package platform

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// Deadline bounds one Obligations call (PF-PL8). Past it the answer is one
// unknown row and no family.
const Deadline = 20 * time.Millisecond

// SourcePlatform is the Source of every row Obligations returns.
const SourcePlatform = "platform"

// RunView is what Obligations reads of a run: the project root, the paths the
// run wrote (relative to Root, or absolute under it) and the pgraph log to
// read. LogPath is filled by nothing yet: the first card that gives the hook a
// log path (pgraph sync) wires it; until then the one caller is a test.
type RunView struct {
	Root         string
	WrittenPaths []string
	LogPath      string
}

// Family is one Go package a run's writes touched. Package is its import
// path, the pgraph log's "pkg:" subject without the prefix. Hash is the hex
// SHA-256 over that package's live facts in the view's order (pgraph's
// View lines: subject, predicate, object, tier, config) whose subject is the
// package or a symbol it declares, a method's type included: a change
// detector per package, so a package whose facts change gets a new Hash. It is
// not pgraph's symbol identity (pgraph/identity.go names symbols, not
// packages).
type Family struct {
	Package string
	Hash    string
}

// Obligations is one platform_impact row per family the run's writes touched,
// each open, its family the package and its Hash, so a package whose facts
// change owes a new row. A run with no writes or no log owes none. When the
// log cannot be read whole within Deadline, or is unknown to this package, it
// is one unknown row with no family: never a partial answer.
func Obligations(run RunView) []verify.Obligation {
	return obligationsWithin(run, Deadline)
}

// obligationsWithin is Obligations under deadline d: the seam a test that
// checks the rows' content, not the deadline, runs with a generous d so a
// loaded machine cannot turn its answer into the timeout's unknown row.
func obligationsWithin(run RunView, d time.Duration) []verify.Obligation {
	if run.LogPath == "" || len(run.WrittenPaths) == 0 {
		return []verify.Obligation{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	fams, err := Impact(ctx, run.Root, run.LogPath, run.WrittenPaths)
	if err != nil {
		return []verify.Obligation{platformRow("", verify.ObligationUnknown)}
	}
	out := make([]verify.Obligation, 0, len(fams))
	for _, f := range fams {
		out = append(out, platformRow(f.Package+"@"+f.Hash, verify.ObligationOpen))
	}
	return out
}

func platformRow(family string, state verify.ObligationState) verify.Obligation {
	return verify.Obligation{ID: verify.ObligationID(verify.ScopeSession, verify.PlatformImpact, family),
		Scope: verify.ScopeSession, Kind: verify.PlatformImpact, Source: SourcePlatform, State: state,
		Evidence: []int64{}}
}

// Impact is the families of the packages paths belong to, read from the log at
// logPath, sorted by Package. A path's package is Go's own rule (packageOf);
// a package the log holds no "pkg:" subject for has no family. The decode
// runs in a worker (safe.Go: a panic in it is an error, never the process)
// that looks at ctx every checkEvery records; Impact returns ctx's error as
// soon as ctx is done, and the worker exits at its next look.
func Impact(ctx context.Context, root, logPath string, paths []string) ([]Family, error) {
	return impactWithin(ctx, root, logPath, paths, MaxLogBytes)
}

func impactWithin(ctx context.Context, root, logPath string, paths []string, limit int64) ([]Family, error) {
	type result struct {
		fams []Family
		err  error
	}
	done := make(chan result, 1)
	hook := recordHook
	safe.Go(func() {
		fams, err := impact(ctx, root, logPath, paths, limit, hook)
		done <- result{fams, err}
	}, func(err error) { done <- result{nil, err} })
	select {
	case r := <-done:
		return r.fams, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// impact resolves paths to packages, then decodes the log, at most limit
// bytes, and builds each wanted package's family from its live facts.
func impact(ctx context.Context, root, logPath string, paths []string, limit int64, hook func()) ([]Family, error) {
	mods := map[string]goMod{}
	var wanted []string
	for _, p := range paths {
		if pkg := packageOf(root, p, mods); pkg != "" {
			wanted = append(wanted, pkg)
		}
	}
	slices.Sort(wanted)
	wanted = slices.Compact(wanted)

	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, ErrOverCap
	}
	live, err := decode(ctx, f, hook)
	if err != nil {
		return nil, err
	}
	bySubject := map[string][]fact{}
	for k := range live {
		bySubject[k.subject] = append(bySubject[k.subject], k)
	}
	fams := []Family{}
	for _, pkg := range wanted {
		if _, ok := bySubject["pkg:"+pkg]; !ok {
			continue
		}
		fams = append(fams, Family{Package: pkg, Hash: familyHash(bySubject, "pkg:"+pkg)})
	}
	return fams, nil
}

// familyHash is the SHA-256 of the view lines whose subject is the package or
// a symbol it declares: its declares objects, and theirs, which are a type's
// methods (pgraph/extract.go). Lines are in the view's order: subject,
// predicate, object, then tier rank, then config (pgraph/log.go keyLess).
func familyHash(bySubject map[string][]fact, pkgSubject string) string {
	members := []string{pkgSubject}
	for _, d := range bySubject[pkgSubject] {
		if d.predicate != predDeclares {
			continue
		}
		members = append(members, d.object)
		for _, m := range bySubject[d.object] {
			if m.predicate == predDeclares {
				members = append(members, m.object)
			}
		}
	}
	slices.Sort(members)
	members = slices.Compact(members)
	var facts []fact
	for _, s := range members {
		facts = append(facts, bySubject[s]...)
	}
	slices.SortFunc(facts, func(a, b fact) int {
		return cmp.Or(cmp.Compare(a.subject, b.subject), cmp.Compare(a.predicate, b.predicate),
			cmp.Compare(a.object, b.object), cmp.Compare(tierRank(a.tier), tierRank(b.tier)), cmp.Compare(a.config, b.config))
	})
	h := sha256.New()
	for _, f := range facts {
		h.Write([]byte(f.subject + "\t" + f.predicate + "\t" + f.object + "\t" + f.tier + "\t" + f.config + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
