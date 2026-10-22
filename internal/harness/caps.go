package harness

// CapsVersion is the version of the table Caps reads. Identity.CapsVersion
// is stamped with this constant so a stored record can be read against the
// table that produced it even after this table changes.
const CapsVersion = 1

// Caps returns the capability table for one (ID, Surface) pair.
//
// What this table claims today, and why it stops where it does:
//
//   - Claude Code is the harness this product has always run against: every
//     capability below names a lever rashomon's existing hook, post and
//     declaration paths already use (internal/hook). It is "yes" for
//     ClaudeCode and ClaudeCodePresumed on every surface, with one named
//     exception.
//   - CapMirrorInBand is Unverified on SurfaceSDK and SurfaceUnknown. An
//     SDK-embedding program is only told about rashomon's note if it chooses
//     to render SDKInformationalMessage, which rashomon cannot observe, and a
//     presumed identity with no entrypoint carries the same uncertainty
//     (Archive 60.2.9: "Caps(claude_code_presumed, unknown) treats
//     mirror_in_band as unverified"). Nothing else about Claude Code's
//     levers depends on surface.
//   - Cursor and Codex get "no" for every capability, on every surface, for
//     a verifiable reason rather than a guess: neither adapter exists in
//     this tree (no internal/adapter package). A table that said "yes" for a
//     harness rashomon cannot yet speak to would be a claim with nothing
//     behind it. The real per-lever table for those harnesses is lever
//     routing and adapter work (6.2.4, 6.2.5), out of this change's scope;
//     this function is the seam that work fills in, one harness at a time,
//     without any caller changing.
//   - Unknown gets "no" for every capability: rashomon does not know what it
//     is talking to, so it claims nothing.
func Caps(id ID, s Surface) CapSet {
	out := make(CapSet, len(allCapabilities))
	switch id {
	case ClaudeCode, ClaudeCodePresumed:
		for _, c := range allCapabilities {
			out[c] = Yes
		}
		if s == SurfaceSDK || s == SurfaceUnknown {
			out[CapMirrorInBand] = Unverified
		}
	default: // Cursor, Codex, Unknown, and any future value
		for _, c := range allCapabilities {
			out[c] = No
		}
	}
	return out
}

// blindOf returns the capability names in caps whose state is not Yes,
// sorted. allCapabilities is already in a fixed order, so the result is
// sorted without an extra pass.
func blindOf(caps CapSet) []string {
	var blind []string
	for _, c := range allCapabilities {
		if caps[c] != Yes {
			blind = append(blind, string(c))
		}
	}
	return blind
}
