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
		if f.Tier() != Proven || f.Commit() != "abc123" || f.Extractor() != "gotypes@v1" || f.Config() != prog.Config().String() {
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

// PL5 with the second tier and the new ids, on two real commits: the stored
// log, decoded and reconciled forward by both producers, has the view a full
// rebuild has. Commit 2 swaps one implementer of Sizer for another, so the
// ALGORITHMIC tier changes by one assertion and one tombstone, and the
// PROVEN tier changes around it.
func TestPL5_TwoTiersIncrementalViewByteIdenticalToFull(t *testing.T) {
	dir, commits := idRepo(t)
	both := func(l *Log, commit string) (Delta, Delta) {
		prog := loadAt(t, dir, commit)
		dp, err := Sync(l, prog, commit)
		if err != nil {
			t.Fatal(err)
		}
		dc, err := SyncCHA(l, prog, commit)
		if err != nil {
			t.Fatal(err)
		}
		return dp, dc
	}
	full := func(commit string) *Log {
		var l Log
		both(&l, commit)
		return &l
	}
	step := func(prev *Log, commit string) (*Log, Delta, Delta) {
		l, err := DecodeLog(prev.Encode())
		if err != nil {
			t.Fatal(err)
		}
		dp, dc := both(l, commit)
		return l, dp, dc
	}

	l1 := full(commits[0])
	v1 := l1.View()
	large := Triple{"func:" + idm + "/core.Measure", MayCall, "method:" + idm + "/core.Large.Size"}
	medium := Triple{"func:" + idm + "/core.Measure", MayCall, "method:" + idm + "/core.Medium.Size"}
	small := Triple{"func:" + idm + "/core.Measure", MayCall, "method:" + idm + "/core.Small.Size"}
	live := map[Triple]Tier{}
	for _, f := range l1.Live() {
		live[f.Triple()] = f.Tier()
	}
	if live[large] != Algorithmic || live[small] != Algorithmic || live[medium] != 0 {
		t.Fatalf("commit 1 may-call facts wrong: %v %v %v", live[large], live[small], live[medium])
	}

	l2, dp, dc := step(l1, commits[1])
	if !bytes.Equal(l2.View(), full(commits[1]).View()) {
		t.Fatal("commit 2: incremental view differs from full rebuild")
	}
	if dc != (Delta{Asserted: 1, Retracted: 1}) {
		t.Errorf("CHA delta = %+v, want one assertion (Medium) and one tombstone (Large)", dc)
	}
	if dp.Asserted == 0 || dp.Retracted == 0 {
		t.Errorf("PROVEN delta = %+v", dp)
	}
	live = map[Triple]Tier{}
	for _, f := range l2.Live() {
		live[f.Triple()] = f.Tier()
	}
	if live[large] != 0 || live[medium] != Algorithmic || live[small] != Algorithmic {
		t.Errorf("commit 2 may-call facts: %v %v %v", live[large], live[medium], live[small])
	}
	if !bytes.HasPrefix(l2.Encode(), l1.Encode()) {
		t.Error("commit 2: history rewritten")
	}
	tomb := false
	for _, r := range l2.Records() {
		if r.Op() == Retract && r.Fact().Triple() == large && r.Fact().Tier() == Algorithmic && r.Fact().Commit() == commits[1] {
			tomb = true
		}
	}
	if !tomb {
		t.Error("the withdrawn dispatch edge left no ALGORITHMIC tombstone")
	}

	// Back to commit 1: both tiers return to the first view.
	l3, _, _ := step(l2, commits[0])
	if !bytes.Equal(l3.View(), v1) || !bytes.Equal(l3.View(), full(commits[0]).View()) {
		t.Fatal("commit 1 again: view did not return")
	}
	// An unchanged tree appends nothing in either tier.
	l4, dp, dc := step(l3, commits[0])
	if l4.Len() != l3.Len() || dp != (Delta{}) || dc != (Delta{}) {
		t.Fatalf("no-op sync appended: %+v %+v", dp, dc)
	}
}

// A log written by the previous extractor (its ids spelled the old way, its
// label gotypes@v0) migrates on the next Sync: the old facts are tombstoned
// and the new ones asserted, with the rest of the log untouched.
func TestPL5_OldExtractorFactsAreTombstonedOnSync(t *testing.T) {
	prog := loadT(t, testModule)
	var l Log
	v0, err := newProducer("gotypes", "v0", Proven)
	if err != nil {
		t.Fatal(err)
	}
	oldFact := Triple{"method:(*example.test/mod/a.Square).Area", Calls, "func:fmt.Sprint"}
	if _, err := l.Reconcile(v0, prog.Config().String(), "c0", []Triple{oldFact}); err != nil {
		t.Fatal(err)
	}
	other := mustProposed(t, "model")
	if _, err := l.Reconcile(other, prog.Config().String(), "c0", []Triple{tri("a", Calls, "b")}); err != nil {
		t.Fatal(err)
	}
	d, err := Sync(&l, prog, "c1")
	if err != nil || d.Retracted != 1 || d.Asserted == 0 {
		t.Fatalf("%+v %v", d, err)
	}
	for _, f := range l.Live() {
		if f.Triple() == oldFact {
			t.Error("old-spelling fact still live")
		}
	}
	var mine int
	for _, f := range l.Live() {
		if f.Tier() == Proposed && f.Triple() == tri("a", Calls, "b") {
			mine++
		}
	}
	if mine != 1 {
		t.Error("another producer's fact was disturbed")
	}
}
