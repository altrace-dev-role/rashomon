package hook

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/share"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// stStoreLine is integrity/store-touched's user line for the task file
// (archive 40.4.7; card P5B, 5.7).
const stStoreLine = "rashomon record: this call wrote '.claude/rashomon/task/current.json', under .claude/rashomon, " +
	"which holds the spec and the task file rashomon reads."

// stTaskWrite is a Write of the task file under the fixture's cwd.
var stTaskWrite = map[string]any{"file_path": "/tmp/project/.claude/rashomon/task/current.json", "content": "{}"}

// TestSTR58_IntegrityFlagsNeverReachTheModel (archive 40.4.7; card P5B, 5.7):
// a call that writes the task file is raised at its own post as an
// integrity/store-touched flag, at a success and at a failure event, with
// consent current or none: the user's systemMessage carries the line, no
// model-visible key prints, and the steer line is flag_only with no model
// text. A user-only pattern never rises to a note (N15).
func TestSTR58_IntegrityFlagsNeverReachTheModel(t *testing.T) {
	for _, c := range []struct {
		name    string
		exit    int
		consent int
	}{
		{"ok, no consent", 0, 0},
		{"ok, consent current", 0, steer.DisclosureVersion},
		{"failed, no consent", 1, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newST(t)
			b := f.call("toolu_s", "Write", stTaskWrite, c.exit).Steer(stEnv(c.consent))
			var out map[string]any
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("printed %q: %v", b, err)
			}
			if msg, _ := out["systemMessage"].(string); msg != stStoreLine {
				t.Errorf("systemMessage %q, want %q", msg, stStoreLine)
			}
			if _, model := out["hookSpecificOutput"]; model {
				t.Errorf("a model-visible key printed: %s", b)
			}
			lines := steerLines(t, f.st)
			if len(lines) != 1 {
				t.Fatalf("%d steer lines, want 1", len(lines))
			}
			l := lines[0]
			if l.Pattern != string(pattern.Integrity) || l.Kind != "store-touched" || l.Level != steer.LevelFlag.String() ||
				l.ModelText != "" || l.UserText != stStoreLine || l.Delivery == nil || *l.Delivery != steer.DeliveryFlagOnly {
				t.Errorf("steer line %s/%s at %s, model %q, user %q, delivery %v; want integrity/store-touched, flag, no model text, flag_only",
					l.Pattern, l.Kind, l.Level, l.ModelText, l.UserText, l.Delivery)
			}
		})
	}
}

// TestIV1a_StoreNamedScansTheRawInput (D-B7): the trigger reads the raw
// tool_input for .claude/rashomon, with its slash plain or escaped as JSON
// may write it; a name that only contains the words, or another directory,
// does not trip it. A superset of the store's paths is the point: whether a
// call wrote there is verify's to say.
func TestIV1a_StoreNamedScansTheRawInput(t *testing.T) {
	for in, want := range map[string]bool{
		`{"file_path":"/p/.claude/rashomon/task/current.json"}`: true,
		`{"file_path":"\/p\/.claude\/rashomon\/task\/x.json"}`:  true,
		`{"command":"cat > .claude/rashomon/spec.json"}`:        true,
		`{"file_path":"/p/.claude/rashomonx/y"}`:                true,
		`{"file_path":"/p/.claude/settings.json"}`:              false,
		`{"file_path":"/p/docs/rashomon/task/current.json"}`:    false,
		`{"command":"rashomon status"}`:                         false,
		``:                                                      false,
	} {
		if got := storeNamed([]byte(in)); got != want {
			t.Errorf("storeNamed(%s) = %v, want %v", in, got, want)
		}
	}
}

// TestIV7_AStoreFlagCountsAsAnIntegrityFire (PF-IV7, D-B10): the flag is
// counted where the steer log's fires are counted now, one fired line under
// integrity in the outcomes bundle.
func TestIV7_AStoreFlagCountsAsAnIntegrityFire(t *testing.T) {
	f := newST(t)
	if b := f.call("toolu_s", "Write", stTaskWrite, 0).Steer(stEnv(0)); len(b) == 0 {
		t.Fatal("the store write printed nothing")
	}
	out, err := share.BuildOutcomes(f.st)
	if err != nil {
		t.Fatal(err)
	}
	if out.Counts.Fired != 1 || len(out.ByPattern) != 1 || out.ByPattern[0].Pattern != pattern.Integrity || out.ByPattern[0].Fired != 1 {
		t.Errorf("outcomes fired %d by pattern %+v; want one integrity fire", out.Counts.Fired, out.ByPattern)
	}
}
