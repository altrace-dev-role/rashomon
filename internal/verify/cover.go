package verify

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// noProject is the project of a write whose call the hook proved in none
// (card STALE-COVER1): PROJECT-RESOLVE1 keys no project for a call whose own
// directory, or whose file's, is under no .git and outside the cwd, and the
// record then holds a null project beside a cwd, nothing withheld. "" is no
// project key, so no gate's root is it.
var noProject string

// projectOf is the project a write of d is read in: d's key; noProject where
// the hook proved the call in none; nil where the record does not say (the
// key or the cwd withheld, or no cwd), which every gate covers. A record
// before PROJECT-RESOLVE1 keyed every call with a cwd, so it never reads
// noProject.
func projectOf(d *store.Declaration) *string {
	switch {
	case d.Project != nil:
		return d.Project
	case d.ProjectWithheld == nil && d.CWD != nil:
		return &noProject
	}
	return nil
}

// covers reports whether a gate whose project key is root covers w, a write
// that names its path (card STALE-COVER1): the file is inside root, read as
// relativise reads paths, from the record's strings alone, so a record reads
// the same on any machine. A path relativise left relative is in its call's
// project, which a shell line keys by where it ran, not by the path it names:
// covered where that project is root or inside root, or holds root and the
// path runs into it (a session in the directory above its worktrees). An
// absolute path, outside its call's project and cwd, is covered when it is
// under root; a relative one that climbs out ("../") is not placed, and not
// covered. Fail closed: a gate with no root, or a write whose project the
// record does not say, is covered, as every write was before the card.
func covers(root string, w *write) bool {
	project := w.project()
	if root == "" || project == nil {
		return true
	}
	p, root := *w.Path, slashed(root)
	if filepath.IsAbs(p) {
		_, in := under(p, root)
		return in
	}
	proj := slashed(*project)
	if proj == "" || p == ".." || strings.HasPrefix(p, "../") {
		return false
	}
	if _, in := under(proj, root); in || proj == root {
		return true
	}
	rel, in := under(root, proj)
	return in && (p == rel || strings.HasPrefix(p, rel+"/"))
}

// coveredByAny reports whether a gate of roots covers w; nil roots cover
// every write (gateRoots: some gate's root is not known).
func coveredByAny(roots []string, w *write) bool {
	if roots == nil {
		return true
	}
	for _, r := range roots {
		if covers(r, w) {
			return true
		}
	}
	return false
}

// gateRoots is the roots the turn's writes are counted under: each gate's
// and the snapshot's own, each once; nil, every write counted, when any of
// them is not known or the recording is incomplete (D80: an incomplete turn
// that wrote reads unknown, never none).
func gateRoots(snap *Snapshot) []string {
	if snap.Recording != Complete || snap.ProjectRoot == "" {
		return nil
	}
	roots := []string{snap.ProjectRoot}
	for i := range snap.Gates {
		r := snap.Gates[i].root
		if r == "" {
			return nil
		}
		if !slices.Contains(roots, r) {
			roots = append(roots, r)
		}
	}
	return roots
}

// turnProjects is the projects the calls of the turn ran in (card F2), over
// the window countTurn reads: each declaration's (projectOf: a file tool's
// file's repository, a shell line's where it ran), distinct, in record order.
// nil reads every gate as the turn's: a call whose project the record does
// not say, more than maxProjectKeys, or no call in any project (a turn only
// under /tmp or home reads the session's gates, as before the card, D40).
func turnProjects(run *store.Run, promptID string, at int64) []string {
	if promptID == "" {
		promptID = latestPrompt(run.Declarations)
	}
	var ps []string
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if d.PromptID == nil || *d.PromptID == "" || *d.PromptID != promptID || d.Seq > at {
			continue
		}
		switch p := projectOf(d); {
		case p == nil:
			return nil
		case *p == noProject || slices.Contains(ps, *p):
		case len(ps) == maxProjectKeys:
			return nil
		default:
			ps = append(ps, *p)
		}
	}
	return ps
}

// touches reports whether a call in one of projects may act on the files of
// a gate whose root is root (card F2): the call's project is root, inside it,
// or holds it (a session in the directory above its worktrees). Fail closed:
// nil projects, or a gate with no root, touches every gate.
func touches(projects []string, root string) bool {
	if projects == nil || root == "" {
		return true
	}
	root = slashed(root)
	for _, p := range projects {
		p = slashed(p)
		if _, in := under(p, root); in || p == root {
			return true
		}
		if _, in := under(root, p); in {
			return true
		}
	}
	return false
}

// markTurn marks each gate no call of the turn ran in the project of
// (touches): the row, the done verdict and the turn's record-green point pass
// over it, so one masked call in one worktree is not read into every later
// turn in another (card F2; eng-4's call 3060, 14 lines over seven hours).
// The gate keeps its state for the report and the next turn in its project.
// O(G x P), P bounded by maxProjectKeys.
func markTurn(gates []GateState, projects []string) {
	for i := range gates {
		gates[i].offTurn = !touches(projects, gates[i].root)
	}
}

// onTurnRoots is the done window's roots (doneWindow): those of the gates a
// call of the turn ran in the project of (markTurn, card F2), so an edit only
// a gate off the turn covers opens no window, and a masked call in another
// worktree is not read into the turn's done line through one (the manager's
// ruling of 11-03 09:3x; eng-4 9420). all is gateRoots': nil, every write,
// stays nil; no gate of the turn is no root, so no write opens a window.
func onTurnRoots(snap *Snapshot, all []string) []string {
	if all == nil {
		return nil
	}
	roots := []string{}
	for i := range snap.Gates {
		if g := &snap.Gates[i]; !g.offTurn && !slices.Contains(roots, g.root) {
			roots = append(roots, g.root)
		}
	}
	return roots
}

// maxProjectKeys bounds projectKeys: a run whose gate runs name more
// projects reads the rest's as not known, whose gates cover every write.
const maxProjectKeys = 255

// projectKeys is the distinct projects of a run's gate runs, in record
// order: a gate run's ProjectID is its project's place plus one, 0 for none
// known. A learned family's root is its last run's (stateOf). Linear: a
// run's gate runs name a handful of projects, and the bound keeps the walk
// O(R x maxProjectKeys) on one that names many.
type projectKeys []string

// id is p's place plus one, added where it is new and there is room; 0 for
// nil or past the bound.
func (k *projectKeys) id(p *string) uint8 {
	if p == nil {
		return 0
	}
	for i, s := range *k {
		if s == *p {
			return uint8(i + 1)
		}
	}
	if len(*k) >= maxProjectKeys {
		return 0
	}
	*k = append(*k, *p)
	return uint8(len(*k))
}

// key is the project id names, "" for 0 or an id k does not hold.
func (k projectKeys) key(id uint8) string {
	if id == 0 || int(id) > len(k) {
		return ""
	}
	return k[id-1]
}
