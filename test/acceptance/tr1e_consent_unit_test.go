package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestTR1e_ConsentUnitShowsAndAppends: --show prints the unit's screen and
// writes nothing; --yes prints the same screen and appends one consent_yes
// record for the unit, on, hashing those bytes and claiming no install
// scope; off asks nothing, prints its one line and appends the unit off,
// hashing that line (card 4.2). It runs the built binary, as TRU4 and TRU5
// do: consent resolves its own executable, and install.Executable refuses a
// go-build test binary.
func TestTR1e_ConsentUnitShowsAndAppends(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	before := e.machine()
	show := e.run("", nil, "consent", "bodies", "--show")
	if show.exitCode != 0 || !strings.Contains(show.stdout, "\nBODIES\n") || !strings.Contains(show.stdout, "transcript") {
		t.Errorf("consent bodies --show: exit %d, stdout %q, stderr %q; want the bodies screen", show.exitCode, show.stdout, show.stderr)
	}
	e.requireUnchanged(before, "consent bodies --show")

	cmd := e.command("", nil, "consent", "bodies", "--yes")
	noTerminal(cmd)
	yes := e.wait(cmd)
	if yes.exitCode != 0 || !strings.HasPrefix(yes.stdout, show.stdout) {
		t.Fatalf("consent bodies --yes: exit %d, stdout %q, stderr %q; want the --show screen first", yes.exitCode, yes.stdout, yes.stderr)
	}
	sum := sha256.Sum256([]byte(show.stdout))
	recs := e.consentRecords()
	if len(recs) != 2 {
		t.Fatalf("after --yes: %d records, want the watch record and one more: %v", len(recs), recs)
	}
	if r := recs[1]; r["via"] != "consent_yes" || r["unit"] != "bodies" || r["unit_on"] != true || r["text_sha256"] != hex.EncodeToString(sum[:]) ||
		r["scope_version"] != float64(0) || r["record_scope"] != "" || r["steer_disclosure"] != float64(0) {
		t.Errorf("the --yes record is %v; want consent_yes, bodies on, the screen's hash, no install claim", r)
	}

	cmd = e.command("", nil, "consent", "bodies", "off")
	noTerminal(cmd)
	off := e.wait(cmd)
	if off.exitCode != 0 || strings.Count(off.stdout, "\n") != 1 {
		t.Errorf("consent bodies off: exit %d, stdout %q, stderr %q; want one line", off.exitCode, off.stdout, off.stderr)
	}
	offSum := sha256.Sum256([]byte(off.stdout))
	recs = e.consentRecords()
	if len(recs) != 3 || recs[2]["unit"] != "bodies" || recs[2]["unit_on"] != false || recs[2]["text_sha256"] != hex.EncodeToString(offSum[:]) {
		t.Errorf("after off: %v; want a third record, bodies off, hashing %q", recs, off.stdout)
	}
}
