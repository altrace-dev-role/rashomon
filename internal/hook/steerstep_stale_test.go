package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// snLine is stale-evidence's note for the fixture's edit of internal/a.go,
// declared two minutes after its green go test (stFix.call moves the clock
// a minute after each half of a call).
const snLine = "rashomon record: gate 'go' exited 0 earlier in this session (2 min ago); this call then wrote 'internal/a.go'. " +
	"No later run of the gate is recorded."

// snEnv is stEnv with consent current and the holdout off, so a nudge the
// router passes is delivered whatever the session's draw.
func snEnv() SteerEnv {
	env := stEnv(steer.DisclosureVersion)
	env.HoldoutOff = true
	return env
}

// newSN is newST with the payloads' cwd a real directory, so the project
// key resolves (baseline.Key) and the index has a place.
func newSN(t *testing.T) *stFix {
	t.Helper()
	f := newST(t)
	f.cwd = t.TempDir()
	return f
}

// green lands the fixture's green go test and runs its step, which arms the
// stale trigger.
func (f *stFix) green() {
	f.t.Helper()
	f.call("toolu_g", "Bash", goTest, 0).Steer(snEnv())
}

// editOf lands an Edit of rel under the fixture's cwd under id.
func (f *stFix) editOf(id, rel string) *Post {
	f.t.Helper()
	return f.call(id, "Edit", map[string]any{"file_path": filepath.Join(f.cwd, rel), "old_string": "a", "new_string": "b"}, 0)
}

// family is the go test's gate family as the run records it.
func (f *stFix) family() string {
	f.t.Helper()
	run, err := f.st.ReadRun(stSession)
	if err != nil {
		f.t.Fatal(err)
	}
	snap := verify.Evaluate(run, nil, nil, verify.Point{Seq: 1 << 40, SessionID: stSession}, f.now)
	if len(snap.Gates) != 1 {
		f.t.Fatalf("premise: %d gates, want the go test's one", len(snap.Gates))
	}
	return snap.Gates[0].Family
}

// indexPath is the project index of the fixture's cwd.
func (f *stFix) indexPath() string {
	return verify.IndexPath(f.st.Root(), baseline.Key(f.cwd))
}

// require writes the project index with family required: two sessions, the
// last at the step's clock.
func (f *stFix) require(family string) {
	f.t.Helper()
	ms := snEnv().Now.UnixMilli()
	idx := &verify.ProjectIndex{ProjectKey: baseline.Key(f.cwd), Entries: []verify.IndexEntry{
		{Family: family, Kind: shape.RunnerTest, Sessions: 2, LastSession: "s0", FirstSeen: ms, LastSeen: ms}}}
	if _, err := verify.WriteIndex(f.indexPath(), idx); err != nil {
		f.t.Fatal(err)
	}
}

// staleLines is the session's stale-evidence steer lines.
func staleLines(t *testing.T, st *store.Store) []steer.Line {
	t.Helper()
	var out []steer.Line
	for _, l := range steerLines(t, st) {
		if l.Pattern == string(pattern.StaleEvidence) {
			out = append(out, l)
		}
	}
	return out
}

// TestIP1_PostJudgesUnderTheProjectIndex (card INDEX-POST1, folded by D164):
// the post reads the project index as the Stop path does. With the index
// requiring the go test's family, the edit after its green raises
// stale-evidence; judged with no index, nothing would, because no family is
// established (D164's departure), so the line is the index's.
func TestIP1_PostJudgesUnderTheProjectIndex(t *testing.T) {
	f := newSN(t)
	f.green()
	f.require(f.family())
	f.editOf("toolu_e", "internal/a.go").Steer(snEnv())
	if got := staleLines(t, f.st); len(got) != 1 || got[0].ModelText != snLine {
		t.Fatalf("%d stale-evidence lines (%+v), want one carrying %q", len(got), got, snLine)
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("the judged edit left the trigger armed")
	}
}

