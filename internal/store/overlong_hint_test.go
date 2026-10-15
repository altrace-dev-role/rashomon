package store

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestForgetWindow_NamesTheFileAndTheRemedyForAnOverlongLine: a run holding
// a line over the reader's limit cannot be forgotten by rewrite -- there is
// nothing to write the line back as -- and used to say only
// errLineTooLong's bare text. The error now also names which file holds the
// line and that Evict, which never reads a run's lines, still removes the
// whole run.
func TestForgetWindow_NamesTheFileAndTheRemedyForAnOverlongLine(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-huge-line"
	if err := st.AppendDeclaration(testDecl(session, "toolu_1")); err != nil {
		t.Fatal(err)
	}
	records := filepath.Join(st.RunDir(session), FileRecords)
	appendRaw(t, records, append(bytes.Repeat([]byte("x"), maxLine+1), '\n'))

	_, err = st.ForgetWindow(nil, nil, time.Now())
	if err == nil {
		t.Fatalf("ForgetWindow on a run with an overlong line: no error")
	}
	if !strings.Contains(err.Error(), FileRecords) {
		t.Errorf("error %v, want it to name %s", err, FileRecords)
	}
	if !strings.Contains(err.Error(), "Evict") {
		t.Errorf("error %v, want it to point at Evict as the way to still remove the run", err)
	}
}
