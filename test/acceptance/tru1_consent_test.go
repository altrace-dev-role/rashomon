package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// TRU-1, TRU-3, TRU-4 and TRU-5 (Technical spec 7.2.3): watch shows the
// consent screen and asks on the terminal before it writes anything, every
// question comes before the first write, --show prints the screen and writes
// nothing, no terminal means no install without --yes, an agent session
// means none at all, and consent.json records the hash of the screen that
// was shown.

const (
	questionInstall = "Install? [y/N] "
	questionSDK     = "Also record Agent SDK programs on this machine? [y/N] "
)

// tree is every file and directory under each of dirs, by path, with the
// bytes of each file; a dir that does not exist is recorded as absent.
func tree(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range dirs {
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			out[dir] = "<absent>"
			continue
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				out[p] = "<dir>"
				return nil
			}
			b, err := os.ReadFile(p)
			out[p] = string(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// machine is everything watch may write: the store root and the Claude
// configuration directory (settings.json is in it).
func (e *env) machine() map[string]string {
	e.t.Helper()
	return tree(e.t, e.home, e.configDir)
}

func (e *env) requireUnchanged(before map[string]string, what string) {
	e.t.Helper()
	if after := e.machine(); !reflect.DeepEqual(before, after) {
		e.t.Errorf("%s changed the machine:\nbefore %v\nafter  %v", what, keys(before), keys(after))
		for k, v := range after {
			if before[k] != v {
				e.t.Errorf("  %s: %q -> %q", k, clip(before[k]), clip(v))
			}
		}
		for k := range before {
			if _, ok := after[k]; !ok {
				e.t.Errorf("  %s: removed", k)
			}
		}
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func clip(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// consentRecords reads consent.json's lines.
func (e *env) consentRecords() []map[string]any {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "consent.json"))
	if err != nil {
		e.t.Fatalf("reading consent.json: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			e.t.Fatalf("consent.json line %q: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

// sdkRecording reads harnesses.json's surfaces.sdk.record.
func (e *env) sdkRecording() string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "harnesses.json"))
	if err != nil {
		e.t.Fatalf("reading harnesses.json: %v", err)
	}
	var doc struct {
		Surfaces map[string]map[string]string `json:"surfaces"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		e.t.Fatal(err)
	}
	return doc.Surfaces["sdk"]["record"]
}

// requireInstalled checks that every event carries this install's entry.
func (e *env) requireInstalled() {
	e.t.Helper()
	var doc struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		e.t.Fatalf("settings.json does not parse: %v", err)
	}
	id := e.installID()
	if len(doc.Hooks) != len(allEvents) {
		e.t.Errorf("settings.json has entries under %d events, want %d", len(doc.Hooks), len(allEvents))
	}
	for _, ev := range allEvents {
		ours := 0
		for _, entry := range doc.Hooks[ev] {
			if strings.Contains(string(entry), "--install "+id) {
				ours++
			}
		}
		if ours != 1 {
			e.t.Errorf("%s carries %d entries of install %s, want 1", ev, ours, id)
		}
	}
}

// watchOnPTY starts plain watch with a pseudo-terminal as its controlling
// terminal and "y" lines waiting on its stdin, which watch must never read.
func (e *env) watchOnPTY(extraEnv ...string) *ptySession {
	e.t.Helper()
	cmd := e.command("y\ny\nyes\n", extraEnv, "watch")
	return startOnPTY(e.t, cmd)
}

// allEvents is every event watch installs an entry for, in its order.
var allEvents = []string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure", "SessionStart",
	"SessionEnd", "Stop", "StopFailure", "UserPromptSubmit",
}

const seededSettings = "{\n  \"model\": \"opus\"\n}\n"

func TestTRU1_WatchWritesNothingBeforeYes(t *testing.T) {
	declines := map[string]func(s *ptySession){
		"no":         func(s *ptySession) { s.answer("n") },
		"empty line": func(s *ptySession) { s.answer("") },
		"not yes":    func(s *ptySession) { s.answer("yess") },
		"EOF":        func(s *ptySession) { s.eof() },
	}
	for name, decline := range declines {
		t.Run("first question: "+name, func(t *testing.T) {
			e := newEnv(t)
			e.writeSettings(seededSettings)
			before := e.machine()
			s := e.watchOnPTY()
			s.waitFor(questionInstall)
			decline(s)
			res := s.wait()
			if res.exitCode != 1 {
				t.Fatalf("declined watch: exit %d, want 1\nstderr: %s", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stderr, "rashomon: nothing was written") {
				t.Errorf("declined watch does not say nothing was written: %q", res.stderr)
			}
			if s.countOf(questionSDK) != 0 {
				t.Error("a declined first question went on to the second")
			}
			e.requireUnchanged(before, "a declined watch")
		})
	}

	for name, sig := range map[string]syscall.Signal{"SIGINT": syscall.SIGINT, "SIGHUP": syscall.SIGHUP} {
		for _, at := range []string{questionInstall, questionSDK} {
			t.Run(name+" at "+strings.TrimSpace(at), func(t *testing.T) {
				e := newEnv(t)
				e.writeSettings(seededSettings)
				before := e.machine()
				s := e.watchOnPTY()
				s.waitFor(questionInstall)
				if at == questionSDK {
					s.answer("y")
					s.waitFor(questionSDK)
				}
				if err := s.cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				res := s.wait()
				if res.exitCode != 1 {
					t.Fatalf("watch after %s: exit %d, want 1\nstderr: %s", name, res.exitCode, res.stderr)
				}
				if !strings.Contains(res.stderr, "nothing was written") {
					t.Errorf("watch after %s does not say nothing was written: %q", name, res.stderr)
				}
				e.requireUnchanged(before, "a signalled watch")
			})
		}
	}

	t.Run("both questions come before any write; EOF at the second is no", func(t *testing.T) {
		e := newEnv(t)
		e.writeSettings(seededSettings)
		before := e.machine()
		s := e.watchOnPTY()
		s.waitFor(questionInstall)
		e.requireUnchanged(before, "the screen")
		s.answer("Y")
		s.waitFor(questionSDK)
		e.requireUnchanged(before, "a watch waiting at its second question")
		s.eof()
		res := s.wait()
		if res.exitCode != 0 {
			t.Fatalf("watch after y: exit %d\nstderr: %s\nterminal:\n%s", res.exitCode, res.stderr, s.out.String())
		}
		e.requireInstalled()
		recs := e.consentRecords()
		if len(recs) != 1 || recs[0]["via"] != "watch_tty" || recs[0]["tty"] != true || recs[0]["steer_disclosure"] != float64(2) {
			t.Errorf("consent.json after a terminal y = %v, want one watch_tty record, tty true, steer_disclosure 2", recs)
		}
		if got := e.sdkRecording(); got != "paused" {
			t.Errorf("EOF at the Agent SDK question recorded %q, want paused", got)
		}
	})

	t.Run("yes to both", func(t *testing.T) {
		e := newEnv(t)
		s := e.watchOnPTY()
		s.waitFor(questionInstall)
		s.answer("yes")
		s.waitFor(questionSDK)
		s.answer("y")
		if res := s.wait(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d\nstderr: %s", res.exitCode, res.stderr)
		}
		e.requireInstalled()
		if got := e.sdkRecording(); got != "on" {
			t.Errorf("y at the Agent SDK question recorded %q, want on", got)
		}
		// A later --yes asks no question, so it has no answer to record:
		// the y on file stays, byte for byte.
		harnesses := filepath.Join(e.home, "harnesses.json")
		before, err := os.ReadFile(harnesses)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(e.home, "consent.json")); err != nil {
			t.Fatal(err)
		}
		cmd := e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		if res := e.wait(cmd); res.exitCode != 0 {
			t.Fatalf("watch --yes after a y: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if after, err := os.ReadFile(harnesses); err != nil || string(after) != string(before) {
			t.Errorf("watch --yes rewrote the Agent SDK answer: %s (%v), want %s", after, err, before)
		}
	})

	refusals := map[string]struct {
		managed, user, want string
	}{
		"unparsable settings":   {user: "{", want: "settings"},
		"disableAllHooks":       {managed: `{"disableAllHooks": true}`, want: "disabled by the managed settings layer"},
		"allowManagedHooksOnly": {managed: `{"allowManagedHooksOnly": true}`, want: "allowManagedHooksOnly"},
	}
	for name, c := range refusals {
		t.Run("refused: "+name, func(t *testing.T) {
			e := newEnv(t)
			if c.user != "" {
				e.writeSettings(c.user)
			}
			if c.managed != "" {
				if err := os.WriteFile(e.managedPath(), []byte(c.managed), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := e.machine()
			res := e.watch()
			if res.exitCode != 1 || !strings.Contains(res.stderr, c.want) {
				t.Errorf("watch --yes under %s: exit %d, stderr %q; want exit 1 naming %q", name, res.exitCode, res.stderr, c.want)
			}
			e.requireUnchanged(before, "a refused watch")
		})
	}
	t.Run("allowManagedHooksOnly outside the managed layer is not read", func(t *testing.T) {
		e := newEnv(t)
		e.writeSettings(`{"allowManagedHooksOnly": true}`)
		if res := e.watch(); res.exitCode != 0 {
			t.Errorf("watch with the key in the user layer: exit %d, stderr %q", res.exitCode, res.stderr)
		}
	})
}

// TestTRU1_SignalDuringTheWriteIsHeld: SIGINT or SIGHUP arriving while the
// last step writes is held until the writes commit or roll back. Held
// through a commit, watch exits 1 naming the signal, and settings.json is
// byte for byte what the install writes, the store root exactly the four
// files it creates. Held through a rollback -- a later write fails after the
// signal arrived -- every earlier write is undone and the machine is byte
// for byte as it was.
func TestTRU1_SignalDuringTheWriteIsHeld(t *testing.T) {
	// signalMidWrite runs watch to its writes, held open just after the
	// store opens, calls during there, then sends sig.
	signalMidWrite := func(t *testing.T, e *env, sig syscall.Signal, during func()) result {
		t.Helper()
		s := e.watchOnPTY("RASHOMON_FAULT=watch.writing:hang=2")
		s.waitFor(questionInstall)
		s.answer("y")
		s.waitFor(questionSDK)
		s.answer("n")
		deadline := time.Now().Add(20 * time.Second)
		for !fileExists(filepath.Join(e.home, "install.json")) {
			if time.Now().After(deadline) {
				t.Fatalf("watch never opened the store; terminal:\n%s", s.out.String())
			}
			time.Sleep(5 * time.Millisecond)
		}
		during()
		if err := s.cmd.Process.Signal(sig); err != nil {
			t.Fatal(err)
		}
		return s.wait()
	}

	for name, sig := range map[string]syscall.Signal{"SIGINT": syscall.SIGINT, "SIGHUP": syscall.SIGHUP} {
		t.Run(name+" through a commit", func(t *testing.T) {
			e := newEnv(t)
			e.writeSettings(seededSettings)
			res := signalMidWrite(t, e, sig, func() {})
			if res.exitCode != 1 {
				t.Fatalf("watch signalled mid-write: exit %d, want 1\nstdout:\n%s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
			}
			if !strings.Contains(res.stderr, "arrived while watch was writing") {
				t.Errorf("watch does not say it held the signal: %q", res.stderr)
			}
			e.requireInstalled()

			exe, err := filepath.EvalSymlinks(rashomonBin)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := settings.Parse([]byte(seededSettings))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := install.Apply(doc, install.Spec{Executable: exe, InstallID: e.installID()}); err != nil {
				t.Fatal(err)
			}
			if got := e.settingsBytes(); !bytes.Equal(got, doc.Bytes()) {
				t.Errorf("settings.json after a held signal is not byte for byte the install:\ngot  %s\nwant %s", got, doc.Bytes())
			}

			entries, err := os.ReadDir(e.home)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, d := range entries {
				names = append(names, d.Name())
			}
			// record_scope and steer_disclosure are consent.json's markers: the
			// consent write that completed wrote them beside the line (1.2q, 2.2c).
			if want := []string{"consent.json", "harnesses.json", "install.json", "install.key", "record_scope", "steer_disclosure"}; !reflect.DeepEqual(names, want) {
				t.Errorf("store root holds %q, want %q", names, want)
			}
			if recs := e.consentRecords(); len(recs) != 1 {
				t.Errorf("consent.json holds %d records, want 1", len(recs))
			}
			const wantHarnesses = `{"surfaces":{"sdk":{"record":"paused","steer":"off"}},"version":1}` + "\n"
			if got, err := os.ReadFile(filepath.Join(e.home, "harnesses.json")); err != nil || string(got) != wantHarnesses {
				t.Errorf("harnesses.json = %q (%v), want %q", got, err, wantHarnesses)
			}
		})

		t.Run(name+" through a rollback", func(t *testing.T) {
			if runtime.GOOS == "windows" || os.Geteuid() == 0 {
				t.Skip("a read-only directory does not stop this user's writes here")
			}
			e := newEnv(t)
			e.writeSettings(seededSettings)
			before := e.machine()
			res := signalMidWrite(t, e, sig, func() {
				// settings.json is replaced through a temporary file beside
				// it, so a directory it cannot write fails that step.
				if err := os.Chmod(e.configDir, 0o500); err != nil {
					t.Fatal(err)
				}
			})
			if err := os.Chmod(e.configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if res.exitCode != 1 {
				t.Fatalf("watch signalled, then failing: exit %d, want 1\nstderr: %s", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stderr, "editing settings.json failed") ||
				!strings.Contains(res.stderr, "nothing was left written") {
				t.Errorf("watch does not name the failed step and the rollback: %q", res.stderr)
			}
			e.requireUnchanged(before, "a signal held through a rollback")
		})
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestTRU1_AFailedWriteIsUndoneInReverse: each of the last step's four
// writes is failed in turn; watch undoes the ones before it in reverse,
// names the one that failed, exits 1, and the machine is byte for byte as it
// was -- including directories the store and settings.json would have
// needed, which watch made and then removed.
func TestTRU1_AFailedWriteIsUndoneInReverse(t *testing.T) {
	steps := []struct{ point, name string }{
		{"watch.store", "opening the store"},
		{"watch.sdk", "recording the Agent SDK answer"},
		{"watch.settings", "editing settings.json"},
		{"watch.consent", "recording your consent"},
	}
	for _, fresh := range []bool{false, true} {
		for _, step := range steps {
			label := "existing files, " + step.point
			if fresh {
				label = "fresh machine, " + step.point
			}
			t.Run(label, func(t *testing.T) {
				e := newEnv(t)
				if fresh {
					base := t.TempDir()
					e.home = filepath.Join(base, "state", "rashomon")
					e.configDir = filepath.Join(base, "config", "claude")
				} else {
					e.writeSettings(seededSettings)
				}
				dirs := []string{e.home, e.configDir}
				if fresh {
					dirs = []string{filepath.Dir(e.home), filepath.Dir(e.configDir)}
				}
				before := tree(t, dirs...)
				res := e.watch("RASHOMON_FAULT=" + step.point + ":fail=1")
				if res.exitCode != 1 {
					t.Fatalf("watch failing at %s: exit %d, want 1\nstderr: %s", step.point, res.exitCode, res.stderr)
				}
				if !strings.Contains(res.stderr, step.name+" failed") {
					t.Errorf("watch does not name the failed step %q: %q", step.name, res.stderr)
				}
				if strings.Contains(res.stderr, "COULD NOT UNDO") {
					t.Errorf("watch could not undo a write: %q", res.stderr)
				}
				if after := tree(t, dirs...); !reflect.DeepEqual(before, after) {
					t.Errorf("after a failure at %s the machine is not as it was:\nbefore %v\nafter  %v", step.point, keys(before), keys(after))
				}
			})
		}
	}
}

// TestTRU3_ShowIsTheScreenAndWritesNothing: --show prints the screen, the
// same bytes each time, exits 0 and writes nothing -- also from an agent
// session, which is where the root skill runs it.
func TestTRU3_ShowIsTheScreenAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	before := e.machine()
	first := e.run("", []string{"CLAUDECODE=1"}, "watch", "--show")
	if first.exitCode != 0 {
		t.Fatalf("watch --show: exit %d, stderr %q", first.exitCode, first.stderr)
	}
	for _, h := range []string{"1. HOOK ENTRIES", "2. WHAT IS RECORDED", "3. WHAT RASHOMON ADDS", "4. LESSONS", "5. WHAT YOU WILL SEE", "rashomon purge"} {
		if !strings.Contains(first.stdout, h) {
			t.Errorf("watch --show does not print %q:\n%s", h, first.stdout)
		}
	}
	if strings.Contains(first.stdout, questionInstall) {
		t.Error("watch --show asks the question")
	}
	e.requireUnchanged(before, "watch --show")
	if again := e.run("", nil, "watch", "--show"); again.stdout != first.stdout {
		t.Error("watch --show printed different bytes on the same machine")
	}
}

// TestTRU4_NonInteractiveWatchNeedsYes: with no terminal watch prints the
// screen and refuses; with --yes it installs and records watch_yes at
// steer_disclosure 0 with the Agent SDK answer at its default; from an agent
// session it refuses with or without --yes, and neither refusal names --yes.
func TestTRU4_NonInteractiveWatchNeedsYes(t *testing.T) {
	t.Run("no terminal", func(t *testing.T) {
		e := newEnv(t)
		before := e.machine()
		cmd := e.command("y\n", nil, "watch")
		noTerminal(cmd)
		res := e.wait(cmd)
		if res.exitCode != 1 {
			t.Fatalf("watch with no terminal: exit %d, want 1", res.exitCode)
		}
		if !strings.Contains(res.stdout, "1. HOOK ENTRIES") {
			t.Errorf("watch with no terminal did not print the screen:\n%s", res.stdout)
		}
		if !strings.Contains(res.stderr, "watch needs a terminal to ask; nothing was written") {
			t.Errorf("watch with no terminal: stderr %q", res.stderr)
		}
		if strings.Contains(res.stderr, "--yes") {
			t.Errorf("the no-terminal refusal names --yes: %q", res.stderr)
		}
		e.requireUnchanged(before, "watch with no terminal")
	})

	t.Run("--yes", func(t *testing.T) {
		e := newEnv(t)
		cmd := e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		res := e.wait(cmd)
		if res.exitCode != 0 {
			t.Fatalf("watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if !strings.Contains(res.stdout, "1. HOOK ENTRIES") {
			t.Error("watch --yes did not print the screen")
		}
		e.requireInstalled()
		recs := e.consentRecords()
		if len(recs) != 1 || recs[0]["via"] != "watch_yes" || recs[0]["steer_disclosure"] != float64(0) || recs[0]["tty"] != false {
			t.Errorf("consent.json after --yes = %v, want one watch_yes record at steer_disclosure 0, tty false", recs)
		}
		if got := e.sdkRecording(); got != "paused" {
			t.Errorf("--yes recorded the Agent SDK answer %q, want its default, paused", got)
		}
		again := e.watch()
		if again.exitCode != 0 || !strings.HasPrefix(again.stdout, "rashomon: already watching (install "+e.installID()+")") {
			t.Errorf("a second watch: exit %d, stdout %q; want already watching", again.exitCode, again.stdout)
		}
		// Entries in place but no consent on file -- a store from before
		// consent.json -- is not "already watching": the screen is shown
		// and the consent recorded, the entries left as they are.
		settingsBefore := e.settingsBytes()
		if err := os.Remove(filepath.Join(e.home, "consent.json")); err != nil {
			t.Fatal(err)
		}
		third := e.watch()
		if third.exitCode != 0 || !strings.Contains(third.stdout, "1. HOOK ENTRIES") {
			t.Errorf("watch with entries but no consent: exit %d, stdout %q; want the screen", third.exitCode, third.stdout)
		}
		if len(e.consentRecords()) != 1 {
			t.Error("watch with entries but no consent did not record one")
		}
		if !bytes.Equal(e.settingsBytes(), settingsBefore) {
			t.Error("watch with entries in place rewrote settings.json")
		}
	})

	// Named apart from their flags: a subtest's name is part of its temp
	// directory's path, which the screen prints.
	for name, args := range map[string][]string{"plain": {"watch"}, "with the yes flag": {"watch", "--yes"}} {
		t.Run("agent session, "+name, func(t *testing.T) {
			e := newEnv(t)
			before := e.machine()
			res := e.run("", []string{"CLAUDECODE=1"}, args...)
			if res.exitCode != 1 {
				t.Fatalf("%v from an agent session: exit %d, want 1", args, res.exitCode)
			}
			if !strings.Contains(res.stderr, "so only you can run it, in a terminal outside Claude Code: `rashomon watch`. Nothing was written.") {
				t.Errorf("%v from an agent session: stderr %q", args, res.stderr)
			}
			if i := strings.Index(res.stderr+res.stdout, "--yes"); i >= 0 {
				out := res.stderr + res.stdout
				t.Errorf("the agent-session refusal names --yes: %q", out[max(0, i-200):i+5])
			}
			e.requireUnchanged(before, "watch from an agent session")
		})
	}

	t.Run("agent session at a terminal", func(t *testing.T) {
		e := newEnv(t)
		before := e.machine()
		s := e.watchOnPTY("CLAUDECODE=1")
		res := s.wait()
		if res.exitCode != 1 || s.countOf(questionInstall) != 0 {
			t.Errorf("watch from an agent session at a terminal: exit %d, asked %d times; want refused unasked", res.exitCode, s.countOf(questionInstall))
		}
		e.requireUnchanged(before, "watch from an agent session at a terminal")
	})
}

// TestTRU5_ConsentRecordHashesTheScreenShown: the screen shown on the
// terminal is the screen --show prints on the same machine, and consent.json
// records the SHA-256 of exactly those bytes; under --yes, of the screen it
// printed.
func TestTRU5_ConsentRecordHashesTheScreenShown(t *testing.T) {
	sum := func(s string) string {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:])
	}

	t.Run("terminal", func(t *testing.T) {
		e := newEnv(t)
		show := e.run("", nil, "watch", "--show")
		if show.exitCode != 0 {
			t.Fatalf("watch --show: exit %d", show.exitCode)
		}
		s := e.watchOnPTY()
		s.waitFor(questionInstall)
		if !strings.Contains(s.out.String(), show.stdout) {
			t.Errorf("the terminal did not show the --show screen")
		}
		s.answer("y")
		s.waitFor(questionSDK)
		s.answer("n")
		if res := s.wait(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		recs := e.consentRecords()
		if len(recs) != 1 || recs[0]["text_sha256"] != sum(show.stdout) {
			t.Errorf("text_sha256 = %v, want the hash of the screen shown, %s", recs, sum(show.stdout))
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(filepath.Join(e.home, "consent.json"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("consent.json mode %o, want 600", info.Mode().Perm())
			}
		}
	})

	t.Run("--yes", func(t *testing.T) {
		e := newEnv(t)
		show := e.run("", nil, "watch", "--show")
		res := e.watch()
		if res.exitCode != 0 || !strings.HasPrefix(res.stdout, show.stdout) {
			t.Fatalf("watch --yes: exit %d; its output does not open with the --show screen", res.exitCode)
		}
		recs := e.consentRecords()
		if len(recs) != 1 || recs[0]["text_sha256"] != sum(show.stdout) {
			t.Errorf("text_sha256 = %v, want %s", recs, sum(show.stdout))
		}
	})
}
