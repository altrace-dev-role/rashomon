package harness

import (
	"slices"
	"testing"
)

// TestPE1_CapIsCapsWithoutTheTable (card PERF-EDGE1): Cap reads one
// capability as Caps' table holds it, for every harness, surface and
// capability, a harness the table does not know included. Control for the
// allocation test below: a Cap that reads Caps' table passes this one.
// Break: a rule Cap drops (the mirror's Unverified off cli, the person's No
// on sdk), and steerEnv or the prompt's attendance reads another answer
// than the table the record is stamped with.
func TestPE1_CapIsCapsWithoutTheTable(t *testing.T) {
	ids := []ID{ClaudeCode, ClaudeCodePresumed, Cursor, Codex, Unknown, ID("future")}
	surfaces := []Surface{SurfaceCLI, SurfaceIDE, SurfaceDesktop, SurfaceSDK, SurfaceCloud, SurfaceUnknown, Surface("")}
	for _, id := range ids {
		for _, s := range surfaces {
			table := Caps(id, s)
			for _, c := range append(allCapabilities[:len(allCapabilities):len(allCapabilities)], Capability("made_up")) {
				if got := Cap(id, s, c); got != table[c] {
					t.Errorf("Cap(%s, %s, %s) = %q, Caps holds %q", id, s, c, got, table[c])
				}
			}
		}
	}
}

// TestPE1_ACapReadAllocatesNothing: every post builds steerEnv, which reads
// one capability; building Caps' table of seventeen for it cost about 1.2 KB
// a call (pe1-bench's profile, harness.Caps flat under cmdPost). Cap reads
// the rule, not a table, so a read allocates nothing. Break: Cap built from
// Caps' table.
func TestPE1_ACapReadAllocatesNothing(t *testing.T) {
	var got CapState
	if n := testing.AllocsPerRun(100, func() { got = Cap(ClaudeCode, SurfaceSDK, CapMirrorInBand) }); n != 0 {
		t.Errorf("a Cap read allocates %v times, want 0", n)
	}
	if got != Unverified {
		t.Errorf("premise: Cap(claude_code, sdk, mirror_in_band) = %q, want unverified", got)
	}
}

// TestPE1_NoEntrypointLeavesBothUnverified pins the unknown surface's
// column, which no test read before PE-1's rows found it: a Claude Code
// with no entrypoint has a mirror nobody is known to read (Archive 60.2.9)
// and a prompt nobody is known to be at, unverified on both and neither the
// sdk's no nor yes, so its identity is blind to exactly those two. Break:
// either reads yes, and a session with no entrypoint claims a reader
// nothing says is there.
func TestPE1_NoEntrypointLeavesBothUnverified(t *testing.T) {
	got := Detect(intake(t, `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`),
		Env{ConfigDir: claudeConfigDir})
	if got.ID != ClaudeCodePresumed || got.Surface != SurfaceUnknown {
		t.Fatalf("premise: Detect = %s on %s, want claude_code_presumed on unknown", got.ID, got.Surface)
	}
	for _, c := range []Capability{CapMirrorInBand, CapPersonAtPrompt} {
		if s := Cap(got.ID, got.Surface, c); s != Unverified {
			t.Errorf("Cap(claude_code_presumed, unknown, %s) = %q, want unverified", c, s)
		}
	}
	if want := []string{string(CapMirrorInBand), string(CapPersonAtPrompt)}; !slices.Equal(got.Blind, want) {
		t.Errorf("Blind = %v, want exactly %v on the unknown surface", got.Blind, want)
	}
}
