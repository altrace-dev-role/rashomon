package acceptance

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRubricERRORS4_WatchAndDetachRefuseADanglingSettingsLink: settings.json
// is a dotfile manager's symlink whose target is gone. watch and detach both
// exit non-zero naming the link, and the link is still a link afterwards,
// pointing where it did. watch used to exit 0 and replace it with a regular
// file, so the managed file stopped being the one Claude Code reads.
func TestRubricERRORS4_WatchAndDetachRefuseADanglingSettingsLink(t *testing.T) {
	e := newEnv(t)
	target := filepath.Join(t.TempDir(), "dotfiles", "settings.json")
	link := e.settingsPath()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	for name, res := range map[string]result{"watch": e.watch(), "detach --all": e.run("", nil, "detach", "--all")} {
		if res.exitCode == 0 {
			t.Errorf("%s over a dangling settings link exited 0; stdout %q", name, res.stdout)
		}
		if !strings.Contains(res.stderr, target) {
			t.Errorf("%s did not name the link's target %s: stderr %q", name, target, res.stderr)
		}
		info, err := os.Lstat(link)
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("after %s the settings link is gone or a regular file (err %v)", name, err)
		}
		if got, err := os.Readlink(link); err != nil || got != target {
			t.Errorf("after %s the link points at %q (%v), want %q", name, got, err, target)
		}
	}
}
