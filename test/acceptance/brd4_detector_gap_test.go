package acceptance

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BRD-4 ruling, 2026-10-24: the detector gap is a bug, and an Unknown session
// is recorded, counted, and shown in the report so a non-zero count is chased.
// The Detect rows live in internal/harness; these run the compiled binary.

// firstRealPreToolUse is the first PreToolUse of the 2.1.280 capture, as the
// capture holds it: transcript_path is the scrubbed "~/.claude/projects/..."
// form, which names no directory this machine's config dir is.
func firstRealPreToolUse(t *testing.T) string {
	t.Helper()
	f, err := os.Open(filepath.Join(realFixtureDir, "payloads.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"hook_event_name":"PreToolUse"`) {
			return sc.Text()
		}
	}
	t.Fatal("the capture holds no PreToolUse")
	return ""
}

// noUnknownCount fails if harness_counts.json holds any claude@unknown count.
func noUnknownCount(t *testing.T, e *env) {
	t.Helper()
	b, err := os.ReadFile(harnessCountsPath(e))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "claude@unknown") {
		t.Errorf("harness_counts.json counted an unknown harness:\n%s", b)
	}
}

// TestBRD4_CapturedSessionWithNoEntrypointOrConfigDirMatchReadsAsClaude is
// the case CI hit, through the binary: a real capture, CLAUDE_CODE_ENTRYPOINT
// unset, a transcript outside the config dir, no CLAUDE_PROJECT_DIR. It used
// to be recorded with harness_id "unknown" and counted; it is now Claude Code.
func TestBRD4_CapturedSessionWithNoEntrypointOrConfigDirMatchReadsAsClaude(t *testing.T) {
	e := newEnv(t)
	e.watched(realSession)
	if res := e.hook(firstRealPreToolUse(t), "CLAUDE_CODE_ENTRYPOINT="); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	decls := e.declarations(realSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	if got := decls[0].str("harness_id"); got != "" {
		t.Errorf("declaration harness_id = %q, want none: the capture is Claude Code", got)
	}
	noUnknownCount(t, e)
}

// TestBRD4_ProjectDirEnvAloneReadsAsClaude: a payload with nothing Claude-shaped
// in it, under no entrypoint, is Claude Code when the hook was started with
// CLAUDE_PROJECT_DIR.
func TestBRD4_ProjectDirEnvAloneReadsAsClaude(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if res := e.hook(defaultPayload().build(t), "CLAUDE_CODE_ENTRYPOINT=", "CLAUDE_PROJECT_DIR="+e.cwd); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	if got := decls[0].str("harness_id"); got != "" {
		t.Errorf("declaration harness_id = %q, want none", got)
	}
	noUnknownCount(t, e)
}

const unknownLine = "harness unknown: 1 session in the last 30 days (each one is a detector gap to chase)\n"

// TestBRD4_ReportShowsTheUnknownHarnessCount: an Unknown session is recorded
// with harness_id "unknown", counted, and the report says how many, in text and
// in JSON. A session that detects as Claude Code reads "none" and 0.
func TestBRD4_ReportShowsTheUnknownHarnessCount(t *testing.T) {
	jsonCount := func(e *env) float64 {
		t.Helper()
		res := e.run("", nil, "report", "--json")
		if res.exitCode != 0 {
			t.Fatalf("report --json: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
			t.Fatal(err)
		}
		n, ok := doc["harness_unknown_sessions"].(float64)
		if !ok {
			t.Fatalf("report --json carries no numeric harness_unknown_sessions: %v", doc["harness_unknown_sessions"])
		}
		return n
	}

	healthy := newEnv(t)
	healthy.watched(testSession)
	healthy.mustHook(defaultPayload().build(t))
	res := healthy.run("", nil, "report")
	if !strings.Contains(res.stdout, "harness unknown: none\n") {
		t.Errorf("report for a session detected as Claude Code lacks \"harness unknown: none\":\n%s", res.stdout)
	}
	if n := jsonCount(healthy); n != 0 {
		t.Errorf("healthy harness_unknown_sessions = %v, want 0", n)
	}

	e := newEnv(t)
	e.watched(testSession)
	if res := e.hook(defaultPayload().build(t), "CLAUDE_CODE_ENTRYPOINT="); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := e.declarations(testSession)[0].str("harness_id"); got != "unknown" {
		t.Fatalf("premise: declaration harness_id = %q, want %q", got, "unknown")
	}
	res = e.run("", nil, "report")
	if !strings.Contains(res.stdout, unknownLine) {
		t.Errorf("report lacks %q:\n%s", unknownLine, res.stdout)
	}
	if n := jsonCount(e); n != 1 {
		t.Errorf("harness_unknown_sessions = %v, want 1", n)
	}
}
