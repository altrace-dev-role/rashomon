package hookio_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/events"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// exitPlanInput is an ExitPlanMode tool_input as PLAN.md section 14 (the
// line beginning "The post path") records the call: the keys plan and
// planFilePath, read from a transcript at Claude Code 2.1.280.
const exitPlanInput = `{"plan":"# Plan\n\nEdit internal/graph/g.go","planFilePath":"/home/u/.claude/plans/p.md"}`

// TestPlanEvent_OnlyPostOfExitPlanMode: a PostToolUse of ExitPlanMode is the
// plan kind; the Pre of the same tool, any other tool and any other event are
// not, and EventKind still maps PreToolUse to act_declared.
func TestPlanEvent_OnlyPostOfExitPlanMode(t *testing.T) {
	k, ok := hookio.PlanEvent("PostToolUse", "ExitPlanMode")
	if !ok || k != events.Plan {
		t.Errorf("Post ExitPlanMode = %q %v, want plan true", k, ok)
	}
	for _, c := range [][2]string{{"PreToolUse", "ExitPlanMode"}, {"PostToolUse", "Edit"}, {"PostToolUseFailure", "ExitPlanMode"},
		{"UserPromptSubmit", ""}, {"PostToolUse", ""}, {"PostToolUse", "exitplanmode"}} {
		if k, ok := hookio.PlanEvent(c[0], c[1]); ok || k != "" {
			t.Errorf("PlanEvent(%q, %q) = %q %v, want none", c[0], c[1], k, ok)
		}
	}
	if k, _, ok := hookio.EventKind("PreToolUse", ""); !ok || k != events.ActDeclared {
		t.Errorf("EventKind(PreToolUse) = %q, want act_declared", k)
	}
	if k, _, ok := hookio.EventKind("PostToolUse", ""); !ok || k != events.ActLanded {
		t.Errorf("EventKind(PostToolUse) = %q, want act_landed", k)
	}
}

// TestPlanText_TheKeysString: the "plan" key's string, false when the key is
// absent, not a string, empty, or the input is not an object.
func TestPlanText_TheKeysString(t *testing.T) {
	got, ok := hookio.PlanText(json.RawMessage(exitPlanInput))
	if !ok || string(got) != "# Plan\n\nEdit internal/graph/g.go" {
		t.Errorf("PlanText = %q %v", got, ok)
	}
	for name, in := range map[string]string{
		"absent":     `{"planFilePath":"/x.md"}`,
		"number":     `{"plan":7}`,
		"object":     `{"plan":{"a":"b"}}`,
		"null":       `{"plan":null}`,
		"empty":      `{"plan":""}`,
		"not object": `["plan"]`,
		"empty raw":  ``,
		"garbage":    `{"plan":"x`,
	} {
		if b, ok := hookio.PlanText(json.RawMessage(in)); ok || b != nil {
			t.Errorf("%s: PlanText = %q %v, want none", name, b, ok)
		}
	}
	big := `{"plan":"` + strings.Repeat("a", 120000) + `"}`
	if b, ok := hookio.PlanText(json.RawMessage(big)); !ok || len(b) != 120000 {
		t.Errorf("a 120000-byte plan: %d bytes %v", len(b), ok)
	}
}
