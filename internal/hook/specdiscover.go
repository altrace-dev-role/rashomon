package hook

import (
	"crypto/rand"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// errNoCWD is the flag of a call whose payload names no cwd: no spec, never
// a guess at one (card 3.1 Gate A 2).
const errNoCWD = "spec: the payload names no cwd"

// DiscoverSpec is the spec that applies to a call run in cwd (card 3.1 (c),
// D119, D128): spec.Discover over the OS, with the candidates SpecLocations
// names, and the managed spec at managedDir/rashomon/spec.json ("" for none).
// The repository spec is trusted when the bindings file under root names its
// project, path and digest (card SPEC-TRUST1); that file is read only when a
// repository spec was, and an unreadable or unsigned one (D173), or a root
// that is "", binds nothing. It is nil when nothing applies. An empty cwd or
// an I/O error is no spec and one flag, spec.Unreadable. The root is a
// TrustedRoot, which only TrustRoot and NoRun make (card SPEC-TRUST2), so a
// caller cannot hand it a raw store path.
func DiscoverSpec(cwd, projectDir, managedDir string, root TrustedRoot) *spec.Compiled {
	return discoverSpecAt(cwd, baseline.Key(cwd), projectDir, managedDir, root)
}

// discoverSpecAt is DiscoverSpec given cwd's project key, baseline.Key(cwd),
// which the post's judge also reads the project index under: it walks the
// key once for both (card STALE-NUDGE1), where a second walk was most of the
// step the index read added to a judged post.
func discoverSpecAt(cwd, key, projectDir, managedDir string, root TrustedRoot) *spec.Compiled {
	if cwd == "" {
		return spec.Unreadable(errNoCWD)
	}
	managed := ""
	if managedDir != "" {
		managed = filepath.Join(managedDir, "rashomon", "spec.json")
	}
	bound := func(path, digest string) bool {
		if root.root == "" {
			return false
		}
		b, err := spec.ReadBindings(spec.OSFS{}, root.root)
		return err == nil && b.Holds(SpecProject(path), path, digest)
	}
	sp, err := spec.DiscoverBound(spec.OSFS{}, specLocationsAt(cwd, key, projectDir), managed, bound)
	if err != nil {
		return spec.Unreadable(err.Error())
	}
	return sp
}

// TrustedRoot is the store root DiscoverSpec reads trust bindings under,
// or "" for none (card SPEC-TRUST2). Its field is unexported, so a caller
// outside this package cannot set it: TrustRoot and NoRun are the only
// makers (H-142 pins that). The type does not stop a caller passing a raw
// store path through NoRun; what pins that is who may call it, an allowlist
// of one production file (H-142, cmd/rashomon/brief.go). Every other caller
// says which run's store writes it checked, through TrustRoot (D217).
type TrustedRoot struct{ root string }

// NoRun is the TrustedRoot of a caller that holds no run to check for a
// store write: root as it is. It is sound only where no write by the
// session being read can have happened, or where the caller is not bound to
// a session: the probe at SessionStart reads before the session's first
// call, and the brief it builds prints only at a run's first record. A
// caller that reads a session's own record has a run and uses TrustRoot.
func NoRun(root string) TrustedRoot { return TrustedRoot{root: root} }

// TrustRoot is the store root DiscoverSpec reads trust bindings under for a
// point at seq at of run: storeRoot, or "" once the session wrote under it
// (D173), so for the rest of a session that could have written its own
// binding, no repository spec reads as trusted. The steer step, the Stop and
// the prompt's preflight note (D203) read it between reading the run and
// discovering the spec. A nil run is no proof that the session wrote nothing
// under the store, so it reads as written (fail closed).
func TrustRoot(run *store.Run, storeRoot string, at int64) TrustedRoot {
	if run == nil || verify.StoreWritten(run, storeRoot, at) {
		return TrustedRoot{}
	}
	return TrustedRoot{root: storeRoot}
}

// WriteSpecBindings writes b as the bindings file under storeRoot, the one
// DiscoverSpec reads (store.ReplaceFile: 0600, renamed over the old one),
// signed under the store's trust key, which it makes on the first binding
// (specTrustKey). Only the verb calls it. An encoding spec.EncodeBindings
// refuses, over spec.MaxTrustBytes, writes nothing.
func WriteSpecBindings(storeRoot string, b *spec.Bindings) error {
	key, err := specTrustKey(storeRoot)
	if err != nil {
		return err
	}
	data, err := spec.EncodeBindings(b, key)
	if err != nil {
		return err
	}
	return store.ReplaceFile(filepath.Join(storeRoot, spec.TrustFile), data)
}

// specTrustKey is the store's trust key, spec.TrustKeyBytes random bytes at
// spec.TrustKeyFile (0600), learn's ensureKey at the store root: read, or
// made when absent or of another length, which leaves any bindings signed
// under the old one unsigned.
func specTrustKey(storeRoot string) ([]byte, error) {
	path := filepath.Join(storeRoot, spec.TrustKeyFile)
	if b, err := os.ReadFile(path); err == nil && len(b) == spec.TrustKeyBytes {
		return b, nil
	}
	key := make([]byte, spec.TrustKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := store.ReplaceFile(path, key); err != nil {
		return nil, err
	}
	return key, nil
}

// SpecLocations is the repository spec candidates for cwd: spec.Locations
// from cwd up to spec.WalkRoot of the .git root above cwd (gitRoot) and
// projectDir (CLAUDE_PROJECT_DIR), both resolved as baseline.Key resolves a
// cwd, so the lexical walk compares one form (macOS's /tmp is /private/tmp).
// `rashomon spec trust` names the spec it binds from the same list.
func SpecLocations(cwd, projectDir string) []string {
	return specLocationsAt(cwd, baseline.Key(cwd), projectDir)
}

// specLocationsAt is SpecLocations given cwd's project key, baseline.Key(cwd)
// (discoverSpecAt).
//
// With no git root and no project dir the walk has no root and no candidate
// exists (spec.WalkRoot, spec.Locations), so a project with no spec costs the
// one Lstat gitRoot makes: no symlink is evaluated and no path is built
// (card SPEC-RULES1, D210).
func specLocationsAt(cwd, key, projectDir string) []string {
	git := gitRoot(key)
	if git == "" && projectDir == "" {
		return nil
	}
	cwd = resolved(cwd)
	return spec.Locations(cwd, spec.WalkRoot(cwd, git, resolved(projectDir)))
}

// SpecProject is the project of the repository spec at path, the
// ProjectRoot of its trust binding: baseline.Key of the directory that holds
// .claude/rashomon. It is read from the path alone, so the hook, where
// CLAUDE_PROJECT_DIR is set, and a terminal, where it is not, name one root.
func SpecProject(path string) string {
	return baseline.Key(filepath.Dir(filepath.Dir(filepath.Dir(path))))
}

// gitRoot is key, a cwd's baseline.Key, when a .git is there, else "": with
// no .git, Key is cwd itself, which as a root would stop the walk at cwd and
// never reach a CLAUDE_PROJECT_DIR above it.
func gitRoot(key string) string {
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
