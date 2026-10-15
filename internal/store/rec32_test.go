package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// chtimesTree sets the times of a run directory's files, which is what
// eviction reads a run's age from.
func chtimesTree(dir string, at time.Time) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Chtimes(filepath.Join(dir, f.Name()), at, at); err != nil {
			return err
		}
	}
	return nil
}

// testDecl is a declaration as a hook writes one, minus the fields these
// tests do not look at.
func testDecl(session, id string) Declaration {
	return Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: id, SessionID: session,
		RecordedAtMS: time.Now().UnixMilli()}
}

// appendRaw writes bytes to the end of a store file as they are, the way a
// damaged or foreign writer would.
func appendRaw(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
}

// declLineOfLength is a parseable declaration line exactly n bytes long, not
// counting its newline: the transcript path is padded to fit.
func declLineOfLength(t *testing.T, session, id string, n int) []byte {
	t.Helper()
	d := testDecl(session, id)
	base, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	d.TranscriptPath = strings.Repeat("p", n-len(base))
	line, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != n {
		t.Fatalf("built a %d-byte line, want %d", len(line), n)
	}
	return append(line, '\n')
}

// TestREC32_OverlongLineSkippedAndCounted: one line longer than the reader's
// limit used to fail the whole read (bufio.Scanner's ErrTooLong), so one
// record could take report, the end probe, forget and eviction down for the
// whole run. It is now skipped and counted like any line that does not
// parse, and the lines on either side of it read. A line of exactly maxLine
// bytes is still read.
func TestREC32_OverlongLineSkippedAndCounted(t *testing.T) {
	const session = "sess-overlong"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(testDecl(session, "toolu_before")); err != nil {
		t.Fatal(err)
	}
	records := filepath.Join(st.RunDir(session), FileRecords)
	appendRaw(t, records, append(bytes.Repeat([]byte("x"), maxLine+1), '\n'))
	appendRaw(t, records, declLineOfLength(t, session, "toolu_at_limit", maxLine))
	if err := st.AppendDeclaration(testDecl(session, "toolu_after")); err != nil {
		t.Fatal(err)
	}

	for name, read := range map[string]func() (*Run, error){
		"ReadRun":           func() (*Run, error) { return st.ReadRun(session) },
		"ReadRunConsistent": func() (*Run, error) { return st.ReadRunConsistent(session, 10*time.Millisecond) },
	} {
		run, err := read()
		if err != nil {
			t.Fatalf("%s failed on one over-long line: %v", name, err)
		}
		if run.Skipped != 1 {
			t.Errorf("%s: skipped %d lines, want 1 (the over-long line)", name, run.Skipped)
		}
		var ids []string
		for _, d := range run.Declarations {
			ids = append(ids, d.ToolUseID)
		}
		if got, want := strings.Join(ids, ","), "toolu_before,toolu_at_limit,toolu_after"; got != want {
			t.Errorf("%s read declarations %s, want %s", name, got, want)
		}
	}

	// The seq counter's recovery reads the same file through the same reader.
	if err := os.Remove(filepath.Join(st.RunDir(session), FileSeq)); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(testDecl(session, "toolu_recovered")); err != nil {
		t.Fatalf("seq recovery failed on an over-long line: %v", err)
	}
}

// TestREC32a_OverlongGapLineDoesNotHideTheGaps: the gaps file is read by the
// same reader, so one over-long line there cannot fail every report.
func TestREC32a_OverlongGapLineDoesNotHideTheGaps(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gap := Gap{Type: TypeGap, SchemaVersion: SessionSchemaVersion, SessionID: "s", Reason: GapForget}
	if err := st.AppendGap(gap); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, filepath.Join(st.Root(), FileGaps), append(bytes.Repeat([]byte("x"), maxLine+1), '\n'))
	if err := st.AppendGap(gap); err != nil {
		t.Fatal(err)
	}
	gaps, _, err := st.ReadGaps()
	if err != nil {
		t.Fatalf("ReadGaps failed on one over-long line: %v", err)
	}
	if len(gaps) != 2 {
		t.Errorf("read %d gaps around an over-long line, want 2", len(gaps))
	}
}

