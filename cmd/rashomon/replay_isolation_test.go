package main

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// plantHome builds a home directory that looks like a used machine's: a real
// store (installed, with a recorded run, a project index, a baseline, a consent
// record and a pause marker), a Claude Code configuration directory with
// settings, an install entry and a transcript, and an unrelated dotfile. It
// points the process environment at it the way a user's shell would, with no
// override variables set, so the default locations are the ones a replay would
// touch if it touched anything real.
func plantHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"RASHOMON_HOME", "XDG_STATE_HOME", "CLAUDE_CONFIG_DIR", "RASHOMON_MANAGED_SETTINGS_PATH",
		"CLAUDE_CODE_ENTRYPOINT", "CLAUDE_PROJECT_DIR", "AI_AGENT", "CI", "RASHOMON_HOLDOUT", "RASHOMON_STORE_CAP_BYTES"} {
		t.Setenv(k, "")
	}
	root := filepath.Join(home, ".local", "state", "rashomon")
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, ".claude")
	doc, err := settings.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Apply(doc, install.Spec{Executable: "/usr/local/bin/rashomon", InstallID: st.InstallID()}); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "consent.json"):                           `{"v":1,"steer":"on","via":"watch_yes"}` + "\n",
		filepath.Join(root, "pause"):                                  "",
		filepath.Join(root, "runs", "live-session", "records.ndjson"): `{"type":"declaration","seq":1}` + "\n",
		filepath.Join(root, "verify", "projects", "-repo-abc.json"):   `{"project_key":"/repo","entries":[]}` + "\n",
		filepath.Join(root, "baselines", "repo-abc.json"):             `{"baseline":"real"}` + "\n",
		filepath.Join(cfg, "settings.json"):                           string(doc.Bytes()),
		filepath.Join(cfg, "projects", "-repo", "live-session.jsonl"): `{"type":"user"}` + "\n",
		filepath.Join(home, ".bashrc"):                                "export PATH=$PATH\n",
	}
	for p, body := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// fileState is one path's identity: kind, mode, size and content digest. The
// modification time is deliberately not in it: a replay that rewrote a file
// with the bytes it had would change a time and no byte, and the byte-identical
// assertion is about bytes.
type fileState struct {
	Dir  bool
	Mode fs.FileMode
	Size int64
	Sum  string
}

func snapshotTree(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		fsx := fileState{Dir: d.IsDir(), Mode: info.Mode()}
		if !d.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			fsx.Size, fsx.Sum = int64(len(b)), hex.EncodeToString(sum[:])
		}
		out[rel] = fsx
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func treeDiff(a, b map[string]fileState) []string {
	var d []string
	for p, x := range a {
		y, ok := b[p]
		switch {
		case !ok:
			d = append(d, "removed: "+p)
		case x != y:
			d = append(d, "changed: "+p)
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			d = append(d, "created: "+p)
		}
	}
	sort.Strings(d)
	return d
}

// captureStd points os.Stdout and os.Stderr at pipes until the returned func
// is called, which restores them and returns what was written to each. A
// replay's output must reach neither.
func captureStd(t *testing.T) func() (stdout, stderr string) {
	t.Helper()
	read := func(f **os.File) (restore func() string) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		old := *f
		*f = w
		done := make(chan string, 1)
		go func() { b, _ := io.ReadAll(r); done <- string(b) }()
		return func() string {
			*f = old
			w.Close()
			s := <-done
			r.Close()
			return s
		}
	}
	rOut, rErr := read(&os.Stdout), read(&os.Stderr)
	return func() (string, string) { return rOut(), rErr() }
}

