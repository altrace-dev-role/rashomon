package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestH82_RecordingStateDistinguishesPausedAbsentAndUnknown calls
// RecordingState directly, in process, exactly as H-82 in
// docs/spec-claude-code-integration.md asks: status is not edited by this
// change, so the exported function is what a reader must be able to trust,
// and this test is what stands in for status calling it.
//
// Break (to see this fail): collapse RecordingPausedNoStore into
// RecordingActive in RecordingState, i.e. return RecordingActive whenever
// store.Paused reports false OR whenever no store exists, regardless of the
// pause marker. That is exactly the collapse H-82 forbids: "I turned it off"
// (paused, no store yet) would read the same as "it was never installed"
// (never paused, no store).
func TestH82_RecordingStateDistinguishesPausedAbsentAndUnknown(t *testing.T) {
	t.Run("active: nothing paused, no store", func(t *testing.T) {
		t.Setenv("RASHOMON_HOME", t.TempDir())
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingActive {
			t.Errorf("state is %q, want %q", state, RecordingActive)
		}
		if !since.IsZero() {
			t.Errorf("since is %v, want zero", since)
		}
	})

	t.Run("paused, no store recorded here yet", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		now := time.Now()
		if _, _, err := store.Pause(root, now); err != nil {
			t.Fatalf("store.Pause: %v", err)
		}
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPausedNoStore {
			t.Errorf("state is %q, want %q", state, RecordingPausedNoStore)
		}
		if since.UnixMilli() != now.UnixMilli() {
			t.Errorf("since is %v, want %v", since, now)
		}
		// The premise this whole state exists for: pausing before ever
		// recording must not have minted an install identity.
		if _, err := os.Stat(filepath.Join(root, "install.json")); err == nil {
			t.Errorf("a store now exists, which RecordingPausedNoStore promises did not happen")
		}
	})

	t.Run("paused, with a store", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		// store.Open, not `watch`: watch refuses to install a go-test binary,
		// and all this state needs is install.json to exist, which is what
		// storeExistsAt actually checks.
		if _, err := store.Open(root); err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		if _, _, err := store.Pause(root, time.Now()); err != nil {
			t.Fatalf("store.Pause: %v", err)
		}
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPaused {
			t.Errorf("state is %q, want %q", state, RecordingPaused)
		}
		if since.IsZero() {
			t.Errorf("since is zero, want the pause instant")
		}
	})

	t.Run("unknown: the marker exists but will not parse", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "pause"), []byte("not a number\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// A marker that cannot be parsed still reads as paused, with an
		// unknown start rather than a guessed one -- RecordingState does not
		// surface a Go error for this shape, since the file is readable and
		// the answer ("paused") is not in doubt, only its start time.
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPausedNoStore {
			t.Errorf("state is %q, want %q", state, RecordingPausedNoStore)
		}
		if !since.IsZero() {
			t.Errorf("since is %v, want zero for an unparseable marker", since)
		}
	})

	t.Run("paused (marker unreadable): the marker exists but cannot be read", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("chmod 000 does not stop this user reading the file")
		}
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		if _, _, err := store.Pause(root, time.Now()); err != nil {
			t.Fatalf("store.Pause: %v", err)
		}
		marker := filepath.Join(root, "pause")
		if err := os.Chmod(marker, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows

		// This is RecordingPausedUnreadable's whole reason to exist: checkPaused
		// treats this marker as a pause, so RecordingState must not render an
		// error here -- that would read as "unknown" at the status line, over
		// an answer the program already has.
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v, want no error for an unreadable marker", err)
		}
		if state != RecordingPausedUnreadable {
			t.Errorf("state is %q, want %q", state, RecordingPausedUnreadable)
		}
		if !since.IsZero() {
			t.Errorf("since is %v, want zero: the marker could not be read", since)
		}
	})
}

