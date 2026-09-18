package baseline

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

// TestKey_FindsTheGitRootByWalkingUp covers the ordinary case and pins the
// method: no subprocess. os/exec is forbidden in this program's dependency
// graph, so `git rev-parse` is not available to it even though it would be the
// obvious way to do this.
func TestKey_FindsTheGitRootByWalkingUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}

	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := Key(deep); got != want {
		t.Errorf("Key(%q) = %q, want the git root %q", deep, got, want)
	}
}

// TestKey_WorktreeGitFileCountsAsARoot: in a git worktree .git is a FILE, not a
// directory. Checking only for a directory would walk past it and key every
// worktree to its parent, merging several projects' baselines into one.
func TestKey_WorktreeGitFileCountsAsARoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	want, _ := filepath.EvalSymlinks(root)
	if got := Key(sub); got != want {
		t.Errorf("Key(%q) = %q, want %q; in a worktree .git is a file", sub, got, want)
	}
}

// TestKey_NoGitRootFallsBackToTheDirectory keeps a session outside any
// repository in its own project rather than keying it to the filesystem root,
// which would merge every such session into one baseline.
func TestKey_NoGitRootFallsBackToTheDirectory(t *testing.T) {
	dir := t.TempDir()
	want, _ := filepath.EvalSymlinks(dir)
	if got := Key(dir); got != want {
		t.Errorf("Key(%q) = %q, want %q", dir, got, want)
	}
}

// TestKey_ResolvesSymlinks is the macOS case that would otherwise double every
// baseline: /tmp is a symlink to /private/tmp, so the same directory arrives
// under two names and would become two projects, each reporting the other's
// hosts as novel.
func TestKey_ResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if Key(link) != Key(real) {
		t.Errorf("Key(%q) = %q and Key(%q) = %q; one directory under two names must be "+
			"one project", link, Key(link), real, Key(real))
	}
}

// TestUpdate_FirstSessionEstablishesTheBaseline: the first session in a project
// has nothing to be novel against, so every host it reached would be reported
// as new -- true and useless.
func TestUpdate_FirstSessionEstablishesTheBaseline(t *testing.T) {
	root := t.TempDir()
	res := Update(root, "/proj", "sess-1", t0, []string{"a.example", "b.example"})

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !res.Established {
		t.Error("established = false on the first session in a project")
	}
	if len(res.Novel) != 0 {
		t.Errorf("novel = %v on the establishing session, want empty", res.Novel)
	}
	if res.Known != 2 {
		t.Errorf("known = %d, want 2", res.Known)
	}
}

// TestUpdate_SecondSessionsNewHostIsNovel is the positive fixture.
func TestUpdate_SecondSessionsNewHostIsNovel(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example"})

	res := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "new.example"})

	if res.Established {
		t.Error("established = true on the second session")
	}
	if !reflect.DeepEqual(res.Novel, []string{"new.example"}) {
		t.Errorf("novel = %v, want [new.example]", res.Novel)
	}
}

// TestUpdate_NoFireWhenEveryHostWasSeenBefore is the negative fixture, and the
// case that must hold on the overwhelming majority of sessions.
func TestUpdate_NoFireWhenEveryHostWasSeenBefore(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example", "b.example"})

	res := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "b.example"})
	if len(res.Novel) != 0 {
		t.Errorf("novel = %v, want empty: every host was reached in the previous session",
			res.Novel)
	}
}

// TestUpdate_UbiquitousHostsAreNeverNovel. Their first appearance in a project
// is not information: every Go, Node or Python project reaches them, so a line
// that fired on them would fire on nearly every second session and be ignored
// from then on.
func TestUpdate_UbiquitousHostsAreNeverNovel(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example"})

	res := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{
		"pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "github.com",
		"api.github.com", "objects.githubusercontent.com", "proxy.golang.org",
		"sum.golang.org", "formulae.brew.sh", "ghcr.io",
		"genuinely-new.example",
	})

	if !reflect.DeepEqual(res.Novel, []string{"genuinely-new.example"}) {
		t.Errorf("novel = %v, want only [genuinely-new.example]; the ten ubiquitous hosts "+
			"must never be novel", res.Novel)
	}
}

