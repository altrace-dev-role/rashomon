package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// H-154 -- a project reached through a symlink has one location.
//
// A declared gate is located at the project's physical path (the project key
// resolves symlinks) and a run at the cwd the payload spelled. Under a
// symlink, or macOS's /var and /tmp, the two digests differed and a trusted
// spec's gate never matched its own runs. The binary now digests the
// directory as it is on disk, so the same project spelled three ways, through
// a link, through the temp directory and physically, records one cwd_digest;
// and a directory that is a different place records another.
//
// Break: digest the cwd as spelled and the three spellings are three
// locations.
func TestH154_ASymlinkedProjectIsOneLocation(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	phys, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	cwds := []string{link, real, phys, filepath.Join(base, "elsewhere")}
	for i, cwd := range cwds {
		p := defaultPayload()
		p.CWD = cwd
		p.ToolInput = map[string]any{"command": "go test ./..."}
		p.ToolUseID = "toolu_" + string(rune('a'+i))
		e.mustHook(p.build(t))
	}
	decls := e.declarations(testSession)
	if len(decls) != len(cwds) {
		t.Fatalf("%d declarations, want %d", len(decls), len(cwds))
	}
	d := make([]string, len(decls))
	for i := range decls {
		d[i] = decls[i].str("cwd_digest")
	}
	if d[0] == "" || d[0] != d[1] || d[1] != d[2] {
		t.Errorf("one project spelled three ways has digests %q, %q, %q; want one", d[0], d[1], d[2])
	}
	if d[3] == d[0] {
		t.Errorf("another directory shares the project's digest %q", d[3])
	}
}
