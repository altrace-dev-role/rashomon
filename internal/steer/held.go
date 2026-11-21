package steer

import "slices"

// Pair is one pattern and kind: what a note is held at flag by, and what
// the operator's level override names (Context.LevelOverride).
type Pair struct {
	Pattern PatternID
	Kind    Kind
}

// heldAtFlag is the pairs whose notes print as flags until their precision,
// judged on real sessions, promotes them: a held pair's note never reaches
// the model, and its line reads as a flag finding's. A promotion removes its
// pair's row. It is a table, not a switch, for the reason levelNames is
// one, and it holds no row until a pattern is held.
var heldAtFlag []Pair

// HeldAtFlag reports whether p's notes are held at flag.
func HeldAtFlag(p Pair) bool {
	return slices.Contains(heldAtFlag, p)
}

// heldLevel is f's level after the hold: a nudge of a held pair that the
// context's override does not name is a flag, with no reason, so its line
// is a flag finding's. Every other level is unchanged, and so is every
// other clamp, which decide applies after this one.
func heldLevel(ctx *Context, f Finding, level Level) Level {
	if level != LevelNudge {
		return level
	}
	if p := (Pair{Pattern: f.Pattern, Kind: f.Kind}); HeldAtFlag(p) && ctx.LevelOverride != p {
		return LevelFlag
	}
	return level
}
