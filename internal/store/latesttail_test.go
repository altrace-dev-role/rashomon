package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedRecords writes a run's records file holding lines.
func seedRecords(t *testing.T, st *Store, session string, lines ...string) {
	t.Helper()
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileRecords), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func verifyLine(tag string) string {
	return fmt.Sprintf(`{"type": "verify", "schema_version": %d, "tag": %q}`, EventSchemaVersion, tag)
}

// TestRP2r_EachRecordOfKindStreamsInOrderAndStops (card REPORT1b LLD 1): the
// lines of the kind arrive in file order and as written, a line of another
// kind, another version or no JSON is not handed over, and fn returning false
// ends the read at that line. Break: hand over every line, or ignore the
// stop.
func TestRP2r_EachRecordOfKindStreamsInOrderAndStops(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedRecords(t, st, "s1", verifyLine("a"), `{"type":"prompt","schema_version":5}`, verifyLine("b"),
		`{"type":"verify","schema_version":99}`, `not json`, verifyLine("c"))
	var got []string
	skipped, err := st.EachRecordOfKind("s1", TypeVerify, func(l []byte) bool {
		got = append(got, string(l))
		return true
	})
	want := []string{verifyLine("a"), verifyLine("b"), verifyLine("c")}
	if err != nil || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("EachRecordOfKind = %q, err %v; want %q", got, err, want)
	}
	_ = skipped
	got = nil
	if _, err := st.EachRecordOfKind("s1", TypeVerify, func(l []byte) bool {
		got = append(got, string(l))
		return len(got) < 2
	}); err != nil || len(got) != 2 {
		t.Errorf("a stop after two: got %d lines, err %v", len(got), err)
	}
	if n, err := st.EachRecordOfKind("none", TypeVerify, func([]byte) bool { t.Error("called for a run with no file"); return true }); err != nil || n != 0 {
		t.Errorf("no run: skipped %d, err %v", n, err)
	}
}

// TestRP2s_LatestRecordTailReadsTheEndAndPassesTheCutLine (card REPORT1b
// LLD 1): over a file longer than tail only the last tail bytes are read, a
// line the seek cut is not returned though it is a record of the kind, and a
// file no longer than tail is read whole. Break: read from the file's start,
// or keep the cut line.
func TestRP2s_LatestRecordTailReadsTheEndAndPassesTheCutLine(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pad := `{"type":"declaration","schema_version":5,"pad":"` + strings.Repeat("x", 300) + `"}`
	seedRecords(t, st, "long", verifyLine("old"), pad, pad, pad, verifyLine("new"), pad)
	line, ok, n, err := st.LatestRecordTail("long", TypeVerify, 500)
	if err != nil || !ok || string(line) != verifyLine("new") || n > 500 || n == 0 {
		t.Errorf("tail 500: %q ok %t read %d err %v; want the newest record, at most 500 bytes", line, ok, n, err)
	}
	// A tail that holds only the end of the last pad line has no record.
	if line, ok, _, err := st.LatestRecordTail("long", TypeVerify, 340); err != nil || ok {
		t.Errorf("tail 340 holds no whole verify line: %q ok %t err %v", line, ok, err)
	}
	// A tail that begins inside "old" must not return it.
	seedRecords(t, st, "cut", verifyLine("old"), `{"type":"x","schema_version":5}`)
	if line, ok, _, err := st.LatestRecordTail("cut", TypeVerify, 60); err != nil || ok {
		t.Errorf("the cut line was returned: %q ok %t err %v", line, ok, err)
	}
	line, ok, n, err = st.LatestRecordTail("long", TypeVerify, 1<<20)
	if err != nil || !ok || string(line) != verifyLine("new") {
		t.Errorf("whole file: %q ok %t err %v", line, ok, err)
	}
	if info, _ := os.Stat(filepath.Join(st.RunDir("long"), FileRecords)); n != info.Size() {
		t.Errorf("a file no longer than tail is read whole: read %d of %d", n, info.Size())
	}
	if _, ok, n, err := st.LatestRecordTail("none", TypeVerify, 400); err != nil || ok || n != 0 {
		t.Errorf("no file: ok %t read %d err %v", ok, n, err)
	}
}
