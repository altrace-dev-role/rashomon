package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-29 -- the first run, before anything is installed.
//
// This is the state every user is in exactly once, and it is the state in which
// a wrong answer is most expensive: they have no way to tell a broken tool from
// one that has nothing to say yet.

// TestH29_ReportCreatesNoStore is the same rule status already follows, applied
// to the command that was missing it.
//
// status stats the install marker before opening anything, and says why in its
// own comment: "key material and all, so the command whose whole answer may be
// 'nothing is installed on this machine' must not be the command that installs
// something." report asks a question too, and it was opening the store, which
// CREATES the root, the install identity and the per-install HMAC key.
//
// Found by running the three first-run commands in order against a fresh home.
// The visible symptom was in status, not report: it reported "present: yes
// (install ...)" for a store that report had fabricated one command earlier,
// so the answer to "has this been set up" was yes because you had once asked
// something else.
func TestH29_ReportCreatesNoStore(t *testing.T) {
	e := newEnv(t)
	fresh := filepath.Join(t.TempDir(), "never-used")

	r := e.run("", []string{"RASHOMON_HOME=" + fresh}, "report")

	if r.exitCode != 0 {
		t.Errorf("report on a fresh home: exit %d, stderr %q. Nothing recorded is not an "+
			"error; a user with no sessions yet must not be told the tool failed",
			r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "no sessions recorded") {
		t.Errorf("report on a fresh home does not say so:\n%s", r.stdout)
	}
	if _, err := os.Stat(fresh); err == nil {
		entries, _ := os.ReadDir(fresh)
		var names []string
		for _, x := range entries {
			names = append(names, x.Name())
		}
		t.Errorf("report created the store it was asked to read: %v. The per-install HMAC "+
			"key is written by that path, so asking a question mints key material.", names)
	}
}

// TestH29_ReportJSONIsTheSameShapeWithAndWithoutAStore. A consumer must not
// have to know whether a store exists to parse the answer.
//
// The sessions field also has to be an ARRAY in both cases. It rendered null,
// which is not the same value as empty to anything that iterates it: the
// natural consumer loop throws on one and is a no-op on the other, and "no
// sessions" is the case a first-time user hits.
func TestH29_ReportJSONIsTheSameShapeWithAndWithoutAStore(t *testing.T) {
	e := newEnv(t)
	fresh := filepath.Join(t.TempDir(), "never-used")

	noStore := e.run("", []string{"RASHOMON_HOME=" + fresh}, "report", "--json")
	if noStore.exitCode != 0 {
		t.Fatalf("report --json on a fresh home: exit %d, stderr %q", noStore.exitCode, noStore.stderr)
	}
	// An existing store with no runs: watch creates it, nothing records into it.
	e.run("", nil, "watch")
	withStore := e.run("", nil, "report", "--json")
	if withStore.exitCode != 0 {
		t.Fatalf("report --json on an empty store: exit %d, stderr %q", withStore.exitCode, withStore.stderr)
	}

	for _, c := range []struct{ name, body string }{
		{"no store", noStore.stdout},
		{"empty store", withStore.stdout},
	} {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal([]byte(c.body), &doc); err != nil {
			t.Fatalf("%s: not valid JSON: %v\n%s", c.name, err, c.body)
		}
		raw, ok := doc["sessions"]
		if !ok {
			t.Errorf("%s: no sessions field at all", c.name)
			continue
		}
		if strings.TrimSpace(string(raw)) == "null" {
			t.Errorf("%s: sessions is null rather than an empty array. A consumer "+
				"iterating it throws on null and does nothing on [], and this is the "+
				"state a first-time user is in.", c.name)
		}
		var sessions []json.RawMessage
		if err := json.Unmarshal(raw, &sessions); err != nil {
			t.Errorf("%s: sessions does not parse as an array: %v", c.name, err)
		}
		if len(sessions) != 0 {
			t.Errorf("%s: sessions has %d entries, want none", c.name, len(sessions))
		}
	}
}

// TestH29_ForgetCreatesNoStoreAndIsNotAnError covers the other read-shaped
// command, and the reason it is not an error.
//
// forget is the privacy action. Asking to forget something on a machine that
// has recorded nothing is a SATISFIED request, so exiting non-zero would tell a
// user their privacy action failed when it had nothing to do -- and creating a
// store to say so would leave behind the install identity and HMAC key they
// were trying to avoid having.
func TestH29_ForgetCreatesNoStoreAndIsNotAnError(t *testing.T) {
	for _, args := range [][]string{
		{"forget", "--host", "never-seen.example"},
		{"forget", "--since", "24h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			e := newEnv(t)
			fresh := filepath.Join(t.TempDir(), "never-used")

			r := e.run("", []string{"RASHOMON_HOME=" + fresh}, args...)

			if r.exitCode != 0 {
				t.Errorf("exit %d, stderr %q: forgetting what was never recorded is a "+
					"satisfied request", r.exitCode, r.stderr)
			}
			if !strings.Contains(r.stdout, "nothing has been recorded here") {
				t.Errorf("does not say there was nothing to forget:\n%s", r.stdout)
			}
			if _, err := os.Stat(fresh); err == nil {
				t.Error("a privacy command created the install identity and HMAC key the " +
					"user was trying not to have")
			}
		})
	}
}
