package hook

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const (
	planWords = "SENTINEL-PLAN-WORDS-9f3a"
	planPath  = "internal/x/brand_new_file.go"
)

// planFix drives an ExitPlanMode call through the post path with a port that
// answers one planned file under one rule, counting the port's calls.
type planFix struct {
	*stFix
	calls int
	env   SteerEnv
}

func newPlanFix(t *testing.T, scope string) *planFix {
	t.Helper()
	f := &planFix{stFix: newST(t)}
	if err := store.WriteRecordScopeMarker(f.st.Root(), scope); err != nil {
		t.Fatal(err)
	}
	f.env = stEnv(steer.DisclosureVersion)
	f.env.Plan = PlanPort{Check: func(plan []byte, _ string, _ *spec.Compiled) ([]PlanItem, bool) {
		f.calls++
		return []PlanItem{{Path: planPath, RuleID: "protect-x", Kind: "protect_path", Trust: "trusted", Planned: true}}, true
	}}
	return f
}

func (f *planFix) exit(id string) []byte {
	f.t.Helper()
	return f.call(id, "ExitPlanMode", map[string]any{"plan": "# Plan\n" + planWords + " edits " + planPath, "planFilePath": "/tmp/p.md"}, 0).Steer(f.env)
}

// TestPC2_AnAcceptedPlanIsCheckedOnceAndKeptWithoutItsWords: at either record
// scope the post of ExitPlanMode delivers one note naming the path and the
// rule, keeps one plan_check record and one steer line, and nothing the store
// holds carries the plan's words or the path (the line and the record hold
// counts and keyed digests); the same plan again says nothing and is not a
// second note. Break: record the delivered form, or drop the dedupe, and this
// fails.
func TestPC2_AnAcceptedPlanIsCheckedOnceAndKeptWithoutItsWords(t *testing.T) {
	for _, scope := range []string{store.RecordScopeV4, store.RecordScopeV5} {
		t.Run(scope, func(t *testing.T) {
			f := newPlanFix(t, scope)
			out := f.exit("toolu_plan1")
			if !strings.Contains(string(out), planPath) || !strings.Contains(string(out), "protect-x") {
				t.Fatalf("delivered = %q, want the note naming %q and protect-x", out, planPath)
			}
			t.Logf("delivered: %s", out)
			lines := steerLines(t, f.st)
			if len(lines) != 1 || lines[0].Pattern != "plan-check" || lines[0].Level == "" {
				t.Fatalf("steer lines = %+v, want one plan-check line", lines)
			}
			t.Logf("stored: model_text=%q slots=%v", lines[0].ModelText, lines[0].Slots)
			if again := f.exit("toolu_plan2"); len(again) != 0 {
				t.Errorf("the same plan said %q a second time", again)
			}
			if n := len(steerLines(t, f.st)); n != 1 {
				t.Errorf("%d steer lines after the same plan twice, want 1", n)
			}
			run, err := f.st.ReadRun(stSession)
			if err != nil {
				t.Fatal(err)
			}
			if len(run.PlanChecks) != 2 || run.PlanChecks[0].Planned != 1 || run.PlanChecks[0].Named != 1 || len(run.PlanChecks[0].PathDigests) != 1 {
				t.Errorf("PlanChecks = %+v, want one per checked plan with counts and a digest", run.PlanChecks)
			}
			if run.Skipped != 0 {
				t.Errorf("Skipped = %d, want 0", run.Skipped)
			}
			_ = filepath.WalkDir(f.st.Root(), func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				b, _ := os.ReadFile(p)
				for _, leak := range []string{planWords, planPath, "brand_new_file"} {
					if strings.Contains(string(b), leak) {
						t.Errorf("%s holds %q", p, leak)
					}
				}
				return nil
			})
		})
	}
}

// TestPC3_OnlyAnAcceptedPlanPaysForTheCheck: a Pre of ExitPlanMode, any other
// tool, a plan with the key absent, and a session without consent never call
// the port. Break: check the tool after decoding, or before consent.
func TestPC3_OnlyAnAcceptedPlanPaysForTheCheck(t *testing.T) {
	f := newPlanFix(t, store.RecordScopeV5)
	f.call("toolu_b", "Bash", goTest, 0).Steer(f.env)
	f.call("toolu_e", "ExitPlanMode", map[string]any{"planFilePath": "/tmp/p.md"}, 0).Steer(f.env)
	if f.calls != 0 {
		t.Fatalf("port called %d times for non-plans", f.calls)
	}
	f.env = withConsent(f.env, 1, nil)
	if out := f.exit("toolu_p"); len(out) != 0 || f.calls != 0 {
		t.Errorf("no consent: printed %q, port calls %d, want nothing and 0", out, f.calls)
	}
	if run, err := f.st.ReadRun(stSession); err == nil && len(run.PlanChecks) != 0 {
		t.Errorf("no consent kept %d plan_check records", len(run.PlanChecks))
	}
}
