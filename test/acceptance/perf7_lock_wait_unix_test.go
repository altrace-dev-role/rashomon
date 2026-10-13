//go:build unix

package acceptance

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRubricPERF7_StatusShowsTheLongestLockWait: how long hooks wait for the
// store's locks was invisible until a wait ran out and cost a record. A hook
// that waits notes it, and status shows the longest any hook waited and the
// run it was in, beside the limit an invocation's waits share. Before any
// hook has waited, status says none is recorded rather than showing zero.
func TestRubricPERF7_StatusShowsTheLongestLockWait(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	if got := fieldLine(t, e.status().stdout, "lock wait"); got != "none recorded" {
		t.Errorf("status lock wait line before any contention is %q, want \"none recorded\"", got)
	}

	const hold = 300 * time.Millisecond
	release := holdLock(t, filepath.Join(e.home, "runs", testSession, "records.ndjson"))
	time.AfterFunc(hold, release)
	p := defaultPayload()
	p.ToolUseID = "toolu_waited"
	e.mustHook(p.build(t))

	got := fieldLine(t, e.status().stdout, "lock wait")
	var ms int
	var run string
	if _, err := fmt.Sscanf(got, "longest %d ms, in run %s", &ms, &run); err != nil {
		t.Fatalf("status lock wait line after a %v hold is %q: %v", hold, got, err)
	}
	if ms < int(hold/time.Millisecond)-50 || ms > 3000 || run != testSession {
		t.Errorf("status lock wait line is %q, want about %v in run %s", got, hold, testSession)
	}
	if !strings.HasSuffix(got, "(a hook may wait 3s in all)") {
		t.Errorf("status lock wait line %q does not name the limit a hook's waits share", got)
	}
}
