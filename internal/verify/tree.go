package verify

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The tree a gate run is bound to (card P5A; PF-PG6, INV-14; D140). A named
// write's post records the sha256 of the file it names as the call left it
// (store.ExecutionV5.ContentSHA256). A green run's tree is, per relevant path
// the session named in a write declared before the run, the sha its last such
// write recorded; the claim point's is the same files' content now, which the
// Stop hook reads (TreeTargets, then Point.Tree), since verify reads no file.
// The evidence is bound where the two agree on every path of the run's tree,
// compared path by path: nothing stores a digest of the pairs, so one would
// add a hash and say less. Content only: HEAD is no part of it, so the
// session's own commit after a green run is not an edit (D140 F2, a PF-PG6
// deviation). The paths are the relevant ones writtenSince counts (F3).

// MaxTreeFiles is the most relevant paths a session's tree binds (D140): over
// it TreeTargets names none, the Stop reads no file, and the tree is unknown.
const MaxTreeFiles = 64

// TreeTarget is one file the claim point's tree reads: Path, project-relative
// (relativise), the key both trees use; File, the path the Stop hook opens:
// as the last write named it, joined to that declaration's cwd when relative.
type TreeTarget struct {
	Path string
	File string
}

// TreeTargets is every relevant path run's hashed writes name (classify's
// writes, D135), once, in path order; empty with none, and nil over
// MaxTreeFiles. A relative path whose declaration has no cwd has no file to
// open and no target, so a run's tree that holds it reads unknown. A Bash
// write's paths are not read: its post records no sha, so a tree that holds
// one is unknown whatever the file says.
func TreeTargets(run *store.Run) []TreeTarget {
	if run == nil {
		return []TreeTarget{}
	}
	type named struct {
		seq  int64
		file string
		ok   bool
	}
	last := map[string]named{}
	var w []write
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if !hashed(d) {
			continue
		}
		if w, _ = appendWrites(w[:0], nil, d); !Relevant(w[0].Class) {
			continue
		}
		if cur, seen := last[*w[0].Path]; !seen || d.Seq > cur.seq {
			file, ok := *d.FilePath, true
			if !filepath.IsAbs(file) {
				if ok = d.CWD != nil; ok {
					file = filepath.Join(*d.CWD, file)
				}
			}
			last[*w[0].Path] = named{d.Seq, file, ok}
		}
	}
	if len(last) > MaxTreeFiles {
		return nil
	}
	out := make([]TreeTarget, 0, len(last))
	for p, n := range last {
		if n.ok {
			out = append(out, TreeTarget{Path: p, File: n.file})
		}
	}
	slices.SortFunc(out, func(a, b TreeTarget) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// treeBind is how a green run's tree stands against the claim point's.
type treeBind uint8

const (
	boundUnknown treeBind = iota // no tree at the point, or a sha either side cannot give
	boundMet                     // every path of the run's tree reads as it did
	boundOwn                     // a path moved, and the session wrote unnamed since the run
	boundForeign                 // a path moved, and nothing in the session's record wrote since
)

// bind is the green run declared at ref against tree, the claim point's
// (Point.Tree), with writes in seq order and the unnamed writes' seqs. A
// path moved where the point reads a sha that differs, or nil: the file is
// gone, not regular or over 1 MiB, none of which a run was bound to. One
// moved path decides; else a path the run's tree has no sha for, or the
// point has no entry for (unreadable, or no file to open), is unknown. A
// move beside an unnamed write in (ref, at] is the session's own, else
// foreign; writes the session named are writtenSince's, above this.
func bind(writes []write, unnamed []int64, ref, at int64, tree map[string]*string) treeBind {
	if tree == nil {
		return boundUnknown
	}
	run := map[string]*string{}
	for _, w := range writes {
		if w.Seq >= ref {
			break
		}
		if w.Path != nil && Relevant(w.Class) {
			run[*w.Path] = w.SHA
		}
	}
	moved, known := false, true
	for p, sha := range run {
		now, ok := tree[p]
		switch {
		case sha == nil || !ok:
			known = false
		case now == nil || *now != *sha:
			moved = true
		}
	}
	switch {
	case moved && slices.ContainsFunc(unnamed, func(s int64) bool { return s > ref && s <= at }):
		return boundOwn
	case moved:
		return boundForeign
	case !known:
		return boundUnknown
	}
	return boundMet
}
