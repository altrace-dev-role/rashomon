package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRemove covers the two cases where Remove must leave the file alone: it
// changed since it was read, or it is a symlink, which is the user's
// arrangement. The first is reported as ErrChanged, so the caller re-reads;
// the second as not removed, so the caller writes instead.
func TestRemove(t *testing.T) {
	const body = "{}\n"

	t.Run("unchanged", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		removed, err := Remove(path, []byte(body))
		if err != nil || !removed {
			t.Fatalf("Remove: %v, %v; want true, nil", removed, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the file is still there: %v", err)
		}
	})

	t.Run("changed since it was read", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		if err := os.WriteFile(path, []byte(`{"hooks":{"Stop":[]}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		removed, err := Remove(path, []byte(body))
		if !errors.Is(err, ErrChanged) || removed {
			t.Fatalf("Remove: %v, %v; want false, ErrChanged", removed, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("a changed file was removed: %v", err)
		}
	})

	t.Run("a symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "managed.json")
		if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "hooks.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		removed, err := Remove(path, []byte(body))
		if err != nil || removed {
			t.Fatalf("Remove: %v, %v; want false, nil", removed, err)
		}
		for _, p := range []string{path, target} {
			if _, err := os.Lstat(p); err != nil {
				t.Errorf("%s was removed: %v", filepath.Base(p), err)
			}
		}
	})
}
