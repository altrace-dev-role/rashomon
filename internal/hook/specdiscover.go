package hook

import (
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// errNoCWD is the flag of a call whose payload names no cwd: no spec, never
// a guess at one (card 3.1 Gate A 2).
const errNoCWD = "spec: the payload names no cwd"

// DiscoverSpec is the spec that applies to a call run in cwd (card 3.1 (c),
// D119, D128): spec.Discover over the OS, with the candidates
// spec.Locations names from cwd up to spec.WalkRoot of the .git root above
// cwd (gitRoot) and projectDir (CLAUDE_PROJECT_DIR), and the managed spec at
// managedDir/rashomon/spec.json ("" for none). cwd and projectDir are
// resolved as baseline.Key resolves a cwd, so the lexical walk compares one
// form (macOS's /tmp is /private/tmp). It is nil when nothing applies. An
// empty cwd or an I/O error is no spec and one flag, spec.Unreadable.
func DiscoverSpec(cwd, projectDir, managedDir string) *spec.Compiled {
	if cwd == "" {
		return spec.Unreadable(errNoCWD)
	}
	cwd = resolved(cwd)
	root := spec.WalkRoot(cwd, gitRoot(cwd), resolved(projectDir))
	managed := ""
	if managedDir != "" {
		managed = filepath.Join(managedDir, "rashomon", "spec.json")
	}
	sp, err := spec.Discover(spec.OSFS{}, spec.Locations(cwd, root), managed)
	if err != nil {
		return spec.Unreadable(err.Error())
	}
	return sp
}

// gitRoot is baseline.Key(cwd) when a .git is there, else "": with no .git,
// Key is cwd itself, which as a root would stop the walk at cwd and never
// reach a CLAUDE_PROJECT_DIR above it.
func gitRoot(cwd string) string {
	key := baseline.Key(cwd)
	if _, err := os.Lstat(filepath.Join(key, ".git")); err != nil {
		return ""
	}
	return key
}

// resolved is p with its symlinks evaluated, as baseline.Key resolves a cwd,
// else p as given.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
