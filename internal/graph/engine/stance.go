package engine

import "strings"

// One reading of what a clause says of the thing it names (W-M1.43), read
// by both the frame (parseSentence) and the name reader (Names). The name
// reader looked at a clause's first word alone, so a clause that opened
// with a conjunction ("B374 again, but do not touch internal/spend") named
// the unit it excluded and the frame barred nothing; and the frame read
// "leave X alone" as a kept X, which bars nothing.

// stance is what a clause says of the thing it names: open, the thing to
// change or a fact about it; kept, an invariant on the work ("keep the API
// stable"), which bars nothing; excluded, what the change leaves alone
// ("do not touch the store"), whose object bars the unit it names. The
// name reader reads names from an open clause alone.
type stance int

const (
	stanceOpen stance = iota
	stanceKept
	stanceExcluded
)

// clauseStance is the clause's stance and where the word it is read from
// stands: the clause's own first word, past a leading but, and, yet or
// though. A negation excludes ("without", "except", "excluding", "skip",
// "avoid", "ignore", a clause-initial "not"), as do "never", "do not" and
// "don't" over a verb that is not keep-class ("do not keep the old lock
// file" is work on it, W-M1.35), and "leave" with "alone", "untouched" or
// "as is" after its object; any other keep-class head keeps. A bare "not"
// after a conjunction is open: it names the gap the request fills ("but not
// for Maven").
func clauseStance(cl []string) (stance, int) {
	i := 0
	for i < len(cl) && leadWords.get()[low(cl[i])] {
		i++
	}
	if i == len(cl) {
		return stanceOpen, i
	}
	head, rest := low(cl[i]), cl[i+1:]
	switch {
	case head == "not" && i > 0:
		return stanceOpen, i
	case head == "never" || dontWords.get()[head]:
		return negatedStance(rest), i
	case head == "do" && len(rest) > 0 && low(rest[0]) == "not":
		return negatedStance(rest[1:]), i
	case negationWords.get()[head]:
		return stanceExcluded, i
	case head == "leave":
		return leaveStance(rest), i
	case verbClass.get()[head] == "keep":
		return stanceKept, i
	}
	return stanceOpen, i
}

// negatedStance is the stance of a negating head over the words after it:
// excluded unless no verb follows or the verb is keep-class.
func negatedStance(rest []string) stance {
	if len(rest) == 0 || verbClass.get()[low(rest[0])] == "keep" {
		return stanceKept
	}
	return stanceExcluded
}

// leaveStance is what "leave" says of its object: kept over the rest of a
// thing ("leave the rest of the report alone": that rest stays in the unit
// the change edits), excluded with "alone", "untouched" or "as is" after
// the object ("leave the hook alone"), and kept otherwise ("leave the
// checks where they are").
func leaveStance(rest []string) stance {
	w := func(k int) string {
		if k < len(rest) {
			return low(cleanWord(rest[k]))
		}
		return ""
	}
	at := 0
	if w(0) == "the" {
		at = 1
	}
	if w(at) == "rest" && w(at+1) == "of" || (w(0) == "everything" || w(0) == "all") && w(1) == "else" {
		return stanceKept
	}
	for k := 1; k < len(rest); k++ {
		if x := w(k); x == "alone" || x == "untouched" || x == "as" && w(k+1) == "is" {
			return stanceExcluded
		}
	}
	return stanceKept
}

// headClass is the verb class a clause's head is read by: a don't is read
// as "do not" is, a keep-class head whose negated verb negatedKeep reads.
func headClass(head string) string {
	if dontWords.get()[head] {
		return "keep"
	}
	return verbClass.get()[head]
}

// negatedKeep reads a keep-class clause whose head negates the verb after
// it ("never touch the store", "do not embed the summary", "don't embed
// the summary"): that verb is a verb of the clause, as after a modal, and
// the thing after it is left alone, an exclusion. A negated keep-class
// verb ("do not keep the old lock file", "never leave the lock file
// behind") means the opposite, work on the thing, and keeps the class's
// shape, a kept thing that bars nothing (W-M1.35). The verb, the rest
// without it, and whether the thing is excluded.
func negatedKeep(head string, rest []string) (string, []string, bool) {
	i := -1
	switch {
	case head == "never" || dontWords.get()[head]:
		i = 0
	case head == "do" && len(rest) > 0 && low(rest[0]) == "not":
		i = 1
	}
	if i < 0 || i >= len(rest) || verbClass.get()[low(rest[i])] == "keep" {
		return "", rest, false
	}
	return low(rest[i]), append(append([]string{}, rest[:i]...), rest[i+1:]...), true
}

// splitExclusion is the words before the first exclusion word, and what
// exclusionOf reads from the words from it on.
func splitExclusion(words []string) ([]string, []Phrase, []string) {
	for i, w := range words {
		if exclusionWords.get()[low(w)] {
			cons, aside := exclusionOf(words[i:])
			return words[:i], cons, aside
		}
	}
	return words, nil, nil
}

// exclusionOf is, as constraints, the thing the exclusion word at words[0]
// leaves alone, less the gerund that carries it; and the carrier words
// themselves: "without changing the log format" leaves the log format
// alone and "without changing" is aside, while a word in -ing with nothing
// after it is the thing ("without logging").
func exclusionOf(words []string) ([]Phrase, []string) {
	tail, aside := words[1:], words[:1]
	if len(tail) > 1 && strings.HasSuffix(low(tail[0]), "ing") {
		tail, aside = tail[1:], words[:2]
	}
	cons := phrasesOf(tail)
	for k := range cons {
		cons[k].Excluded = true
	}
	return cons, aside
}

// leadWords are the conjunctions a clause may open with before its own
// first word.
var leadWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"but": true, "and": true, "yet": true, "though": true}
}}

// dontWords are "don't" as a request writes it: with an apostrophe, with
// the right single quotation mark a pasted text carries (U+2019), or with
// none.
var dontWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"don't": true, "don’t": true, "dont": true}
}}

// negationWords are the heads that exclude their clause's object whatever
// follows them.
var negationWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"not": true, "without": true, "avoid": true, "except": true, "excluding": true, "skip": true, "ignore": true}
}}

// exclusionHeads open what the change leaves alone at a clause's head, as
// exclusionWords do anywhere ("skip the cache", "avoid the store", "ignore
// the hook tests"); after the head they are words of its clause ("make the
// generator skip packages"). A clause-initial bare "not" opens none: the
// frame reads it as it reads any other clause.
var exclusionHeads = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"skip": true, "avoid": true, "ignore": true}
}}