// TestIP1b_OverCapIndexReadsAsNone: the same index padded past MaxIndexBytes
// is read as no index (ReadIndex), so the judged edit raises nothing.
func TestIP1b_OverCapIndexReadsAsNone(t *testing.T) {
	f := newSN(t)
	f.green()
	f.require(f.family())
	b, err := os.ReadFile(f.indexPath())
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, strings.Repeat(" ", verify.MaxIndexBytes)...)
	if err := os.WriteFile(f.indexPath(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	f.editOf("toolu_e", "internal/a.go").Steer(snEnv())
	if got := staleLines(t, f.st); len(got) != 0 {
		t.Errorf("%d stale-evidence lines under an over-cap index, want 0", len(got))
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("the edit was not judged: the trigger is still armed")
	}
}

// TestSN1f_MirrorAndLog: the nudge reaches the model and, mirrored, the
// user, and the steer log holds it as delivered.
func TestSN1f_MirrorAndLog(t *testing.T) {
	f := newSN(t)
	f.green()
	f.require(f.family())
	b := f.editOf("toolu_e", "internal/a.go").Steer(snEnv())
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("printed %q: %v", b, err)
	}
	if _, model := out["hookSpecificOutput"]; !model || !strings.Contains(string(b), snLine) {
		t.Errorf("printed %s; want the note model-visible and mirrored", b)
	}
	got := staleLines(t, f.st)
	if len(got) != 1 {
		t.Fatalf("%d stale-evidence lines, want 1", len(got))
	}
	if l := got[0]; l.Level != steer.LevelNudge.String() || l.Delivery == nil || *l.Delivery != steer.DeliveryDelivered {
		t.Errorf("line at %s, delivery %v; want nudge, delivered", l.Level, l.Delivery)
	}
}

// TestSN1g_OnlyAnArmedRelevantWriteIsJudged (D166): the step judges a write
// only while the session is armed, for a path under the project a gate
// counts, in a project with an index (or, with none, a spec that declares a
// required gate: D222), and that write takes the arm; every other write
// leaves the arm as it found it and reads no run. A relevant write in a
// project with no index and no declared required gate is judged to nothing
// and takes the arm, so the run is read once, not at every write.
func TestSN1g_OnlyAnArmedRelevantWriteIsJudged(t *testing.T) {
	t.Run("armed, relevant, indexed", func(t *testing.T) {
		f := newSN(t)
		f.green()
		f.require(f.family())
		if _, _, ok := f.editOf("toolu_e", "internal/a.go").judge(snEnv()); !ok {
			t.Fatal("the armed relevant write was not judged")
		}
		if armed, _ := f.st.StaleArmed(stSession); armed {
			t.Error("the judged write left the arm in place")
		}
	})
	t.Run("not armed", func(t *testing.T) {
		f := newSN(t)
		f.call("toolu_g", "Bash", goTest, 0) // landed, but its step never ran
		f.require(f.family())
		if _, _, ok := f.editOf("toolu_e", "internal/a.go").judge(snEnv()); ok {
			t.Error("an unarmed write was judged")
		}
	})
	for _, c := range []struct {
		name  string
		rel   string
		index bool
	}{
		{"a docs file", "README.md", true},
		{"outside the project", filepath.Join("..", "elsewhere", "a.go"), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSN(t)
			f.green()
			if c.index {
				f.require(f.family())
			}
			if _, _, ok := f.editOf("toolu_e", c.rel).judge(snEnv()); ok {
				t.Error("the write was judged")
			}
			if armed, _ := f.st.StaleArmed(stSession); !armed {
				t.Error("the write took the arm it was not judged on")
			}
		})
	}
}

// TestSN1g2_NoIndexNoDeclaredGateTakesTheArmUnjudged (D222): with no index
// and no spec declaring a required gate, the first relevant write is not
// judged and takes the arm, so the next one pays one Lstat.
func TestSN1g2_NoIndexNoDeclaredGateTakesTheArmUnjudged(t *testing.T) {
	f := newSN(t)
	f.green()
	if _, _, ok := f.editOf("toolu_e", "internal/a.go").judge(snEnv()); ok {
		t.Error("the write was judged")
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("the write left the arm in place")
	}
}

