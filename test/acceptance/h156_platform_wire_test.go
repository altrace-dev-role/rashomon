package acceptance

// H-156: the platform graph has a producer and a reader in a real install
// (card PL-WIRE1). `pgraph sync <root>`, the pgraph module's own command,
// writes the log and the per-file digests under root/.rashomon; the Stop
// hook, `rashomon recap` as watch installs it, reads them for the session's
// written Go files, and the ledger `rashomon report --ledger` prints owes
// each written package's test gate: open after the edit, met after a `go
// test` that names the package. The risk it holds down is the one this card
// exists for: a log nothing writes and a reader nothing calls, each green
// against its own fixture. Here the writer is the real pgraph binary on a
// real module and the reader is the installed hook, so the two sides of the
// file boundary (its path, its header, the digests' format) are held to each
// other, and an edit after the sync withholds the package's hash until a
// re-sync gives it back.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	pgraphOnce sync.Once
	pgraphBin  string
	pgraphErr  error
)

// pgraphBinary builds the pgraph command from this checkout's pgraph module
// once per test process.
func pgraphBinary(t *testing.T) string {
	t.Helper()
	pgraphOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pgraph-bin-")
		if err != nil {
			pgraphErr = err
			return
		}
		pgraphBin = filepath.Join(dir, "pgraph")
		cmd := exec.Command("go", "build", "-o", pgraphBin, "./cmd/pgraph")
		cmd.Dir = filepath.Join(moduleRoot, "pgraph")
		if b, err := cmd.CombinedOutput(); err != nil {
			pgraphErr = err
			pgraphBin = string(b)
		}
	})
	if pgraphErr != nil {
		t.Fatalf("building pgraph: %v\n%s", pgraphErr, pgraphBin)
	}
	return pgraphBin
}

// h156Sync runs `pgraph sync dir` and fails the test on a nonzero exit.
func h156Sync(t *testing.T, dir string) {
	t.Helper()
	out, err := exec.Command(pgraphBinary(t), "sync", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("pgraph sync: %v\n%s", err, out)
	}
	t.Logf("pgraph sync: %s", out)
}

// h156Files is a Go module whose CI runs `go test ./...`, the project's own
// word for its test gate, with a package a and a package b that calls it.
var h156Files = map[string]string{
	"go.mod":                   "module example.com/m\n\ngo 1.25.0\n",
	"a/a.go":                   "package a\n\nfunc F() int { return 1 }\n",
	"b/b.go":                   "package b\n\nimport \"example.com/m/a\"\n\nfunc G() int { return a.F() }\n",
	".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n",
}

// h156Call records one tool call of session in dir through the installed
// PreToolUse and PostToolUse entries, ok.
func (e *env) h156Call(dir, session, id, tool string, input map[string]any) {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	extra := map[string]any{"cwd": dir, "session_id": session}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	extra["tool_response"] = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
	if res := e.post(stPayload(e.t, "PostToolUse", id, tool, input, extra), envs...); res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
}

