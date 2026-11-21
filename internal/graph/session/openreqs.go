package session

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// supersededMark follows the name of a superseded requirement. superseded
// is not met, so a list of unfinished requirements includes it, marked
// (ReqStatus); the mark says a later prompt's line moved past it.
const supersededMark = " (at an earlier prompt)"

// maxOpenNames is how many names the first line prints before "and N more".
const maxOpenNames = 2

// OpenReqsLines is the detail of the units rashomon named this session that
// no write has met since: the opened_unit requirements whose status is open
// or superseded and whose path listable takes. It returns nil when it lists
// none, and otherwise exactly two lines without the indent a reader puts
// before them: the names, then the sentence that says what they are. The
// words alone pass 80 columns under the Stop's continuation indent at every
// count, so the detail goes on lines of its own (D86).
//
// Open rows come first, then superseded rows with supersededMark after the
// name; in each group the newest LastStatedAt comes first, ties by path.
// At most maxOpenNames names print, then "and N more" for the rest, where N
// counts both groups.
// A name is the path slot's text and prints whole: a path is never cut, and
// a first line that long paths push past 80 columns prints whole too.
//
// opened_unit is the one predicate whose requirement the fold meets by
// evidence (openedUnit), so no other predicate's requirement is listed.
// met_once is history, never read as met or held, and is not listed;
// neither is dropped. OpenReqsLines reads reqs and never changes it.
func OpenReqsLines(reqs []ReqState) []string {
	var open, superseded []ReqState
	for _, r := range reqs {
		if r.Pred != steer.PredOpenedUnit || !listable(r.Arg) {
			continue
		}
		switch r.Status {
		case ReqOpen:
			open = append(open, r)
		case ReqSuperseded:
			superseded = append(superseded, r)
		}
	}
	n := len(open) + len(superseded)
	if n == 0 {
		return nil
	}
	newestFirst := func(a, b ReqState) int {
		return cmp.Or(cmp.Compare(b.LastStatedAt, a.LastStatedAt), strings.Compare(a.Arg, b.Arg))
	}
	slices.SortFunc(open, newestFirst)
	slices.SortFunc(superseded, newestFirst)
	listed := slices.Concat(open, superseded)
	names := make([]string, 0, maxOpenNames)
	for _, r := range listed[:min(n, maxOpenNames)] {
		name := slot.Path(r.Arg).Text()
		if r.Status == ReqSuperseded {
			name += supersededMark
		}
		names = append(names, name)
	}
	var first string
	switch more := n - len(names); {
	case more > 0:
		first = strings.Join(names, ", ") + " and " + strconv.Itoa(more) + " more"
	case len(names) == 1:
		first = names[0]
	default:
		first = strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
	second := "units rashomon named this session; no write under any seen since."
	switch n {
	case 1:
		second = "a unit rashomon named this session; no write under it seen since."
	case 2:
		second = "units rashomon named this session; no write under either seen since."
	}
	return []string{"open: " + first, second}
}

// listable reports whether a unit's path may be named on the line, by the
// path slot's rule (slot.Path.Validate) and nothing else: the rule the Stop
// line prints a unit by and a unit line binds by. A unit whose path the rule
// refuses is neither named nor counted. The rule refuses an empty path, a
// "." or ".." segment, a leading '/', a segment with a leading '-', any byte
// outside its segment charset (a newline among them), a secret shape and a
// segment the entropy test refuses; the keyed digest that stands for a path
// at record scope v4 is one such form. Leaving such a unit out is a miss,
// never a false line.
func listable(arg string) bool {
	return slot.Path(arg).Validate() == nil
}
