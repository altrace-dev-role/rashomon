package platform

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"
	"time"
)

// The blast-radius read (card ARCH-INLINE1, PF-PL7): after a Go edit, the
// other packages whose code the platform graph shows calling into the edited
// file's package. The v0 log holds no import edge, so the set is the
// packages with a PROVEN calls fact whose object is a symbol of the edited
// package: a lower bound on its importers, never "none" (PF-PL11), since a
// package that uses only its types or constants, or reaches it through an
// interface (may-call, ALGORITHMIC), is not in it.

// Site is where a Go edit lands for the read: the sync root (the repository
// whose .pgraph holds the log), the edited file's package and the log's
// sha256 the walk file vouches for, the read's key (one read per package and
// log version).
type Site struct {
	Root, Package, Log string
}

// Caller is one other package with a PROVEN call into the site's package:
// its import path, its directory relative to the root (slash-separated, "."
// for the root), and whether that directory holds a test file.
type Caller struct {
	Package, Dir string
	Tests        bool
}

// Locate is the site of an edit of path under root, and false for a path
// that is no .go file, a _test.go file (no sync reads one, so no fact rests
// on it), in no package by Go's rule, or under a root whose walk file is
// absent or vouches for no pair: a log no sync vouched for is never read
// here, whatever a pgraph from before the walk file wrote. It reads the walk
// file first, so a repository with no sync pays one failed open, then the
// go.mod files above path; never the log.
func Locate(root, file string) (Site, bool) {
	if root == "" || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return Site{}, false
	}
	v, err := readVouch(root)
	if err != nil || !v.on {
		return Site{}, false
	}
	l := locate(root, file, map[string]goMod{})
	if l.pkg == "" {
		return Site{}, false
	}
	return Site{Root: root, Package: l.pkg, Log: v.log}, true
}

// Callers is the site's callers, sorted by Package, read within guard (the
// crash guard's role, as RunView.Deadline is the Stop's). The walk file
// must still name site.Log, and the digests file the sum it names, or the
// pair is errNotVouched; the log is read through readLog, which checks its
// sha256 as it decodes, keeping only records whose object may name a symbol
// of the site's package (objectIn). A PROVEN calls record counts where its
// object is the site's package's by packageIn and its subject another
// package's (callerPackage); a caller is kept only where the digests vouch
// its files as they are now (fresh): a caller edited since the sync may no
// longer call in, so it is not named, and the set stays a lower bound.
func Callers(site Site, guard time.Duration) ([]Caller, error) {
	ctx, cancel := context.WithTimeout(context.Background(), guard)
	defer cancel()
	v, err := readVouch(site.Root)
	if err != nil || !v.on || v.log != site.Log {
		return nil, errNotVouched
	}
	dg, err := readDigests(digestsPath(site.Root))
	if err != nil {
		return nil, err
	}
	if dg.sum != v.digests {
		return nil, errNotVouched
	}
	hook := recordHook
	live, err := inWorker(ctx, func() (map[fact]struct{}, error) {
		return readLog(ctx, LogPath(site.Root), MaxLogBytes, site.Log, hook, objectIn(site.Package))
	})
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for f := range live {
		if f.predicate != predCalls || f.tier != tierProven {
			continue
		}
		if packageIn(dg, f.object) != site.Package {
			continue
		}
		if sub := callerPackage(dg, f.subject); sub != "" && sub != site.Package {
			in[sub] = true
		}
	}
	pkgs := make([]string, 0, len(in))
	for p := range in {
		pkgs = append(pkgs, p)
	}
	slices.Sort(pkgs)
	out := []Caller{}
	hashed := 0
	for _, p := range pkgs {
		if !fresh(site.Root, dg, p, nil, &hashed) {
			continue
		}
		dir := path.Dir(dg.byPkg[p][0])
		out = append(out, Caller{Package: p, Dir: dir, Tests: hasTests(site.Root, dir)})
	}
	return out, nil
}

// objectIn is readLog's keep for the records whose object may name a symbol
// of pkg (pgraph's spelling: kind, ':', the declaring package's path, '.',
// the rest): a cheap prefix test before any allocation. A path that is
// another's prefix up to a dot (gopkg.in/yaml, gopkg.in/yaml.v3) lets a few
// more through, which Callers' packageIn then refuses.
func objectIn(pkg string) func(subject, object []byte) bool {
	return func(_, object []byte) bool {
		i := bytes.IndexByte(object, ':')
		if i < 0 {
			return false
		}
		rest := object[i+1:]
		return len(rest) > len(pkg) && rest[len(pkg)] == '.' && string(rest[:len(pkg)]) == pkg
	}
}

// callerPackage is the package of a calls record's subject: packageIn's for
// a function or method, and for a package's own "pkg:" subject (a variable
// initialiser's call), the package itself where the digests list it. ""
// for none: such a caller is not counted.
func callerPackage(dg *digests, id string) string {
	if p, ok := strings.CutPrefix(id, "pkg:"); ok {
		if _, listed := dg.byPkg[p]; listed {
			return p
		}
		return ""
	}
	return packageIn(dg, id)
}
