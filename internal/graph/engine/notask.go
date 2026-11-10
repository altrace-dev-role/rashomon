package engine

import (
	"fmt"
	"slices"
	"strings"
	"time"
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

// notice reads harness notices off a prompt by their envelope alone, as a
// notice's own text is free text that matches code (a peer's summary of
// its turn, a background command's description). A notice is a line that
// opens with a bracketed label ending " notice", through the line that
// closes a «summary» it opens; or an element whose tag ends
// "-notification", from its opening tag through its closing one, text
// after the closing tag on its line kept. It fails safe: an element never
// closed is no envelope and is read as typed, and a summary never closed
// takes only the label's line. It returns what is left and whether a
// notice was read; a prompt with none is returned as it is.
func notice(prompt string) (string, bool) {
	lines := strings.Split(prompt, "\n")
	var keep []string
	held := false
	for i := 0; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if noticeLabel(s) {
			held, i = true, summaryEnd(lines, i)
			continue
		}
		if tag, ok := notificationTag(s); ok {
			if end, after, ok := closing(lines, i, tag); ok {
				held, i, keep = true, end, keepText(keep, after)
				continue
			}
		}
		keep = append(keep, lines[i])
	}
	if !held {
		return prompt, false
	}
	return strings.Join(keep, "\n"), true
}

// noticeLabel is whether line s opens with a bracketed label ending
// " notice" ("[Cross-session idle notice]").
func noticeLabel(s string) bool {
	if !strings.HasPrefix(s, "[") {
		return false
	}
	label, _, ok := strings.Cut(s[1:], "]")
	return ok && strings.HasSuffix(strings.ToLower(label), " notice")
}

// summaryEnd is the last line of the notice that opens at line i: the
// line that closes the «summary» the label's line opens and leaves open,
// else the label's line itself.
func summaryEnd(lines []string, i int) int {
	_, open, ok := strings.Cut(lines[i], "«")
	if !ok || strings.Contains(open, "»") {
		return i
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.Contains(lines[j], "»") {
			return j
		}
	}
	return i
}

// notificationTag is the tag line s opens with when it is one word ending
// "-notification" ("<task-notification>"). A closing tag is none, as no
// line closes "/task-notification".
func notificationTag(s string) (string, bool) {
	if !strings.HasPrefix(s, "<") {
		return "", false
	}
	tag, _, ok := strings.Cut(s[1:], ">")
	return tag, ok && strings.HasSuffix(tag, "-notification") && !strings.ContainsAny(tag, " \t")
}

// closing is the line from i on that closes tag and what follows the
// closing tag there; false when no line does.
func closing(lines []string, i int, tag string) (int, string, bool) {
	for j := i; j < len(lines); j++ {
		if _, after, ok := strings.Cut(lines[j], "</"+tag+">"); ok {
			return j, after, true
		}
	}
	return 0, "", false
}

// keepText is keep with s appended when s holds more than space.
func keepText(keep []string, s string) []string {
	if s = strings.TrimSpace(s); s != "" {
		return append(keep, s)
	}
	return keep
}
