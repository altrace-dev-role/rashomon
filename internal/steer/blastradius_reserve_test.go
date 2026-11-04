package steer

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// blastNote is a blast-radius finding (card ARCH-INLINE1) under key, the
// edited package and its callers' digest as the hook keys it.
func blastNote(t *testing.T, key string, afterSeq int64) Finding {
	t.Helper()
	m, err := render.New(pattern.BlastRadius, render.BlastRadiusBinding{
		Count: 4, Dirs: slot.NewPathList([]slot.Path{"internal/verify", "cmd/rashomon"}, 2), Tested: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := pattern.Priority(pattern.BlastRadius)
	return Finding{Pattern: pattern.BlastRadius, Key: key, Priority: p, MaxLevel: LevelNudge, StopForm: StopFormNone,
		Message: m, AfterSeq: afterSeq}
}

// staleNote is a stale-evidence finding for gate, the correction a
// blast-radius note must never crowd out.
func staleNote(t *testing.T, gate string, afterSeq int64) Finding {
	t.Helper()
	m, err := render.New(pattern.StaleEvidence, render.StaleEvidenceBinding{
		Gate: slot.Gate(gate), Minutes: 3, Paths: slot.NewPathList([]slot.Path{"internal/a.go"}, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := pattern.Priority(pattern.StaleEvidence)
	return Finding{Pattern: pattern.StaleEvidence, Key: gate, Priority: p, MaxLevel: LevelNudge, StopForm: StopFormNone,
		Message: m, AfterSeq: afterSeq}
}

// TestAI1_BlastRadiusLeavesTheLastNoteToACorrection: the prompt's cap is
// three notes, spent in delivery order across calls, so a priority alone
// cannot keep a context note from spending the slot a later correction needs
// (A8). With one note given (the prompt's preflight, here any note), a
// blast-radius note delivers; a second, for another package, is
// budget_prompt, since it would leave no slot; and a stale-evidence note
// later in the prompt still delivers, the third. Break: drop the reserve
// from charge's budget_prompt case, and the second blast-radius note
// delivers and the stale-evidence note is refused.
func TestAI1_BlastRadiusLeavesTheLastNoteToACorrection(t *testing.T) {
	ctx := routerContext(t)
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "first", LevelNudge)}); out.AdditionalContext == "" {
		t.Fatal("premise: the prompt's first note did not print")
	}
	first := blastNote(t, "example.com/m/a@0123456789ab", 6)
	if out := Route(ctx, []Finding{first}); out.AdditionalContext != first.Message.Text() {
		t.Fatalf("the first blast-radius note printed %+v, want its text", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateDelivered, "", DeliveryDelivered)
	if out := Route(ctx, []Finding{blastNote(t, "example.com/m/b@0123456789ab", 7)}); out != (Output{}) {
		t.Errorf("a second blast-radius note, at two notes given, printed %+v", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByBudgetPrompt, DeliverySuppressedBudget)
	stale := staleNote(t, "go test", 8)
	if out := Route(ctx, []Finding{stale}); out.AdditionalContext != stale.Message.Text() {
		t.Errorf("the stale-evidence note after them printed %+v, want its text", out)
	}
}

// TestAI1_WithNoBlastRadiusTheCapIsAsBefore: the control. A prompt with no
// blast-radius note spends its three notes on stale-evidence and
// regression-location as on main, and the fourth is budget_prompt: the
// reserve of every other id is 0. Break: give the reserve to every pattern,
// and the third note here is refused.
func TestAI1_WithNoBlastRadiusTheCapIsAsBefore(t *testing.T) {
	ctx := routerContext(t)
	notes := []Finding{
		staleNote(t, "go test", 5),
		regressionNote(t, "go vet", 6, "r1", LevelNudge),
		staleNote(t, "go build", 7),
	}
	for i, f := range notes {
		if out := Route(ctx, []Finding{f}); out.AdditionalContext != f.Message.Text() {
			t.Fatalf("note %d (%s) printed %+v, want its text", i+1, f.Pattern, out)
		}
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 8, "r2", LevelNudge)}); out != (Output{}) {
		t.Errorf("the fourth note printed %+v", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByBudgetPrompt, DeliverySuppressedBudget)
}

// TestAI1_InOneCallTheCorrectionWins: a write that raises stale-evidence and
// blast-radius in one call prints the stale-evidence note; the blast-radius
// line is superseded (budget_batch), the lowest priority (25) yielding.
// Break: rank blast-radius above stale-evidence, and it prints instead.
func TestAI1_InOneCallTheCorrectionWins(t *testing.T) {
	ctx := routerContext(t)
	stale := staleNote(t, "go test", 5)
	out := Route(ctx, []Finding{blastNote(t, "example.com/m/a@0123456789ab", 5), stale})
	if out.AdditionalContext != stale.Message.Text() {
		t.Fatalf("printed %+v, want the stale-evidence note", out)
	}
	lines := steerLines(t, ctx)
	if len(lines) != 2 || lines[1].Pattern != string(pattern.BlastRadius) {
		t.Fatalf("lines = %+v, want stale-evidence then blast-radius", lines)
	}
	wantLine(t, lines[1], LineStateSuperseded, SuppressedByBudgetBatch, DeliverySuppressedBudget)
}
