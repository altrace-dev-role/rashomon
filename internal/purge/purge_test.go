package purge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fsRoot is the filesystem root of the volume t.TempDir is on: "/" on Unix,
// the drive's root on Windows.
func fsRoot(t *testing.T) (root, tmp string) {
	t.Helper()
	tmp = t.TempDir()
	return filepath.VolumeName(tmp) + string(filepath.Separator), tmp
}

// TestTRU29_CheckRootRefusesTheFilesystemRootHomeAndWhatEnclosesHome: the
// three roots no store can be, each refused by name, and the roots beside
// and below home that a store can be, each accepted -- including the one
// whose name only begins with home's.
func TestTRU29_CheckRootRefusesTheFilesystemRootHomeAndWhatEnclosesHome(t *testing.T) {
	top, tmp := fsRoot(t)
	home := filepath.Join(tmp, "home", "u")
	for _, c := range []struct {
		root, want string
	}{
		{top, "filesystem root"},
		{home, "is your home directory"},
		{filepath.Join(tmp, "home"), "encloses your home directory"},
		{tmp, "encloses your home directory"},
	} {
		err := CheckRoot(c.root, home)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("CheckRoot(%q, %q) = %v, want an error saying %q", c.root, home, err, c.want)
		}
	}
	for _, ok := range []string{
		filepath.Join(home, ".local", "state", "rashomon"),
		filepath.Join(tmp, "home", "u2"),
		filepath.Join(tmp, "elsewhere"),
	} {
		if err := CheckRoot(ok, home); err != nil {
			t.Errorf("CheckRoot(%q, %q) = %v, want nil", ok, home, err)
		}
	}
}

// TestTRU29_WorkTreeTopFindsAGitEntryOfEitherKind: a .git directory and a
// linked worktree's .git file both mark a top, found from any depth below
// it; with neither above, there is no work tree.
func TestTRU29_WorkTreeTopFindsAGitEntryOfEitherKind(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	linked := filepath.Join(tmp, "linked")
	plain := filepath.Join(tmp, "plain", "a")
	for _, d := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "a", "b"), filepath.Join(linked, "c"), plain} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := WorkTreeTop(filepath.Join(repo, "a", "b")); got != repo {
		t.Errorf("WorkTreeTop under a repository = %q, want %q", got, repo)
	}
	if got := WorkTreeTop(filepath.Join(linked, "c")); got != linked {
		t.Errorf("WorkTreeTop under a linked worktree = %q, want %q", got, linked)
	}
	if got := WorkTreeTop(plain); got != "" && !strings.HasPrefix(tmp, got) {
		t.Errorf("WorkTreeTop outside any work tree = %q, want none below %s", got, tmp)
	}
}

// TestTRU28_FilterCreatedDirsKeepsOnlyWhatWatchCreates: install.json is
// editable, so a created_dirs entry is removed only if it is the root, the
// configuration directory, or a parent of either -- and never home, a parent
// of home, a filesystem root or a relative path.
func TestTRU28_FilterCreatedDirsKeepsOnlyWhatWatchCreates(t *testing.T) {
	top, tmp := fsRoot(t)
	home := filepath.Join(tmp, "h")
	root := filepath.Join(home, ".local", "state", "rashomon")
	config := filepath.Join(home, ".claude")
	keep, refused := FilterCreatedDirs([]string{
		root, filepath.Join(home, ".local", "state"), filepath.Join(home, ".local"), config,
		home, tmp, top, filepath.Join(home, "Documents"), "relative", filepath.Join(root, "runs"),
	}, root, config, home)
	wantKeep := []string{root, filepath.Join(home, ".local", "state"), filepath.Join(home, ".local"), config}
	if !slices.Equal(keep, wantKeep) {
		t.Errorf("kept %q, want %q", keep, wantKeep)
	}
	if len(refused) != 6 {
		t.Errorf("refused %q, want the six that are not watch's to remove", refused)
	}
}

