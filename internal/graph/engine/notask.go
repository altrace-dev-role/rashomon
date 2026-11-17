package engine

import (
	"fmt"
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
)

// The control rule (W-M1.42, PLAN 6.1). The hook briefs every request it
// is handed, and some carry no task the graph can answer: a harness
// notice with nothing typed after it, and a short request that names
// nothing and on which the ladder found no unit, where every line left
// is rank's match on a common word ("stop" brought internal/events
// declaring Stop). Such a request is sent no brief, as a brief that would
// say only what is not known is not (keyed).

// noTaskWords is the most content words a request that names nothing and
// finds no unit may hold and still be refused. On the five sets of
// W-M1.42's registration no row whose truth names a unit is refused at 3,
// 4 or 5 words and the first is at 6; 3 is the least at which the control
// set sends nothing.
const noTaskWords = 3

// taskFreeWords are the stems of the question and auxiliary words, which
// open or carry a request and name nothing in it.
var taskFreeWords = &lazySet[bool]{f: taskFreeSet}

// taskFreeSet builds taskFreeWords. It is a named function, so the init scan
// (RES-2) does not read its get calls as init-time calls.
func taskFreeSet() map[string]bool {
	out := map[string]bool{}
	for _, set := range []map[string]bool{questionWords.get(), auxiliaryWords.get()} {
		for w := range set {
			if s := stem(w); s != "" {
				out[s] = true
			}
		}
	}
	return out
}

// contentWords is tokens less the question and auxiliary words.
func contentWords(tokens []string) []string {
	free := taskFreeWords.get()
	return slices.DeleteFunc(slices.Clone(tokens), func(t string) bool { return free[t] })
}

// The control rule's branches, named in each refusal's decision so the
// silences are counted per rule: W3 a request that names nothing and finds
// no unit in at most noTaskWords content words; U1 one content word that
// is the whole target; U3 a harness notice with no request after it.
const (
	ruleNoUnit  = "W3"
	ruleOneWord = "U1"
	ruleNotice  = "U3"
)

// noTask is the rule that refuses a request as carrying no task the graph
// can brief and why, "" when none does; why holds counts and no word of
// the request, as Evidence prints it and carries nothing of the prompt. A
// request that names a unit, a record or a path the graph lacks is never
// refused; nor is one on which the ladder found a unit, kept or withheld,
// unless its one content word is its whole target, a unit found by that
// word alone ("steer it" finds internal/steer by its name; on W-M1.42's
// 573 rows U1 refuses only that one).
func noTask(r Roles, tokens []string) (string, string) {
	if n := r.Named; n.Unit != "" || len(n.Knowledge) > 0 || len(n.Unknown) > 0 {
		return "", ""
	}
	words := contentWords(tokens)
	switch {
	case !r.Found() && len(words) <= noTaskWords:
		return ruleNoUnit, fmt.Sprintf("nothing named, no unit found, %d of at most %d content words", len(words), noTaskWords)
	case len(words) == 1 && slices.Equal(contentWords(r.Target), words):
		return ruleOneWord, "one content word, the whole target"
	}
	return "", ""
}

// noticeRead is the decision a brief carries when it read the request
// after a harness notice.
const noticeRead = "read: the request after a harness notice"

// unsent is b sent no brief by rule for why: no lines, the refusal its
// last decision ("not sent: W3, ...").
func (b Brief) unsent(rule, why string, start time.Time) Brief {
	b.Decisions = append(b.Decisions, Decision{Why: "not sent: " + rule + ", " + why})
	b.Elapsed = time.Since(start)
	return b
}

// notice reads harness frames off a prompt (harness.Cut): what is left
// and whether a frame was read; a prompt with none is returned as it is.
// cut is whether the reader took only the prompt's head (Options.PromptCut).
func notice(prompt string, cut bool) (string, bool) {
	return harness.Cut(prompt, cut)
}
