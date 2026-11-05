package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizeOrigin_Forms (W-M1.4, PLAN B2): the scheme and any user
// go, the host is lowercased, an scp-like or ssh destination reads as
// host/path, a trailing .git, slash, query or fragment goes, and the
// path's case stays.
func TestNormalizeOrigin_Forms(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/Karthik/Rashomon.git": "github.com/Karthik/Rashomon",
		"https://User:Pass@GitHub.COM/x/y/":       "github.com/x/y",
		"git@github.com:x/y.git":                  "github.com/x/y",
		"github.com:x/y":                          "github.com/x/y",
		"ssh://git@github.com/x/y.git":            "github.com/x/y",
		"ssh://git@GitHub.com:2222/x/y":           "github.com:2222/x/y",
		"git://host/x/y":                          "host/x/y",
		"https://host/x/y?ref=1#frag":             "host/x/y",
		"file:///tmp/Other.git":                   "/tmp/Other",
		"/tmp/other/":                             "/tmp/other",
		"../sibling.git":                          "../sibling",
		"  https://host/x/y  ":                    "host/x/y",
		"":                                        "",
	} {
		if got := NormalizeOrigin(in); got != want {
			t.Errorf("NormalizeOrigin(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestRepoID_ReadsTheOriginThroughAWorktree (W-M1.4, HD11): the id is the
// normalized url of the remote named origin in the repository's config,
// read from .git/config, from a directory below the root, and through a
// worktree's gitdir pointer and commondir file; it is not ephemeral.
func TestRepoID_ReadsTheOriginThroughAWorktree(t *testing.T) {
	main := t.TempDir()
	gitdir := filepath.Join(main, ".git")
	put(t, filepath.Join(gitdir, "config"), "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:Org/Repo.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n[branch \"main\"]\n\tremote = origin\n")
	put(t, filepath.Join(main, "sub", "keep"), "")
	const want = "github.com/Org/Repo"
	for name, dir := range map[string]string{"root": main, "below the root": filepath.Join(main, "sub")} {
		if rid, eph := RepoID(dir); rid != want || eph {
			t.Errorf("%s: RepoID = %q, %v; want %q, false", name, rid, eph, want)
		}
	}
	wt := t.TempDir()
	wtgit := filepath.Join(gitdir, "worktrees", "wt")
	put(t, filepath.Join(wtgit, "commondir"), "../..\n")
	put(t, filepath.Join(wtgit, "HEAD"), "ref: refs/heads/x\n")
	put(t, filepath.Join(wt, ".git"), "gitdir: "+wtgit+"\n")
	if rid, eph := RepoID(wt); rid != want || eph {
		t.Errorf("worktree: RepoID = %q, %v; want %q, false", rid, eph, want)
	}
}

// TestRepoID_PathFormWithoutAnOrigin (W-M1.4): with no remote named origin
// (none, or one by another name), or no repository at all, the id is
// path: and the sha256 of the project key, ephemeral, stable across calls
// and distinct between two directories.
func TestRepoID_PathFormWithoutAnOrigin(t *testing.T) {
	a := t.TempDir()
	put(t, filepath.Join(a, ".git", "config"), "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = https://h/x/y\n")
	b := t.TempDir()
	sum := sha256.Sum256([]byte(Key(a)))
	want := "path:" + hex.EncodeToString(sum[:])
	for name, dir := range map[string]string{"another remote": a, "no repository": b} {
		rid, eph := RepoID(dir)
		if !eph || len(rid) != len(want) || rid[:5] != "path:" {
			t.Errorf("%s: RepoID = %q, %v; want an ephemeral path form", name, rid, eph)
		}
		if again, _ := RepoID(dir); again != rid {
			t.Errorf("%s: RepoID changed between calls: %q then %q", name, rid, again)
		}
	}
	if rid, _ := RepoID(a); rid != want {
		t.Errorf("RepoID(a) = %q; want %q, the sha256 of its project key", rid, want)
	}
	if ra, _ := RepoID(a); ra == func() string { r, _ := RepoID(b); return r }() {
		t.Errorf("two directories share an ephemeral id")
	}
	if rid, eph := RepoID(""); rid != "" || !eph {
		t.Errorf("RepoID(\"\") = %q, %v; want nothing, ephemeral", rid, eph)
	}
}

// put writes text at path, making its directories.
func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGitDirs_ResolvesThePointerAndTheCommonDir (W-M1.8): a .git directory
// is its own git dir and common dir; a worktree's .git file points at its
// git dir, relative to the root or absolute, and that dir's commondir file
// names the common dir, relative to it or absolute; a directory with no
// .git, a .git file that is no pointer, and "" are no repository.
func TestGitDirs_ResolvesThePointerAndTheCommonDir(t *testing.T) {
	main := t.TempDir()
	gitdir := filepath.Join(main, ".git")
	put(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/main\n")
	if g, c, ok := GitDirs(main); !ok || g != gitdir || c != gitdir {
		t.Errorf("main: GitDirs = %q, %q, %v; want %q twice, true", g, c, ok, gitdir)
	}
	wt := t.TempDir()
	wtgit := filepath.Join(gitdir, "worktrees", "wt")
	put(t, filepath.Join(wtgit, "commondir"), "../..\n")
	put(t, filepath.Join(wt, ".git"), "gitdir: "+wtgit+"\n")
	if g, c, ok := GitDirs(wt); !ok || g != wtgit || c != gitdir {
		t.Errorf("worktree: GitDirs = %q, %q, %v; want %q, %q, true", g, c, ok, wtgit, gitdir)
	}
	rel := t.TempDir()
	put(t, filepath.Join(rel, ".git"), "gitdir: .gitdir\n")
	put(t, filepath.Join(rel, ".gitdir", "commondir"), gitdir+"\n")
	if g, c, ok := GitDirs(rel); !ok || g != filepath.Join(rel, ".gitdir") || c != gitdir {
		t.Errorf("relative pointer, absolute commondir: GitDirs = %q, %q, %v", g, c, ok)
	}
	none := t.TempDir()
	put(t, filepath.Join(none, ".git"), "not a pointer\n")
	for name, dir := range map[string]string{"no .git": t.TempDir(), "no pointer": none, "empty": ""} {
		if g, c, ok := GitDirs(dir); ok {
			t.Errorf("%s: GitDirs = %q, %q, true; want no repository", name, g, c)
		}
	}
}
