package settings

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// symlinkTo makes link point at target, skipping where symlinks need a
// privilege the test does not have.
func symlinkTo(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
}

func assertStillALink(t *testing.T, link, target string) {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("the link is gone: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("%s was replaced by a regular file", link)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("the link now points at %q (%v), want %q", got, err, target)
	}
}

// TestRubricERRORS4_DanglingSymlinkIsRefused: a settings.json that is a
// symlink to nothing -- a dotfile manager's link whose target was moved or
// not yet created -- is refused by name, and neither the link nor anything
// under its target is touched. Before, the failed resolution read as "a
// path that does not exist yet" and the rename replaced the link with a
// regular file, the bug the symlink handling exists to prevent.
func TestRubricERRORS4_DanglingSymlinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	link := filepath.Join(dir, "settings.json")
	symlinkTo(t, target, link)

	if _, err := ReadOriginal(link); err == nil || !strings.Contains(err.Error(), target) {
		t.Errorf("ReadOriginal on a dangling link = %v, want an error naming its target %s", err, target)
	}
	err := Write(link, nil, []byte("{}\n"))
	if err == nil {
		t.Fatal("Write over a dangling symlink succeeded")
	}
	if !strings.Contains(err.Error(), link) || !strings.Contains(err.Error(), target) {
		t.Errorf("error %q does not name the link %s and its target %s", err, link, target)
	}
	assertStillALink(t, link, target)
	if _, err := os.Stat(filepath.Dir(target)); err == nil {
		t.Errorf("the target's directory was created")
	}
}

// TestRubricERRORS4_LoadRefusesADanglingSymlink: Load on a settings.json
// that is a symlink to nothing reads as unreadable, the same answer
// ReadOriginal and Write already give it, not as an absent file. Before,
// Load's own os.ReadFile saw the same ENOENT an absent file gives and
// returned an empty Document, so status and watch read "not installed"
// over a settings file that was actually unreadable, a dotfiles repo gone
// missing rather than a fresh machine.
func TestRubricERRORS4_LoadRefusesADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	link := filepath.Join(dir, "settings.json")
	symlinkTo(t, target, link)

	if _, err := Load(link); err == nil || !strings.Contains(err.Error(), target) {
		t.Errorf("Load on a dangling link = %v, want an error naming its target %s", err, target)
	}
}

// TestRubricERRORS4_WorkingSymlinkIsWrittenThrough is the control: a link
// that resolves is followed, the target gets the new bytes, and the link
// stays a link.
func TestRubricERRORS4_WorkingSymlinkIsWrittenThrough(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	symlinkTo(t, target, link)

	original, err := ReadOriginal(link)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("{\"hooks\": {}}\n")
	if err := Write(link, original, want); err != nil {
		t.Fatalf("Write through a working link: %v", err)
	}
	assertStillALink(t, link, target)
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, want) {
		t.Errorf("target holds %q (%v), want %q", got, err, want)
	}
}

// TestRubricERRORS4_AbsentFileIsAFirstInstall: a path with nothing at it,
// link or file, is still the ordinary first install.
func TestRubricERRORS4_AbsentFileIsAFirstInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original, err := ReadOriginal(path)
	if err != nil || original != nil {
		t.Fatalf("ReadOriginal on an absent file = %q, %v; want nil, nil", original, err)
	}
	if err := Write(path, original, []byte("{}\n")); err != nil {
		t.Fatalf("Write on an absent file: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{}\n" {
		t.Errorf("file holds %q (%v)", got, err)
	}
}
