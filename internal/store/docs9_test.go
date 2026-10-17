package store

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// TestRubricDOCS9_AZeroStoreWritesNothing: every path a Store writes under is
// joined onto its root, so a Store that did not come from Open would write
// runs/, gaps.ndjson and the seq counters under whatever directory the
// process happened to be in, digesting under an empty HMAC key. Each writer
// now refuses it with errUnopenedStore, and the cwd stays empty.
func TestRubricDOCS9_AZeroStoreWritesNothing(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	var st Store
	now := time.Now()
	for name, write := range map[string]func() error{
		"AppendDeclaration": func() error { return st.AppendDeclaration(testDecl("s", "toolu_1")) },
		"AppendExecution":   func() error { return st.AppendExecution(Execution{SessionID: "s"}) },
		"AppendTerminal":    func() error { return st.AppendTerminal(Terminal{SessionID: "s"}) },
		"SpillTerminal":     func() error { return st.SpillTerminal(Terminal{SessionID: "s"}) },
		"AppendCoverage":    func() error { return st.AppendCoverage(Coverage{SessionID: "s"}) },
		"AppendGap":         func() error { return st.AppendGap(Gap{SessionID: "s"}) },
		"MarkProbe":         func() error { return st.MarkProbe("s", now) },
		"Evict":             func() error { _, err := st.Evict(1, "", now); return err },
		"ForgetWindow":      func() error { _, err := st.ForgetWindow(nil, nil, now); return err },
		"ForgetHost":        func() error { _, err := st.ForgetHost("h", now); return err },
	} {
		if err := write(); !errors.Is(err, errUnopenedStore) {
			t.Errorf("%s on a zero Store: %v, want errUnopenedStore", name, err)
		}
	}
	if entries, err := os.ReadDir(cwd); err != nil || len(entries) != 0 {
		t.Errorf("a zero Store wrote %d entries under the working directory (%v)", len(entries), err)
	}
	if d := st.HostDigest("example.com"); d != "" {
		t.Errorf("a zero Store digests under an empty key: %q", d)
	}
}

// TestRubricDOCS9_OneStoreIsSafeAcrossGoroutines backs the Store's doc: an
// undeferred Store holds no state its appends change, and each append takes
// its file's lock through a descriptor of its own, so goroutines sharing one
// Store get a total order with every seq handed out once. The central race
// run (-race) is what proves the "no data race" half.
func TestRubricDOCS9_OneStoreIsSafeAcrossGoroutines(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const writers, each = 8, 10
	errs := make(chan error, writers*each)
	var done []<-chan struct{}
	for w := 0; w < writers; w++ {
		done = append(done, safe.Go(func() {
			for i := 0; i < each; i++ {
				errs <- st.AppendDeclaration(testDecl("sess-shared", "toolu"))
			}
		}, func(err error) { errs <- err }))
	}
	for _, d := range done {
		<-d
	}
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err := st.ReadRun("sess-shared")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, d := range run.Declarations {
		if seen[d.Seq] || d.Seq < 1 || d.Seq > writers*each {
			t.Errorf("seq %d handed out twice or out of range", d.Seq)
		}
		seen[d.Seq] = true
	}
	if len(seen) != writers*each {
		t.Errorf("%d distinct seqs for %d appends", len(seen), writers*each)
	}
}

// TestRubricLLD8_KeyIsACopy: Key used to return the Store's own slice, so a
// caller that wrote into it changed every digest the Store made afterwards.
func TestRubricLLD8_KeyIsACopy(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before, key := st.HostDigest("example.com"), st.Key()
	original := bytes.Clone(key)
	key[0] ^= 0xff
	if got := st.HostDigest("example.com"); got != before {
		t.Errorf("writing into the slice Key returned changed the host digest")
	}
	if !bytes.Equal(st.Key(), original) {
		t.Errorf("writing into the slice Key returned changed the key")
	}
}