// TestREPLAY7_ReplayTouchesNothingOutsideItsWorld replays both fixtures with
// the process environment pointing at a used-looking home, and asserts that
// home, which holds the store the entry points would open by default, its
// project index, baseline, consent and pause marker, the Claude Code
// configuration with its settings and a transcript, and a dotfile, is
// byte for byte what it was; that nothing reached the process's stdout or
// stderr; that everything the replay wrote went into its world; and that the
// environment and the recorder's clock are what they were. It then aims a
// replay at the real locations on purpose and expects a refusal.
func TestREPLAY7_ReplayTouchesNothingOutsideItsWorld(t *testing.T) {
	home := plantHome(t)
	before := snapshotTree(t, home)
	envBefore := os.Environ()
	sort.Strings(envBefore)

	for _, version := range pairedFixtures {
		f := loadFixtureSession(t, version)
		scratch := t.TempDir()
		w, err := newReplayWorld(scratch, replayIdentity{})
		if err != nil {
			t.Fatal(err)
		}
		finish := captureStd(t)
		res, err := w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{})
		stdout, stderr := finish()
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("%s: the replay reached the process's streams: stdout %q, stderr %q", version, stdout, stderr)
		}
		if res.Fed == 0 || len(res.Outputs) != 0 {
			t.Errorf("%s: fed %d events and kept %d outputs without being asked to", version, res.Fed, len(res.Outputs))
		}
		if d := treeDiff(before, snapshotTree(t, home)); len(d) > 0 {
			t.Fatalf("%s: the replay changed the home directory it was not given:\n  %s", version, strings.Join(d, "\n  "))
		}

		// Everything it wrote is in the world: the store, with the run of the
		// session, its project index and its steer log, and the stand-in
		// configuration.
		dump := dumpStore(t, w.root)
		if kindCount(dump, "declaration") == 0 || kindCount(dump, "verify") == 0 {
			t.Errorf("%s: the world's store holds no record of the replay", version)
		}
		var index bool
		for _, r := range dump {
			index = index || strings.HasPrefix(r.File, "verify/projects/")
		}
		if !index {
			t.Errorf("%s: the world's store holds no project index: the index write went elsewhere", version)
		}
		var top []string
		entries, _ := os.ReadDir(scratch)
		for _, e := range entries {
			top = append(top, e.Name())
		}
		if !reflect.DeepEqual(top, []string{"claude", "store"}) {
			t.Errorf("%s: the scratch directory holds %v, not exactly the world's two directories", version, top)
		}
	}

	envAfter := os.Environ()
	sort.Strings(envAfter)
	if !reflect.DeepEqual(envBefore, envAfter) {
		t.Errorf("the replay left the environment changed:\nbefore %v\nafter  %v", envBefore, envAfter)
	}
	if d := time.Since(entryNow()); d < 0 || d > time.Minute {
		t.Errorf("the recorder's clock reads %v away from now after a replay: it was not restored", d)
	}
	if stopSeen != nil {
		t.Error("the Stop tap was left set after a replay")
	}

	// A world cannot be aimed at the real places, or at anything holding or
	// inside them, or at a directory that has things in it.
	realRoot := filepath.Join(home, ".local", "state", "rashomon")
	realCfg := filepath.Join(home, ".claude")
	for name, scratch := range map[string]string{
		"the real store root":                 realRoot,
		"inside the real store":               filepath.Join(realRoot, "replay"),
		"the real configuration directory":    realCfg,
		"inside the real configuration":       filepath.Join(realCfg, "replay"),
		"a directory holding the real store":  filepath.Join(home, ".local"),
		"a directory holding the real config": home,
		"a directory with files in it":        filepath.Join(home, ".local", "state", "rashomon", "runs"),
	} {
		if _, err := newReplayWorld(scratch, replayIdentity{}); err == nil {
			t.Errorf("a world was made at %s (%s)", name, scratch)
		}
	}
	// A directory that is not the caller's but has something in it is refused
	// too: a world is the only thing in its scratch directory.
	full := t.TempDir()
	if err := os.WriteFile(filepath.Join(full, "keep.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newReplayWorld(full, replayIdentity{}); err == nil {
		t.Error("a world was made in a directory with a file in it")
	}
	if b, err := os.ReadFile(filepath.Join(full, "keep.txt")); err != nil || string(b) != "mine" {
		t.Errorf("the file in the refused directory is %q, %v", b, err)
	}
	if d := treeDiff(before, snapshotTree(t, home)); len(d) > 0 {
		t.Fatalf("refusing the real places changed them:\n  %s", strings.Join(d, "\n  "))
	}
}

