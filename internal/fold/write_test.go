package fold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWrite_WritesAtomicallyAndReadsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.json")
	f := &File{V: Version, Writer: "rashomon-test", Records: Cursor{Offset: 10, Seq: 1, SizeSeen: 10}}

	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got File
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Writer != "rashomon-test" || got.Records.Offset != 10 {
		t.Errorf("read back %+v, want the written file", got)
	}

	// No leftover temp files in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "fold.json" {
			t.Errorf("leftover entry %q after Write", e.Name())
		}
	}
}

func TestWrite_ModeIs0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes do not apply")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.json")
	if err := Write(path, &File{V: Version, Writer: "w"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
}

func TestWrite_ReplacesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.json")
	if err := Write(path, &File{V: Version, Writer: "first", Records: Cursor{Offset: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, &File{V: Version, Writer: "second", Records: Cursor{Offset: 2}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got File
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Writer != "second" || got.Records.Offset != 2 {
		t.Errorf("read back %+v after a second Write, want the second file's content", got)
	}
}

func TestWrite_RefusesOverMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.json")
	// A Verify section well past 64 KiB forces the whole marshalled
	// document past MaxBytes.
	f := &File{V: Version, Writer: "w", Verify: json.RawMessage(`"` + strings.Repeat("x", int(MaxBytes)) + `"`)}

	err := Write(path, f)
	if err == nil {
		t.Fatal("want an error for a fold over MaxBytes, got nil")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("a refused Write must not create or touch the target path")
	}
}

func TestWrite_NeverLeavesATempFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.json")
	f := &File{V: Version, Writer: "w", Verify: json.RawMessage(`"` + strings.Repeat("x", int(MaxBytes)) + `"`)}
	if err := Write(path, f); err == nil {
		t.Fatal("want an error for a fold over MaxBytes, got nil")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("directory holds %v after a refused Write, want it untouched", entries)
	}
}
