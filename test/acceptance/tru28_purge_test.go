package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TRU-28 and TRU-29 (Technical spec 7.2.9): purge removes what rashomon put on
// the machine, refuses a root it cannot prove is a store, and never removes
// a path it did not print first.
//
// Every test here runs the binary against a HOME that is a t.TempDir, with
// the store and the Claude configuration at their default places under it,
// because purge deletes files and HOME is what its refusals are measured
// against. The real HOME is never handed to the binary: env.fakeHome is set
// before the first invocation and environ passes it on every one.

// newPurgeEnv is a machine whose HOME is empty and whose store and Claude
// configuration will be created by watch at their default places under it.
func newPurgeEnv(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	return &env{
		t:         t,
		home:      filepath.Join(home, ".local", "state", "rashomon"),
		configDir: filepath.Join(home, ".claude"),
		cwd:       t.TempDir(),
		fakeHome:  home,
	}
}

func (e *env) purge(args ...string) result {
	e.t.Helper()
	return e.run("", nil, append([]string{"purge"}, args...)...)
}

// snapshot is every path under dir with what it is: a directory, a symlink
// and its target, or a file and the digest of its bytes and its mode.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[path] = "symlink " + target
		case d.IsDir():
			out[path] = "dir"
		default:
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(body)
			out[path] = "file " + info.Mode().Perm().String() + " " + hex.EncodeToString(sum[:])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotting %s: %v", dir, err)
	}
	return out
}

// diffSnapshots names every path whose state differs between two snapshots.
func diffSnapshots(before, after map[string]string) []string {
	var out []string
	for p, v := range before {
		if w, ok := after[p]; !ok {
			out = append(out, "removed "+p)
		} else if w != v {
			out = append(out, "changed "+p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			out = append(out, "added "+p)
		}
	}
	sort.Strings(out)
	return out
}

// plannedRemovals reads the paths purge printed under "this removes", before
// it asked anything or removed anything.
func plannedRemovals(t *testing.T, stdout string) map[string]bool {
	t.Helper()
	planned := map[string]bool{}
	in := false
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case line == purgeRemovesHeader:
			in = true
			continue
		case !strings.HasPrefix(line, "  "):
			in = false
			continue
		}
		if in {
			p := strings.TrimSuffix(strings.TrimPrefix(line, "  "), purgeIfEmpty)
			planned[p] = true
		}
	}
	if len(planned) == 0 {
		t.Fatalf("purge printed no plan:\n%s", stdout)
	}
	return planned
}

// The plan's own words, as purge prints them.
const (
	purgeRemovesHeader = "rashomon purge removes:"
	purgeIfEmpty       = " (if empty)"
)