// h156Stop runs the installed Stop entry for session in dir.
func (e *env) h156Stop(dir, session string) {
	e.t.Helper()
	b, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": session, "cwd": dir,
		"transcript_path": "/tmp/transcripts/" + session + ".jsonl", "stop_hook_active": false,
		"last_assistant_message": "I changed a/a.go."})
	if err != nil {
		e.t.Fatal(err)
	}
	if res := e.recap(string(b), stAgent, "CI="); res.exitCode != 0 {
		e.t.Fatalf("recap: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}

// h156Rows is the session's platform_impact rows as `report --ledger` prints
// them, each split into its seven fields.
func (e *env) h156Rows(session string) [][]string {
	e.t.Helper()
	res := e.run("", nil, "report", "--ledger", "--session", session)
	if res.exitCode != 0 {
		e.t.Fatalf("report --ledger: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rows [][]string
	for _, line := range strings.Split(res.stdout, "\n") {
		if f := strings.Fields(line); len(f) == 7 && f[2] == "platform_impact" {
			rows = append(rows, f)
		}
	}
	e.t.Logf("ledger:\n%s", res.stdout)
	return rows
}

// h156Session is a probed session in dir that edits a/a.go, leaving its
// bytes as they are, and stops.
func (e *env) h156Session(dir, session string) {
	e.t.Helper()
	if res := e.probe("start", session); res.exitCode != 0 {
		e.t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.h156Call(dir, session, "toolu_e", "Edit", map[string]any{"file_path": filepath.Join(dir, "a", "a.go"), "old_string": "1", "new_string": "1"})
	e.h156Stop(dir, session)
}

// TestH156_ASyncedPackageOwesItsTestUntilAGoTestNamesIt: after `pgraph
// sync`, an edit of a/a.go owes package a's row, open, its family the
// package and the hash of its facts; after `go test ./a/...` the row is met.
// Positive twin of TestH156b. Break: leave the Stop's Platform unset, or move
// where pgraph writes the log, and the ledger has no such row.
func TestH156_ASyncedPackageOwesItsTestUntilAGoTestNamesIt(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	e.h156Session(dir, "sess-h156")
	rows := e.h156Rows("sess-h156")
	if len(rows) != 1 || rows[0][3] != "platform" || rows[0][4] != "open" || !strings.HasPrefix(rows[0][6], "example.com/m/a@") ||
		len(strings.TrimPrefix(rows[0][6], "example.com/m/a@")) != 64 {
		t.Fatalf("after the edit: rows %q, want package a's row open under its hash", rows)
	}
	hash := rows[0][6]
	e.h156Call(dir, "sess-h156", "toolu_g", "Bash", map[string]any{"command": "go test ./a/..."})
	e.h156Stop(dir, "sess-h156")
	if rows := e.h156Rows("sess-h156"); len(rows) != 1 || rows[0][4] != "met" || rows[0][6] != hash {
		t.Errorf("after go test ./a/...: rows %q, want it met", rows)
	}
}

// TestH156b_AnEditAfterTheSyncWithholdsTheHashUntilAResync: the file changed
// after the sync reads stale, the package's hash withheld and its row still
// owed; a second `pgraph sync` gives a hash back, a new one, since a's facts
// changed. Break: compare no digest, and the stale hash prints as a's.
func TestH156b_AnEditAfterTheSyncWithholdsTheHashUntilAResync(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a", "a.go"), []byte("package a\n\nfunc F() int { return 2 }\n\nfunc H() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.h156Session(dir, "sess-h156b")
	if rows := e.h156Rows("sess-h156b"); len(rows) != 1 || rows[0][4] != "open" || rows[0][6] != "example.com/m/a@stale" {
		t.Fatalf("after an edit past the sync: rows %q, want a's row open under a@stale", rows)
	}
	h156Sync(t, dir)
	rows := e.h156Rows("sess-h156b")
	if len(rows) != 1 || rows[0][6] == "example.com/m/a@stale" || len(strings.TrimPrefix(rows[0][6], "example.com/m/a@")) != 64 {
		t.Errorf("after a re-sync: rows %q, want a's hash back", rows)
	}
}

// TestH156c_NoLogOrACorruptOneIsUnknown: a project nobody synced owes a's
// row unknown, never none (the package exists whether or not the graph does);
// a log that does not decode is one unknown row with no family, never a
// partial answer. Negative twins of TestH156. Break: read an absent log as
// no write, and the edit owes nothing.
func TestH156c_NoLogOrACorruptOneIsUnknown(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	e.h156Session(dir, "sess-h156c")
	if rows := e.h156Rows("sess-h156c"); len(rows) != 1 || rows[0][4] != "unknown" || rows[0][6] != "example.com/m/a@unknown" {
		t.Errorf("no log: rows %q, want a's row unknown", rows)
	}
	h156Sync(t, dir)
	log := filepath.Join(dir, ".rashomon", "pgraph.log")
	if err := os.WriteFile(log, []byte("pgraph-log v9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rows := e.h156Rows("sess-h156c"); len(rows) != 1 || rows[0][4] != "unknown" || rows[0][6] != "-" {
		t.Errorf("a corrupt log: rows %q, want one unknown row with no family", rows)
	}
}
