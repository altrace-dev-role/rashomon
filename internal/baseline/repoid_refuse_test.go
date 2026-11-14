package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// treeOrigin is the origin gitTree's config names, as RepoID reads it.
const treeOrigin = "host.example/org/repo"

// gitTree is a worktree of a main repository, each directory with its
// symlinks resolved, so a path RepoID reads is the one named here: the
// worktree's .git file points at its git directory, whose commondir file
// names the main .git, whose config holds the origin. files names the three
// files RepoID reads: the pointer, commondir and config.
func gitTree(t *testing.T) (wt string, files map[string]string) {
	t.Helper()
	resolved := func() string {
		d, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	gitdir := filepath.Join(resolved(), ".git")
	wtgit := filepath.Join(gitdir, "worktrees", "wt")
	wt = resolved()
	files = map[string]string{
		"pointer":   filepath.Join(wt, ".git"),
		"commondir": filepath.Join(wtgit, "commondir"),
		"config":    filepath.Join(gitdir, "config"),
	}
	put(t, files["config"], "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n")
	put(t, files["commondir"], "../..\n")
	put(t, files["pointer"], "gitdir: "+wtgit+"\n")
	return wt, files
}

// grow pads the file at path to size bytes with text its reader skips: a
// comment line for a config, spaces for the pointer and commondir files,
// which their reads trim (and a single byte as a space for any of them).
func grow(t *testing.T, path string, size int) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	switch n := size - len(b); {
	case n < 1:
		t.Fatalf("%s is %d bytes; cannot grow it to %d", path, len(b), size)
	case n >= 2 && filepath.Base(path) == "config":
		b = append(b, "#"+strings.Repeat("x", n-2)+"\n"...)
	default:
		b = append(b, strings.Repeat(" ", n)...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRepoID_EachGitReadIsCapped: each of the three
// files RepoID reads is read up to MaxGitFileBytes. At the cap the origin
// reads as before; one byte over it RepoIDErr is an error naming the file
// and the cap, and RepoID is no id ("", ephemeral), never the path form,
// which would key the repository's rows apart from its origin's. Break:
// drop any one file's cap, and its row reads the origin one byte over.
func TestRepoID_EachGitReadIsCapped(t *testing.T) {
	for _, name := range []string{"pointer", "commondir", "config"} {
		t.Run(name, func(t *testing.T) {
			wt, files := gitTree(t)
			grow(t, files[name], MaxGitFileBytes)
			if rid, eph, err := RepoIDErr(wt); rid != treeOrigin || eph || err != nil {
				t.Fatalf("control, at the cap: RepoIDErr = %q, %v, %v; want %q, false, nil", rid, eph, err, treeOrigin)
			}
			grow(t, files[name], MaxGitFileBytes+1)
			rid, eph, err := RepoIDErr(wt)
			if rid != "" || !eph || err == nil || !strings.Contains(err.Error(), files[name]) ||
				!strings.Contains(err.Error(), strconv.Itoa(MaxGitFileBytes)) {
				t.Errorf("one byte over: RepoIDErr = %q, %v, %v; want no id and an error naming %s and %d",
					rid, eph, err, files[name], MaxGitFileBytes)
			}
			if rid, eph := RepoID(wt); rid != "" || !eph {
				t.Errorf("one byte over: RepoID = %q, %v; want no id, never the path form", rid, eph)
			}
		})
	}
}

// TestGitDirs_RefusedCommondirIsNoFallback: a
// commondir that is there and refused, here a directory in its place, is
// GitDirsErr's error matching regfile.ErrNotRegular, GitDirs false and
// RepoID no id. It is never a fall back to the worktree's own git
// directory, whose config may name another origin: before this change any
// commondir read error fell back, and this worktree read other.example's.
// The controls, on the same tree: commondir a regular file reads the main
// config's origin, and no commondir at all falls back to the git directory,
// as git does. Break: fall back on any commondir error.
func TestGitDirs_RefusedCommondirIsNoFallback(t *testing.T) {
	wt, files := gitTree(t)
	wtgit := filepath.Dir(files["commondir"])
	put(t, filepath.Join(wtgit, "config"), "[remote \"origin\"]\n\turl = https://other.example/x/y.git\n")
	if rid, _ := RepoID(wt); rid != treeOrigin {
		t.Fatalf("control, commondir a regular file: RepoID = %q; want %q", rid, treeOrigin)
	}
	if err := os.Remove(files["commondir"]); err != nil {
		t.Fatal(err)
	}
	if rid, _ := RepoID(wt); rid != "other.example/x/y" {
		t.Fatalf("control, no commondir: RepoID = %q; want the git directory's own origin, other.example/x/y", rid)
	}
	if err := os.Mkdir(files["commondir"], 0o755); err != nil {
		t.Fatal(err)
	}
	if g, c, err := GitDirsErr(wt); g != "" || c != "" || !errors.Is(err, regfile.ErrNotRegular) {
		t.Errorf("commondir a directory: GitDirsErr = %q, %q, %v; want nothing and an error matching %v", g, c, err, regfile.ErrNotRegular)
	}
	if g, c, ok := GitDirs(wt); ok {
		t.Errorf("commondir a directory: GitDirs = %q, %q, true; want false", g, c)
	}
	if rid, eph := RepoID(wt); rid != "" || !eph {
		t.Errorf("commondir a directory: RepoID = %q, %v; want no id, not the worktree's own config", rid, eph)
	}
}

// TestRepoID_APathThroughAFileIsAbsent: a .git pointer that names a regular
// file as its git directory leaves commondir and config at paths through a
// file, which the open reports as ENOTDIR on unix and as not found on
// windows. Both are absent, as a missing file is, not refused: no repository
// git would read, so RepoID is the path form and RepoIDErr no error, as
// before this change. A pin on that behaviour, so it passes before the
// change too. Break: read ENOTDIR as a refusal, and RepoID is no id.
func TestRepoID_APathThroughAFileIsAbsent(t *testing.T) {
	wt, files := gitTree(t)
	notADir := filepath.Join(filepath.Dir(files["pointer"]), "plain")
	put(t, notADir, "a regular file\n")
	put(t, files["pointer"], "gitdir: "+notADir+"\n")
	rid, eph, err := RepoIDErr(wt)
	if !strings.HasPrefix(rid, "path:") || !eph || err != nil {
		t.Errorf("a pointer to a regular file: RepoIDErr = %q, %v, %v; want the path form, ephemeral, nil", rid, eph, err)
	}
}

// TestRepoID_SymlinkedConfigReadsTheOrigin: a config that is a
// symbolic link to a regular file reads as that file, as os.ReadFile read
// it. regfile.Open follows a link (it leaves O_NOFOLLOW out) and refuses
// only what the link reaches when that is not a regular file. A pin on the
// behaviour before this change, so it passes before it too. Break: refuse
// a link, and the origin is lost.
func TestRepoID_SymlinkedConfigReadsTheOrigin(t *testing.T) {
	wt, files := gitTree(t)
	b, err := os.ReadFile(files["config"])
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "shared-config")
	put(t, elsewhere, string(b))
	if err := os.Remove(files["config"]); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, files["config"]); err != nil {
		t.Skipf("no symbolic link here: %v", err)
	}
	if rid, eph, err := RepoIDErr(wt); rid != treeOrigin || eph || err != nil {
		t.Errorf("config a link to a regular file: RepoIDErr = %q, %v, %v; want %q, false, nil", rid, eph, err, treeOrigin)
	}
}
