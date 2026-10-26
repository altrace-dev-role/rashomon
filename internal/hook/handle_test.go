package hook

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// startDirectory folds a line's leading cd targets onto the payload's cwd:
// an absolute target replaces the directory and a relative one joins it, and
// `~` or `~/...` is the hook's own home, an absolute target. Break: keep the
// cwd for a relative target, and `cd ../web && go test` is keyed on wherever
// the shell was, not on web; join `~/x` under the cwd, and it is keyed on a
// directory named ~ inside it.
func TestStartDirectory(t *testing.T) {
	t.Setenv("HOME", "/home/sd")
	for _, tc := range []struct {
		cwd, cmd, want string
	}{
		{"/x", "cd /repo/web/ && go test ./...", "/repo/web"},
		{"/a/b", "cd .. && go test ./...", "/a"},
		{"/a/c", "cd .. && go test ./...", "/a"},
		{"/a", "cd /b && cd c && go test ./...", "/b/c"},
		{"/a", "cd b && cd /c && go test ./...", "/c"},
		{"", "cd sub && go test", ""},
		{"", "cd /b && cd c && go test", "/b/c"},
		{"/a/b", "go test ./...", "/a/b"},
		{"/a/b", "cd /x && cd - && go test ./...", "/a/b"},
		{"/a/b", "cd ~/x && go test ./...", "/home/sd/x"},
		{"/a/b", "cd ~ && go test ./...", "/home/sd"},
		{"", "cd ~/x && cd sub && go test", "/home/sd/x/sub"},
		{"/a/b", "cd sub && cd ~/ && go test", "/home/sd"},
	} {
		in, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		dirs, _ := shape.LeadingDirectory("Bash", in)
		if got := startDirectory(tc.cwd, dirs); got != tc.want {
			t.Errorf("startDirectory(%q, %q) = %q, want %q", tc.cwd, tc.cmd, got, tc.want)
		}
	}
}

// TestCaptureKeepsTheCWDOfAControlByteLine: a line holding a control byte
// moves the call's start directory nowhere, so its cwd digest is the
// payload's cwd. `cd /repo\r && ls` never enters /repo -- the shell reads
// the carriage return as part of the directory's name -- and the hook folded
// it in when it read the leading cds through DeriveWithDirs, which had lost
// LeadingDirectory's guard. The plain line is the control: it does move.
func TestCaptureKeepsTheCWDOfAControlByteLine(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const cwd = "/tmp/project"
	for i, tc := range []struct{ cmd, dir string }{
		{"cd /repo && ls", "/repo"},
		{"cd /repo\r && ls", cwd},
		{"cd /repo\x00 && ls", cwd},
		{"cd /repo\x07 && ls", cwd},
		{"cd /repo\x1b && ls", cwd},
	} {
		payload, err := json.Marshal(map[string]any{
			"session_id": "s-ctl", "tool_use_id": fmt.Sprintf("toolu_%d", i), "tool_name": "Bash",
			"cwd": cwd, "hook_event_name": "PreToolUse", "tool_input": map[string]any{"command": tc.cmd},
		})
		if err != nil {
			t.Fatal(err)
		}
		h := New(st, time.Now)
		h.Close(&Signals{}, h.Capture(strings.NewReader(string(payload))))
		run, err := st.ReadRun("s-ctl")
		if err != nil || len(run.Declarations) != i+1 {
			t.Fatalf("%q: %d declarations, %v", tc.cmd, len(run.Declarations), err)
		}
		if got, want := run.Declarations[i].CWDDigest, shape.CWDDigest(st.Key(), tc.dir); got != want {
			t.Errorf("%q: cwd digest is not %s's", tc.cmd, tc.dir)
		}
	}
}

// TestStartDirectoryWithoutAHome: with no home to expand `~` with, the
// directory is not known, and is returned empty, as past the fold's bound;
// a line without `~` is unaffected. Break: join `~` under the cwd when the
// home is missing, and the run is keyed on a directory the line never named.
func TestStartDirectoryWithoutAHome(t *testing.T) {
	t.Setenv("HOME", "")
	for _, tc := range []struct{ cwd, cmd, want string }{
		{"/a/b", "cd ~/x && go test ./...", ""},
		{"/a/b", "cd /x && cd ~ && go test ./...", ""},
		{"/a/b", "cd sub && go test ./...", "/a/b/sub"},
	} {
		in, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		dirs, _ := shape.LeadingDirectory("Bash", in)
		if got := startDirectory(tc.cwd, dirs); got != tc.want {
			t.Errorf("startDirectory(%q, %q) = %q, want %q", tc.cwd, tc.cmd, got, tc.want)
		}
	}
}