// TestSN1h_OnlyAGreenGateRunArms: a runner call's post at a success event
// arms the trigger; a failed one and an edit do not.
func TestSN1h_OnlyAGreenGateRunArms(t *testing.T) {
	for _, c := range []struct {
		name string
		land func(f *stFix) *Post
		want bool
	}{
		{"green go test", func(f *stFix) *Post { return f.call("toolu_g", "Bash", goTest, 0) }, true},
		{"failed go test", func(f *stFix) *Post { return f.call("toolu_g", "Bash", goTest, 1) }, false},
		{"an edit", func(f *stFix) *Post { return f.editOf("toolu_e", "internal/a.go") }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSN(t)
			c.land(f).Steer(snEnv())
			if armed, _ := f.st.StaleArmed(stSession); armed != c.want {
				t.Errorf("armed = %v, want %v", armed, c.want)
			}
		})
	}
}

// TestSN1i_OncePerGatePerPrompt: the note's key is the gate, so a green
// rerun and a second edit in the same prompt raise it again and the router
// holds it back as the prompt's second note on that key (budget_pattern).
func TestSN1i_OncePerGatePerPrompt(t *testing.T) {
	f := newSN(t)
	f.green()
	f.require(f.family())
	f.editOf("toolu_e1", "internal/a.go").Steer(snEnv())
	f.call("toolu_g2", "Bash", goTest, 0).Steer(snEnv())
	f.editOf("toolu_e2", "internal/b.go").Steer(snEnv())
	got := staleLines(t, f.st)
	if len(got) != 2 {
		t.Fatalf("%d stale-evidence lines, want 2", len(got))
	}
	if d := got[0].Delivery; d == nil || *d != steer.DeliveryDelivered {
		t.Errorf("first line delivery %v, want delivered", d)
	}
	if by := got[1].SuppressedBy; by == nil || *by != steer.SuppressedByBudgetPattern {
		t.Errorf("second line suppressed by %v, want budget_pattern", by)
	}
}

// TestVG1j_ANewerClientDeliversTheNote: the stale note reaches the model on a
// Claude Code release after the probed one, 2.1.314 (the version the
// finding was made on), as on 2.1.280: the channel's table is a floor, not a
// list (card VERSION-GATE1). On 2.1.257, below the floor, the note is
// shadowed and the user gets the channel notice alone.
func TestVG1j_ANewerClientDeliversTheNote(t *testing.T) {
	for _, tc := range []struct {
		agent     string
		delivered bool
		notice    string
	}{
		{"claude-code_2-1-314_harness", true, ""},
		{"claude-code_2-2-0_harness", true, ""},
		{"claude-code_2-1-257_harness", false, "rashomon: a note was built and not sent to the agent (Claude Code 2.1.257, PostToolUse; " +
			"delivery verified from 2.1.280): this version is older. Shown once per session."},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			f := newSN(t)
			f.green()
			f.require(f.family())
			env := snEnv()
			env.AIAgent = tc.agent
			b := f.editOf("toolu_e", "internal/a.go").Steer(env)
			var out map[string]any
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("printed %q: %v", b, err)
			}
			_, model := out["hookSpecificOutput"]
			msg, _ := out["systemMessage"].(string)
			switch {
			case tc.delivered && (!model || !strings.Contains(string(b), snLine)):
				t.Errorf("printed %s; want the note model-visible and mirrored", b)
			case !tc.delivered && (model || len(out) != 1 || msg != tc.notice):
				t.Errorf("printed %s; want the systemMessage %q alone", b, tc.notice)
			}
			got := staleLines(t, f.st)
			if len(got) != 1 {
				t.Fatalf("%d stale-evidence lines, want 1", len(got))
			}
			want := steer.DeliveryDelivered
			if !tc.delivered {
				want = steer.DeliveryShadow
			}
			if l := got[0]; l.Delivery == nil || *l.Delivery != want {
				t.Errorf("line delivery %v, want %s", l.Delivery, want)
			}
		})
	}
}
