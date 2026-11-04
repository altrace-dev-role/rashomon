package main

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestGI2n_TheNoteNamesAMultiWordRunner: an owed note for a runner whose
// row is three words, or holds the interpreter's -m, names it by its row
// (card GATE-INFER2): every test row the gate slot refused by its words
// gives the note where it gave none, the runner's name and the file, as
// `pytest` does. Break: refuse a row at the gate slot.
func TestGI2n_TheNoteNamesAMultiWordRunner(t *testing.T) {
	for _, gate := range []string{"python -m pytest", "python3 -m pytest", "python -m unittest", "python3 -m unittest",
		"uv run pytest", "poetry run pytest", "npm run test", "bundle exec rspec"} {
		want := "rashomon record: gate '" + gate + "' (from 'pyproject.toml') was not run after the last edit."
		f, ok := owedFinding(recap.Owed{Seq: 1, Gate: gate, From: "pyproject.toml"}, 1, nil)
		if !ok || f.Message.Text() != want {
			t.Errorf("%s: %q (ok %v), want %q", gate, f.Message.Text(), ok, want)
		}
	}
}

// TestGI2n_ARefusedNameLeavesATraceOncePerCheckpoint: an owed note whose
// name the gate slot refuses still gives the finding, keyed and levelled as
// the note, its Message zero, which Route logs as render_refused: the log
// says the note was owed and not said, never a silent drop (GATE-INFER2).
// A render_refused line under its key ends it as a delivered one does, so
// the trace is written once per turn end; another key's does not. Control:
// an owed note with no runner's name and no packages, nothing to say, gives
// none. Break: drop a refused name, give an empty unnamed note, or count no
// refused line as given.
func TestGI2n_ARefusedNameLeavesATraceOncePerCheckpoint(t *testing.T) {
	line := func(key string) []byte {
		return []byte(`{"pattern":"claim-vs-record","state":"suppressed","suppressed_by":"render_refused","key":"` + key + `"}`)
	}
	for _, gate := range []string{"go test -race", "python3 -m", "a/b"} {
		o := recap.Owed{Seq: 12, Gate: gate, From: "Makefile"}
		f, ok := owedFinding(o, 14, nil)
		if !ok || f.Message.Text() != "" || f.Pattern != pattern.ClaimVsRecord || f.Kind != "absent" ||
			f.Key != "inferred:test@12" || f.MaxLevel != steer.LevelNudge || f.AfterSeq != 14 {
			t.Errorf("%s: finding %+v text %q (ok %v), want claim-vs-record/absent at nudge keyed inferred:test@12, no text",
				gate, f, f.Message.Text(), ok)
		}
		if err := f.CheckOn(steer.EventUserPromptSubmit); err != nil {
			t.Errorf("%s: CheckOn: %v, want the router to take it and log it render_refused", gate, err)
		}
		if _, ok := owedFinding(o, 14, [][]byte{line("inferred:test@12")}); ok {
			t.Errorf("%s: a finding after its render_refused line, want none: once per turn end", gate)
		}
		if _, ok := owedFinding(o, 14, [][]byte{line("inferred:test@9")}); !ok {
			t.Errorf("%s: an earlier checkpoint's render_refused line ended it, want the finding", gate)
		}
	}
	if _, ok := owedFinding(recap.Owed{Seq: 12, From: "Makefile"}, 14, nil); ok {
		t.Error("control, no runner's name and no packages: a finding, want none")
	}
}
