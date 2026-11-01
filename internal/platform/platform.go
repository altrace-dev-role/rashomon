// Package platform reads a pgraph fact log for the ledger verb's replay of a
// Stop (PRD 1.5, PF-PL8): the Go packages whose files a run wrote, each a
// family the ledger owes a platform_impact row and holds the package's go
// test against (verify/platform.go), and whether the log's facts about each
// still hold (digests.go).
//
// Boundary. pgraph is a separate module that the rashomon module never
// imports (pgraph/doc.go); the crossing is a file in the format pgraph's
// (*Log).Encode writes, versioned by its header line. This package reads it
// with its own decoder, the header and the record grammar alone, never through
// pgraph.DecodeLog. A header it does not know, a log over its cap, a malformed
// line or a decode past the crash guard is unknown, never a guess (INV-14).
package platform

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// deadline is a crash guard on one Read's decode of the log, not a budget:
// the answer must be the same for the same files (INV-11), and a decode
// within MaxLogBytes is milliseconds (11 to 17 ms for this repository's 3 MB
// log), so only a read that has stalled reaches it, and it reads
// LogTimedOut. A variable so a test can hold a decode past it.
var deadline = 10 * time.Second

// RunView is what Read reads of a run: the project root, the paths the run
// wrote (relative to Root, or absolute under it) and the pgraph log to read.
// The ledger verb fills LogPath with LogPath(Root) where that file exists
// (cmd's platformRead, card PL-WIRE1), else leaves it "".
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

	f, err := regfile.Open(logPath)
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
