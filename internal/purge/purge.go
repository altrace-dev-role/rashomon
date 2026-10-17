// Package purge plans and carries out `rashomon purge`'s removal of a store
// (Technical spec 7.2.9): which roots it refuses, which paths under a root
// are rashomon's, and the order they go in so that a purge that is
// interrupted can be run again to completion.
//
// It removes nothing it did not list in a Plan first, and a Plan lists only
// names rashomon writes. A file a user put in the store root is left, and so
// is the root that holds it.
package purge

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The two files that make a directory a store. They are removed last, so
// a purge that stops early leaves a root the next purge still accepts.
const (
	// InstallMeta is the install record, removed second to last.
	InstallMeta = store.FileInstallMeta
	// InstallKey is the per-install key, removed last.
	InstallKey = store.FileInstallKey
)

// topFiles are the files rashomon writes directly under the store root, other
// than the two anchors: the gap log, the forgotten-host digests, the pause
// marker, and recap's state and its temporary file. Each name is the writing
// package's own constant, so a rename there cannot strand the file here.
var topFiles = []string{store.FileGaps, store.FileForgottenHosts, store.FileRecordingPaused, recap.StateFileName, recap.StateTempName}

// topDirs are the directories under the store root that only rashomon
// writes into: every path under them is planned for removal.
var topDirs = []string{store.DirRuns, baseline.DirName}

// tempPrefixes are the names of an anchor's temporary file, left by a
// process that died between writing and renaming it.
var tempPrefixes = []string{"." + InstallMeta + ".tmp-", "." + InstallKey + ".tmp-"}