// TestRubricPAUSE9_StatusPrintsMarkerUnreadable pins the status
// line itself, not just the state RecordingState resolves to: an unreadable
// pause marker must render as "paused (marker unreadable)", never as
// statusUnknown, because checkPaused is already treating the call as paused
// and status has an answer rather than nothing to report.
func TestRubricPAUSE9_StatusPrintsMarkerUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}
	marker := filepath.Join(root, "pause")
	if err := os.Chmod(marker, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows

	var stdout bytes.Buffer
	statusRecording(&stdout, root)
	if !strings.Contains(stdout.String(), "recording: paused (marker unreadable)") {
		t.Errorf("status = %q, want it to contain %q", stdout.String(), "recording: paused (marker unreadable)")
	}
	if strings.Contains(stdout.String(), statusUnknown) {
		t.Errorf("status = %q, an unreadable marker is a known state, not %q", stdout.String(), statusUnknown)
	}
}

// TestH78_CheckPausedReReadsOnEveryCallInOneProcess is H-78 at the level its
// break actually threatens: not across the separate OS processes Claude Code
// spawns per hook call (a cache could never survive exec anyway, since each
// invocation starts a fresh binary with fresh package state), but within one
// process making several calls to checkPaused/run in a row, as this test
// harness -- and a future daemon-shaped caller -- would.
//
// Break (to see this fail): add a package-level `sync.Once`-guarded cache in
// checkPaused, e.g. `once.Do(func() { cachedPaused, _, _ = store.Paused(root)
// })` in place of the direct store.Paused(root) call. Within a single
// process, pause taken effect mid-sequence would then never be seen: exactly
// "a pause mid-session does nothing until restart."
func TestH78_CheckPausedReReadsOnEveryCallInOneProcess(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	call := func(id string) (declared bool) {
		payload := `{"session_id":"sess-78","hook_event_name":"PreToolUse","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_use_id":"` + id + `"}`
		var stdout, stderr bytes.Buffer
		if res := run([]string{"hook"}, strings.NewReader(payload), &stdout, &stderr); res != exitOK {
			t.Fatalf("hook %s: exit %d, stderr %s", id, res, stderr.String())
		}
		b, err := os.ReadFile(filepath.Join(root, "runs", "sess-78", "records.ndjson"))
		if err != nil {
			return false
		}
		return bytes.Contains(b, []byte(`"tool_use_id":"`+id+`"`))
	}

	if !call("toolu_1") {
		t.Fatalf("first call (unpaused) was not recorded")
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}
	if call("toolu_2") {
		t.Errorf("second call recorded although pause had already taken effect -- " +
			"the check is reading a cached answer rather than the marker file")
	}
	if _, _, err := store.Resume(root, time.Now()); err != nil {
		t.Fatalf("store.Resume: %v", err)
	}
	if !call("toolu_3") {
		t.Errorf("third call (after resume, same process) was not recorded")
	}
}

// TestCheckPaused_H80WritesRecordingPausedWithAStore covers H-80 at the
// function level, in process: a paused invocation against a root that
// already holds a store writes exactly one coverage record carrying
// store.ReasonRecordingPaused, and no declaration.
//
// Break: return early inside checkPaused before the AppendCoverage call (or
// equivalently, delete the hook.RecordPaused line). The gap then renders
// identically to a crash, which is H-80's own break.
func TestCheckPaused_H80WritesRecordingPausedWithAStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	// store.Open, not `watch`: watch refuses to install a go-test binary, and
	// this test needs a store to exist, not a settings.json entry.
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}

	payload := `{"session_id":"sess-check","hook_event_name":"PreToolUse","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_use_id":"toolu_check"}`
	var stdout, stderr bytes.Buffer
	if res := run([]string{"hook"}, strings.NewReader(payload), &stdout, &stderr); res != exitOK {
		t.Fatalf("hook: exit %d, stderr %s", res, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("hook wrote to stdout while paused: %q", stdout.String())
	}

	recPath := filepath.Join(root, "runs", "sess-check", "records.ndjson")
	if b, err := os.ReadFile(recPath); err == nil && len(bytes.TrimSpace(b)) != 0 {
		t.Errorf("a declaration was written while paused:\n%s", b)
	}

	covPath := filepath.Join(root, "runs", "sess-check", "coverage.ndjson")
	body, err := os.ReadFile(covPath)
	if err != nil {
		t.Fatalf("reading coverage.ndjson: %v", err)
	}
	if !bytes.Contains(body, []byte(`"reason":"recording_paused"`)) {
		t.Errorf("coverage.ndjson does not carry recording_paused:\n%s", body)
	}
	if !bytes.Contains(body, []byte(`"state":"unverified"`)) {
		t.Errorf("coverage.ndjson does not carry state unverified:\n%s", body)
	}
}