// TestUpdate_RerenderIsStable is the determinism property from the spec: a
// session re-rendered after a LATER session reached its hosts must produce the
// same novel set as the first time.
//
// This is why ownership is keyed on the session's start time and recorded in
// the file, rather than being decided by which render ran first.
func TestUpdate_RerenderIsStable(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example"})

	first := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "x.example"})
	// A later session reaches the same hosts.
	Update(root, "/proj", "sess-3", t0.Add(2*time.Hour), []string{"a.example", "x.example"})
	// Now re-render session 2.
	again := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "x.example"})

	if !reflect.DeepEqual(first.Novel, again.Novel) {
		t.Errorf("re-render of sess-2 gave novel %v, first render gave %v; a session's novel "+
			"set must be a property of the session, not of the order reports were read in",
			again.Novel, first.Novel)
	}
	if !reflect.DeepEqual(again.Novel, []string{"x.example"}) {
		t.Errorf("novel = %v, want [x.example]", again.Novel)
	}
}

// TestUpdate_EarlierSessionRenderedLaterTakesOwnership covers rendering out of
// chronological order. Correcting toward the earlier session is what makes the
// stability above hold in both directions.
func TestUpdate_EarlierSessionRenderedLaterTakesOwnership(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-0", t0.Add(-time.Hour), []string{"seed.example"})

	// The later session is rendered first and provisionally owns the host.
	later := Update(root, "/proj", "sess-late", t0.Add(time.Hour), []string{"x.example"})
	if !reflect.DeepEqual(later.Novel, []string{"x.example"}) {
		t.Fatalf("premise: later.Novel = %v, want [x.example]", later.Novel)
	}

	// Then the earlier one is rendered and should take it.
	earlier := Update(root, "/proj", "sess-early", t0, []string{"x.example"})
	if !reflect.DeepEqual(earlier.Novel, []string{"x.example"}) {
		t.Errorf("earlier.Novel = %v, want [x.example]: the earlier session reached it first",
			earlier.Novel)
	}
	// And the later one no longer claims it.
	againLater := Update(root, "/proj", "sess-late", t0.Add(time.Hour), []string{"x.example"})
	if len(againLater.Novel) != 0 {
		t.Errorf("later.Novel = %v after the earlier session was rendered, want empty",
			againLater.Novel)
	}
}

// TestUpdate_TwoProjectsDoNotShareABaseline. A shared baseline would report one
// project's ordinary hosts as novel in the other and suppress genuinely new
// ones.
func TestUpdate_TwoProjectsDoNotShareABaseline(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj-a", "a-1", t0, []string{"a.example"})
	Update(root, "/proj-b", "b-1", t0, []string{"b.example"})

	res := Update(root, "/proj-b", "b-2", t0.Add(time.Hour), []string{"a.example"})
	if !reflect.DeepEqual(res.Novel, []string{"a.example"}) {
		t.Errorf("novel = %v, want [a.example]: project A's host is new to project B",
			res.Novel)
	}
}

// TestFileName_DistinctKeysNeverCollide is why the filename carries a digest.
// Sanitising alone maps /a/b and /a-b to the same name, which would merge two
// projects' baselines.
func TestFileName_DistinctKeysNeverCollide(t *testing.T) {
	if fileName("/a/b") == fileName("/a-b") {
		t.Error("/a/b and /a-b share a baseline filename; sanitising alone is not enough")
	}
	if fileName("/proj") != fileName("/proj") {
		t.Error("fileName is not deterministic")
	}
}

