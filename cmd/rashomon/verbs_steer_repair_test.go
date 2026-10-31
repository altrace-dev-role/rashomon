package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stcBroken puts an unparseable config.json in the store and returns its path.
func stcBroken(t *testing.T, st *store.Store) string {
	t.Helper()
	t.Setenv("CLAUDECODE", "")
	p := filepath.Join(st.Root(), store.FileConfig)
	if err := os.WriteFile(p, []byte(`{"steer_user":`), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSTC1o_UnreadableConfigIsSaidAndExplained (card STEER-CLI1): `steer` and
// `steer explain` say the notes are off because config.json does not read, and
// name the file; `steer on` refuses it and says how to repair it.
func TestSTC1o_UnreadableConfigIsSaidAndExplained(t *testing.T) {
	st, _ := stcStore(t)
	p := stcBroken(t, st)
	want := "steer: off (config.json is unreadable: unexpected end of JSON input; fix or delete " + p + ")\n"
	if out := stcRun(t, cmdSteer); out != want {
		t.Errorf("steer = %q, want %q", out, want)
	}
	if out := stcRun(t, cmdSteer, "explain"); !strings.HasPrefix(out, want) {
		t.Errorf("steer explain = %q, want it to open with %q", out, want)
	}
	for _, args := range [][]string{{"on"}, {"off"}, {"on", "--project"}} {
		err := cmdSteer(args, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "steer on --repair") || !strings.Contains(err.Error(), p) {
			t.Errorf("steer %v on an unreadable file = %v, want a refusal naming --repair and the path", args, err)
		}
	}
	if b, _ := os.ReadFile(p); string(b) != `{"steer_user":` {
		t.Errorf("a refused switch changed the file: %q", b)
	}
}

// TestSTC1o_ExplainSaysWhatConfigUnreadableMeans: a decision the hook made
// under an unreadable file is explained as that, not as the user's switch.
func TestSTC1o_ExplainSaysWhatConfigUnreadableMeans(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcDecision{"s1", "task-preflight", "UserPromptSubmit", "flag", "delivered", "config_unreadable", "flag_only", 1_800_000_000_000})
	out := stcRun(t, cmdSteer, "explain")
	for _, want := range []string{"suppressed_by: config_unreadable", "config.json did not read", "steer on --repair"} {
		if !strings.Contains(out, want) {
			t.Errorf("steer explain lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "you turned the notes") {
		t.Errorf("steer explain blamed the user's switch:\n%s", out)
	}
}

// TestSTC1o_RepairReplacesAnUnparseableConfigAfterAsking: yes replaces the
// file with one holding the switch; no, or no terminal, leaves it; an agent
// session is refused without asking.
func TestSTC1o_RepairReplacesAnUnparseableConfigAfterAsking(t *testing.T) {
	st, _ := stcStore(t)
	p := stcBroken(t, st)
	asked := 0
	say := func(ans bool, err error) func(string) (bool, error) {
		return func(prompt string) (bool, error) {
			asked++
			if !strings.Contains(prompt, p) || !strings.Contains(prompt, "[y/N]") || !strings.Contains(prompt, "content is lost") {
				t.Errorf("prompt = %q", prompt)
			}
			return ans, err
		}
	}
	for name, ask := range map[string]func(string) (bool, error){
		"no":          say(false, nil),
		"no terminal": say(false, errors.New("there is no terminal")),
	} {
		if err := steerRepair(io.Discard, false, ask); err == nil {
			t.Errorf("%s: repair succeeded", name)
		}
		if b, _ := os.ReadFile(p); string(b) != `{"steer_user":` {
			t.Errorf("%s: the file changed: %q", name, b)
		}
	}
	t.Setenv("CLAUDECODE", "1")
	before := asked
	if err := steerRepair(io.Discard, false, say(true, nil)); err == nil || !strings.Contains(err.Error(), "agent session") || asked != before {
		t.Errorf("in an agent session: err %v, asked %d times", err, asked-before)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"steer_user":` {
		t.Errorf("an agent session changed the file: %q", b)
	}
	t.Setenv("CLAUDECODE", "")
	var out bytes.Buffer
	if err := steerRepair(&out, false, say(true, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "steer: on (you: on;") {
		t.Errorf("repair printed %q", out.String())
	}
	if c, err := store.ReadConfig(st.Root()); err != nil || c.SteerOffUser() {
		t.Errorf("config after repair = %+v, %v", c, err)
	}
}

// TestSTC1o_RepairReplacesAConfigThatCannotBeOpened: a config.json that is
// there with mode 000 is clamped by the hook as unreadable, so the repair
// must be able to replace it, not fail with the open's raw error.
func TestSTC1o_RepairReplacesAConfigThatCannotBeOpened(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("a mode 000 file opens on Windows and as root")
	}
	st, _ := stcStore(t)
	p := stcBroken(t, st)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadConfig(st.Root()); !errors.Is(err, store.ErrConfigUnreadable) {
		t.Fatalf("premise: ReadConfig = %v, want ErrConfigUnreadable", err)
	}
	var out bytes.Buffer
	if err := steerRepair(&out, false, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatalf("repair of a file that cannot be opened: %v", err)
	}
	if c, err := store.ReadConfig(st.Root()); err != nil || c.SteerOffUser() {
		t.Errorf("config after repair = %+v, %v", c, err)
	}
}

// TestSTC1o_RepairLeavesAReadableConfigAlone: with a file that reads there is
// nothing to repair, so nothing is asked and the other keys stay.
func TestSTC1o_RepairLeavesAReadableConfigAlone(t *testing.T) {
	st, _ := stcStore(t)
	t.Setenv("CLAUDECODE", "")
	stcRun(t, cmdSteer, "off")
	stcRun(t, cmdSummary, "off")
	ask := func(string) (bool, error) {
		t.Error("asked about a file that reads")
		return false, nil
	}
	if err := steerRepair(io.Discard, false, ask); err != nil {
		t.Fatal(err)
	}
	if c, _ := store.ReadConfig(st.Root()); c.SteerOffUser() || c.SummaryOn() {
		t.Errorf("config = %+v, want steer on and summary still off", c)
	}
}

// TestSTC1o_RepairIsOnlyForOn: --repair with off, or anywhere else, is refused.
func TestSTC1o_RepairIsOnlyForOn(t *testing.T) {
	stcStore(t)
	t.Setenv("CLAUDECODE", "")
	for _, args := range [][]string{{"off", "--repair"}, {"explain", "--repair"}, {"log", "--repair"}} {
		if err := cmdSteer(args, io.Discard); err == nil {
			t.Errorf("steer %v was accepted", args)
		}
	}
}