// TestRubricERRORS6_UnreadablePauseMarkerPausesTheHook: a pause marker that
// exists and cannot be read (chmod 000) is a pause, the way a marker that
// does not parse already is. Before, checkPaused read any error from
// store.Paused as "not paused", so the hook recorded the call while status
// said "recording: unknown". The coverage reason is pause_state_unreadable,
// not recording_paused: the call really was paused, but not by an ordinary
// `rashomon pause`, and a reader needs the two distinguishable.
func TestRubricERRORS6_UnreadablePauseMarkerPausesTheHook(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}
	marker := filepath.Join(root, "pause")
	if err := os.Chmod(marker, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows
	if _, _, err := store.Paused(root); err == nil {
		t.Fatal("premise: store.Paused read a chmod-000 marker without error")
	}

	payload := `{"session_id":"sess-unreadable","hook_event_name":"PreToolUse","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_use_id":"toolu_unreadable"}`
	var stdout, stderr bytes.Buffer
	if res := run([]string{"hook"}, strings.NewReader(payload), &stdout, &stderr); res != exitOK {
		t.Fatalf("hook: exit %d, stderr %s", res, stderr.String())
	}
	runDir := filepath.Join(root, "runs", "sess-unreadable")
	if b, err := os.ReadFile(filepath.Join(runDir, "records.ndjson")); err == nil && len(bytes.TrimSpace(b)) != 0 {
		t.Errorf("a declaration was written under an unreadable pause marker:\n%s", b)
	}
	body, err := os.ReadFile(filepath.Join(runDir, "coverage.ndjson"))
	if err != nil {
		t.Fatalf("reading coverage.ndjson: %v", err)
	}
	if !bytes.Contains(body, []byte(`"reason":"pause_state_unreadable"`)) {
		t.Errorf("coverage.ndjson does not carry pause_state_unreadable:\n%s", body)
	}
}

// TestRubricPAUSE9_AFaultOnTheStoreRootIsNotAPause: when store.Paused fails for a
// reason that has nothing to do with the pause marker -- here, a store root
// this process cannot search -- checkPaused must not treat the call as
// paused. Before, any error from store.Paused read as a pause, so an EACCES
// or ENOTDIR fault on the store root silently stopped recording with no
// coverage record and nothing on stderr.
func TestRubricPAUSE9_AFaultOnTheStoreRootIsNotAPause(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("root bypasses a directory's permission bits")
	}
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if err := os.Chmod(root, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) }) //nolint:errcheck // TempDir removal follows
	if _, _, err := store.Paused(root); err == nil {
		t.Fatal("premise: store.Paused read an unsearchable root without error")
	}
	if store.MarkerUnreadable(root) {
		t.Fatal("premise: MarkerUnreadable reported true for a root fault, not a marker fault")
	}

	if paused := checkPaused(nil, strings.NewReader(""), store.PhaseCall, io.Discard); paused {
		t.Error("checkPaused reported paused for a store root it could not search, " +
			"not for a pause marker it could not read")
	}
}

