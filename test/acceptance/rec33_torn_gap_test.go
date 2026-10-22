package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// appendRawBytes writes bytes to the end of a store file exactly as given,
// with no newline added: the shape a write cut short leaves behind.
func appendRawBytes(t *testing.T, path, b string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close() //nolint:errcheck // test fixture, write error below is what matters
	if _, err := f.WriteString(b); err != nil {
		t.Fatalf("appending to %s: %v", path, err)
	}
}

// tornGap is the front half of a gap record, cut off before its newline.
const tornGap = `{"type":"gap","schema_version":3,"session_id":"another-session","reason":"forget"`

// TestREC33_TornGapUnverifies: a gap line that does not parse cannot be
// attributed to any session -- the session id is what failed to read -- so it
// makes every session read unverified, in the report as records_unreadable
// and in the turn digest as records_skipped. ReadGaps used to drop it without
// a count, and a gap glued onto a torn tail was lost the same way: after
// `forget --host`, the session it had just removed records from read
// verified with no gap at all.
func TestREC33_TornGapUnverifies(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://acme-secret.internal/x"}
	e.mustHook(p.build(t))
	keep := defaultPayload()
	keep.ToolUseID = "toolu_keep"
	e.mustHook(keep.build(t))
	e.probe("end", testSession)
	if rep := e.report(testSession); rep.Coverage.State != "verified" {
		t.Fatalf("premise broken: the healthy run reads %s %v", rep.Coverage.State, rep.Coverage.Reasons)
	}

	appendRawBytes(t, filepath.Join(e.home, "gaps.ndjson"), tornGap)
	e.expectDamagedStore("a torn gap", "gaps.ndjson:1 is not one JSON object")

	rep := e.report(testSession)
	if rep.Coverage.State == "verified" || !e.hasReason(rep, "records_unreadable") {
		t.Errorf("with an unreadable gap line the session reads %s %v, want unverified with records_unreadable",
			rep.Coverage.State, rep.Coverage.Reasons)
	}
	d := e.digest("--session", testSession, "--prompt", p.PromptID)
	if d.Coverage.State == "verified" || !e.hasCoverageReason(d, "records_skipped") {
		t.Errorf("the digest reads %s %v, want unverified with records_skipped", d.Coverage.State, d.Coverage.Reasons)
	}

	if res := e.run("", nil, "forget", "--host", "acme-secret.internal"); res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := e.declarations(testSession); len(got) != 1 {
		t.Fatalf("premise broken: %d declarations after forget --host, want 1", len(got))
	}
	rep = e.report(testSession)
	if rep.Coverage.State == "verified" {
		t.Errorf("after forget --host over a torn gaps tail the session reads verified %v", rep.Coverage.Reasons)
	}
}
