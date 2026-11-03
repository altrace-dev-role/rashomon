package acceptance

// `rashomon audit` (card SEED1, PF-AU1): the corrections a person made to
// their agent, counted from Claude Code's own transcripts, through the real
// binary against a fake configuration directory. What is asserted is the
// command's contract with a machine: it prints the card, it writes nothing
// and creates no store, and its window is the one asked for.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// seedLine is one transcript line of session in Claude Code 2.1.280's shape.
func seedLine(t *testing.T, session string, m map[string]any) string {
	t.Helper()
	m["sessionId"], m["isSidechain"] = session, false
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// seedSession is a session in which the person rejects a git push.
func seedSession(t *testing.T, session string) []string {
	user := func(content any) map[string]any {
		return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}}
	}
	call := map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant",
		"content": []map[string]any{{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{"command": "git push --force"}}}}}
	rejected := user([]map[string]any{{"type": "tool_result", "tool_use_id": "t1", "is_error": true,
		"content": "The user doesn't want to proceed with this tool use. The tool use was rejected."}})
	rejected["toolDenialKind"] = "user-rejected"
	return []string{seedLine(t, session, user("ship it")), seedLine(t, session, call), seedLine(t, session, rejected)}
}

// configState is every file under dir with its size and modification time.
func configState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[p] = fmt.Sprintf("%v %v %d", info.Mode(), info.ModTime(), info.Size())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestSEED1c_AuditPrintsTheCardAndWritesNothing: two sessions with the same
// rejection print the card with the row that recurs; the store root stays
// empty and the configuration directory is what it was, name for name and
// mtime for mtime. The second half is the control: it holds on a stub too.
func TestSEED1c_AuditPrintsTheCardAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	writeSessionTranscript(t, e.configDir, "sess-1", seedSession(t, "sess-1")...)
	writeSessionTranscript(t, e.configDir, "sess-2", seedSession(t, "sess-2")...)
	before := configState(t, e.configDir)

	res := e.run("", nil, "audit")
	if res.exitCode != 0 {
		t.Fatalf("audit exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, want := range []string{
		"rashomon audit: the last 14 days of Claude Code on this machine\n",
		"  2 sessions in 1 project, 2 tool calls\n",
		"       2     2  you said no to a version-control command\n",
		"0 tokens",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("audit stdout does not hold %q:\n%s", want, res.stdout)
		}
	}
	entries, err := os.ReadDir(e.home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the store root is not empty after `rashomon audit`: %v", entries)
	}
	if after := configState(t, e.configDir); !reflect.DeepEqual(before, after) {
		t.Errorf("the configuration directory changed after `rashomon audit`")
	}
}

// TestSEED1d_TheWindowIsTheOneAskedFor: a transcript last written 30 days ago
// is outside the default 14 and said, not silently dropped; --days 60 reads
// it; a window that is not a positive day count is refused; help names it.
func TestSEED1d_TheWindowIsTheOneAskedFor(t *testing.T) {
	e := newEnv(t)
	p := writeSessionTranscript(t, e.configDir, "sess-old", seedSession(t, "sess-old")...)
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	res := e.run("", nil, "audit")
	want := "rashomon audit: no Claude Code sessions in the last 14 days on this machine.\n" +
		"  1 transcript older than that was not read.\n"
	if res.exitCode != 0 || res.stdout != want {
		t.Errorf("audit exit %d stdout %q, want %q", res.exitCode, res.stdout, want)
	}
	if res := e.run("", nil, "audit", "--days", "60"); res.exitCode != 0 ||
		!strings.Contains(res.stdout, "the last 60 days") || !strings.Contains(res.stdout, "1 session in 1 project, 1 tool call\n") {
		t.Errorf("audit --days 60: exit %d stdout %q", res.exitCode, res.stdout)
	}
	for _, args := range [][]string{{"audit", "--days", "0"}, {"audit", "--days"}, {"audit", "--json"}} {
		if res := e.run("", nil, args...); res.exitCode == 0 {
			t.Errorf("%v: exit 0, want a refusal; stdout %q", args, res.stdout)
		}
	}
	if res := e.run("", nil, "help"); !strings.Contains(res.stdout, "rashomon audit [--days N]") {
		t.Errorf("help does not name audit:\n%s", res.stdout)
	}
}
