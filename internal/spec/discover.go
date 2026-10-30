package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Discovery (README 3.1; archive 20.2.8, Location and discovery): which spec
// files apply to a call, and the one Compiled they make. WalkRoot, Locations
// and Discover are the pure core, a path walk and a merge over an FS; OSFS and
// owner (owner_unix.go, owner_other.go) are the shell.

// MaxWalkDepth caps the directories Locations names.
const MaxWalkDepth = 64

// SourceImplied is the Source of a declaration the tool implies rather than a
// file declares (3.2's task-file rule is the first); TrustOf reads it as
// Builtin.
const SourceImplied = -1

// Declared is one gate of the merged spec. ID is "spec:<id>" for a declared
// gate, "ci:<n>" for the repository spec's ci_commands entry n and
// "ci:managed:<n>" for the managed spec's, so the two never share an id.
// Source indexes Compiled.Sources. A ci_commands entry's Gate is the gate
// Parse would give a declared one with that argv: kind custom, match prefix,
// required; its ID is empty, and Declared.ID names it.
type Declared struct {
	ID     string
	Source int
	Gate   Gate
}

// DeclaredRule is one rule of the merged spec, ID "spec:<id>".
type DeclaredRule struct {
	ID     string
	Source int
	Rule   Rule
}

// DeclaredFact is one fact of the merged spec, ID "spec:<id>".
type DeclaredFact struct {
	ID     string
	Source int
	Fact   Fact
}

// Dropped is one repository declaration that a managed one of the same ID
// shadowed; SourceDigest is the repository spec's digest. The user gets one
// flag each (archive 20.2.8).
type Dropped struct {
	ID           string
	SourceDigest string
}

// FS is what Discover reads through: a file's bytes, and a path's owner and
// mode as lstat reports them, a symlink as itself. A test passes a map.
type FS interface {
	ReadFile(path string) ([]byte, error)
	Owner(path string) (uid int, mode fs.FileMode, err error)
}

// OSFS is the operating system's FS. ReadFile refuses anything but a regular
// file before opening it, so a FIFO a repository plants cannot block the
// hook, and reads one byte past MaxBytes at most, which Parse refuses as too
// large.
type OSFS struct{}

