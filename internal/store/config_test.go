package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// TestSTC1_ConfigKeepsWhatItDoesNotName: UpdateConfig puts back a key a later
// build wrote, and every switch it names round-trips.
func TestSTC1_ConfigKeepsWhatItDoesNotName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileConfig), []byte(`{"later":{"a":1},"summary":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConfig(root, func(c *Config) {
		c.SteerUser = SwitchOff
		c.SteerProjects = map[string]string{"/p": SwitchOff}
		c.SummaryProjects = map[string]string{"/q": SwitchOff}
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, FileConfig))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"later", "summary", "steer_user", "steer_projects", "summary_projects"} {
		if _, ok := got[k]; !ok {
			t.Errorf("config.json lost key %q: %s", k, b)
		}
	}
	c, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.SteerOffFor("/p") || !c.SteerOffFor("/anywhere") || c.SummaryOnFor("/q") || c.SummaryOn() {
		t.Errorf("round trip: %+v", c)
	}
}

// TestSTC1n_OnlyExactlyOnIsOn: a steer switch that is set and is not exactly
// "on" reads off, in either scope, so a hand's "Off" or "disabled" clamps the
// notes; an absent switch is the default, on.
func TestSTC1n_OnlyExactlyOnIsOn(t *testing.T) {
	for v, off := range map[string]bool{"": false, "on": false, "off": true, "Off": true, "OFF": true, "disabled": true, "false": true, "On": true, " on": true} {
		if got := (Config{SteerUser: v}).SteerOffUser(); got != off {
			t.Errorf("steer_user %q: off = %v, want %v", v, got, off)
		}
		if got := (Config{SteerProjects: map[string]string{"/p": v}}).SteerOffFor("/p"); got != off {
			t.Errorf("steer_projects %q: off = %v, want %v", v, got, off)
		}
	}
	if (Config{SteerProjects: map[string]string{"/p": "off"}}).SteerOffFor("/other") {
		t.Error("a project's value silenced another project")
	}
}

// TestSTC1_ScopesAreEachOwn: the project's key silences that project and no
// other, the user's silences all, and an unset or other value silences none.
func TestSTC1_ScopesAreEachOwn(t *testing.T) {
	c := Config{SteerProjects: map[string]string{"/p": SwitchOff, "/on": "on"}}
	for proj, want := range map[string]bool{"/p": true, "/other": false, "/on": false, "": false} {
		if got := c.SteerOffFor(proj); got != want {
			t.Errorf("project %q: off = %v, want %v", proj, got, want)
		}
	}
	if !(Config{SteerUser: SwitchOff}).SteerOffFor("/any") {
		t.Error("the user's key did not silence a project")
	}
	if (Config{SteerUser: "on"}).SteerOffUser() {
		t.Error("steer_user on read as off")
	}
	s := Config{SummaryProjects: map[string]string{"/p": SwitchOff}}
	if s.SummaryOnFor("/p") || !s.SummaryOnFor("/q") {
		t.Error("summary_projects is not that project's alone")
	}
}

// TestSTC1_ConcurrentSwitchesBothLand: the lock UpdateConfig takes is what
// keeps two switches set at once from costing the earlier one (the last
// write wins without it).
func TestSTC1_ConcurrentSwitchesBothLand(t *testing.T) {
	root := t.TempDir()
	const n = 16
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := string(rune('a' + i))
			if _, err := UpdateConfig(root, func(c *Config) {
				if c.SteerProjects == nil {
					c.SteerProjects = map[string]string{}
				}
				c.SteerProjects[key] = SwitchOff
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	c, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SteerProjects) != n {
		t.Errorf("%d of %d concurrent switches landed: %v", len(c.SteerProjects), n, c.SteerProjects)
	}
}

// TestSTC1_UnparsableConfigIsNotReplaced: a file that does not parse is an
// error from UpdateConfig and is left as it was.
func TestSTC1_UnparsableConfigIsNotReplaced(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileConfig)
	if err := os.WriteFile(path, []byte(`{"steer_user":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConfig(root, func(c *Config) { c.SteerUser = SwitchOff }); err == nil {
		t.Error("UpdateConfig replaced a file that does not parse")
	}
	if b, _ := os.ReadFile(path); string(b) != `{"steer_user":` {
		t.Errorf("config.json is now %q", b)
	}
}

// TestSTC1_UnopenableConfigIsUnreadable: a config.json that is there but
// cannot be opened (mode 000) is ErrConfigUnreadable from ReadConfig, as one
// that does not parse is, so the hook's clamp and the steer verb's repair
// agree on what is unreadable.
func TestSTC1_UnopenableConfigIsUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("a mode 000 file opens on Windows and as root")
	}
	root := t.TempDir()
	path := filepath.Join(root, FileConfig)
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(root); !errors.Is(err, ErrConfigUnreadable) {
		t.Errorf("ReadConfig of a file that cannot be opened = %v, want ErrConfigUnreadable", err)
	}
}

// TestSTC1_AbsentConfigIsOneLstat: with no file the read is the zero Config
// and no error, and creates nothing.
func TestSTC1_AbsentConfigIsOneLstat(t *testing.T) {
	root := t.TempDir()
	c, err := ReadConfig(root)
	if err != nil || c.SteerOffFor("/p") || !c.SummaryOn() {
		t.Errorf("absent config = %+v, %v", c, err)
	}
	if es, _ := os.ReadDir(root); len(es) != 0 {
		t.Errorf("a read created %d entries", len(es))
	}
}