// ownName reports whether a name directly under the store root is one
// rashomon writes.
func ownName(name string) bool {
	for _, n := range topFiles {
		if name == n {
			return true
		}
	}
	for _, p := range tempPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// CheckRoot refuses a root that is a filesystem root, is home, or encloses
// home. Both paths are absolute and clean. Its caller checks each spelling
// of the two it has (as given and with symlinks resolved), so a root that
// reaches home through a link is refused as home.
func CheckRoot(root, home string) error {
	if filepath.Dir(root) == root {
		return fmt.Errorf("%s is a filesystem root; refusing to purge it", root)
	}
	if home == "" {
		return nil
	}
	if root == home {
		return fmt.Errorf("%s is your home directory; refusing to purge it", root)
	}
	if Encloses(root, home) {
		return fmt.Errorf("%s encloses your home directory %s; refusing to purge it", root, home)
	}
	return nil
}

// Encloses reports whether dir is a proper ancestor of path. Both are
// absolute and clean.
func Encloses(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// WorkTreeTop returns the top of the git work tree dir is inside, or "" when
// it is in none. A .git entry of either kind marks a top: a directory for a
// repository, a file for a linked worktree. It walks up rather than running
// git, which this program does not do (TRU-40).
func WorkTreeTop(dir string) string {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// FilterCreatedDirs keeps the created_dirs entries purge may remove: the
// root, an ancestor of it, the Claude configuration directory, or an
// ancestor of that -- the directories watch creates -- and never home, an
// ancestor of home or a filesystem root. install.json is a file the user (and
// the agent) can edit; what it names is not taken on trust.
func FilterCreatedDirs(dirs []string, root, configDir, home string) (keep, refused []string) {
	for _, d := range dirs {
		d = filepath.Clean(d)
		watchMade := d == root || Encloses(d, root) || d == configDir || Encloses(d, configDir)
		protected := !filepath.IsAbs(d) || filepath.Dir(d) == d || (home != "" && (d == home || Encloses(d, home)))
		if watchMade && !protected {
			keep = append(keep, d)
		} else {
			refused = append(refused, d)
		}
	}
	return keep, refused
}

// Plan is everything a purge will remove, in the order it removes it, and
// what it found and will leave.
type Plan struct {
	Root string
	// Files are rashomon's paths under Root, deepest first, with the two
	// anchors last: install.json, then install.key.
	Files []string
	// RemoveRoot is whether Root goes once it is empty: false when it
	// holds something rashomon did not write.
	RemoveRoot bool
	// Foreign are the entries directly under Root that rashomon did not
	// write. They are left, and so is Root.
	Foreign []string
	// CreatedDirs are removed once empty, deepest first, after Root.
	CreatedDirs []string
	// NotDirs are created_dirs entries that are no longer a directory --
	// a symlink the user put in its place, most often. They are left.
	NotDirs []string
	// Binary, when set, is removed last.
	Binary string
}

// Build lists what a purge of root removes. It reads root and changes
// nothing.
func Build(root string, createdDirs []string, binary string) (Plan, error) {
	p := Plan{Root: root, Binary: binary, RemoveRoot: true}
	entries, err := os.ReadDir(root)
	if err != nil {
		return p, err
	}
	var files []string
	for _, ent := range entries {
		name := ent.Name()
		path := filepath.Join(root, name)
		switch {
		case name == InstallMeta || name == InstallKey:
			// Added last, below.
		case isTopDir(name):
			sub, err := subtree(path)
			if err != nil {
				return p, err
			}
			files = append(files, sub...)
		case ownName(name):
			files = append(files, path)
		default:
			p.Foreign = append(p.Foreign, path)
		}
	}
	// Deepest first, so a directory comes after everything in it.
	sort.SliceStable(files, func(i, j int) bool { return depth(files[i]) > depth(files[j]) })
	p.Files = append(files, filepath.Join(root, InstallMeta), filepath.Join(root, InstallKey))
	p.RemoveRoot = len(p.Foreign) == 0

	for _, d := range createdDirs {
		if dir, err := realDir(d); err == nil && !dir {
			p.NotDirs = append(p.NotDirs, d)
		} else {
			p.CreatedDirs = append(p.CreatedDirs, d)
		}
	}
	sort.SliceStable(p.CreatedDirs, func(i, j int) bool { return depth(p.CreatedDirs[i]) > depth(p.CreatedDirs[j]) })
	return p, nil
}

func isTopDir(name string) bool {
	for _, d := range topDirs {
		if name == d {
			return true
		}
	}
	return false
}

// subtree lists dir and every path under it. A symlink is listed as itself
// and never followed: removing it removes the link.
func subtree(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

func depth(p string) int { return strings.Count(p, string(filepath.Separator)) }

// realDir reports whether path is a directory itself, not a symlink to one
// or a file. A path that does not exist is not one, and is not an error.
func realDir(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsDir(), nil
}

// Outcome is what Execute did.
type Outcome struct {
	// Removed are the paths that went, or were already gone.
	Removed []string
	// Remaining are planned paths still there while install.json and
	// install.key are: a rerun of purge accepts the root and finishes them.
	Remaining []string
	// Stranded are planned paths still there once the anchors have gone. A
	// rerun refuses a root without them, so each has to be removed by hand.
	Stranded []string
	// Left are created directories left because something is in them.
	Left []string
	// NotDir are created directories that had become something else -- a
	// symlink, most often -- by the time purge reached them. They are left.
	NotDir []string
}

// Execute removes the plan, and only the plan. A path that is already gone
// counts as removed, which is what makes a rerun of an interrupted purge
// finish instead of failing.
//
// When any file before the anchors cannot be removed -- a directory a live
// hook has written into since the plan was made is the usual one -- the
// anchors are kept, and so is the root: install.json and install.key are
// what lets the next purge accept this root, so they go only once
// everything else has, and install.key stays while install.json does.
// Whatever fails after install.json has gone is Stranded.
func (p Plan) Execute() Outcome {
	var o Outcome
	anchors := p.Files[len(p.Files)-2:]
	for _, path := range p.Files[:len(p.Files)-2] {
		if remove(path, &o.Removed) != nil {
			o.Remaining = append(o.Remaining, path)
		}
	}
	kept := anchors
	if len(o.Remaining) == 0 {
		fault.Inject(fault.PointPurgeBeforeAnchors)
		if remove(anchors[0], &o.Removed) == nil {
			kept = nil
		}
	}
	if kept != nil {
		o.Remaining = append(o.Remaining, kept...)
		if p.RemoveRoot {
			o.Remaining = append(o.Remaining, p.Root)
		}
		if p.Binary != "" {
			o.Remaining = append(o.Remaining, p.Binary)
		}
		return o
	}

	// install.json is gone: from here a rerun refuses the root.
	strand := func(path string) {
		if remove(path, &o.Removed) != nil {
			o.Stranded = append(o.Stranded, path)
		}
	}
	strand(anchors[1])
	if p.RemoveRoot && len(o.Stranded) == 0 {
		strand(p.Root)
	}
	for _, d := range p.CreatedDirs {
		p.removeCreatedDir(d, &o)
	}
	if p.Binary != "" {
		strand(p.Binary)
	}
	return o
}

// removeCreatedDir removes one created directory if it is still a directory
// and is empty. A symlink in its place is left: os.Remove would unlink it
// whatever it points at.
func (p Plan) removeCreatedDir(d string, o *Outcome) {
	dir, err := realDir(d)
	switch {
	case err != nil:
		o.Stranded = append(o.Stranded, d)
		return
	case !dir:
		if _, err := os.Lstat(d); err == nil {
			o.NotDir = append(o.NotDir, d)
		}
		return
	}
	err = os.Remove(d)
	switch {
	case err == nil || errors.Is(err, fs.ErrNotExist):
		o.Removed = append(o.Removed, d)
	case notEmpty(d):
		o.Left = append(o.Left, d)
	default:
		o.Stranded = append(o.Stranded, d)
	}
}

// notEmpty reports whether dir can be read and holds an entry: the one
// reason a created directory is left without its being a failure.
func notEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// remove removes one planned path and adds it to removed when it went or
// was already gone.
func remove(path string, removed *[]string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		*removed = append(*removed, path)
		return nil
	}
	return err
}