// TestREC32b_ForgetLeavesARunItCannotReadWhole: forget rewrites the files it
// edits, and a line it cannot hold cannot be written back, so a run holding
// one is left exactly as it was and named in the error -- while every other
// run is still forgotten. Before, the first such run ended the whole forget.
func TestREC32b_ForgetLeavesARunItCannotReadWhole(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	named := func(session, id string) Declaration {
		d := testDecl(session, id)
		d.Hosts = []string{"pypi.org"}
		return d
	}
	if err := st.AppendDeclaration(named("sess-bad", "toolu_bad")); err != nil {
		t.Fatal(err)
	}
	badRecords := filepath.Join(st.RunDir("sess-bad"), FileRecords)
	appendRaw(t, badRecords, append(bytes.Repeat([]byte("x"), maxLine+1), '\n'))
	before, err := os.ReadFile(badRecords)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(named("sess-good", "toolu_good")); err != nil {
		t.Fatal(err)
	}
	// The bad run is the older, so it is reached first.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(st.RunDir("sess-bad"), old, old); err != nil {
		t.Fatal(err)
	}

	for name, forget := range map[string]func() ([]Gap, error){
		"ForgetHost":   func() ([]Gap, error) { return st.ForgetHost("pypi.org", time.Now()) },
		"ForgetWindow": func() ([]Gap, error) { return st.ForgetWindow(nil, nil, time.Now()) },
	} {
		_, err := forget()
		if err == nil || !strings.Contains(err.Error(), "sess-bad") {
			t.Errorf("%s: error %v, want one naming the run it could not read", name, err)
		}
		if !errors.Is(err, errLineTooLong) {
			t.Errorf("%s: error %v, want errLineTooLong", name, err)
		}
		after, rerr := os.ReadFile(badRecords)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("%s rewrote a file it could not read whole", name)
		}
	}
	run, err := st.ReadRun("sess-good")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 0 {
		t.Errorf("the readable run kept %d declarations: forget stopped at the run it could not read", len(run.Declarations))
	}
}

// TestREC32_EvictContinuesPastUnreadableRun: eviction reads a run to count
// what its gap removes, and a run it could not read used to stop the whole
// pass, every time, once that run was the oldest -- and the probe threw the
// error away, so the store grew past its cap in silence. The run is now
// removed by size, its gap says the count is unknown (RemovedUnknown), the
// pass goes on to the next run, and the read error is returned. A run with a
// line it skipped is counted as unknown too: what that line held is not
// known.
func TestREC32_EvictContinuesPastUnreadableRun(t *testing.T) {
	permissionsBite(t)
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sess-unreadable", "sess-skipped", "sess-fine"} {
		if err := st.AppendDeclaration(testDecl(s, "toolu_"+s)); err != nil {
			t.Fatal(err)
		}
	}
	// A records file that cannot be read at all: a directory in its place.
	if err := os.Remove(filepath.Join(st.RunDir("sess-unreadable"), FileRecords)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(st.RunDir("sess-unreadable"), FileRecords), dirMode); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, filepath.Join(st.RunDir("sess-skipped"), FileRecords), []byte("not a record\n"))
	// A run whose directory cannot even be listed: its size and age are
	// unknown, so it is left alone, and the runs after it are still evicted.
	if err := st.AppendDeclaration(testDecl("sess-unlistable", "toolu_unlistable")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(st.RunDir("sess-unlistable"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(st.RunDir("sess-unlistable"), dirMode) })
	old := time.Now().Add(-48 * time.Hour)
	for i, s := range []string{"sess-unreadable", "sess-skipped", "sess-fine"} {
		if err := chtimesTree(st.RunDir(s), old.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	gaps, err := st.Evict(1, "", time.Now())
	if err == nil || !strings.Contains(err.Error(), "sess-unreadable") || !strings.Contains(err.Error(), "sess-unlistable") {
		t.Errorf("Evict returned %v, want the errors of the runs it could not read or list", err)
	}
	if _, err := os.Lstat(st.RunDir("sess-unlistable")); err != nil {
		t.Errorf("a run eviction could not list was removed: %v", err)
	}
	removed := map[string]int{}
	for _, g := range gaps {
		removed[g.SessionID] = g.RemovedRecords
	}
	want := map[string]int{"sess-unreadable": RemovedUnknown, "sess-skipped": RemovedUnknown, "sess-fine": 1}
	for s, n := range want {
		got, ok := removed[s]
		if !ok {
			t.Errorf("no gap for %s: eviction stopped before it", s)
			continue
		}
		if got != n {
			t.Errorf("gap for %s says %d records removed, want %d", s, got, n)
		}
		if _, err := os.Stat(st.RunDir(s)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still in the store: %v", s, err)
		}
	}
	readBack, skipped, err := st.ReadGaps()
	if err != nil || skipped != 0 || len(readBack) != 3 {
		t.Errorf("gaps read back: %d gaps, %d skipped, err %v; want 3, 0, nil", len(readBack), skipped, err)
	}
}
