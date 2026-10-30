package hook

import (
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// errNoCWD is the flag of a call whose payload names no cwd: no spec, never
// a guess at one (card 3.1 Gate A 2).
const errNoCWD = "spec: the payload names no cwd"

// DiscoverSpec is the spec that applies to a call run in cwd (card 3.1 (c),
// D119, D128): spec.Discover over the OS, with the candidates SpecLocations
// names, and the managed spec at managedDir/rashomon/spec.json ("" for none).
// The repository spec is trusted when the bindings file under storeRoot
// names its project, path and digest (card SPEC-TRUST1); that file is read
// only when a repository spec was, and an unreadable one, or storeRoot "",
// binds nothing. It is nil when nothing applies. An empty cwd or an I/O error
// is no spec and one flag, spec.Unreadable.
func DiscoverSpec(cwd, projectDir, managedDir, storeRoot string) *spec.Compiled {
	if cwd == "" {
		return spec.Unreadable(errNoCWD)
	}
	managed := ""
	if managedDir != "" {
		managed = filepath.Join(managedDir, "rashomon", "spec.json")
	}
	bound := func(path, digest string) bool {
		if storeRoot == "" {
			return false
		}
		b, err := spec.ReadBindings(spec.OSFS{}, filepath.Join(storeRoot, spec.TrustFile))
		return err == nil && b.Holds(SpecProject(path), path, digest)
	}
	sp, err := spec.DiscoverBound(spec.OSFS{}, SpecLocations(cwd, projectDir), managed, bound)
	if err != nil {
		return spec.Unreadable(err.Error())
	}
	return sp
}

// WriteSpecBindings writes b as the bindings file under storeRoot, the one
// DiscoverSpec reads (store.ReplaceFile: 0600, renamed over the old one).
// An encoding spec.EncodeBindings refuses, over spec.MaxTrustBytes, writes
// nothing.
func WriteSpecBindings(storeRoot string, b *spec.Bindings) error {
	data, err := spec.EncodeBindings(b)
	if err != nil {
		return err
	}
	return store.ReplaceFile(filepath.Join(storeRoot, spec.TrustFile), data)
}

// SpecLocations is the repository spec candidates for cwd: spec.Locations
// from cwd up to spec.WalkRoot of the .git root above cwd (gitRoot) and
// projectDir (CLAUDE_PROJECT_DIR), both resolved as baseline.Key resolves a
// cwd, so the lexical walk compares one form (macOS's /tmp is /private/tmp).
// `rashomon spec trust` names the spec it binds from the same list.
func SpecLocations(cwd, projectDir string) []string {
	cwd = resolved(cwd)
	return spec.Locations(cwd, spec.WalkRoot(cwd, gitRoot(cwd), resolved(projectDir)))
}

// SpecProject is the project of the repository spec at path, the
// ProjectRoot of its trust binding: baseline.Key of the directory that holds
// .claude/rashomon. It is read from the path alone, so the hook, where
// CLAUDE_PROJECT_DIR is set, and a terminal, where it is not, name one root.
func SpecProject(path string) string {
	return baseline.Key(filepath.Dir(filepath.Dir(filepath.Dir(path))))
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