// session runs one of everything a watched machine writes into its store: a
// session's probes, a call declared and executed, a recap, a pause and a
// resume, and a report, which writes the project baseline.
func (e *env) session(t *testing.T) {
	t.Helper()
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	if res := e.recap(stopPayload(testSession, "done", false)); res.exitCode != 0 {
		t.Fatalf("recap: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, args := range [][]string{{"pause"}, {"resume"}, {"report", "--session", testSession}} {
		if res := e.run("", nil, args...); res.exitCode != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, res.exitCode, res.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(e.home, "baseline")); err != nil {
		t.Fatalf("premise: the report wrote no baseline, so purge is not shown removing one: %v", err)
	}
}

// TestTRU28_PurgeLeavesHomeAsFound: on a HOME that held nothing, watch, a
// whole session, and purge leave it holding nothing -- no store, no
// settings file, no directory watch created for either.
func TestTRU28_PurgeLeavesHomeAsFound(t *testing.T) {
	e := newPurgeEnv(t)
	before := snapshot(t, e.fakeHome)
	e.session(t)

	res := e.purge("--yes")
	if res.exitCode != 0 {
		t.Fatalf("purge --yes: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}
	if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
		t.Errorf("purge did not leave HOME as found:\n  %s\nstdout:\n%s", strings.Join(d, "\n  "), res.stdout)
	}
	if !strings.Contains(res.stdout, "rashomon: purge complete") {
		t.Errorf("purge did not say it completed:\n%s", res.stdout)
	}
	// The binary is kept without --binary, and the plan says how to remove it.
	if _, err := os.Stat(rashomonBin); err != nil {
		t.Fatalf("purge without --binary removed the binary: %v", err)
	}
	if !strings.Contains(res.stdout, "--binary") {
		t.Errorf("the plan does not say how to remove the binary it leaves:\n%s", res.stdout)
	}
}

// TestTRU28_PurgeRemovesNothingOutsideThePlan: a HOME already holding a
// settings file with another tool's hook and a permission, other tools'
// state beside the store, and a file of the user's inside the store root.
// Every path purge removes was printed before it removed anything; the
// settings file is back to the bytes it held before watch; everything that
// is not rashomon's is untouched.
func TestTRU28_PurgeRemovesNothingOutsideThePlan(t *testing.T) {
	e := newPurgeEnv(t)
	// Laid out the way Claude Code writes the file, the layout today's editor
	// renders back byte for byte (TRU-44 covers one it cannot).
	settingsBefore := "{\n  \"permissions\": {\"allow\": [\"Bash(ls:*)\"]},\n  \"hooks\": {\n    \"PreToolUse\": [\n      " +
		foreignEntry + "\n    ]\n  }\n}\n"
	if err := os.MkdirAll(e.configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	e.writeSettings(settingsBefore)
	other := filepath.Join(e.fakeHome, ".local", "state", "other-tool", "state.json")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte(`{"x":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.fakeHome, "notes.txt"), []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.session(t)
	usersFile := filepath.Join(e.home, "mine.txt")
	if err := os.WriteFile(usersFile, []byte("not rashomon's\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, e.fakeHome)
	res := e.purge("--yes")
	if res.exitCode != 0 {
		t.Fatalf("purge --yes: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}
	after := snapshot(t, e.fakeHome)
	planned := plannedRemovals(t, res.stdout)
	for p := range before {
		if _, kept := after[p]; !kept && !planned[p] {
			t.Errorf("purge removed %s, which its plan never named", p)
		}
	}
	if got := string(e.settingsBytes()); got != settingsBefore {
		t.Errorf("settings.json is not back to its bytes before watch:\n got %q\nwant %q", got, settingsBefore)
	}
	for _, kept := range []string{other, filepath.Join(e.fakeHome, "notes.txt"), usersFile} {
		if after[kept] != before[kept] || after[kept] == "" {
			t.Errorf("purge touched %s, which is not rashomon's", kept)
		}
	}
	for _, gone := range []string{"install.json", "install.key", "runs", "baseline"} {
		if _, err := os.Lstat(filepath.Join(e.home, gone)); err == nil {
			t.Errorf("purge left %s in the store root", gone)
		}
	}
	if !strings.Contains(res.stdout, usersFile) {
		t.Errorf("purge did not name the file it left in the store root:\n%s", res.stdout)
	}
}

// TestTRU28_InterruptedPurgeCompletesOnRerun: a purge that dies after its
// detach, or with every data file gone but install.json and install.key,
// can be run again, and the second run finishes the job.
func TestTRU28_InterruptedPurgeCompletesOnRerun(t *testing.T) {
	for _, point := range []string{"purge.after_detach", "purge.before_anchors"} {
		t.Run(point, func(t *testing.T) {
			e := newPurgeEnv(t)
			before := snapshot(t, e.fakeHome)
			e.session(t)

			first := e.run("", []string{"RASHOMON_FAULT=" + point + ":plain_panic"}, "purge", "--yes")
			if first.exitCode != 1 {
				t.Fatalf("interrupted purge: exit %d, want 1\nstdout:\n%s\nstderr: %q", first.exitCode, first.stdout, first.stderr)
			}
			for _, anchor := range []string{"install.json", "install.key"} {
				if _, err := os.Stat(filepath.Join(e.home, anchor)); err != nil {
					t.Fatalf("an interrupted purge removed %s, so it cannot be run again: %v", anchor, err)
				}
			}
			if strings.Contains(string(e.settingsBytes()), " --install ") {
				t.Fatalf("the interrupted purge had not detached yet; the premise is a purge that stopped after it")
			}

			second := e.purge("--yes")
			if second.exitCode != 0 {
				t.Fatalf("rerun: exit %d\nstdout:\n%s\nstderr: %q", second.exitCode, second.stdout, second.stderr)
			}
			if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
				t.Errorf("the rerun did not finish the purge:\n  %s\nstdout:\n%s", strings.Join(d, "\n  "), second.stdout)
			}
		})
	}
}

// TestTRU28_PurgeRefusesWhileThePluginIsEnabled: the plugin's next hook
// would treat a missing store as a first run and create it again, so purge
// refuses while the plugin is enabled, names the /plugin step, and writes
// nothing -- enabledPlugins included.
func TestTRU28_PurgeRefusesWhileThePluginIsEnabled(t *testing.T) {
	e := newPurgeEnv(t)
	e.session(t)
	p := newPluginFixture(t, "rashomon@local")
	p.installed(t, e)
	p.enable(t, e, true)
	before := snapshot(t, e.fakeHome)

	res := e.purge("--yes")
	if res.exitCode != 1 {
		t.Fatalf("purge under an enabled plugin: exit %d, want 1\nstdout:\n%s", res.exitCode, res.stdout)
	}
	if !strings.Contains(res.stderr, "/plugin disable rashomon@local") {
		t.Errorf("the refusal does not name the /plugin step: %q", res.stderr)
	}
	if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
		t.Errorf("a refused purge changed HOME:\n  %s", strings.Join(d, "\n  "))
	}

	p.enable(t, e, false)
	if res := e.purge("--yes"); res.exitCode != 0 {
		t.Fatalf("purge once the plugin is disabled: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(e.home, "install.json")); err == nil {
		t.Errorf("purge with the plugin disabled left the store")
	}
}

// TestTRU28_DetachedEntryCreatesNoStore: a live session keeps the entries it
// started with, so after purge its next hook still runs `hook --install
// <id>`. That invocation exits 0 and creates nothing: no root, no key, no
// install id.
func TestTRU28_DetachedEntryCreatesNoStore(t *testing.T) {
	e := newPurgeEnv(t)
	e.session(t)
	args := e.installArgs()
	if res := e.purge("--yes"); res.exitCode != 0 {
		t.Fatalf("purge --yes: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}

	for _, sub := range [][]string{{"hook"}, {"post"}, {"probe", "start"}, {"probe", "end"}, {"recap"}} {
		payload := defaultPayload().build(t)
		switch sub[0] {
		case "post":
			payload = defaultPost().build(t)
		case "probe":
			payload = e.sessionPayload(sub[1], testSession)
		case "recap":
			payload = stopPayload(testSession, "done", false)
		}
		res := e.run(payload, nil, append(sub, args...)...)
		if res.exitCode != 0 || res.stdout != "" {
			t.Errorf("%v after purge: exit %d, stdout %q; a detached entry exits 0 and says nothing", sub, res.exitCode, res.stdout)
		}
		if _, err := os.Lstat(e.home); err == nil {
			t.Fatalf("%v after purge created %s", sub, e.home)
		}
	}
}

// TestTRU28_PurgeBinaryAlsoRemovesTheBinary: --binary removes the binary
// that ran, last, and only that.
func TestTRU28_PurgeBinaryAlsoRemovesTheBinary(t *testing.T) {
	e := newPurgeEnv(t)
	bin := copyBinary(t, rashomonBin, filepath.Join(t.TempDir(), "bin", "rashomon"))
	if res := e.runBin(bin, "", "watch", "--yes"); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	res := e.runBin(bin, "", "purge", "--yes", "--binary")
	if res.exitCode != 0 {
		t.Fatalf("purge --yes --binary: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}
	if _, err := os.Stat(bin); err == nil {
		t.Errorf("purge --binary left the binary at %s", bin)
	}
	if !plannedRemovals(t, res.stdout)[bin] {
		t.Errorf("the plan does not name the binary it removed:\n%s", res.stdout)
	}
	if entries, err := os.ReadDir(e.fakeHome); err != nil || len(entries) != 0 {
		t.Errorf("purge --binary left HOME holding %v (%v)", entries, err)
	}
}

// writeInstallField sets one field of install.json, as a later watch would.
func writeInstallField(t *testing.T, root, key string, value any) {
	t.Helper()
	path := filepath.Join(root, "install.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	doc[key] = value
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestTRU29_PurgeRefusesANonStoreRoot: a root without install.json or
// install.key, a root that is HOME, a root that encloses HOME, and a root
// inside a git work tree nobody confirmed at watch -- tested with a HOME that
// is itself a work tree -- are each refused before anything is removed.
func TestTRU29_PurgeRefusesANonStoreRoot(t *testing.T) {
	refused := func(t *testing.T, e *env, want string) {
		t.Helper()
		before := snapshot(t, e.fakeHome)
		res := e.purge("--yes")
		if res.exitCode != 1 {
			t.Fatalf("purge: exit %d, want 1\nstdout:\n%s", res.exitCode, res.stdout)
		}
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal does not say %q: %q", want, res.stderr)
		}
		if strings.Contains(res.stdout, purgeRemovesHeader) {
			t.Errorf("a refused purge printed a plan:\n%s", res.stdout)
		}
		if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
			t.Errorf("a refused purge changed HOME:\n  %s", strings.Join(d, "\n  "))
		}
	}
	plant := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, body := range map[string]string{
			"install.json": `{"install_id":"0123456789abcdef0123456789abcdef"}` + "\n",
			"install.key":  strings.Repeat("ab", 32) + "\n",
			"keep.txt":     "the user's\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, missing := range []string{"install.json", "install.key"} {
		t.Run("no "+missing, func(t *testing.T) {
			e := newPurgeEnv(t)
			e.session(t)
			if err := os.Remove(filepath.Join(e.home, missing)); err != nil {
				t.Fatal(err)
			}
			refused(t, e, missing)
		})
	}
	t.Run("root is HOME", func(t *testing.T) {
		e := newPurgeEnv(t)
		plant(t, e.fakeHome)
		e.home = e.fakeHome
		refused(t, e, "is your home directory")
	})
	t.Run("root encloses HOME", func(t *testing.T) {
		e := newPurgeEnv(t)
		outer := e.fakeHome
		e.fakeHome = filepath.Join(outer, "home")
		if err := os.Mkdir(e.fakeHome, 0o700); err != nil {
			t.Fatal(err)
		}
		plant(t, outer)
		e.home = outer
		refused(t, e, "encloses your home directory")
	})
	t.Run("root in an unconfirmed work tree", func(t *testing.T) {
		e := newPurgeEnv(t)
		e.session(t)
		if err := os.Mkdir(filepath.Join(e.fakeHome, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
		refused(t, e, "git work tree")

		// Confirmed at watch, the same root is purged.
		writeInstallField(t, e.home, "root_in_work_tree", e.fakeHome)
		if res := e.purge("--yes"); res.exitCode != 0 {
			t.Fatalf("purge of a confirmed work-tree root: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
		}
		if _, err := os.Stat(e.home); err == nil {
			t.Errorf("a confirmed work-tree root was not purged")
		}
	})
}

// TestTRU28_PurgeKeepsAConfigDirReplacedByASymlink: watch created
// ~/.claude, and the user has since moved it into a dotfiles repository and
// linked it back. created_dirs still names ~/.claude, but it is a link now:
// purge plans it as left, removes it neither as a link nor through one, and
// the directory it points at keeps what is in it.
func TestTRU28_PurgeKeepsAConfigDirReplacedByASymlink(t *testing.T) {
	e := newPurgeEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	target := filepath.Join(e.fakeHome, "dotfiles", "claude")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(e.configDir, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, e.configDir); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	writeFiles(t, filepath.Join(target, "keep.json"))

	res := e.purge("--yes")
	if res.exitCode != 0 {
		t.Fatalf("purge --yes: exit %d\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
	}
	if plannedRemovals(t, res.stdout)[e.configDir] {
		t.Errorf("the plan removes the link %s:\n%s", e.configDir, res.stdout)
	}
	if info, err := os.Lstat(e.configDir); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("purge removed the link %s: %v\nstdout:\n%s", e.configDir, err, res.stdout)
	}
	if _, err := os.Stat(filepath.Join(target, "keep.json")); err != nil {
		t.Errorf("the link's target lost its contents: %v", err)
	}
}

// TestTRU28_PurgeRefusesAnEnabledPluginWhoseHooksCannotBeRead: an enabled
// plugin whose hooks.json cannot be read still runs its hooks as far as
// purge can tell, and its next one would create the store again, so purge
// refuses it as it refuses any enabled plugin.
func TestTRU28_PurgeRefusesAnEnabledPluginWhoseHooksCannotBeRead(t *testing.T) {
	e := newPurgeEnv(t)
	e.session(t)
	p := newPluginFixture(t, "rashomon@local")
	if err := os.Remove(filepath.Join(p.root, "hooks", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	p.installed(t, e)
	p.enable(t, e, true)
	before := snapshot(t, e.fakeHome)

	res := e.purge("--yes")
	if res.exitCode != 1 || !strings.Contains(res.stderr, "/plugin disable rashomon@local") {
		t.Fatalf("purge under an enabled plugin with no readable hooks: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if d := diffSnapshots(before, snapshot(t, e.fakeHome)); len(d) > 0 {
		t.Errorf("a refused purge changed HOME:\n  %s", strings.Join(d, "\n  "))
	}
}

// writeFiles creates each path, and its parents, holding a line.
func writeFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("the user's\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTRU29_ARootLeftHoldingOnlyInstallKeyNamesItsCommand: a purge stopped
// between removing install.json and install.key leaves a root holding only
// the key. That root is still refused -- install.key alone does not prove a
// store -- but the refusal says what is left and the command that removes it.
func TestTRU29_ARootLeftHoldingOnlyInstallKeyNamesItsCommand(t *testing.T) {
	e := newPurgeEnv(t)
	key := filepath.Join(e.home, "install.key")
	writeFiles(t, key)
	res := e.purge("--yes")
	if res.exitCode != 1 {
		t.Fatalf("purge of a root holding only install.key: exit %d\nstdout:\n%s", res.exitCode, res.stdout)
	}
	for _, want := range []string{"holds no install.json", "rm '" + key + "'", "rmdir '" + e.home + "'"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal does not say %q: %q", want, res.stderr)
		}
	}
	if _, err := os.Stat(key); err != nil {
		t.Errorf("a refused purge removed %s: %v", key, err)
	}
}
