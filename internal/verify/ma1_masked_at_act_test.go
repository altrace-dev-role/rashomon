package verify

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// headedVet is eng-4's call 3060 (session 02bc13cc, transcript line 6568) as
// the record holds it: `go vet ./internal/gateinfer 2>&1 | head -3` inside a
// longer line, ended ok, its status hidden by the pipe to head, recorded at
// schema 8 under this build's rules with the runner's words. head keeps no
// summary, so nothing in the output says how vet ended. by is the stage word,
// "" for a record that names none. The runner is a literal standing for its
// digest, as gateCall's are.
func headedVet(seq int64, by string) call {
	c := gateCall(seq, "go-vet", store.ExecOK)
	c.class, c.kind, c.masked = shape.VerbExecute, shape.RunnerBuild, shape.MaskedBuild
	c.runnerName, c.maskedBy, c.rules = "go vet", by, shape.RulesVersion
	return c
}

// postAt is SteerAt at call n's own point at the PostToolUse of call id,
// as the hook calls it.
func postAt(t *testing.T, r *store.Run, n int64, id string) SteerView {
	t.Helper()
	return SteerAt(r, nil, nil, nil, "", callPoint(t, r, n), time.Time{}, string(steer.EventPostToolUse), id)
}

// maskedFindings is the view's masked-status candidates.
func maskedFindings(v SteerView) []steer.Finding {
	var out []steer.Finding
	for _, f := range v.Findings {
		if f.Pattern == pattern.MaskedStatus {
			out = append(out, f)
		}
	}
	return out
}

// TestMA1a_AMaskedRunIsSaidAtItsOwnPost (card MASKED-AT-ACT1): an edit, then
// call 3060's vet piped to head. At that call's PostToolUse the view holds one
// masked-status candidate: a nudge keyed by the gate, citing the run, with no
// sub-kind, and a note that states the record's fact and orders nothing. The
// owner's target in its plainest form: a masked exit, caught at the act, where
// the agent can still run it again. Break: drop the producer, and the view
// holds nothing.
func TestMA1a_AMaskedRunIsSaidAtItsOwnPost(t *testing.T) {
	r := runOf(t, edit(1, "internal/gateinfer/gateinfer.go"), headedVet(2, "head"))
	minuteClock(t, r)
	v := postAt(t, r, 2, "t2")
	t.Run("call 3060's line at its post", func(t *testing.T) {
		got := maskedFindings(v)
		if len(got) != 1 {
			t.Fatalf("%d masked-status candidates (%d dropped), want 1", len(got), v.Dropped)
		}
		f := got[0]
		if f.Kind != "" || f.MaxLevel != steer.LevelNudge || f.StopForm != steer.StopFormNone || f.Priority != 45 {
			t.Errorf("candidate kind %q level %d form %q priority %d, want no kind, a nudge, no stop form, 45",
				f.Kind, f.MaxLevel, f.StopForm, f.Priority)
		}
		if !strings.HasPrefix(f.Key, "learned:") || !strings.Contains(f.Key, "go-vet") {
			t.Errorf("key %q, want the go vet gate's id: once per gate per prompt", f.Key)
		}
		if want := []int64{r.Declarations[1].Seq}; len(f.Evidence) != 1 || f.Evidence[0] != want[0] {
			t.Errorf("evidence %v, want the masked run's declaration %v", f.Evidence, want)
		}
		const want = "rashomon record: gate 'go vet' ran in this call and the pipe to head hid its exit status; " +
			"no outcome of this run is recorded."
		if got := f.Message.Text(); got != want {
			t.Errorf("note %q, want %q", got, want)
		}
	})
	if v.Dropped != 0 {
		t.Errorf("%d facts dropped by the mapper, want 0", v.Dropped)
	}
}

