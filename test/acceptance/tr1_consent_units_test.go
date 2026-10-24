package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
)

// Task 4.2: installing grants record and nothing else; every other consent
// unit is off until `rashomon consent <unit>`, which refuses inside an agent
// session and needs a terminal, as watch does, and records the hash of the
// notice it showed.

const questionUnit = "Turn this on? [y/N] "

func hasUnit(t *testing.T, root string, u consent.Unit) bool {
	t.Helper()
	ok, err := consent.Has(root, u)
	if err != nil {
		t.Fatalf("consent.Has(%s): %v", u, err)
	}
	return ok
}

// TestTR1b_InstallGrantsRecordOnly: after a real watch, record is on and the
// other four units are off.
func TestTR1b_InstallGrantsRecordOnly(t *testing.T) {
	e := newEnv(t)
	if hasUnit(t, e.home, consent.UnitRecord) {
		t.Error("record is on before anything is installed")
	}
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !hasUnit(t, e.home, consent.UnitRecord) {
		t.Error("record is off after watch")
	}
	for _, u := range consent.Units[1:] {
		if hasUnit(t, e.home, u) {
			t.Errorf("%s is on after watch alone", u)
		}
	}
}

// TestTR1c_ConsentUnitRefusals: every refusal writes nothing, and the notice
// is shown in each so the person can read what they would be agreeing to.
func TestTR1c_ConsentUnitRefusals(t *testing.T) {
	nothing := newEnv(t)
	before := nothing.machine()
	res := nothing.run("", nil, "consent", "bodies")
	if res.exitCode != 1 || !strings.Contains(res.stderr, "nothing is installed here") {
		t.Errorf("consent bodies with no store: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	nothing.requireUnchanged(before, "consent bodies with no store")

	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	before = e.machine()
	old := e.consentBytes()
	notice := consent.Notice(consent.UnitShareOutcomes)

	for _, tc := range []struct {
		name  string
		args  []string
		env   []string
		tty   bool
		want  string
		shows bool
	}{
		{"unknown unit", []string{"consent", "share.everything"}, nil, true, `"share.everything" is not a consent unit`, false},
		{"record is the install's", []string{"consent", "record"}, nil, true, "record is granted by rashomon watch", false},
		{"two units", []string{"consent", "bodies", "share.bodies"}, nil, true, "one unit at a time", false},
		{"agent session", []string{"consent", "share.outcomes"}, []string{"CLAUDECODE=1"}, true,
			"consent share.outcomes was run from an agent session; nothing was written", true},
		{"agent session with --yes", []string{"consent", "--yes", "share.outcomes"}, []string{"CLAUDECODE=1"}, true,
			"consent share.outcomes was run from an agent session; nothing was written", true},
		{"no terminal", []string{"consent", "share.outcomes"}, nil, false,
			"consent share.outcomes needs a terminal to ask; nothing was written", true},
		{"--yes does not stand in for a person", []string{"consent", "--yes", "share.outcomes"}, nil, false,
			"a unit is agreed to at a terminal", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := e.command("y\n", tc.env, tc.args...)
			if !tc.tty {
				noTerminal(cmd)
			}
			res := e.wait(cmd)
			if res.exitCode != 1 || !strings.Contains(res.stderr, tc.want) {
				t.Errorf("exit %d, stderr %q; want exit 1 and %q", res.exitCode, res.stderr, tc.want)
			}
			if tc.shows != strings.Contains(res.stdout, notice) {
				t.Errorf("notice shown = %v, want %v; stdout %q", !tc.shows, tc.shows, res.stdout)
			}
			if strings.Contains(res.stderr, "--yes") && tc.env != nil {
				t.Errorf("the agent-session refusal names --yes: %q", res.stderr)
			}
			e.requireUnchanged(before, tc.name)
		})
	}

	// A unit sits under record: an installed store whose consent.json holds no
	// act of consent grants no unit, and the refusal says why.
	if err := os.Remove(filepath.Join(e.home, "consent.json")); err != nil {
		t.Fatal(err)
	}
	bare := e.machine()
	res = e.run("", nil, "consent", "bodies")
	if res.exitCode != 1 || !strings.Contains(res.stderr, "record is not on here") {
		t.Errorf("consent bodies with no consent on file: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.requireUnchanged(bare, "consent bodies with no consent on file")
	if err := os.WriteFile(filepath.Join(e.home, "consent.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}

	show := e.run("", []string{"CLAUDECODE=1"}, "consent", "share.outcomes", "--show")
	if show.exitCode != 0 || show.stdout != notice {
		t.Errorf("consent share.outcomes --show: exit %d, stdout %q, want exit 0 and the notice", show.exitCode, show.stdout)
	}
	e.requireUnchanged(before, "consent share.outcomes --show")
}

// TestTR1c_ConsentUnitAtATerminal: n appends nothing; y appends one unit_tty
// record for that unit, carrying the hash of the notice shown, after every
// earlier line, and turns on that unit and no other.
func TestTR1c_ConsentUnitAtATerminal(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	before := e.machine()
	old := e.consentBytes()

	s := startOnPTY(t, e.command("", nil, "consent", "share.outcomes"))
	s.waitFor(questionUnit)
	s.answer("n")
	if res := s.wait(); res.exitCode != 1 || !strings.Contains(res.stderr, "nothing was written") {
		t.Errorf("consent share.outcomes answered n: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.requireUnchanged(before, "a declined unit")

	s = startOnPTY(t, e.command("", nil, "consent", "share.outcomes"))
	s.waitFor(questionUnit)
	notice := consent.Notice(consent.UnitShareOutcomes)
	if !strings.Contains(s.out.String(), notice) {
		t.Error("the terminal did not show the unit's notice")
	}
	s.answer("y")
	if res := s.wait(); res.exitCode != 0 {
		t.Fatalf("consent share.outcomes answered y: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !bytes.HasPrefix(e.consentBytes(), old) {
		t.Error("consent <unit> rewrote the records before its own")
	}
	recs := e.consentRecords()
	sum := sha256.Sum256([]byte(notice))
	if len(recs) != 2 || recs[1]["via"] != "unit_tty" || recs[1]["unit"] != "share.outcomes" ||
		recs[1]["tty"] != true || recs[1]["text_sha256"] != hex.EncodeToString(sum[:]) ||
		recs[1]["scope_version"] != float64(0) || recs[1]["steer_disclosure"] != float64(0) {
		t.Errorf("consent.json after y = %v, want one unit_tty record for share.outcomes hashing the notice", recs)
	}
	for _, u := range consent.Units {
		want := u == consent.UnitRecord || u == consent.UnitShareOutcomes
		if got := hasUnit(t, e.home, u); got != want {
			t.Errorf("after consent share.outcomes: Has(%s) = %v, want %v", u, got, want)
		}
	}
	if again := e.watch(); !strings.HasPrefix(again.stdout, "rashomon: already watching") {
		t.Errorf("watch after a unit grant: %q, want already watching", again.stdout)
	}
}
