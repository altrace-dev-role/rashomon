package acceptance

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricERRORS1_OnlyALockTimeoutIsBlamedOnTheLock: a terminal that closes
// no declaration used to mean lock_timeout, whatever left it alone. That
// held while the only way to lose a declaration was the lock; it stopped
// holding once a declaration could be lost to a damaged or unreadable line
// with its terminal intact, and the report then named a lock wait that
// never happened. lock_timeout is now raised only for a terminal that says
// so itself; any other lone terminal raises declaration_missing.
func TestRubricERRORS1_OnlyALockTimeoutIsBlamedOnTheLock(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	now := time.Now().UnixMilli()
	// A declaration this reader cannot read, whose terminal it can.
	appendRawLine(t, e.recordsPath(testSession), fmt.Sprintf(
		`{"type":"declaration","schema_version":99,"tool_use_id":"toolu_unreadable","session_id":%q}`, testSession))
	appendRawLine(t, e.recordsPath(testSession), fmt.Sprintf(
		`{"type":"terminal","schema_version":4,"seq":null,"recorded_at_unix_ms":%d,"tool_use_id":"toolu_unreadable","session_id":%q,"outcome":"ok","reason":null}`,
		now, testSession))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if !e.hasReason(rep, "declaration_missing") || e.hasReason(rep, "lock_timeout") {
		t.Errorf("a declaration lost to an unreadable line reads %v, want declaration_missing and no lock_timeout",
			rep.Coverage.Reasons)
	}
	d := e.digest("--session", testSession, "--prompt", defaultPayload().PromptID)
	if !e.hasCoverageReason(d, "declaration_missing") || e.hasCoverageReason(d, "lock_timeout") {
		t.Errorf("the digest reads %v, want declaration_missing and no lock_timeout", d.Coverage.Reasons)
	}

	// The terminal a hook writes when the lock refused its declaration says
	// so, and that one is still the lock's.
	appendRawBytes(t, filepath.Join(e.home, "runs", testSession, "spill.ndjson"), fmt.Sprintf(
		`{"type":"terminal","schema_version":4,"seq":null,"recorded_at_unix_ms":%d,"tool_use_id":"toolu_locked","session_id":%q,"outcome":"error","reason":"lock_timeout"}`+"\n",
		now, testSession))
	rep = e.report(testSession)
	if !e.hasReason(rep, "lock_timeout") {
		t.Errorf("a terminal carrying lock_timeout reads %v, want lock_timeout", rep.Coverage.Reasons)
	}
}