// TestMA1b_ControlsRaiseNothing: only a run whose outcome the record cannot
// say, at its own success post, raises the note. A run with its status in
// view, a run whose outcome was read from its output (D229), the call after
// the masked one, a masked line that failed (its failure event, which the
// card does not raise at) and a backgrounded run raise none, and the mapper
// drops nothing.
func TestMA1b_ControlsRaiseNothing(t *testing.T) {
	plain := gateCall(2, "go-vet", store.ExecOK)
	plain.class, plain.kind = shape.VerbExecute, shape.RunnerBuild
	failed := headedVet(2, "head")
	failed.outcome, failed.exit = store.ExecFailed, 1
	bg := headedVet(2, "head")
	bg.bg = true
	for _, tc := range []struct {
		name  string
		calls []call
		n     int64
		event string
	}{
		{"a run with its status in view", []call{edit(1, "internal/a.go"), plain}, 2, string(steer.EventPostToolUse)},
		{"a run read from its output", []call{edit(1, "internal/a.py"), tailedPytest(2, "tail", "summary_pass")}, 2, string(steer.EventPostToolUse)},
		{"the call after the masked run", []call{edit(1, "internal/a.go"), headedVet(2, "head"), edit(3, "internal/b.go")}, 3, string(steer.EventPostToolUse)},
		{"a masked line that failed, at its failure event", []call{edit(1, "internal/a.go"), failed}, 2, string(steer.EventPostToolUseFailure)},
		{"a backgrounded run", []call{edit(1, "internal/a.go"), bg}, 2, string(steer.EventPostToolUse)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, tc.calls...)
			minuteClock(t, r)
			id := "t" + strconv.FormatInt(tc.n, 10)
			v := SteerAt(r, nil, nil, nil, "", callPoint(t, r, tc.n), time.Time{}, tc.event, id)
			if got := maskedFindings(v); len(got) != 0 || v.Dropped != 0 {
				t.Errorf("%d masked-status candidates, %d dropped; want none of either", len(got), v.Dropped)
			}
		})
	}
}

// TestMA1c_TheNoteNamesTheStageThatHidIt: the note says which stage hid the
// status, in the Stop sentence's words: the pipe to a named stage, a pipe the
// lists do not name, and the line itself where the record names no stage (a
// sequence, `go vet ./...; echo done`).
func TestMA1c_TheNoteNamesTheStageThatHidIt(t *testing.T) {
	for _, tc := range []struct{ name, by, want string }{
		{"head", "head", "the pipe to head hid its exit status"},
		{"grep", "grep", "the pipe to grep hid its exit status"},
		{"tail", "tail", "the pipe to tail hid its exit status"},
		{"other", "other", "a pipe hid its exit status"},
		{"none", "", "its line hid its exit status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, edit(1, "internal/a.go"), headedVet(2, tc.by))
			minuteClock(t, r)
			got := maskedFindings(postAt(t, r, 2, "t2"))
			if len(got) != 1 || !strings.Contains(got[0].Message.Text(), tc.want) {
				t.Errorf("candidates %d, note %q; want one saying %q", len(got), noteOf(got), tc.want)
			}
		})
	}
}

// noteOf is the first candidate's note, "" with none.
func noteOf(fs []steer.Finding) string {
	if len(fs) == 0 {
		return ""
	}
	return fs[0].Message.Text()
}

// TestMA1d_EveryStageIsInTheNotesTable: every masked_by word the binary can
// record, an unknown word and none map to a phrase in the note's closed
// table (slot.EnumMaskStage), so a stage shape learns later cannot leave a
// masked run with a note the renderer refuses.
func TestMA1d_EveryStageIsInTheNotesTable(t *testing.T) {
	for _, w := range append(shape.MaskedByWords(), "", "not-a-stage") {
		var by *string
		if w != "" {
			by = &w
		}
		if err := slot.NewEnum(slot.EnumMaskStage, maskedStage(by)).Validate(); err != nil {
			t.Errorf("masked_by %q: phrase %q: %v", w, maskedStage(by), err)
		}
	}
}
