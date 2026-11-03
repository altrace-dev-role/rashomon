package harness

import "testing"

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