// storeAt lays out a store root holding one of each name rashomon writes,
// plus a file and a directory of the user's.
func storeAt(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "rashomon")
	for _, f := range []string{
		InstallMeta, InstallKey, "gaps.ndjson", "forgotten-hosts.ndjson", "pause", "recap.json",
		".install.json.tmp-123", "runs/s1/records.ndjson", "runs/s1/seq", "baseline/k.json",
		"mine.txt", "other/x",
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast: the plan holds every
// name rashomon writes, a directory after what is in it, install.json and
// then install.key at the end; the user's file and directory are foreign,
// and with them in it the root is not planned for removal.
func TestTRU28_PlanListsRashomonsNamesWithTheAnchorsLast(t *testing.T) {
	root := storeAt(t)
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	n := len(p.Files)
	if p.Files[n-2] != filepath.Join(root, InstallMeta) || p.Files[n-1] != filepath.Join(root, InstallKey) {
		t.Errorf("the plan does not end with install.json then install.key: %q", p.Files[n-2:])
	}
	at := func(rel string) int {
		return slices.Index(p.Files, filepath.Join(root, filepath.FromSlash(rel)))
	}
	for _, rel := range []string{"gaps.ndjson", "forgotten-hosts.ndjson", "pause", "recap.json", ".install.json.tmp-123",
		"runs/s1/records.ndjson", "runs/s1/seq", "runs/s1", "runs", "baseline/k.json", "baseline"} {
		if at(rel) < 0 {
			t.Errorf("the plan does not list %s", rel)
		}
	}
	if !(at("runs/s1/seq") < at("runs/s1") && at("runs/s1") < at("runs")) {
		t.Errorf("a directory comes before what is in it: %q", p.Files)
	}
	wantForeign := []string{filepath.Join(root, "mine.txt"), filepath.Join(root, "other")}
	if !slices.Equal(p.Foreign, wantForeign) {
		t.Errorf("foreign = %q, want %q", p.Foreign, wantForeign)
	}
	if p.RemoveRoot {
		t.Errorf("the root is planned for removal while it holds the user's files")
	}
	if at("mine.txt") >= 0 || at("other/x") >= 0 {
		t.Errorf("the plan lists the user's files: %q", p.Files)
	}
}

// TestTRU28_ExecuteKeepsTheAnchorsWhenAPathCannotGo: a file written into a
// planned directory after the plan was made -- a live hook's -- keeps that
// directory, and so keeps install.json, install.key and the root, which is
// what lets the next purge accept the root and finish. Nothing unplanned is
// removed.
func TestTRU28_ExecuteKeepsTheAnchorsWhenAPathCannotGo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rashomon")
	for _, f := range []string{InstallMeta, InstallKey, "runs/s1/records.ndjson"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, []string{root}, "")
	if err != nil {
		t.Fatal(err)
	}
	late := filepath.Join(root, "runs", "s1", "spill.ndjson")
	if err := os.WriteFile(late, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := p.Execute()
	for _, kept := range []string{InstallMeta, InstallKey} {
		if _, err := os.Stat(filepath.Join(root, kept)); err != nil {
			t.Errorf("%s was removed although a planned path could not be: %v", kept, err)
		}
	}
	if _, err := os.Stat(late); err != nil {
		t.Errorf("a file the plan never named was removed: %v", err)
	}
	for _, want := range []string{filepath.Join(root, "runs", "s1"), filepath.Join(root, InstallMeta), root} {
		if !slices.Contains(out.Remaining, want) {
			t.Errorf("Remaining %q does not name %s", out.Remaining, want)
		}
	}
	if !slices.Contains(out.Removed, filepath.Join(root, "runs", "s1", "records.ndjson")) {
		t.Errorf("Removed %q does not name the file that went", out.Removed)
	}
}

// TestTRU28_ExecuteRemovesTheEmptyRootThenCreatedDirsDeepestFirst: once
// everything is gone, the root goes, then each created directory that is
// empty, innermost first; one that is not empty is left, and is not counted
// as a planned path that remains.
func TestTRU28_ExecuteRemovesTheEmptyRootThenCreatedDirsDeepestFirst(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(home, ".local", "state")
	root := filepath.Join(state, "rashomon")
	config := filepath.Join(home, ".claude")
	for _, d := range []string{root, config} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(root, InstallMeta), filepath.Join(root, InstallKey), filepath.Join(config, "keep.json")} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	created := []string{filepath.Join(home, ".local"), state, root, config}
	p, err := Build(root, created, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{root, state, filepath.Join(home, ".local"), config}; !slices.Equal(p.CreatedDirs[:3], want[:3]) {
		t.Errorf("created dirs are not deepest first: %q", p.CreatedDirs)
	}
	out := p.Execute()
	if len(out.Remaining) != 0 {
		t.Errorf("Remaining = %q, want none", out.Remaining)
	}
	for _, gone := range []string{root, state, filepath.Join(home, ".local")} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s is still there", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(config, "keep.json")); err != nil {
		t.Errorf("a created directory that was not empty lost its contents: %v", err)
	}
	if !slices.Equal(out.Left, []string{config}) {
		t.Errorf("Left = %q, want the one created directory that was not empty", out.Left)
	}
}
