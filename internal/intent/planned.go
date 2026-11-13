package intent

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The plan path's reading of named paths (W-M2.6 hook half, finding D). An
// accepted plan names files it has not made yet; Resolve keeps only paths the
// project holds, so a file the plan would add reaches no rule. ResolvePlan is
// the one step that tells such a file apart, and only the plan path calls it:
// the prompt path (Resolve) is unchanged, so a prompt naming a missing file
// under an existing directory resolves as it always did.

// NamedPaths is the path-like tokens of text in the order named, read as
// Resolve reads a request: at most MaxPromptBytes of it.
func NamedPaths(text []byte) []string {
	cut := len(text) > MaxPromptBytes
	if cut {
		text = text[:MaxPromptBytes]
	}
	var out []string
	scan(text, cut, func(tok []byte) {
		if pathLike(tok) {
			out = append(out, string(tok))
		}
	})
	return out
}

// lookup is one Lstat answer kept for the budget's sake.
type lookup struct {
	info fs.FileInfo
	err  error
}

// ResolvePlan splits the named paths of a plan into those the project holds
// (existing: exactly what Resolve keeps for the same names) and the planned
// ones. A planned file is a root-relative named path that Lstat does not find
// and whose parent Lstat finds as a directory inside Root (the root itself
// counts, at no lookup). No absolute path outside Root, no ".." escape, and a
// parent that is a link is not a directory here, so none is followed. Every
// lookup, the existing paths' and the parents', is counted against the one
// budget of MaxLstat; a repeated lookup is answered from the first. At most
// MaxPaths paths are planned.
func ResolvePlan(named []string, p Project) (existing, planned []string) {
	if len(named) == 0 {
		return nil, nil
	}
	lstat := p.Lstat
	if lstat == nil {
		r, err := os.OpenRoot(p.Root)
		if err != nil {
			return nil, nil
		}
		defer r.Close()
		lstat = func(rel string) (fs.FileInfo, error) { return r.Lstat(filepath.FromSlash(rel)) }
	}
	seen := map[string]lookup{}
	spent := 0
	counted := func(rel string) (fs.FileInfo, error) {
		if l, ok := seen[rel]; ok {
			return l.info, l.err
		}
		spent++
		info, err := lstat(rel)
		seen[rel] = lookup{info, err}
		return info, err
	}
	p.Lstat = counted
	existing = resolvePaths(named, p)
	if len(existing) == 0 && spent == 0 {
		// Nothing was looked up (every name left the root): nothing to plan.
		return existing, nil
	}
	// look answers from the budget left, false when there is none.
	look := func(rel string) (lookup, bool) {
		if l, ok := seen[rel]; ok {
			return l, true
		}
		if spent >= MaxLstat {
			return lookup{}, false
		}
		info, err := counted(rel)
		return lookup{info, err}, true
	}
	for _, n := range named {
		if len(planned) == MaxPaths {
			break
		}
		cands := candidates(n, p.Root, p.Cwd)
		if slices.ContainsFunc(cands, func(c string) bool { return slices.Contains(existing, c) || slices.Contains(planned, c) }) {
			continue
		}
		for _, rel := range cands {
			if err := slot.Path(rel).Validate(); err != nil && !errors.Is(err, slot.ErrSecretShaped) {
				continue
			}
			l, ok := look(rel)
			if !ok {
				return existing, planned
			}
			if l.err == nil {
				break // held, though not kept (a link, a device, the cap)
			}
			if !errors.Is(l.err, fs.ErrNotExist) {
				continue
			}
			dir := path.Dir(rel)
			if dir != "." {
				pl, ok := look(dir)
				if !ok {
					return existing, planned
				}
				if pl.err != nil || !pl.info.IsDir() {
					continue
				}
			}
			planned = append(planned, rel)
			break
		}
	}
	return existing, planned
}
