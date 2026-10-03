package hook

import (
	"encoding/json"
	"testing"
)

// startDirectory folds a line's leading cd targets onto the payload's cwd:
// an absolute target replaces the directory and a relative one joins it.
// Break: keep the cwd for a relative target, and `cd ../web && go test` is
// keyed on wherever the shell was, not on web.
func TestStartDirectory(t *testing.T) {
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
	} {
		in, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := startDirectory(tc.cwd, "Bash", in); got != tc.want {
			t.Errorf("startDirectory(%q, %q) = %q, want %q", tc.cwd, tc.cmd, got, tc.want)
		}
	}
}