// TestUpdate_PermissionsAreOwnerOnly. A baseline is every host a project has
// ever reached -- a longer-lived and more revealing artefact than any single
// run, and a hostname is the most sensitive field this product holds.
func TestUpdate_PermissionsAreOwnerOnly(t *testing.T) {
	root := t.TempDir()
	res := Update(root, "/proj", "sess-1", t0, []string{"a.example"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	info, err := os.Stat(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("baseline file mode %04o, want 0600", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(res.Path))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Errorf("baseline dir mode %04o, want 0700", dir.Mode().Perm())
	}
}

// TestUpdate_CorruptBaselineReportsAnErrorRatherThanTreatingAllAsNovel.
// Silently starting fresh would make every host novel and read as a finding
// storm; the caller renders "unknown" instead.
func TestUpdate_CorruptBaselineReportsAnErrorRatherThanTreatingAllAsNovel(t *testing.T) {
	root := t.TempDir()
	res := Update(root, "/proj", "sess-1", t0, []string{"a.example"})
	if err := os.WriteFile(res.Path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	again := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "b.example"})
	if again.Err == nil {
		t.Error("a corrupt baseline produced no error; novelty would render as a fact")
	}
	if len(again.Novel) != 0 {
		t.Errorf("novel = %v on a corrupt baseline, want empty so the caller can say unknown",
			again.Novel)
	}
}

// TestForget_RemovesTheHostFromTheBaseline is what keeps `forget --host` whole:
// evicting the wire rows while the baseline still remembers the host would
// leave it suppressed as "seen before" forever, with nothing left to show why.
func TestForget_RemovesTheHostFromTheBaseline(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example", "gone.example"})

	if err := Forget(root, "/proj", "gone.example"); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// A later session reaching it again finds it novel, because the baseline no
	// longer remembers it.
	res := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"gone.example"})
	if !reflect.DeepEqual(res.Novel, []string{"gone.example"}) {
		t.Errorf("novel = %v, want [gone.example] after the baseline entry was forgotten",
			res.Novel)
	}

	body, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(body), "gone.example") {
		t.Error("premise check: the host should be back in the file via sess-2")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestUpdate_EstablishedIsAPropertyOfTheSessionNotTheRender is the other half of
// TestUpdate_RerenderIsStable, which pinned the novel SET across renders and let
// this flag through.
//
// Deciding it from whether the file existed makes it a property of the render:
// the first render creates the file, so the second render of the SAME session
// reports something different, and `report` -- which looks like a query --
// changes its own answer by being run. Found by an acceptance test asserting
// that two renders of one store agree.
func TestUpdate_EstablishedIsAPropertyOfTheSessionNotTheRender(t *testing.T) {
	root := t.TempDir()

	first := Update(root, "/proj", "sess-1", t0, []string{"a.example"})
	if !first.Established {
		t.Fatal("premise: the first session in a project establishes the baseline")
	}

	again := Update(root, "/proj", "sess-1", t0, []string{"a.example"})
	if !again.Established {
		t.Error("re-rendering the establishing session reports the baseline as already " +
			"established, so running report twice gives two different answers for one session")
	}
	if len(again.Novel) != 0 {
		t.Errorf("again.Novel = %v; the establishing session has nothing to be novel "+
			"against, in either render", again.Novel)
	}
}

// TestUpdate_ALaterSessionDoesNotInheritEstablished guards the fix from the
// obvious wrong version of itself: keying "established" on the file's contents
// rather than on the session would make every session in the project report
// that it established the baseline, and novelty would never fire again.
func TestUpdate_ALaterSessionDoesNotInheritEstablished(t *testing.T) {
	root := t.TempDir()
	Update(root, "/proj", "sess-1", t0, []string{"a.example"})

	later := Update(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"a.example", "x.example"})

	if later.Established {
		t.Error("a later session reports that it established the baseline, which would " +
			"suppress its novelty line")
	}
	if !reflect.DeepEqual(later.Novel, []string{"x.example"}) {
		t.Errorf("later.Novel = %v, want [x.example]", later.Novel)
	}
}

// TestUpdate_EstablishedSurvivesAPreFixBaselineFile. A baseline written before
// the establishing session was recorded names nobody, and the safe reading is
// that nobody established it: novelty is then computed normally rather than
// suppressed for a session that cannot be identified as the first.
func TestUpdate_EstablishedSurvivesAPreFixBaselineFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A v1 file with no establishing session, as the previous code wrote it.
	body := `{"version":1,"project":"/proj","hosts":{"a.example":` +
		`{"first_seen_session":"sess-old","first_seen_at_unix_ms":1}}}`
	if err := os.WriteFile(filepath.Join(dir, fileName("/proj")), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res := Update(root, "/proj", "sess-new", t0, []string{"x.example"})

	if res.Err != nil {
		t.Fatalf("a baseline from the previous version was rejected: %v", res.Err)
	}
	if res.Established {
		t.Error("a session reports establishing a baseline that already existed")
	}
	if !reflect.DeepEqual(res.Novel, []string{"x.example"}) {
		t.Errorf("novel = %v, want [x.example]: novelty must still be computed", res.Novel)
	}
}
