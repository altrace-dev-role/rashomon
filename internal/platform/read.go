package platform

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// MaxFamilies caps the packages one Read gives a family. The rest are
// counted in PlatformRead.More, and the ledger owes one unknown row for
// them, so a session that wrote a thousand packages costs the same as one
// that wrote sixty-four.
const MaxFamilies = 64

// maxDirEntries bounds the entries Read lists of a package's directory to
// find a test file: past it, a directory with none among them reads as
// having none, so its package is never met.
const maxDirEntries = 4096

// Read is the ledger verb's and the Stop's view of the platform graph for
// the run's writes: one family per Go package a written .go file belongs
// to (Go's rule, D132), sorted by Package, at most MaxFamilies. Where
// pgraph's walk file is there, the log and digests are read only as the
// pair it names by sha256 (walk.go); any other pair reads LogUnreadable.
// With no log (LogPath "", or gone by the time it is opened) every family
// is FactsUnknown and none is judged. A log read whole gives each package
// FactsKnown and its Hash where the sync's digests vouch for its files
// (fresh), FactsStale where they do not, and FactsNew where the log holds
// no such package; each such family also says whether its directory holds
// a test file. A log that cannot be read whole, or is no regular file, is
// LogUnreadable, and one held past the crash guard (or RunView.Deadline)
// LogTimedOut, each with no family: never a partial answer. A path that is
// no .go file, or in no package, names none, and so does every path when
// Root is "": a relative path would be read against the process's
// directory.
func Read(run RunView) verify.PlatformRead {
	out := verify.PlatformRead{Log: verify.LogNone, Families: []verify.PlatformFamily{}}
	if run.Root == "" {
		return out
	}
	mods := map[string]goMod{}
	locs := map[string]pkgLoc{}
	written := map[string][]string{}
	for _, p := range run.WrittenPaths {
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		if l := locate(run.Root, p, mods); l.pkg != "" {
			locs[l.pkg] = l
			written[l.pkg] = append(written[l.pkg], p)
		}
	}
	pkgs := make([]string, 0, len(locs))
	for pkg := range locs {
		pkgs = append(pkgs, pkg)
	}
	slices.Sort(pkgs)
	if len(pkgs) > MaxFamilies {
		out.More, pkgs = len(pkgs)-MaxFamilies, pkgs[:MaxFamilies]
	}
	if len(pkgs) == 0 {
		return out
	}
	hashes := map[string]string{}
	var dg *digests
	if run.LogPath != "" {
		var paths []string
		for _, pkg := range pkgs {
			paths = append(paths, written[pkg]...)
		}
		guard := deadline
		if run.Deadline > 0 {
			guard = run.Deadline
		}
		ctx, cancel := context.WithTimeout(context.Background(), guard)
		defer cancel()
		// The walk file first, then the log, then the digests: each is
		// checked against the one walk file read, so a sync that renames
		// either between the reads makes the pair read unknown.
		vouched, verr := readVouch(run.Root)
		fams, err := impactVouched(ctx, run.Root, run.LogPath, paths, MaxLogBytes, vouched.log)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case errors.Is(err, context.DeadlineExceeded):
			return verify.PlatformRead{Log: verify.LogTimedOut, Families: []verify.PlatformFamily{}}
		case err != nil || verr != nil:
			return verify.PlatformRead{Log: verify.LogUnreadable, Families: []verify.PlatformFamily{}}
		default:
			out.Log = verify.LogRead
		}
		for _, f := range fams {
			hashes[f.Package] = f.Hash
		}
		if out.Log == verify.LogRead {
			var err error
			dg, err = readDigests(digestsPath(run.Root))
			if vouched.on && (err != nil || dg.sum != vouched.digests) {
				return verify.PlatformRead{Log: verify.LogUnreadable, Families: []verify.PlatformFamily{}}
			}
		}
	}
	hashed := 0
	for _, pkg := range pkgs {
		l := locs[pkg]
		f := verify.PlatformFamily{Package: pkg, Dir: l.dir, ModuleDir: l.modDir, Facts: verify.FactsUnknown}
		if out.Log == verify.LogRead {
			f.Tests = hasTests(run.Root, l.dir)
			h, held := hashes[pkg]
			switch {
			case !held:
				f.Facts = verify.FactsNew
			case fresh(run.Root, dg, pkg, written[pkg], &hashed):
				f.Facts, f.Hash = verify.FactsKnown, h
			default:
				f.Facts = verify.FactsStale
			}
		}
		out.Families = append(out.Families, f)
	}
	return out
}

// hasTests reports a regular file named *_test.go among the first
// maxDirEntries entries of the directory dir names under root. A path that
// is not a directory has none: regfile.OpenDir opens without blocking and
// refuses it, so a named pipe at the path cannot hold the read (M5).
func hasTests(root, dir string) bool {
	d, err := regfile.OpenDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return false
	}
	defer d.Close()
	es, _ := d.ReadDir(maxDirEntries)
	return slices.ContainsFunc(es, func(e fs.DirEntry) bool {
		return e.Type().IsRegular() && strings.HasSuffix(e.Name(), "_test.go")
	})
}