// TestREPLAY7b_TheWorldCannotBeAimedByTheEnvironment is the other half: with
// the process environment aimed at a different store and configuration
// directory than the world's, the entry points still resolve to the world
// while a replay runs, and to the caller's again after.
func TestREPLAY7b_TheWorldCannotBeAimedByTheEnvironment(t *testing.T) {
	other, otherCfg := t.TempDir(), t.TempDir()
	t.Setenv("RASHOMON_HOME", other)
	t.Setenv("CLAUDE_CONFIG_DIR", otherCfg)
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "sdk-py")
	t.Setenv("RASHOMON_HOLDOUT", "")
	t.Setenv("CI", "1")
	t.Setenv("AI_AGENT", "other-agent/9.9")

	w := newWorld(t)
	var inRoot, inCfg, inEntry, inHoldout, inCI string
	err := w.with(replayConfig{}, func(*replayRun) error {
		inRoot, _ = store.DefaultRoot()
		inCfg, _ = settings.ConfigDir()
		inEntry, inHoldout, inCI = os.Getenv("CLAUDE_CODE_ENTRYPOINT"), os.Getenv("RASHOMON_HOLDOUT"), os.Getenv("CI")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if inRoot != w.root || inCfg != w.config {
		t.Errorf("inside a replay the entry points use store %q and configuration %q, not the world's %q and %q", inRoot, inCfg, w.root, w.config)
	}
	if inEntry != replayEntrypoint || inHoldout != "off" || inCI != "" {
		t.Errorf("inside a replay CLAUDE_CODE_ENTRYPOINT=%q RASHOMON_HOLDOUT=%q CI=%q", inEntry, inHoldout, inCI)
	}
	if r, _ := store.DefaultRoot(); r != other {
		t.Errorf("after the replay the store root is %q, not the caller's %q", r, other)
	}
	if os.Getenv("CLAUDE_CODE_ENTRYPOINT") != "sdk-py" || os.Getenv("CI") != "1" || os.Getenv("AI_AGENT") != "other-agent/9.9" {
		t.Error("the caller's environment was not restored")
	}
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Errorf("the caller's store root was written to: %d entries", len(entries))
	}
}