// TestCmdResume_RecoversTrueStartFromTheGapLedgerWhenTheMarkerCannotBeRead:
// resume's own closing gap must span the whole paused window, not collapse
// to the instant resume ran, when the marker it would read the true start
// from cannot be read. pause already wrote that instant reliably, in its
// own gap record, independent of the marker's content; resume now reads it
// back instead of falling all the way to now. Before, the closing gap ran
// from now to now, so the window read as zero length and a quiet turn that
// fell inside it -- after pause's own zero-width marker and before the
// (wrong) instant resume claimed -- intersected no gap at all.
func TestCmdResume_RecoversTrueStartFromTheGapLedgerWhenTheMarkerCannotBeRead(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	var buf bytes.Buffer
	if err := cmdPause(&buf); err != nil {
		t.Fatalf("cmdPause: %v", err)
	}

	marker := filepath.Join(root, "pause")
	if err := os.Chmod(marker, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows

	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatalf("store.OpenExisting: %v", err)
	}
	before, _, err := st.ReadGaps()
	if err != nil {
		t.Fatalf("ReadGaps before resume: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("got %d gaps after pause, want 1 (pause's own marker): %+v", len(before), before)
	}
	trueSince := before[0].FromUnixMS

	buf.Reset()
	if err := cmdResume(&buf); err != nil {
		t.Fatalf("cmdResume: %v", err)
	}

	after, _, err := st.ReadGaps()
	if err != nil {
		t.Fatalf("ReadGaps after resume: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("got %d gaps after resume, want 2: %+v", len(after), after)
	}
	closing := after[1]
	if closing.FromUnixMS != trueSince {
		t.Errorf("resume's closing gap starts at %d, want %d (pause's own recorded instant); "+
			"got a window that starts at %d (now), which reads the paused window as zero length",
			closing.FromUnixMS, trueSince, closing.ToUnixMS)
	}
}

// TestEarlierPauseStart_SkipsPastAGapWithADifferentReason: the last entry in
// the ledger is not cmdPause's own marker, so it must not be read as one,
// even though its width and timing would otherwise look like a match.
func TestEarlierPauseStart_SkipsPastAGapWithADifferentReason(t *testing.T) {
	since := time.UnixMilli(9000)
	gaps := []store.Gap{{Reason: "something_else", FromUnixMS: 1000, ToUnixMS: 1000}}
	if got := earlierPauseStart(gaps, since); got != since {
		t.Errorf("earlierPauseStart = %v, want %v unchanged: the last gap is not reasoned paused", got, since)
	}
}

// TestEarlierPauseStart_IgnoresAClosedPriorEpisode: the last paused-reasoned
// gap is already closed -- a past episode's own wide span, FromUnixMS and
// ToUnixMS apart -- not cmdPause's open, zero-width marker for the one in
// progress. Reading its (earlier) start would backdate the current episode
// into one that already ended.
func TestEarlierPauseStart_IgnoresAClosedPriorEpisode(t *testing.T) {
	since := time.UnixMilli(5000)
	gaps := []store.Gap{
		{Reason: store.GapPaused, FromUnixMS: 1000, ToUnixMS: 1000},
		{Reason: store.GapPaused, FromUnixMS: 1000, ToUnixMS: 4000},
	}
	if got := earlierPauseStart(gaps, since); got != since {
		t.Errorf("earlierPauseStart = %v, want %v unchanged: the last paused gap is already closed", got, since)
	}
}

// TestEarlierPauseStart_KeepsSinceWhenTheLedgerIsNotEarlier: an open marker
// that is not actually earlier than since must not override it -- recovering
// a later or equal instant is not recovering anything.
func TestEarlierPauseStart_KeepsSinceWhenTheLedgerIsNotEarlier(t *testing.T) {
	since := time.UnixMilli(1000)
	gaps := []store.Gap{{Reason: store.GapPaused, FromUnixMS: 2000, ToUnixMS: 2000}}
	if got := earlierPauseStart(gaps, since); got != since {
		t.Errorf("earlierPauseStart = %v, want %v unchanged: the marker is not earlier than since", got, since)
	}
}

// TestCmdPause_DocCommentIsItsOwn: cmdPause's doc comment must attach to
// cmdPause, not to pausedCoverageNote. Before, the const and its own short
// comment sat between cmdPause's comment and cmdPause itself with no blank
// line below the const's comment, so godoc attached the whole merged block
// -- cmdPause's explanation of the threat model included -- to the const,
// and cmdPause carried no doc comment at all.
func TestCmdPause_DocCommentIsItsOwn(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var cmdPauseDoc, constDoc *ast.CommentGroup
	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == "cmdPause" {
				cmdPauseDoc = d.Doc
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range vs.Names {
					if name.Name == "pausedCoverageNote" {
						constDoc = d.Doc
					}
				}
			}
		}
		return true
	})
	if cmdPauseDoc == nil {
		t.Fatal("cmdPause has no doc comment; it was absorbed by a neighbouring declaration")
	}
	if !strings.Contains(cmdPauseDoc.Text(), "engages recording state") {
		t.Errorf("cmdPause's doc comment reads %q, want it to describe cmdPause", cmdPauseDoc.Text())
	}
	if constDoc == nil || strings.Contains(constDoc.Text(), "engages recording state") {
		t.Errorf("pausedCoverageNote's doc comment reads %q, want it to describe the const alone", constDoc.Text())
	}
}