// ReadFile implements FS.
func (OSFS) ReadFile(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("spec: %s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxBytes+1))
}

// Owner implements FS.
func (OSFS) Owner(path string) (int, fs.FileMode, error) { return owner(path) }

// WalkRoot is the directory Locations stops at: the nearer to cwd of
// projectRoot (the first .git, baseline.Key) and projectDir
// (CLAUDE_PROJECT_DIR), each counted only when it is at or above cwd, which
// must be absolute; "" when neither is, and then Locations names nothing. The
// caller passes the three in one form (all with symlinks resolved, or none).
func WalkRoot(cwd, projectRoot, projectDir string) string {
	if !filepath.IsAbs(cwd) {
		return ""
	}
	cwd = filepath.Clean(cwd)
	root := ""
	for _, r := range [2]string{projectRoot, projectDir} {
		if r = filepath.Clean(r); within(cwd, r) && len(r) > len(root) {
			root = r
		}
	}
	return root
}

// Locations is the candidate repository spec paths,
// <dir>/.claude/rashomon/spec.json for each directory from cwd up to root
// inclusive, nearest first and at most MaxWalkDepth of them. Both paths are
// cleaned first, so ".." cannot climb past root; a cwd that is not root or
// below it, or a path that is not absolute, gives nothing. It reads no file.
func Locations(cwd, root string) []string {
	if !filepath.IsAbs(cwd) || !filepath.IsAbs(root) {
		return nil
	}
	cwd, root = filepath.Clean(cwd), filepath.Clean(root)
	if !within(cwd, root) {
		return nil
	}
	// A capacity hint, one short when root is the volume root.
	below := strings.Count(cwd[len(root):], string(filepath.Separator))
	out := make([]string, 0, min(below+1, MaxWalkDepth))
	for dir := cwd; len(out) < MaxWalkDepth; dir = filepath.Dir(dir) {
		out = append(out, filepath.Join(dir, ".claude", "rashomon", "spec.json"))
		if dir == root {
			break
		}
	}
	return out
}

// within reports whether the clean path p is root or below it.
func within(p, root string) bool {
	if !strings.HasPrefix(p, root) {
		return false
	}
	return len(p) == len(root) || p[len(root)] == filepath.Separator || root[len(root)-1] == filepath.Separator
}

// Discover reads the spec files that apply to a call and merges them (README
// 3.1, rules 1 to 5): the managed spec at managedPath ("" for none) when it is
// admin-owned, then the first of repoCandidates that exists (Locations' order,
// so the nearest wins). A file Parse refuses is an Errors line and applies
// nothing, and the walk does not go on to a farther one. Discover returns nil,
// nil when no file applies and none was refused, else a Compiled. The error
// is an I/O failure other than a missing file; the caller then has no spec
// and raises one flag. The task file (README 3.2) is TaskFile beside the
// repository spec that was read or, with none, under the farthest
// candidate's directory, the project root; a task counts as a file that
// applies, and its failures are discovery.task's. The repository spec is
// untrusted: Discover is DiscoverBound with no binding.
func Discover(fsys FS, repoCandidates []string, managedPath string) (*Compiled, error) {
	return DiscoverBound(fsys, repoCandidates, managedPath, nil)
}

// DiscoverBound is Discover with the user's trust bindings (card
// SPEC-TRUST1): the repository spec it reads is Trusted when bound reports a
// binding of its path and digest, else Untrusted. bound is called at most
// once, and only when a repository spec was read; nil binds nothing.
func DiscoverBound(fsys FS, repoCandidates []string, managedPath string, bound func(path, digest string) bool) (*Compiled, error) {
	var d discovery
	if managedPath != "" {
		if err := d.managed(fsys, managedPath); err != nil {
			return nil, err
		}
	}
	near := "" // the candidate the task file sits beside
	for _, path := range repoCandidates {
		data, err := fsys.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		near = path
		d.add(path, data, Untrusted)
		break
	}
	if n := len(d.c.Sources); n > 0 && near != "" && bound != nil {
		if s := &d.c.Sources[n-1]; s.Path == near && bound(near, s.Digest) {
			s.Trust = Trusted
		}
	}
	if near == "" && len(repoCandidates) > 0 {
		near = repoCandidates[len(repoCandidates)-1]
	}
	if near != "" {
		d.task(fsys, taskPath(near))
	}
	if len(d.c.Sources) == 0 && len(d.c.Errors) == 0 && d.c.Task == nil {
		return nil, nil
	}
	return d.compile(), nil
}

// Unreadable is the Compiled of a discovery the caller could not finish
// (Discover's error, or a call with no cwd): no source, and reason as its one
// Errors line, the user's one flag. Its Digest and Trust are any sourceless
// Compiled's.
func Unreadable(reason string) *Compiled {
	d := discovery{c: Compiled{Errors: []string{reason}}}
	return d.compile()
}

// discovery is Discover's state: the Compiled so far and, parallel to its
// Sources, each source's File.
type discovery struct {
	c     Compiled
	files []*File
}

// managed adds the managed spec when it exists and it and its directory are
// admin-owned; one that is not is an Errors line and is never read.
func (d *discovery) managed(fsys FS, path string) error {
	uid, mode, err := fsys.Owner(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dirUID, dirMode, err := fsys.Owner(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !adminOwned(uid, mode, dirUID, dirMode) {
		d.c.Errors = append(d.c.Errors, path+": managed spec ignored: not admin-owned")
		return nil
	}
	data, err := fsys.ReadFile(path)
	if err != nil {
		return err
	}
	d.add(path, data, Managed)
	return nil
}

// adminOwned is rule 2: the file is a regular file and its directory a
// directory, both owned by uid 0 and neither group- nor world-writable. A
// symlink at either is not admin-owned, since its target may be anyone's.
func adminOwned(fileUID int, fileMode fs.FileMode, dirUID int, dirMode fs.FileMode) bool {
	file := fileUID == 0 && fileMode.IsRegular() && fileMode.Perm()&0o022 == 0
	dir := dirUID == 0 && dirMode.IsDir() && dirMode.Perm()&0o022 == 0
	return file && dir
}

// add parses one file: a source when Parse accepts it, an Errors line naming
// the path and Parse's error when not.
func (d *discovery) add(path string, data []byte, trust Trust) {
	f, digest, err := Parse(data)
	if err != nil {
		d.c.Errors = append(d.c.Errors, path+": "+err.Error())
		return
	}
	d.c.Sources = append(d.c.Sources, Source{Path: path, Digest: digest, Trust: trust})
	d.files = append(d.files, f)
}

// compile finishes the Compiled: Digest over the source digests in Sources
// order (each is DigestPrefix and 64 hex digits, so their concatenation is
// unambiguous), Trust the lowest folded over the sources, untrusted with
// none, each source merged in turn, and then the rule a task file implies
// (taskRule).
func (d *discovery) compile() *Compiled {
	c := &d.c
	h := sha256.New()
	for _, s := range c.Sources {
		io.WriteString(h, s.Digest)
	}
	c.Digest = DigestPrefix + hex.EncodeToString(h.Sum(nil))
	c.Trust = Untrusted
	if len(c.Sources) > 0 {
		c.Trust = c.Sources[0].Trust
		for _, s := range c.Sources[1:] {
			c.Trust = Lowest(c.Trust, s.Trust)
		}
	}
	for i, f := range d.files {
		c.merge(i, f)
	}
	if c.Task != nil {
		c.Rules = append(c.Rules, taskRule())
	}
	return c
}

// merge adds source src's declarations after those of the sources before it
// (rule 3, managed first): a gate, rule or fact whose ID an earlier source
// declared goes to Dropped instead. ci_commands entries are numbered per
// source in their own id space, so they never collide. The holdout is the
// first source's that has the object (a parsed one has a RateBP) and is
// managed or trusted: an untrusted repository spec's holdout object is not
// read, so a repository cannot opt its user into a session whose notes
// reach neither the agent nor the user.
func (c *Compiled) merge(src int, f *File) {
	ci := "ci:"
	if c.Sources[src].Trust == Managed {
		ci = "ci:managed:"
	}
	for _, g := range f.Gates {
		id := "spec:" + g.ID
		if slices.ContainsFunc(c.Gates, func(d Declared) bool { return d.ID == id }) {
			c.drop(id, src)
			continue
		}
		c.Gates = append(c.Gates, Declared{ID: id, Source: src, Gate: g})
	}
	for n, argv := range f.CICommands {
		required := true
		g := Gate{Kind: "custom", Argv: argv, Match: "prefix", Required: &required}
		c.Gates = append(c.Gates, Declared{ID: ci + strconv.Itoa(n), Source: src, Gate: g})
	}
	for _, r := range f.Rules {
		id := "spec:" + r.ID
		if slices.ContainsFunc(c.Rules, func(d DeclaredRule) bool { return d.ID == id }) {
			c.drop(id, src)
			continue
		}
		c.Rules = append(c.Rules, DeclaredRule{ID: id, Source: src, Rule: r})
	}
	for _, x := range f.Facts {
		id := "spec:" + x.ID
		if slices.ContainsFunc(c.Facts, func(d DeclaredFact) bool { return d.ID == id }) {
			c.drop(id, src)
			continue
		}
		c.Facts = append(c.Facts, DeclaredFact{ID: id, Source: src, Fact: x})
	}
	if t := c.Sources[src].Trust; c.Holdout.RateBP == 0 && (t == Managed || t == Trusted) {
		c.Holdout = f.Holdout
	}
}

func (c *Compiled) drop(id string, src int) {
	c.Dropped = append(c.Dropped, Dropped{ID: id, SourceDigest: c.Sources[src].Digest})
}

// TrustOf is the trust of a declaration's source: Sources[source].Trust, or
// Builtin for SourceImplied, which needs no source. Any other source outside
// Sources panics, as an index does.
func (c *Compiled) TrustOf(source int) Trust {
	if source == SourceImplied {
		return Builtin
	}
	return c.Sources[source].Trust
}
