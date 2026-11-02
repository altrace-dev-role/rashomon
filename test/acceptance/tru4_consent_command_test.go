package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// `rashomon consent [--yes] [--show]` (Technical spec 7.2.3, 7.2.11): the
// screen of an installed store under watch's terminal rule, appending one
// consent record and touching nothing else.

const questionConsent = "Agree to what this screen describes? [y/N] "

func (e *env) consentBytes() []byte {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "consent.json"))
	if err != nil {
		e.t.Fatalf("reading consent.json: %v", err)
	}
	return b
}

// TestTRU4_ConsentFollowsWatchsTerminalRule: consent refuses with no
// terminal and from an agent session, takes --yes only outside one, and
// refuses on a machine with nothing installed, writing nothing each time.
func TestTRU4_ConsentFollowsWatchsTerminalRule(t *testing.T) {
	t.Run("nothing installed", func(t *testing.T) {
		e := newEnv(t)
		before := e.machine()
		res := e.run("", nil, "consent", "--yes")
		if res.exitCode != 1 || !strings.Contains(res.stderr, "nothing is installed here") {
			t.Errorf("consent with no store: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		e.requireUnchanged(before, "consent with no store")
	})

	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	before := e.machine()

	cmd := e.command("y\n", nil, "consent")
	noTerminal(cmd)
	res := e.wait(cmd)
	if res.exitCode != 1 || !strings.Contains(res.stderr, "consent needs a terminal to ask; nothing was written") ||
		!strings.Contains(res.stdout, "NEVER STORED:") {
		t.Errorf("consent with no terminal: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.requireUnchanged(before, "consent with no terminal")

	res = e.run("", []string{"CLAUDECODE=1"}, "consent", "--yes")
	if res.exitCode != 1 || !strings.Contains(res.stderr, "consent was run from an agent session; nothing was written") {
		t.Errorf("consent --yes from an agent session: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if strings.Contains(res.stderr, "--yes") {
		t.Errorf("the agent-session refusal names --yes: %q", res.stderr)
	}
	e.requireUnchanged(before, "consent from an agent session")

	show := e.run("", []string{"CLAUDECODE=1"}, "consent", "--show")
	if show.exitCode != 0 || !strings.Contains(show.stdout, "rashomon consent --full") {
		t.Errorf("consent --show: exit %d, stdout %q", show.exitCode, show.stdout)
	}
	e.requireUnchanged(before, "consent --show")

	old := e.consentBytes()
	cmd = e.command("", nil, "consent", "--yes")
	noTerminal(cmd)
	if res := e.wait(cmd); res.exitCode != 0 {
		t.Fatalf("consent --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	recs := e.consentRecords()
	if len(recs) != 2 || recs[1]["via"] != "consent_yes" || recs[1]["steer_disclosure"] != float64(steer.DisclosureVersion) {
		t.Errorf("consent.json after consent --yes = %v, want a second, consent_yes record at steer_disclosure %d", recs, steer.DisclosureVersion)
	}
	if !bytes.HasPrefix(e.consentBytes(), old) {
		t.Error("consent --yes rewrote the records before its own")
	}
}

// TestTRU5_ConsentAppendsAndHashesItsScreen: at a terminal, y appends one
// consent_tty record holding the hash of the screen shown -- the screen
// --show prints, opening with what changed when the consent on file is for
// an older scope -- and leaves every earlier line and every other file as it
// was, but for the steer_disclosure marker, which follows the line it views;
// n appends nothing.
func TestTRU5_ConsentAppendsAndHashesItsScreen(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// The consent on file becomes a --yes one from before this build: the
	// scope before this build's, at steer disclosure 0 (as watch --yes wrote
	// it before STEER-DISCLOSE-YES1), with its marker.
	older := strings.Replace(string(e.consentBytes()), `"scope_version":5`, `"scope_version":4`, 1)
	older = strings.Replace(older, `"steer_disclosure":`+strconv.Itoa(steer.DisclosureVersion), `"steer_disclosure":0`, 1)
	if !strings.Contains(older, `"scope_version":4`) || !strings.Contains(older, `"steer_disclosure":0`) {
		t.Fatalf("premise: the watch record does not carry scope_version 5 and steer_disclosure %d", steer.DisclosureVersion)
	}
	if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.home, "steer_disclosure"), []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := e.machine()

	s := startOnPTY(t, e.command("y\n", nil, "consent"))
	s.waitFor(questionConsent)
	s.answer("n")
	if res := s.wait(); res.exitCode != 1 || !strings.Contains(res.stderr, "nothing was written") {
		t.Errorf("consent answered n: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.requireUnchanged(before, "a declined consent")

	show := e.run("", nil, "consent", "--show")
	if !strings.Contains(show.stdout, "CHANGED SINCE YOU LAST AGREED") {
		t.Errorf("consent over an older scope does not open with what changed:\n%s", show.stdout)
	}
	s = startOnPTY(t, e.command("", nil, "consent"))
	s.waitFor(questionConsent)
	if !strings.Contains(s.out.String(), show.stdout) {
		t.Error("the terminal did not show the --show screen")
	}
	s.answer("y")
	if res := s.wait(); res.exitCode != 0 {
		t.Fatalf("consent answered y: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !bytes.HasPrefix(e.consentBytes(), []byte(older)) {
		t.Error("consent rewrote the records before its own")
	}
	recs := e.consentRecords()
	sum := sha256.Sum256([]byte(show.stdout))
	if len(recs) != 2 || recs[1]["via"] != "consent_tty" || recs[1]["scope_version"] != float64(5) ||
		recs[1]["steer_disclosure"] != float64(steer.DisclosureVersion) || recs[1]["text_sha256"] != hex.EncodeToString(sum[:]) {
		t.Errorf("consent.json after y = %v, want a consent_tty record for scope 5 hashing the screen shown", recs)
	}
	after := e.machine()
	// steer_disclosure is consent.json's view of the highest steer disclosure
	// on file: the older --yes record's 0 before, this record's after (2.2c).
	marker := filepath.Join(e.home, "steer_disclosure")
	if want := strconv.Itoa(steer.DisclosureVersion) + "\n"; before[marker] != "0\n" || after[marker] != want {
		t.Errorf("the steer marker went from %q to %q, want \"0\\n\" to %q", before[marker], after[marker], want)
	}
	delete(after, marker)
	delete(before, marker)
	delete(after, filepath.Join(e.home, "consent.json"))
	delete(before, filepath.Join(e.home, "consent.json"))
	if len(after) != len(before) {
		t.Errorf("consent touched files other than consent.json: %v", keys(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("consent changed %s", k)
		}
	}
	if again := e.watch(); !strings.HasPrefix(again.stdout, "rashomon: already watching") {
		t.Errorf("watch after consent to this scope: %q, want already watching", again.stdout)
	}
}
