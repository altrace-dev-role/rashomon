package store

import (
	"fmt"
	"testing"
)

// TestRubricPERF6_ARecordLongerThanTheInitialBufferReadsBack: readChunk is
// where a read starts, not a size every record fits. A declaration naming
// thousands of hosts is written as one line far over 64 KiB, and it must read
// back whole, with nothing skipped (rubric fix list entry 68).
func TestRubricPERF6_ARecordLongerThanTheInitialBufferReadsBack(t *testing.T) {
	const session = "sess-perf6"
	hosts := make([]string, 8000)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("host-%05d.example.com", i)
	}
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_wide", SessionID: session, Hosts: hosts}
	line, err := marshalLine(rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) <= 2*readChunk {
		t.Fatalf("the record is %d bytes; it must be well over the initial buffer to test anything", len(line))
	}
	if err := st.AppendDeclaration(rec); err != nil {
		t.Fatal(err)
	}
	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatalf("a %d-byte record failed the read: %v", len(line), err)
	}
	if run.Skipped != 0 || len(run.Declarations) != 1 {
		t.Fatalf("read back %d declarations with %d skipped, want 1 and 0", len(run.Declarations), run.Skipped)
	}
	if got := run.Declarations[0].Hosts; len(got) != len(hosts) || got[len(got)-1] != hosts[len(hosts)-1] {
		t.Errorf("read back %d hosts, want %d", len(got), len(hosts))
	}
}
