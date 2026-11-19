package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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
		"  https://host/x/y  ":                    "host/x/y",
		"":                                        "",
	} {
		if got := NormalizeOrigin(in); got != want {
			t.Errorf("NormalizeOrigin(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestNormalizeOrigin_DropsADefaultPort (graph-5): a port that is its
// scheme's default, or empty, is dropped (RFC 3986, section 6.2.3), for each
// scheme git fetches over: ssh and its aliases git+ssh and ssh+git 22, git
// 9418, http 80, https 443, ftp 21, ftps 990, the scheme's case folded and
// the port read as a number. So the ssh, scp-like and https forms of one
// repository are one id, and two checkouts of it by two of those forms share
// it. Any other port, or a port on a scheme outside that list, is kept, and
// a bracketed IPv6 host keeps its colons. Break: keep the default port, drop
// every port, or cut the host at its last colon without the bracket check.
func TestNormalizeOrigin_DropsADefaultPort(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:org/repo.git":            "github.com/org/repo",
		"ssh://git@github.com:22/org/repo.git":   "github.com/org/repo",
		"https://github.com:443/org/repo":        "github.com/org/repo",
		"SSH://git@GitHub.com:22/org/repo":       "github.com/org/repo",
		"git+ssh://git@host.example:22/org/repo": "host.example/org/repo",
		"ssh+git://host.example:22/org/repo":     "host.example/org/repo",
		"git://host.example:9418/org/repo":       "host.example/org/repo",
		"http://host.example:80/org/repo":        "host.example/org/repo",
		"ftp://host.example:21/org/repo":         "host.example/org/repo",
		"ftps://host.example:990/org/repo":       "host.example/org/repo",
		"ssh://host.example:/org/repo":           "host.example/org/repo",
		"ssh://host.example:022/org/repo":        "host.example/org/repo",
		"ssh://git@[::1]:22/org/repo":            "[::1]/org/repo",
		"ssh://git@[::1]/org/repo":               "[::1]/org/repo",
		"ssh://git@GitHub.com:2222/x/y":          "github.com:2222/x/y",
		"https://host.example:22/org/repo":       "host.example:22/org/repo",
		"ssh://host.example:443/org/repo":        "host.example:443/org/repo",
		"web+x://host.example:443/org/repo":      "host.example:443/org/repo",
	} {
		if got := NormalizeOrigin(in); got != want {
			t.Errorf("NormalizeOrigin(%q) = %q; want %q", in, got, want)
		}
	}
	a, b := t.TempDir(), t.TempDir()
	put(t, filepath.Join(a, ".git", "config"), "[remote \"origin\"]\n\turl = ssh://git@github.com:22/org/repo.git\n")
	put(t, filepath.Join(b, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:org/repo.git\n")
	ra, ea := RepoID(a)
	rb, eb := RepoID(b)
	if ra != "github.com/org/repo" || rb != ra || ea || eb {
		t.Errorf("one repository by its ssh and scp-like urls: RepoID = %q, %v and %q, %v; want github.com/org/repo, false twice", ra, ea, rb, eb)
	}
}

// TestNormalizeOrigin_RelativePathIsNoID (graph-4): a local path that is not
// absolute names another repository from each directory it is read in, so
// it is no repository id: "", and RepoID gives the checkout's path form
// (TestRepoID_RelativeOriginTakesThePathForm). An absolute path stays an id.
// Break: return the relative path, or refuse an absolute one.
func TestNormalizeOrigin_RelativePathIsNoID(t *testing.T) {
	for in, want := range map[string]string{
		"../sibling.git":       "",
		"./sibling":            "",
		"sibling":              "",
		`..\sibling`:           "",
		".":                    "",
		"/srv/git/sibling.git": "/srv/git/sibling",
	} {
		if got := NormalizeOrigin(in); got != want {
			t.Errorf("NormalizeOrigin(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestNormalizeOrigin_DriveLetterReadsAsAHost (x35.7): a pin of a limit,
// not a want. A Windows drive-letter path as the origin reads as an
// scp-like destination whose host is the drive letter: C:/r/x.git is
// c//r/x and C:\r\x.git is c/\r\x, so one directory spelled with either
// slash gets two ids, a split rather than a merge. A fix changes these rows
// on purpose.
func TestNormalizeOrigin_DriveLetterReadsAsAHost(t *testing.T) {
	for in, want := range map[string]string{
		"C:/r/x.git": "c//r/x",
		`C:\r\x.git`: `c/\r\x`,
	} {
		if got := NormalizeOrigin(in); got != want {
			t.Errorf("NormalizeOrigin(%q) = %q; want %q, the pinned limit", in, got, want)
		}
	}
}

// TestNormalizeOrigin_KeepsNoUserPart: a user name or password holding a
// delimiter (/ ? # @ : or a space) leaves no byte of itself in the id, in the
// scheme, scp-like and bare forms. A marker on each side of the delimiter,
// inside the secret, appears neither in NormalizeOrigin's result nor in
// RepoID's over a real .git/config, and no result holds an @; a refused url
// ("") gives the path form. Each of the 30 results is one of two:
// the host and path, host.example/org/repo, or "", so a result that holds
// neither marker nor @ and is still not the host and path (a user name kept
// with no marker in it, the host alone) fails too. The control: each form
// with a plain secret, which reads as its host and path where the secret
// follows a scheme or is an scp-like user name alone, and is refused where
// an @ follows the host part. Break: drop any of the three refusals.
func TestNormalizeOrigin_KeepsNoUserPart(t *testing.T) {
	const left, right = "lmark7", "rmark7" // lowercase, as the host is lowercased
	forms := []struct{ name, url, plain string }{
		{"scheme, user and password", "https://user:SECRET@host.example/org/repo.git", "host.example/org/repo"},
		{"scheme, user", "https://SECRET@host.example/org/repo.git", "host.example/org/repo"},
		{"scp-like, user", "SECRET@host.example:org/repo.git", "host.example/org/repo"},
		{"scp-like, user and password", "user:SECRET@host.example:org/repo.git", ""},
		{"bare", "//user:SECRET@host.example/org/repo.git", ""},
	}
	delims := []string{"/", "?", "#", "@", ":", " "}
	n := 0
	for _, f := range forms {
		if got := NormalizeOrigin(strings.Replace(f.url, "SECRET", "s3cret", 1)); got != f.plain {
			t.Errorf("control, %s: a plain secret reads %q; want %q", f.name, got, f.plain)
		}
		for _, d := range delims {
			url := strings.Replace(f.url, "SECRET", left+d+right, 1)
			dir := t.TempDir()
			put(t, filepath.Join(dir, ".git", "config"), "[remote \"origin\"]\n\turl = "+url+"\n")
			norm := NormalizeOrigin(url)
			if norm != "" && norm != "host.example/org/repo" {
				t.Errorf("%s, delimiter %q: NormalizeOrigin(%q) = %q; want host.example/org/repo or \"\"", f.name, d, url, norm)
			}
			rid, eph := RepoID(dir)
			for what, got := range map[string]string{"NormalizeOrigin": norm, "RepoID": rid} {
				low := strings.ToLower(got)
				if strings.Contains(low, left) || strings.Contains(low, right) || strings.Contains(got, "@") {
					t.Errorf("%s, delimiter %q: %s of %q = %q keeps the user part", f.name, d, what, url, got)
				}
			}
			if norm != "" && (rid != norm || eph) || norm == "" && (!eph || !strings.HasPrefix(rid, "path:")) {
				t.Errorf("%s, delimiter %q: RepoID = %q, %v; want %q, false, or the path form where it is refused", f.name, d, rid, eph, norm)
			}
			n++
		}
	}
	if want := len(forms) * len(delims); n != want {
		t.Fatalf("the table ran %d urls; want %d", n, want)
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

// TestRepoID_RelativeOriginTakesThePathForm (graph-4): two checkouts in two
// directories whose origin is one relative url get two ids, each its own
// path form, ephemeral, with no error: a relative url names another
// repository from each directory, and a false split costs less than a false
// merge, which would key two repositories' rows as one. The limit: two
// clones of one repository by a relative url get two ids too. The control:
// one absolute url in both gives one id, not ephemeral. Break: give a
// relative url as the id.
func TestRepoID_RelativeOriginTakesThePathForm(t *testing.T) {
	pathForm := func(dir string) string {
		sum := sha256.Sum256([]byte(Key(dir)))
		return "path:" + hex.EncodeToString(sum[:])
	}
	for _, url := range []string{"../upstream", "../upstream.git", "./upstream", "upstream"} {
		var ids []string
		for _, dir := range []string{t.TempDir(), t.TempDir()} {
			put(t, filepath.Join(dir, ".git", "config"), "[remote \"origin\"]\n\turl = "+url+"\n")
			rid, eph, err := RepoIDErr(dir)
			if want := pathForm(dir); rid != want || !eph || err != nil {
				t.Errorf("origin %q: RepoIDErr = %q, %v, %v; want the checkout's path form %q, true, nil", url, rid, eph, err, want)
			}
			ids = append(ids, rid)
		}
		if ids[0] == ids[1] {
			t.Errorf("origin %q: two checkouts in two directories share the id %q", url, ids[0])
		}
	}
	var ids []string
	for _, dir := range []string{t.TempDir(), t.TempDir()} {
		put(t, filepath.Join(dir, ".git", "config"), "[remote \"origin\"]\n\turl = /srv/git/upstream.git\n")
		rid, eph, err := RepoIDErr(dir)
		if rid != "/srv/git/upstream" || eph || err != nil {
			t.Errorf("control, an absolute origin: RepoIDErr = %q, %v, %v; want /srv/git/upstream, false, nil", rid, eph, err)
		}
		ids = append(ids, rid)
	}
	if ids[0] != ids[1] {
		t.Errorf("control: one absolute origin gives two ids, %q and %q", ids[0], ids[1])
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
