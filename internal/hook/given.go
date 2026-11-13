package hook

import (
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// Given is the one rule for what a session's steer log says it was given,
// read by every note said once per key: the blast-radius note (its last
// given caller digest), the plan check (its plan's key) and the unit brief
// (cmd, its unit whatever the digest). It calls fn, in log order, with the
// rest of the key of each line of pattern pat whose key starts with prefix
// and holds no "@" after it (a key names its subject up to its last "@"),
// and that was given: state delivered, which a held-out line keeps
// (steer decide), or suppressed_by arm_off, whose decision gave it. With
// armOffNeedsNudge an arm_off line counts only at level nudge, as measure
// counts a withheld note (measure notes). Lines are located with jsonscan,
// never decoded (RES-5).
func Given(lines [][]byte, pat pattern.ID, prefix string, armOffNeedsNudge bool, fn func(rest string)) {
	var sp [5]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, givenKeys(), sp[:]) != nil || spanText(b, sp[0]) != string(pat) {
			continue
		}
		rest, ok := strings.CutPrefix(spanText(b, sp[2]), prefix)
		if !ok || strings.Contains(rest, "@") {
			continue
		}
		if spanText(b, sp[1]) == string(steer.LineStateDelivered) ||
			spanText(b, sp[3]) == string(steer.SuppressedByArmOff) && (!armOffNeedsNudge || spanText(b, sp[4]) == steer.LevelNudge.String()) {
			fn(rest)
		}
	}
}

// givenKeys is the one KeySet Given reads a line with, built on its first
// call and never at init (RES-2).
func givenKeys() *jsonscan.KeySet {
	givenKeysOnce.Do(func() {
		givenKeysOnce.v = jsonscan.NewKeySet("pattern", "state", "key", "suppressed_by", "level")
	})
	return givenKeysOnce.v
}

var givenKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}
