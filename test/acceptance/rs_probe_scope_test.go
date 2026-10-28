package acceptance

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The SessionStart probe keeps the record_scope marker equal to consent.json,
// once per session: an install whose consent predates the marker gains it at
// its next session start, a marker that disagrees with consent.json is set to
// consent.json's answer, and a consent.json the probe cannot read removes the
// marker (v4, which withholds) and says so on the start coverage record. The
// probe writes only on a difference, and nothing else in the hook reads
// consent.json: not PreToolUse, not PostToolUse, not SessionEnd.

// markerState is what the marker holds, and false when it is absent.
func markerState(t *testing.T, e *env) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "record_scope"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// startReason is the reason on the session's last start coverage record.
func startReason(t *testing.T, e *env) any {
	t.Helper()
	recs := e.coverage(testSession, "start")
	if len(recs) == 0 {
		t.Fatal("no start coverage record")
	}
	return recs[len(recs)-1].fields["reason"]
}

// TestRS11_SessionStartSetsTheMarkerFromConsent: an install whose consent.json
// names v5 and that has no marker -- every install from before the marker --
// gains a v5 marker at its next session start, and the session's calls record
// the content fields; a stale marker is set to consent.json's answer.
func TestRS11_SessionStartSetsTheMarkerFromConsent(t *testing.T) {
	t.Run("consent from before the marker", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
			t.Fatal(err)
		}
		e.probe("start", testSession)
		if got, ok := markerState(t, e); !ok || got != "v5\n" {
			t.Errorf("after a session start the marker holds %q (present %v), want \"v5\\n\"", got, ok)
		}
		if got := startReason(t, e); got != nil {
			t.Errorf("start coverage reason = %v, want null", got)
		}
		e.mustHook(defaultPayload().build(t))
		if _, ok := pathValue(t, e.declarations(testSession)[0], "cwd"); !ok {
			t.Error("the call after the healing start has no cwd")
		}
	})
	t.Run("a marker that disagrees", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if err := os.WriteFile(filepath.Join(e.home, "record_scope"), []byte("v6\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		e.probe("start", testSession)
		if got, ok := markerState(t, e); !ok || got != "v5\n" {
			t.Errorf("after a session start the marker holds %q (present %v), want \"v5\\n\"", got, ok)
		}
	})
}

// TestRS11b_NoConsentStaysWithoutAMarker: a store with no consent on file
// keeps no marker through a session start; absent already reads v4, so there
// is no difference to write, and nothing to name on the coverage record (this
// store has no settings entry, so the start record names that instead).
func TestRS11b_NoConsentStaysWithoutAMarker(t *testing.T) {
	e := newEnv(t)
	e.probe("start", testSession)
	if got, ok := markerState(t, e); ok {
		t.Errorf("a store with no consent has a marker holding %q after a session start", got)
	}
	if got := startReason(t, e); got == "record_scope_unresolved" {
		t.Errorf("start coverage reason = %v for a store with no consent.json, which is no consent, not an unreadable one", got)
	}
}

// TestRS11c_UnreadableConsentRemovesTheMarker: a consent.json the probe cannot
// read leaves no marker, whatever the marker said, and the start coverage
// record names the reason.
func TestRS11c_UnreadableConsentRemovesTheMarker(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	consentPath := filepath.Join(e.home, "consent.json")
	if err := os.Remove(consentPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(consentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := markerState(t, e); !ok {
		t.Fatal("premise: watch left no v5 marker for the probe to remove")
	}
	e.probe("start", testSession)
	if got, ok := markerState(t, e); ok {
		t.Errorf("an unreadable consent.json left the marker holding %q", got)
	}
	if got := startReason(t, e); got != "record_scope_unresolved" {
		t.Errorf("start coverage reason = %v, want record_scope_unresolved", got)
	}
}

// TestRS11d_WritesOnlyOnADifference: a marker that already matches is left as
// it is -- the same file, not rewritten -- so a session start costs one read
// of consent.json and no write.
func TestRS11d_WritesOnlyOnADifference(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	path := filepath.Join(e.home, "record_scope")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	e.probe("start", testSession)
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the marker went: %v", err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(old) {
		t.Errorf("a matching marker was rewritten (same file %v, mtime %v, want %v)", os.SameFile(before, after), after.ModTime(), old)
	}
}

// TestRS11e_OnlySessionStartReadsConsent: SessionEnd, PreToolUse and
// PostToolUse leave a missing marker missing, so the calls of a session that
// started before the marker went keep withholding until the next start.
func TestRS11e_OnlySessionStartReadsConsent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
		t.Fatal(err)
	}
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	e.probe("end", testSession)
	if got, ok := markerState(t, e); ok {
		t.Errorf("a hook other than SessionStart wrote the marker: %q", got)
	}
	wantNoPath(t, e.declarations(testSession)[0], "cwd", "no_consent")
}
