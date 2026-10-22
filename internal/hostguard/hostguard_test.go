package hostguard

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/harness"
)

func identity(id harness.ID) harness.Identity {
	return harness.Identity{ID: id, Surface: harness.SurfaceUnknown}
}

// TestBRD4_ClaudeEntriesStandDownUnderCursor: a Claude-installed entry must
// stand down on any detected identity outside the Claude family -- Cursor,
// Codex, and Unknown alike (6.2.3: "stand down on foreign or unknown
// harnesses") -- and must NOT stand down on either Claude identity,
// confirmed or presumed.
func TestBRD4_ClaudeEntriesStandDownUnderCursor(t *testing.T) {
	cases := []struct {
		name string
		id   harness.ID
		want bool
	}{
		{"claude code positive matches installed-for", harness.ClaudeCode, false},
		{"claude code presumed matches installed-for", harness.ClaudeCodePresumed, false},
		{"cursor is foreign", harness.Cursor, true},
		{"codex is foreign", harness.Codex, true},
		{"unknown is neither claude nor confirmed foreign, still stands down", harness.Unknown, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StandsDown(FamilyClaude, identity(c.id))
			if got != c.want {
				t.Errorf("StandsDown(claude, %s) = %v, want %v", c.id, got, c.want)
			}
		})
	}
}

// TestStandsDown_EveryFamilyMatchesItsOwnDetected pins the general rule
// StandsDown implements, not just the claude-specific BRD-4 row: an entry
// never stands down against its own family, and always stands down against
// every other one -- including Unknown, which belongs to none.
func TestStandsDown_EveryFamilyMatchesItsOwnDetected(t *testing.T) {
	families := map[Family]harness.ID{
		FamilyClaude: harness.ClaudeCode,
		FamilyCursor: harness.Cursor,
		FamilyCodex:  harness.Codex,
	}
	for installedFor, ownID := range families {
		if StandsDown(installedFor, identity(ownID)) {
			t.Errorf("StandsDown(%s, %s) = true, want false (own family)", installedFor, ownID)
		}
		for otherFamily, otherID := range families {
			if otherFamily == installedFor {
				continue
			}
			if !StandsDown(installedFor, identity(otherID)) {
				t.Errorf("StandsDown(%s, %s) = false, want true (foreign family)", installedFor, otherID)
			}
		}
		if !StandsDown(installedFor, identity(harness.Unknown)) {
			t.Errorf("StandsDown(%s, unknown) = false, want true", installedFor)
		}
	}
	// claude_code_presumed is the claude family's second member; every other
	// family's own identity has no second member in the ID set.
	if StandsDown(FamilyClaude, identity(harness.ClaudeCodePresumed)) {
		t.Error("StandsDown(claude, claude_code_presumed) = true, want false")
	}
}

func TestCounterKey(t *testing.T) {
	cases := []struct {
		installedFor Family
		id           harness.ID
		want         string
	}{
		{FamilyClaude, harness.Unknown, "claude@unknown"},
		{FamilyClaude, harness.Cursor, "claude@cursor"},
		{FamilyClaude, harness.Codex, "claude@codex"},
		{FamilyClaude, harness.ClaudeCode, "claude@claude_code"},
	}
	for _, c := range cases {
		got := CounterKey(c.installedFor, identity(c.id))
		if got != c.want {
			t.Errorf("CounterKey(%s, %s) = %q, want %q", c.installedFor, c.id, got, c.want)
		}
	}
}

var claudeConfigDir = "/home/u/.claude"
var claudeEnv = harness.Env{Entrypoint: "cli", ConfigDir: claudeConfigDir}

// TestEvaluate_ClaudePayloadNeverStandsDown is the companion to
// TestEvaluate_ForeignPayloadStandsDown: the one documented Claude Code
// shape in these tests must never be read as foreign by the function the
// five entry points actually call.
func TestEvaluate_ClaudePayloadNeverStandsDown(t *testing.T) {
	raw := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","transcript_path":"/home/u/.claude/projects/p1/s1.jsonl"}`
	id, down := Evaluate([]byte(raw), nil, claudeEnv, FamilyClaude)
	if down {
		t.Fatalf("Evaluate stood down on a documented Claude Code payload: %+v", id)
	}
	if id.ID != harness.ClaudeCode {
		t.Errorf("id.ID = %s, want %s", id.ID, harness.ClaudeCode)
	}
}

// TestEvaluate_ForeignPayloadStandsDown: a Cursor-shaped payload, read
// through a Claude-installed entry's own intake path, must be read as
// Cursor and must stand down.
func TestEvaluate_ForeignPayloadStandsDown(t *testing.T) {
	raw := `{"session_id":"s1","hook_event_name":"afterFileEdit","cursor_version":"1.4.2"}`
	id, down := Evaluate([]byte(raw), nil, claudeEnv, FamilyClaude)
	if !down {
		t.Fatalf("Evaluate did not stand down on a Cursor-shaped payload: %+v", id)
	}
	if id.ID != harness.Cursor {
		t.Errorf("id.ID = %s, want %s", id.ID, harness.Cursor)
	}
}

// TestEvaluate_ReadErrorNeverStandsDown: a payload this process could not
// even buffer (hookio.Read's own error, e.g. ErrTooLarge) carries no
// evidence either way. Evaluate must defer to the caller's existing
// error-handling path rather than invent a stand-down for a payload it
// never looked at -- see the package doc for why this matters for the
// coverage reason an oversized payload already writes today.
func TestEvaluate_ReadErrorNeverStandsDown(t *testing.T) {
	id, down := Evaluate(nil, errReadTooLarge, claudeEnv, FamilyClaude)
	if down {
		t.Fatalf("Evaluate stood down on a read error: %+v", id)
	}
}

// TestEvaluate_MalformedPayloadNeverStandsDown: a payload hookio.Intake
// itself refuses (bad JSON) is the same story one layer in: no identity was
// ever decided, so nothing stands down, and the downstream Capture/RunProbe
// call that re-scans these same bytes hits the identical error unchanged.
func TestEvaluate_MalformedPayloadNeverStandsDown(t *testing.T) {
	id, down := Evaluate([]byte(`not json`), nil, claudeEnv, FamilyClaude)
	if down {
		t.Fatalf("Evaluate stood down on malformed JSON: %+v", id)
	}
}

var errReadTooLarge = &sentinelErr{"hookio: payload exceeds the read limit"}

type sentinelErr struct{ s string }

func (e *sentinelErr) Error() string { return e.s }

func TestFamilyString(t *testing.T) {
	// Family is a plain string type; CounterKey's "@"-joined form is the
	// only thing that needs to round-trip, so this just pins that the three
	// constants are exactly what the spec's examples show, not a drifted
	// synonym (e.g. "claude_code").
	for _, f := range []Family{FamilyClaude, FamilyCursor, FamilyCodex} {
		if strings.Contains(string(f), "@") {
			t.Errorf("Family %q must never itself contain '@'", f)
		}
	}
	if FamilyClaude != "claude" || FamilyCursor != "cursor" || FamilyCodex != "codex" {
		t.Fatalf("family constants drifted: %q %q %q", FamilyClaude, FamilyCursor, FamilyCodex)
	}
}
