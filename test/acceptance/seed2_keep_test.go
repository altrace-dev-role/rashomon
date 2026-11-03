package acceptance

// `rashomon audit --keep` (card SEED1-KEEP): the count of what the person
// corrected before AutoSteer, kept in the store as the before of a before
// and after. Through the real binary: it refuses with no store (it never
// makes one) and in an agent session; it keeps no count of no sessions; it
// counts only what was written before the install; and a recount that finds
// fewer sessions keeps the old count.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/seed"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stampedAt is lines with a timestamp at at, one second apart.
func stampedAt(lines []string, at time.Time) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		ts := at.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		out[i] = strings.Replace(l, "{", `{"timestamp":"`+ts+`",`, 1)
	}
	return out
}

func TestSEED2c_KeepCountsTheBeforeAndOnlyGrows(t *testing.T) {
	e := newEnv(t)
	writeSessionTranscript(t, e.configDir, "sess-0", seedSession(t, "sess-0")...)
	if res := e.run("", nil, "audit", "--keep"); res.exitCode == 0 || !strings.Contains(res.stderr, "rashomon watch") {
		t.Errorf("--keep with no store: exit %d stderr %q, want a refusal that names rashomon watch", res.exitCode, res.stderr)
	}
	if entries, _ := os.ReadDir(e.home); len(entries) != 0 {
		t.Fatalf("--keep with no store made one: %v", entries)
	}
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	rec, err := store.ReadInstallRecord(e.home)
	if err != nil || rec.CreatedAtUnixMS == 0 {
		t.Fatalf("install record %+v, %v", rec, err)
	}
	installed := time.UnixMilli(rec.CreatedAtUnixMS)
	if err := os.Remove(filepath.Join(e.configDir, "projects", "-work-project", "sess-0.jsonl")); err != nil {
		t.Fatal(err)
	}
	// No session before the install: nothing to keep, and no record of zero.
	if res := e.run("", nil, "audit", "--keep"); res.exitCode != 0 || !strings.Contains(res.stdout, "nothing kept") {
		t.Errorf("--keep with no session before the install: exit %d stdout %q stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if _, err := os.Lstat(filepath.Join(e.home, seed.KeptFile)); err == nil {
		t.Fatalf("--keep kept a count of no sessions")
	}
	writeSessionTranscript(t, e.configDir, "sess-1", stampedAt(seedSession(t, "sess-1"), installed.Add(-time.Hour))...)
	writeSessionTranscript(t, e.configDir, "sess-2", stampedAt(seedSession(t, "sess-2"), installed.Add(-2*time.Hour))...)
	writeSessionTranscript(t, e.configDir, "sess-3", stampedAt(seedSession(t, "sess-3"), installed.Add(time.Minute))...)

	if res := e.run("", []string{"CLAUDECODE=1"}, "audit", "--keep"); res.exitCode == 0 || !strings.Contains(res.stderr, "so only you can run it, in a terminal outside Claude Code: `rashomon audit --keep`. Nothing was kept.") {
		t.Errorf("--keep in an agent session: exit %d stderr %q, want a refusal", res.exitCode, res.stderr)
	}
	if _, err := os.Lstat(filepath.Join(e.home, seed.KeptFile)); err == nil {
		t.Fatalf("--keep in an agent session wrote %s", seed.KeptFile)
	}

	res := e.run("", nil, "audit", "--keep")
	if res.exitCode != 0 {
		t.Fatalf("--keep: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, want := range []string{
		"rashomon audit: the 14 days before AutoSteer, on this machine\n",
		"  2 sessions in 1 project, 2 tool calls\n",
		"       2     2  you said no to a version-control command\n",
		"  Kept as your before count; nothing was sent, and it cost 0 tokens.\n",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("--keep stdout does not hold %q:\n%s", want, res.stdout)
		}
	}
	info, err := os.Stat(filepath.Join(e.home, seed.KeptFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%s: %v %v, want mode 0600", seed.KeptFile, err, info)
	}
	k, ok, err := seed.ReadKept(e.home)
	if err != nil || !ok || k.All.Sessions != 2 || k.Days != 14 || k.Until.UnixMilli() != rec.CreatedAtUnixMS {
		t.Errorf("kept %+v ok %v err %v, want 2 sessions up to the install", k, ok, err)
	}

	// Claude Code's cleanup takes a transcript; the recount finds one session.
	if err := os.Remove(filepath.Join(e.configDir, "projects", "-work-project", "sess-2.jsonl")); err != nil {
		t.Fatal(err)
	}
	res = e.run("", nil, "audit", "--keep")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "  Your kept count covers more sessions, so it stays as it was.\n") {
		t.Errorf("a smaller recount: exit %d stdout %q", res.exitCode, res.stdout)
	}
	if k, ok, _ := seed.ReadKept(e.home); !ok || k.All.Sessions != 2 {
		t.Errorf("a smaller recount replaced the kept count: %+v", k)
	}
}
