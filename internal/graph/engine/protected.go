package engine

import "slices"

// protectedConflicts is one protected_path per file in the diff's Files
// that the rule applies to. A unit id the rule names is not a path: Files
// holds files alone.
func protectedConflicts(g Reader, d Diff, r Rule) []Conflict {
	var out []Conflict
	for _, id := range r.AppliesTo {
		if slices.Contains(d.Files, id) {
			out = append(out, ruleConflict(g, d, r, ConflictProtectedPath, id))
		}
	}
	return out
}
