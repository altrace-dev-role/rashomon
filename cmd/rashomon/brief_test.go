package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

// briefSession is an installed store and a project directory holding
// CLAUDE.md and go.mod, for probe start and the verb to brief.
type briefSession struct {
	t         *testing.T
	installID string
	dir       string
}

func newBriefSession(t *testing.T) briefSession {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	s := briefSession{t: t, installID: benchInstall(t, root, cfg), dir: t.TempDir()}
	for name, body := range map[string]string{"CLAUDE.md": "# rules\n", "go.mod": "module github.com/x/y\n"} {
		if err := os.WriteFile(filepath.Join(s.dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// start runs probe start on a SessionStart payload from s.dir with source
// (none when ""), and returns its stdout; stderr must stay empty.
func (s briefSession) start(source string) string {
	s.t.Helper()
	head := newBenchHead("SessionStart")
	head.CWD = s.dir
	in := map[string]any{"session_id": head.SessionID, "transcript_path": head.TranscriptPath, "cwd": head.CWD,
		"permission_mode": head.PermissionMode, "hook_event_name": head.HookEventName}
	if source != "" {
		in["source"] = source
	}
	var stdout, stderr bytes.Buffer
	if code := cmdProbe([]string{"start", install.Marker, s.installID}, bytes.NewReader(benchJSON(s.t, in)), &stdout, &stderr); code != exitOK || stderr.Len() != 0 {
		s.t.Fatalf("probe start (source %q): exit %d, stderr %q", source, code, stderr.String())
	}
	return stdout.String()
}

// message is the systemMessage of out, which must be exactly one JSON object
// whose only key is systemMessage.
func message(t *testing.T, out string) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("stdout %q is not one JSON object: %v", out, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout %q holds more than one JSON value", out)
	}
	var msg string
	if len(obj) != 1 || json.Unmarshal(obj["systemMessage"], &msg) != nil {
		t.Fatalf("stdout %q: want one key, systemMessage, a string", out)
	}
	return msg
}

// namesRoot reports whether line is prefix and then root, or "..." and a
// tail of root where root is too long for the line.
func namesRoot(line, prefix, root string) bool {
	rest, ok := strings.CutPrefix(line, prefix)
	tail, cut := strings.CutPrefix(rest, "...")
	return ok && (rest == root || cut && tail != "" && strings.HasSuffix(root, tail))
}

// TestBR1e_StartupPrintsToTheUserOnly: probe start at startup prints the
// brief as one JSON object whose only key is systemMessage, the channel the
// user reads; nothing goes to the model (no additionalContext, no
// hookSpecificOutput). An absent source reads as startup. Its verify line,
// from a settings install, names the CLI's report of the payload's own
// session (the plugin's entry: TestBR1i_).
func TestBR1e_StartupPrintsToTheUserOnly(t *testing.T) {
	s := newBriefSession(t)
	for _, source := range []string{"startup", "clear", ""} {
		msg := message(t, s.start(source))
		first, _, _ := strings.Cut(msg, "\n")
		if !namesRoot(first, "rashomon brief: ", baseline.Key(s.dir)) || !strings.Contains(msg, "\n  instructions: CLAUDE.md 8 B\n") ||
			!strings.Contains(msg, "\n  build: go.mod (module github.com/x/y)\n") ||
			!strings.HasSuffix(msg, "\n  verify: rashomon report --session "+benchSessionID) {
			t.Errorf("source %q: systemMessage %q, want the brief of %s", source, msg, s.dir)
		}
	}
}

// TestBR1f_ResumeIsOneLine: at resume the brief is one line pointing at the
// verb by this binary's path (D189; the test binary's has nothing to quote);
// at compact probe start prints nothing.
func TestBR1f_ResumeIsOneLine(t *testing.T) {
	s := newBriefSession(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got := message(t, s.start("resume"))
	shown, ok := strings.CutSuffix(got, "; "+exe+" brief for the full brief")
	if !ok || strings.Contains(got, "\n") || !namesRoot(shown, "rashomon brief: resumed ", baseline.Key(s.dir)) {
		t.Errorf("resume: %q, want one line naming %s and the verb", got, baseline.Key(s.dir))
	}
	if out := s.start("compact"); out != "" {
		t.Errorf("compact: stdout %q, want nothing", out)
	}
}

// TestBR1g_TheVerbPrintsTheSameBrief: `rashomon brief DIR` prints, as plain
// text, the brief probe start printed for a session in DIR, but for the
// verify line: at a terminal there is no session to name.
func TestBR1g_TheVerbPrintsTheSameBrief(t *testing.T) {
	s := newBriefSession(t)
	start := message(t, s.start("startup"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"brief", s.dir}, strings.NewReader(""), &stdout, &stderr); code != exitOK || stderr.Len() != 0 {
		t.Fatalf("brief: exit %d, stderr %q", code, stderr.String())
	}
	body, _, _ := strings.Cut(start, "\n  verify: ")
	if want := body + "\n  verify: rashomon status for coverage; rashomon report --session ID\n"; stdout.String() != want {
		t.Errorf("the verb printed\n%s\nwant\n%s", stdout.String(), want)
	}
}
