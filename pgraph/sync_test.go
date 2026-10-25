package pgraph

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// PL5 done-when: an incremental rebuild (decode the stored log, reconcile the
// new snapshot onto it) has a view byte-identical to a full rebuild.
func TestPL5_IncrementalViewByteIdenticalToFull(t *testing.T) {
	dir := copyTree(t, testModule)
	full := func(commit string) *Log {
		var l Log
		if _, err := Sync(&l, loadT(t, dir), commit); err != nil {
			t.Fatal(err)
		}
		return &l
	}
	step := func(prev *Log, commit string) *Log {
		l, err := DecodeLog(prev.Encode()) // through the stored form
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Sync(l, loadT(t, dir), commit); err != nil {
			t.Fatal(err)
		}
		return l
	}

	l1 := full("c1")
	v1 := l1.View()

	// c2: add a function and an edge, remove an edge (Closure no longer calls Helper).
	appendTo(t, filepath.Join(dir, "a", "a.go"), "\nfunc Extra() { Helper() }\n")
	appendTo(t, filepath.Join(dir, "b", "b.go"), "\nfunc Extra2() { _ = a.New(2) }\n")
	src, _ := os.ReadFile(filepath.Join(dir, "a", "a.go"))
	os.WriteFile(filepath.Join(dir, "a", "a.go"), bytes.Replace(src, []byte("func() { Helper() }"), []byte("func() {}"), 1), 0o644)

	l2 := step(l1, "c2")
	if !bytes.Equal(l2.View(), full("c2").View()) {
		t.Fatal("c2: incremental view differs from full rebuild")
	}
	if bytes.Equal(l2.View(), v1) {
		t.Fatal("c2: edit did not change the view; test proves nothing")
	}
	hasTombstone := false
	for _, r := range l2.Records() {
		hasTombstone = hasTombstone || r.Op() == Retract
	}
	if !hasTombstone {
		t.Fatal("c2: removed edge left no tombstone")
	}
	if !bytes.HasPrefix(l2.Encode(), l1.Encode()) {
		t.Fatal("c2: history rewritten")
	}

	// c3: revert everything; retracted facts come back by fresh assertion.
	os.WriteFile(filepath.Join(dir, "a", "a.go"), mustRead(t, filepath.Join(testModule, "a", "a.go")), 0o644)
	os.WriteFile(filepath.Join(dir, "b", "b.go"), mustRead(t, filepath.Join(testModule, "b", "b.go")), 0o644)
	l3 := step(l2, "c3")
	if !bytes.Equal(l3.View(), v1) || !bytes.Equal(l3.View(), full("c3").View()) {
		t.Fatal("c3: revert did not return to the original view")
	}
	if l3.Len() <= l2.Len() {
		t.Fatal("c3: reverting appended nothing")
	}

	// Syncing an unchanged tree appends nothing.
	l4 := step(l3, "c4")
	if l4.Len() != l3.Len() {
		t.Fatalf("no-op sync appended %d records", l4.Len()-l3.Len())
	}
}

func TestPL5_SyncRecordsProvenance(t *testing.T) {
	var l Log
	prog := loadT(t, testModule)
	d, err := Sync(&l, prog, "abc123")
	if err != nil || d.Asserted == 0 || d.Retracted != 0 {
		t.Fatalf("%+v %v", d, err)
	}
	for _, f := range l.Live() {
		if f.Tier() != Proven || f.Commit() != "abc123" || f.Extractor() != "gotypes@v0" || f.Config() != prog.Config().String() {
			t.Fatalf("provenance: %+v", f)
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
