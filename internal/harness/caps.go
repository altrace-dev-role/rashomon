package harness

import "slices"

// CapsVersion is the version of the table Caps reads. Identity.CapsVersion
// is stamped with this constant so a stored record can be read against the
// table that produced it even after this table changes. 2 adds
// CapPersonAtPrompt (PREFLIGHT-CLOUD1).
const CapsVersion = 2

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
//     mirror_in_band as unverified"). Apart from CapPersonAtPrompt, below,
//     nothing else about Claude Code's levers depends on surface.
//   - CapPersonAtPrompt is whether a person reads the session at its
//     prompt: No on SurfaceSDK, Unverified on SurfaceUnknown, Yes on cli,
//     ide, desktop and cloud. It stops at sdk because an SDK entrypoint
//     names a program driving Claude Code, the one surface that says no one
//     is there; a presumed Claude Code with no entrypoint says nothing
//     either way. It is a column apart from CapMirrorInBand because ED-9
//     withholds a nudge when the mirror has no reader (steer/decide.go,
//     SuppressedByNoMirror, D51): that is a question about a person, not
//     about the channel, and the mirror's Unverified cannot tell an SDK
//     program from an unknown surface. The prompt path reads it for
//     attendance (hook.attendanceOf, PREFLIGHT-CLOUD1). Caps is keyed by
//     surface, so the GitHub Action is Yes with the rest of cloud: an
//     Action run gets a note nobody reads live, at the cost of one log
//     line, until a reason splits it from Claude Code on the web.
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
	for _, c := range allCapabilities {
		out[c] = Cap(id, s, c)
	}
	return out
}

// Cap is Caps(id, s)[c], read from the rule without building the table: a
// caller that reads one capability on every call pays no map for it
// (PERF-EDGE1). Caps is built from it, so the two cannot disagree.
func Cap(id ID, s Surface, c Capability) CapState {
	if !slices.Contains(allCapabilities, c) {
		return "" // not a key of the table
	}
	switch id {
	case ClaudeCode, ClaudeCodePresumed:
	default: // Cursor, Codex, Unknown, and any future value
		return No
	}
	switch {
	case c == CapMirrorInBand && (s == SurfaceSDK || s == SurfaceUnknown):
		return Unverified
	case c == CapPersonAtPrompt && s == SurfaceSDK:
		return No
	case c == CapPersonAtPrompt && s == SurfaceUnknown:
		return Unverified
	}
	return Yes
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
