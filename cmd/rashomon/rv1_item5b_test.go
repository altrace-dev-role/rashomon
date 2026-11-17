package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// rv5Incomplete is the value line's sentence for a turn whose record is not
// complete, the one a standing coverage notice already explains.
const rv5Incomplete = "recording incomplete for this turn; verification state unknown"

// rv5fMessage is the turn's final message: it claims nothing and names no
// failure, so recap.Line has nothing to say and the value line speaks.
const rv5fMessage = "I updated the comment on handle in internal/batch."

// rv5fTurn is a started session with one turn the hooks recorded: an Edit
// of a file in a project. Its coverage records are set by the tests, as
// rv5Env's are.
type rv5fTurn struct {
	*rv5Env
	cwd string
}

func newRV5fTurn(t *testing.T) *rv5fTurn {
	t.Helper()
	root, installID := benchStarted(t)
	cwd := benchProjectCWD(t)
	f := &rv5fTurn{rv5Env: &rv5Env{t: t, root: root, installID: installID}, cwd: cwd}
	file := filepath.Join(cwd, "handle.go")
	if err := os.WriteFile(file, []byte("package batch\n\n// handle processes one batch.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.call("toolu_01000000000000000000000001", "Edit",
		map[string]any{"file_path": file, "old_string": "one batch", "new_string": "one batch of records", "replace_all": false},
		map[string]any{"filePath": file, "oldString": "one batch", "newString": "one batch of records",
			"originalFile": "package batch\n\n// handle processes one batch.\n", "userModified": false, "replaceAll": false})
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	f.st = st
	return f
}

// call runs one tool call of the turn's prompt through cmdHook and cmdPost.
func (f *rv5fTurn) call(id, tool string, input, response map[string]any) {
	f.t.Helper()
	in, err := json.Marshal(input)
	if err != nil {
		f.t.Fatal(err)
	}
	out, err := json.Marshal(response)
	if err != nil {
		f.t.Fatal(err)
	}
	body := func(event string, resp []byte) []byte {
		head := newBenchHead(event)
		head.CWD = f.cwd
		return benchJSON(f.t, benchToolPayload{benchHead: head, PromptID: payload.PromptID(0), ToolName: tool,
			ToolInput: in, ToolUseID: id, ToolResponse: resp})
	}
	args := []string{install.Marker, f.installID}
	var stderr bytes.Buffer
	if code := cmdHook(args, bytes.NewReader(body("PreToolUse", nil)), &stderr); code != exitOK {
		f.t.Fatalf("%s pre: exit %d, stderr %q", tool, code, stderr.String())
	}
	if code := cmdPost(args, bytes.NewReader(body("PostToolUse", out)), io.Discard, &stderr); code != exitOK {
		f.t.Fatalf("%s post: exit %d, stderr %q", tool, code, stderr.String())
	}
}

// stop runs the turn's Stop as the turn's first (recap.json removed), with a
// final message that claims nothing, and returns the systemMessage, "" when
// it printed none.
func (f *rv5fTurn) stop() string {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.root, "recap.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.t.Fatal(err)
	}
	head := newBenchHead("Stop")
	head.CWD = f.cwd
	in := benchJSON(f.t, struct {
		benchHead
		StopHookActive       bool   `json:"stop_hook_active"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}{head, false, rv5fMessage})
	var stdout, stderr bytes.Buffer
	if code := cmdRecap([]string{install.Marker, f.installID}, bytes.NewReader(in), &stdout, &stderr); code != exitOK {
		f.t.Fatalf("recap: exit %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		return ""
	}
	return systemMessage(f.t, stdout.String())
}

// catchUp runs the next prompt's recap pending as the turn's catch-up
// (recap.json removed, so no recap is known to have reached the turn, and a
// prompt with no prompt_id, so the prompt is not the running turn's) and
// returns stdout whole: the catch-up's line rides the prompt's envelope.
func (f *rv5fTurn) catchUp() string {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.root, "recap.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.t.Fatal(err)
	}
	head := newBenchHead("UserPromptSubmit")
	head.CWD = f.cwd
	in := benchJSON(f.t, struct {
		benchHead
		Prompt string `json:"prompt"`
	}{head, "Now add a test for a key that is too long."})
	var stdout, stderr bytes.Buffer
	if code := cmdRecap([]string{"pending", install.Marker, f.installID}, bytes.NewReader(in), &stdout, &stderr); code != exitOK {
		f.t.Fatalf("recap pending: exit %d, stderr %q", code, stderr.String())
	}
	return stdout.String()
}

// view is the Stop view recapTurn builds for event, the catch-up's when
// pending, over the record as it stands.
func (e *rv5Env) view(event string, pending bool) stopTurn {
	e.t.Helper()
	run, err := digest.ReadRun(e.st, payload.Session)
	if err != nil {
		e.t.Fatal(err)
	}
	now := time.Now()
	d, err := digest.BuildFrom(run, e.st, payload.Session, "", payload.FinalMessage, now)
	if err != nil {
		e.t.Fatal(err)
	}
	p := recapPayload{SessionID: payload.Session, LastAssistantMessage: payload.FinalMessage,
		Harness: harnessSignal{HookEventName: event}, OK: true, CWD: payload.CWD}
	s, err := stopView(e.st, run, d, p, pending, now)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// runFiles is the run directory's file names.
func (f *rv5fTurn) runFiles() []string {
	f.t.Helper()
	ents, err := os.ReadDir(f.st.RunDir(payload.Session))
	if err != nil {
		f.t.Fatal(err)
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// TestRV1_5f_AStandingNoticeQuietsTheBareIncompleteLine: while the coverage
// notice stands (the claim gate reads coverage off, the notice was said and
// the cleared line was not), a turn that wrote prints no value line that
// would only say its record is incomplete; the notice already said why.
// (i) The first Stop says the notice. (ii) The next Stop prints nothing and
// writes no new file to the run directory. (iii) Control: a record
// incomplete for another reason, with no notice, keeps its sentence. (iv)
// After the cleared line, coverage off again keeps the per-turn sentence, so
// off, on, off is never silent. (v) Under a standing notice, a line that
// also carries a conflict line prints: an incomplete record holds no spec
// verdict and no platform read, so a rule or platform line cannot occur on
// it, and the conflicts come from the run, through the seam the conflict
// tests use. (vi) The catch-up, under the same standing notice, is quiet as
// the Stop is; control: with a conflict line it prints. (vii) After the
// notice, coverage back on and the record incomplete for another reason: the
// sentence prints, and no cleared line rides it, since that line needs a
// complete record. Break: quiet nothing (ii and vi fail), or quiet every
// incomplete row (iii and iv fail), or quiet a line with a rider (v and vi
// fail), or quiet only at the Stop (vi fails), or let the notice stand with
// the checks back on (vii fails).
func TestRV1_5f_AStandingNoticeQuietsTheBareIncompleteLine(t *testing.T) {
	t.Run("i and ii: the notice, then nothing", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage()
		f.wantOff("coverage_absent")
		if msg := f.stop(); !strings.Contains(msg, rv5Off) || strings.Contains(msg, rv5Incomplete) {
			t.Fatalf("(i) Stop 1: %q, want the notice %q and not %q", msg, rv5Off, rv5Incomplete)
		}
		before := f.runFiles()
		if msg := f.stop(); msg != "" {
			t.Errorf("(ii) Stop 2 under the standing notice: %q, want nothing", msg)
		}
		if after := f.runFiles(); !slices.Equal(before, after) {
			t.Errorf("(ii) run directory %q after Stop 2, want %q: the quiet check writes nothing", after, before)
		}
	})
	t.Run("iii: incomplete for another reason keeps its sentence", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage(f.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeFresh))
		rec, err := os.OpenFile(filepath.Join(f.st.RunDir(payload.Session), store.FileRecords), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rec.WriteString("{not a record\n"); err != nil {
			t.Fatal(err)
		}
		if err := rec.Close(); err != nil {
			t.Fatal(err)
		}
		if r, notes := f.gate(); r == verify.Complete || verify.CoverageOff(notes) {
			t.Fatalf("fixture: recording %v, notes %q; want incomplete with coverage on", r, notes)
		}
		if msg := f.stop(); !strings.Contains(msg, rv5Incomplete) || strings.Contains(msg, rv5Any) {
			t.Errorf("(iii) Stop: %q, want %q and no notice", msg, rv5Incomplete)
		}
	})
	t.Run("iv: off again after the cleared line keeps the sentence", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage()
		if msg := f.stop(); !strings.Contains(msg, rv5Off) {
			t.Fatalf("fixture: Stop 1: %q, want the notice", msg)
		}
		f.add(f.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeFresh))
		f.wantOn()
		if msg := f.stop(); !strings.Contains(msg, rv5On) {
			t.Fatalf("fixture: Stop 2: %q, want the cleared line", msg)
		}
		f.add(f.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeUnknown))
		f.wantOff("coverage_unverified")
		if msg := f.stop(); !strings.Contains(msg, rv5Incomplete) || strings.Contains(msg, rv5Any) {
			t.Errorf("(iv) Stop 3, off after the cleared line: %q, want %q and no notice", msg, rv5Incomplete)
		}
	})
	t.Run("v: a conflict line prints under the standing notice", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage()
		if msg := f.stop(); !strings.Contains(msg, rv5Off) {
			t.Fatalf("fixture: Stop 1: %q, want the notice", msg)
		}
		old := conflictsSeen
		conflictsSeen = func(*store.Run, int64) []verify.ConflictSeen {
			return []verify.ConflictSeen{seenConflict("boundary_cross", "no-ui-db", "internal/ui", 0, 3)}
		}
		t.Cleanup(func() { conflictsSeen = old })
		if msg := f.stop(); !strings.Contains(msg, "spec rule no-ui-db:") || !strings.Contains(msg, rv5Incomplete) {
			t.Errorf("(v) Stop 2 with a conflict line: %q, want the conflict line and %q", msg, rv5Incomplete)
		}
	})
	t.Run("vi: the catch-up is quiet as the Stop is", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage()
		if msg := f.stop(); !strings.Contains(msg, rv5Off) {
			t.Fatalf("fixture: Stop 1: %q, want the notice", msg)
		}
		if out := f.catchUp(); strings.Contains(out, rv5Incomplete) {
			t.Errorf("(vi) the catch-up under the standing notice printed %q, want no %q", out, rv5Incomplete)
		}
		old := conflictsSeen
		conflictsSeen = func(*store.Run, int64) []verify.ConflictSeen {
			return []verify.ConflictSeen{seenConflict("boundary_cross", "no-ui-db", "internal/ui", 0, 3)}
		}
		t.Cleanup(func() { conflictsSeen = old })
		if out := f.catchUp(); !strings.Contains(out, "spec rule no-ui-db:") || !strings.Contains(out, rv5Incomplete) {
			t.Errorf("(vi) control, the catch-up with a conflict line: %q, want the conflict line and %q", out, rv5Incomplete)
		}
	})
	t.Run("vii: the checks back on, incomplete for another reason, keeps its sentence", func(t *testing.T) {
		f := newRV5fTurn(t)
		f.setCoverage()
		if msg := f.stop(); !strings.Contains(msg, rv5Off) {
			t.Fatalf("fixture: Stop 1: %q, want the notice", msg)
		}
		f.add(f.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeFresh))
		rec, err := os.OpenFile(filepath.Join(f.st.RunDir(payload.Session), store.FileRecords), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rec.WriteString("{not a record\n"); err != nil {
			t.Fatal(err)
		}
		if err := rec.Close(); err != nil {
			t.Fatal(err)
		}
		if r, notes := f.gate(); r == verify.Complete || verify.CoverageOff(notes) {
			t.Fatalf("fixture: recording %v, notes %q; want incomplete with coverage on", r, notes)
		}
		if msg := f.stop(); !strings.Contains(msg, rv5Incomplete) || strings.Contains(msg, rv5Any) {
			t.Errorf("(vii) Stop 2, the checks back on: %q, want %q and no notice", msg, rv5Incomplete)
		}
	})
}

// TestRV1_5g_TheCatchUpCarriesNoNotice: with claim checks off and the notice
// not yet said, the catch-up's view carries no notice and no marker: the
// notice is the Stop's alone, so a catch-up never takes the marker the next
// Stop's notice needs. Control: the Stop's view of the same record carries
// the notice and its marker. Break: let the catch-up's view carry the notice.
func TestRV1_5g_TheCatchUpCarriesNoNotice(t *testing.T) {
	e := newRV5Env(t)
	e.setCoverage()
	e.wantOff("coverage_absent")
	if s := e.view("UserPromptSubmit", true); s.notice != "" || s.marker != "" {
		t.Errorf("the catch-up's view: notice %q, marker %q; want both empty", s.notice, s.marker)
	}
	if s := e.view("Stop", false); !strings.Contains(s.notice, rv5Off) || s.marker != rv5Marker {
		t.Errorf("control, the Stop's view: notice %q, marker %q; want %q and %q", s.notice, s.marker, rv5Off, rv5Marker)
	}
}