// timeNowFunctions lists the functions of a directory's non-test Go files
// that read time.Now, time.Since or time.Until, as "file:Func".
func timeNowFunctions(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		for _, decl := range f.Decls {
			name := ""
			switch d := decl.(type) {
			case *ast.FuncDecl:
				name = d.Name.Name
			case *ast.GenDecl:
				name = "(package level)"
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if se, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "time" && (se.Sel.Name == "Now" || se.Sel.Name == "Since" || se.Sel.Name == "Until") {
						out[filepath.ToSlash(rel)+":"+name] = true
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestREPLAY8_RecordsCarryTheEventsHistoricalTime replays a synthetic
// transcript dated far from now and asserts that every time the recorder
// wrote is the time of the transcript line its event came from: each
// declaration the tool_use line's, each execution the tool_result line's, the
// Stop's checkpoint and the project index the turn's last line's, the liveness
// marker the first event's. No record carries a time that is not one of those,
// so none came from time.Now. It then lists every non-test site that reads the
// clock, in cmd/rashomon and in internal/, and asserts the list is the one the
// design reviewed: a new time.Now on the path would be caught here.
func TestREPLAY8_RecordsCarryTheEventsHistoricalTime(t *testing.T) {
	d := newTdoc()
	d.promptLine("p1")                                     // 1
	d.uses("m1", tWrite("w1", "/repo/calc.py"))            // 2
	d.results("", tOK("w1"))                               // 3
	d.uses("m2", tBash("t1", "python3 -m unittest"))       // 4
	d.results("", tOK("t1"))                               // 5
	d.ends("m3", "The change is made and all tests pass.") // 6
	w := newWorld(t)
	res, err := w.replayTranscript(d.reader(), replay.Options{TranscriptPath: tPath}, replayConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fed != 6 || len(res.Stops) != 1 || !res.Stops[0].Evaluated {
		t.Fatalf("fed %d events, stops %+v", res.Fed, res.Stops)
	}
	ms := func(line int) float64 { return float64(tAt(line).UnixMilli()) }
	byID := map[string]map[string]float64{} // type -> tool_use_id -> time
	var all []float64
	for _, r := range dumpStore(t, w.root) {
		for k, v := range r.V {
			if f, ok := v.(float64); ok && strings.Contains(k, "unix_ms") && !strings.HasPrefix(k, "created") {
				all = append(all, f)
				if id, _ := r.V["tool_use_id"].(string); id != "" {
					typ, _ := r.V["type"].(string)
					if byID[typ] == nil {
						byID[typ] = map[string]float64{}
					}
					byID[typ][id] = f
				}
			}
		}
	}
	for id, line := range map[string]int{"w1": 2, "t1": 4} {
		if got := byID["declaration"][id]; got != ms(line) {
			t.Errorf("declaration %s was recorded at %v, the tool_use line (%d) is at %v", id, got, line, ms(line))
		}
	}
	for id, line := range map[string]int{"w1": 3, "t1": 5} {
		if got := byID["execution"][id]; got != ms(line) {
			t.Errorf("execution %s was recorded at %v, the tool_result line (%d) is at %v", id, got, line, ms(line))
		}
	}
	allowed := map[float64]bool{}
	for line := 1; line <= 6; line++ {
		allowed[ms(line)] = true
	}
	for _, v := range all {
		if !allowed[v] {
			t.Errorf("a record carries the time %v, which is no line of the transcript: %v", time.UnixMilli(int64(v)).UTC(), v)
		}
	}
	if len(all) < 10 {
		t.Errorf("only %d recorded times to check", len(all))
	}
	if stop := res.Stops[0]; !stop.At.Equal(tAt(6)) {
		t.Errorf("the Stop is at %v, the turn's last line is at %v", stop.At, tAt(6))
	}

	// The sites. cmd/rashomon: none on a hook path, and the package-level
	// entryNow is the one place the clock is named.
	cmdSites := timeNowFunctions(t, ".")
	wantCmd := []string{
		"consent_unit.go:cmdConsentUnit", "forget.go:cmdForget", "main.go:cmdDigest", "main.go:cmdPause", "main.go:cmdReport",
		"main.go:cmdResume", "main.go:cmdSpend", "main.go:forgetHost", "replay.go:(package level)", "watch.go:record",
	}
	if got := sortedKeys(cmdSites); !reflect.DeepEqual(got, wantCmd) {
		t.Errorf("the functions of cmd/rashomon that read the clock are\n  %v\nnot the reviewed\n  %v\nA hook entry point must read entryNow; a command a replay does not call may read time.Now.", got, wantCmd)
	}
	// internal/: lock waits and deadlines (store), the install record's
	// creation time (pinned before the store opens), and recap's cleanup of
	// stale temporary files. None stamps a record.
	wantInternal := []string{
		"recap/state.go:removeStaleTemps", "store/deadline.go:LimitLockWaits", "store/deadline.go:lockWait", "store/evict.go:Evict",
		"store/lock_unix.go:lockFileWait", "store/lock_windows.go:lockFileWait", "store/read.go:ReadRunConsistent", "store/store.go:loadOrCreateMeta",
	}
	if got := sortedKeys(timeNowFunctions(t, "../../internal")); !reflect.DeepEqual(got, wantInternal) {
		t.Errorf("the functions of internal/ that read the clock are\n  %v\nnot the reviewed\n  %v", got, wantInternal)
	}
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestREPLAY9_HostGuardIsPinnedNotInherited replays the 2.1.312 fixture under
// three process environments: a clean one, one saying the session is another
// harness's (CLAUDE_CODE_ENTRYPOINT=sdk-py, CLAUDE_PROJECT_DIR set, CI,
// AI_AGENT), and one with the transcript path of a place that is no Claude Code
// layout. The recorded stores are identical (the replay pins its own host-guard
// environment, replayEntrypoint), and none stands down or stamps an unknown
// harness. Then it shows what the pin is for: the same payloads fed to the
// entry points with the guard left to the environment and a path that is no
// Claude Code layout are stood down on, and recorded as nothing.
func TestREPLAY9_HostGuardIsPinnedNotInherited(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	var base []storedRecord
	for name, env := range map[string]map[string]string{
		"clean":   {},
		"hostile": {"CLAUDE_CODE_ENTRYPOINT": "sdk-py", "CLAUDE_PROJECT_DIR": "/elsewhere", "CI": "1", "AI_AGENT": "someone/1.2.3", "RASHOMON_HOLDOUT": "on"},
	} {
		for k, v := range env {
			t.Setenv(k, v)
		}
		got, _ := runRebuilt(t, f.openTranscript(t), f.opt)
		dump := dumpStore(t, got.root)
		if len(dump) == 0 {
			t.Fatalf("%s: nothing recorded", name)
		}
		for _, r := range dump {
			if r.V["harness_id"] == "unknown" {
				t.Errorf("%s: %s stamps harness_id unknown", name, r.File)
			}
		}
		if base == nil {
			base = dump
		} else if d := storeDiffs(base, dump, 5); len(d) > 0 {
			t.Errorf("%s environment gives a different store:\n  %s", name, strings.Join(d, "\n  "))
		}
		for k := range env {
			t.Setenv(k, "")
		}
	}

	// A path no Claude Code layout names: the replay does not read it.
	opt := f.opt
	opt.TranscriptPath = "/backups/session-copy.jsonl"
	odd, _ := runRebuilt(t, f.openTranscript(t), opt)
	oddDump := dumpStore(t, odd.root)
	if kindCount(oddDump, "declaration") != kindCount(base, "declaration") {
		t.Error("a transcript path that is no Claude Code layout changed what the replay recorded")
	}
	for _, r := range oddDump {
		if r.V["harness_id"] == "unknown" {
			t.Errorf("a transcript path that is no Claude Code layout made the replay stamp harness_id unknown on %s", r.File)
		}
	}

	// What the pin is for. The same payloads fed to the entry points with the
	// guard left to the environment (no entrypoint, that path, no project
	// directory) are recorded as the unknown harness, which no Claude Code
	// session is.
	for _, k := range []string{"CLAUDE_CODE_ENTRYPOINT", "CLAUDE_PROJECT_DIR"} {
		t.Setenv(k, "")
	}
	w := newWorld(t)
	t.Setenv("RASHOMON_HOME", w.root)
	t.Setenv("CLAUDE_CONFIG_DIR", w.config)
	steps, _ := f.liveSteps(t)
	for _, s := range steps {
		if s.Event != install.EventPreToolUse {
			continue
		}
		raw := strings.ReplaceAll(string(s.Raw), f.path, opt.TranscriptPath)
		var errb strings.Builder
		cmdHook([]string{install.Marker, w.id.InstallID}, strings.NewReader(raw), &errb)
	}
	decls, unknown := 0, 0
	for _, r := range dumpStore(t, w.root) {
		if r.V["type"] == "declaration" {
			decls++
			if r.V["harness_id"] == "unknown" {
				unknown++
			}
		}
	}
	if decls == 0 || unknown != decls {
		t.Errorf("unpinned, %d of %d declarations carry harness_id unknown; the pin would not be needed", unknown, decls)
	}
}

func storeAt(t *testing.T, w *replayWorld) *store.Store {
	t.Helper()
	st, err := store.OpenExisting(w.root)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
