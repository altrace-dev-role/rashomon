package acceptance

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// TestTRU4b_ConsentWritesTheSessionContentMarker (W-M2.11 step 1d): the
// consent watch --yes and consent --yes write, at this build's scope at record
// scope v5, writes session_content after its line, the marker the hook reads
// before it keeps any of a session's content; a refused consent writes none;
// and a session start writes it again from consent.json when it is missing,
// and removes it when consent.json no longer covers it.
func TestTRU4b_ConsentWritesTheSessionContentMarker(t *testing.T) {
	e := newEnv(t)
	path := filepath.Join(e.home, "session_content")
	marker := func(what string, want bool) {
		t.Helper()
		b, err := os.ReadFile(path)
		switch {
		case want && (err != nil || string(b) != "1\n"):
			t.Errorf("%s: the marker is %q (%v), want \"1\\n\"", what, b, err)
		case !want && !errors.Is(err, fs.ErrNotExist):
			t.Errorf("%s: the marker is %q (%v), want none", what, b, err)
		}
	}
	remove := func() {
		t.Helper()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}

	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	marker("after watch --yes", true)

	remove()
	if res := e.run("", []string{"CLAUDECODE=1"}, "consent", "--yes"); res.exitCode != 1 {
		t.Errorf("consent --yes from an agent session: exit %d, want 1", res.exitCode)
	}
	marker("after a refused consent", false)

	cmd := e.command("", nil, "consent", "--yes")
	noTerminal(cmd)
	if res := e.wait(cmd); res.exitCode != 0 {
		t.Fatalf("consent --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	marker("after consent --yes", true)
	recs := e.consentRecords()
	if len(recs) == 0 {
		t.Fatal("consent.json holds no record after consent --yes")
	}
	if last := recs[len(recs)-1]; last["scope_version"] != float64(disclose.ScopeVersion) || last["record_scope"] != "v5" {
		t.Errorf("the consent --yes record = %v, want scope_version %d at record scope v5", last, disclose.ScopeVersion)
	}

	remove()
	if res := e.probe("start", testSession); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	marker("after a session start with the marker missing", true)

	current := `"scope_version":` + strconv.Itoa(disclose.ScopeVersion)
	older := strings.ReplaceAll(string(e.consentBytes()), current, `"scope_version":5`)
	if strings.Contains(older, current) || !strings.Contains(older, `"scope_version":5`) {
		t.Fatalf("premise: consent.json still holds %s after the edit to scope 5", current)
	}
	if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := e.probe("start", "sess-tru4b-2"); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	marker("after a session start over a consent at scope 5", false)
}
