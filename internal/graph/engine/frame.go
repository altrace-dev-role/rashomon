package engine

import (
	"strconv"
	"strings"
)

// The prompt's frame (W-M1.12): what the request names as the place of
// the change (targets), what it adds or shows (additions), what it keeps
// (constraints) and what it says to use (uses), read by a rule table
// over clauses rather than a parser. A clause that opens with a verb of a
// known class gives its object; the last place preposition in the clause,
// or a clause that opens with one, gives the place; a clause with a modal
// (should, must) names its subject as the target; a use word (respect,
// honor, via, using) opens the thing the change must take up, which is
// what the request names as existing elsewhere. A test clause ("add a
// test that X", "write tests for X") names X as the target, not the test
// as an addition (T1c: PL12's off-task reuse came from the test phrase).
// A sentence with no verb of a known class names its subject: a symptom
// ("the report prints a row twice when...") or a question ("where is the
// deadline enforced?") names the thing before its first boundary. Words
// keep their case so an identifier's tokens split (EvaluateStop is two
// tokens, not one unknown word); the word lists are read in lower case.
// The frame lives in memory and Explain alone prints its words (D160).

// Phrase is a run of the prompt's words as one thing it names: its tokens
// (Tokens), whether the prompt marks it as existing (the, existing,
// current, its, our, this, or "already" after it), whether it marks it as
// the surface the change lands on (an anchor: "wherever X already
// appear", "the existing X"; W-M1.14 R1), and how many sentences name it.
type Phrase struct {
	Tokens   []string
	Definite bool
	Anchor   bool
	Owner    bool
	Count    int
	// Adverb is the phrase's last token when its word has an adverb's
	// form ("lazily", "weekly") and the phrase has another: the verb's
	// manner, not the thing's head, unless a name holds it (the roles
	// stage decides, by the index: "the message assembly" keeps its head
	// where assembly is a name).
	Adverb string
	// Pronoun is the pronoun the phrase opened with when no determiner
	// followed it ("it faster"): the pronoun stands for a thing another
	// clause names and the words after it say what the thing should be,
	// so the roles stage claims nothing from them unless a name holds
	// one (W-M1.32, K5).
	Pronoun string
	// Excluded is whether the phrase names what the change leaves alone
	// ("without touching the store", "do not touch the store"): an
	// exclusion, which the roles stage bars from the candidates, where a
	// kept thing ("keep the store tests green") is an invariant on the
	// work and bars nothing (W-M1.34, W-M1.35).
	Excluded bool
}

// Frame is what the prompt asks: the places it names as the target of
// the change, best first (named by most sentences, then definite, then
// first named), the things it adds or shows, what it keeps, what it says
// to use, and the verbs it used to say so (as tokens: what the prompt
// does, not a thing it names).
type Frame struct {
	Targets, Additions, Constraints, Uses []Phrase
	Verbs                                 []string
	// Aside is the words that carry a constraint clause, its exclusion
	// word and the gerund after it ("without changing"): they frame the
	// thing left alone and name neither it nor the work, so no stage
	// weighs them (W-M1.33, O7).
	Aside []string
}

// Parse reads the frame of prompt. A sentence with no verb clause is a
// symptom, and its subject is the target when the verb sentences name
// nothing ("Note: the budget store drops rows. Fix it."); a label
// sentence ("Part 2", "Phase B") names nothing.
func Parse(prompt string) Frame {
	var f Frame
	var verbs, aside []string
	var symptoms []Phrase
	for _, sentence := range sentencesOf(prompt) {
		if isLabel(sentence) {
			continue
		}
		c := parseSentence(sentence)
		if c.empty() {
			symptoms = addPhrases(symptoms, firstSubject(sentence))
		}
		f.Targets = addPhrases(f.Targets, c.targets)
		f.Additions = addPhrases(f.Additions, c.additions)
		f.Constraints = addPhrases(f.Constraints, c.constraints)
		f.Uses = addPhrases(f.Uses, c.uses)
		verbs = append(verbs, c.verbs...)
		aside = append(aside, c.aside...)
	}
	if len(f.Targets) == 0 && len(f.Additions) == 0 {
		if len(verbs) == 0 {
			f.Targets = addPhrases(f.Targets, firstSubject(prompt))
		} else {
			f.Targets = addPhrases(f.Targets, symptoms)
		}
	}
	orderPhrases(f.Targets)
	f.Verbs = uniq(Tokens(strings.Join(verbs, " ")))
	f.Aside = uniq(Tokens(strings.Join(aside, " ")))
	return f
}

// firstSubject is the subject of the prompt's first clause, the target
// when no sentence has a verb clause or a modal: a symptom ("platformRow
// double counts a row when..."), a question ("Why does the Stop verdict
// say unknown..."), a fragment ("purging the sessions"). The subject is
// what the sentence is about; the words after its verb are the symptom.
func firstSubject(prompt string) []Phrase {
	for _, sentence := range sentencesOf(prompt) {
		if isLabel(sentence) {
			continue
		}
		for _, cl := range clausesOf(sentence) {
			if len(cl) > 0 {
				return subjectOf(cl)
			}
		}
	}
	return nil
}

// sentencesOf splits the prompt at sentence ends: a newline, "!", "?",
// ";", and "." or ":" followed by white space or the end, so a path
// (internal/x/y.go), a qualified name (verify.EvaluateStop) and a line
// reference (stop.go:214) stay whole.
func sentencesOf(prompt string) []string {
	var out []string
	start := 0
	for i := 0; i < len(prompt); i++ {
		c := prompt[i]
		end := c == '\n' || c == '!' || c == '?' || c == ';'
		if (c == '.' || c == ':') && (i+1 == len(prompt) || prompt[i+1] == ' ' || prompt[i+1] == '\t' || prompt[i+1] == '\n') {
			end = true
		}
		if end {
			if s := strings.TrimSpace(prompt[start:i]); s != "" {
				out = append(out, s)
			}
			start = i + 1
		}
	}
	if s := strings.TrimSpace(prompt[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// isLabel is whether a sentence is a heading, not a thing: a word and an
// enumerator ("Part 2", "Phase B", "Step iv"), the kind a prompt numbers
// its parts with.
func isLabel(sentence string) bool {
	words := strings.Fields(sentence)
	if len(words) != 2 {
		return false
	}
	e := strings.TrimRight(low(words[1]), ".:)")
	digits, roman := e != "", e != ""
	for _, r := range e {
		digits = digits && r >= '0' && r <= '9'
		roman = roman && strings.ContainsRune("ivx", r)
	}
	return digits || roman || len(e) == 1
}

// low is the word as the word lists hold it.
func low(w string) string { return strings.ToLower(w) }

// clauses is one sentence's phrases by class; verbs is the verb-class
// words that opened a clause or followed its modal; aside the words
// that carried a constraint clause.
type clauses struct {
	targets, additions, constraints, uses []Phrase
	verbs, aside                          []string
}

// parseSentence reads one sentence's clauses.
func parseSentence(s string) clauses {
	var c clauses
	last := ""
	all := clausesOf(s)
	for _, cl := range all {
		if len(cl) == 0 {
			continue
		}
		head := low(cl[0])
		if exclusionWords.get()[head] {
			_, cons, aside := splitExclusion(cl)
			c.constraints = addPhrases(c.constraints, cons)
			c.aside = append(c.aside, aside...)
			continue
		}
		if useWords.get()[head] {
			c.uses = addPhrase(c.uses, phraseOf(cl[1:]))
			continue
		}
		if cls := verbClass.get()[head]; cls != "" {
			last = cls
			c.verbs = append(c.verbs, head)
			rest, use := splitUse(cl[1:])
			c.uses = addPhrase(c.uses, use)
			rest, cons, aside := splitExclusion(rest)
			c.constraints = addPhrases(c.constraints, cons)
			c.aside = append(c.aside, aside...)
			if under, ok := testClause(rest); ok && cls == "add" {
				c.targets = addPhrases(c.targets, subjectOf(under))
				continue
			}
			if causativeWords.get()[head] {
				rest = rest[:bareVerbAt(rest)]
			}
			verb, rest, excluded := negatedKeep(head, rest)
			if excluded {
				c.verbs = append(c.verbs, verb)
			}
			thing, place := splitPlace(rest, cls)
			switch cls {
			case "add", "show":
				c.additions, c.targets = addPhrases(c.additions, thing), addPhrases(c.targets, place)
			case "edit":
				c.targets = addPhrases(addPhrases(c.targets, thing), place)
			case "keep":
				if excluded {
					for k := range thing {
						thing[k].Excluded = true
					}
				}
				c.constraints = addPhrases(c.constraints, thing)
			}
			continue
		}
		if last != "" && isPlace(head, last) {
			rest, use := splitUse(cl[1:])
			c.uses = addPhrase(c.uses, use)
			for _, p := range phrasesOf(rest) {
				p.Anchor = p.Anchor || (p.Definite && head == "wherever")
				c.targets = addPhrase(c.targets, p)
			}
			continue
		}
		if k := modalAt(cl); k > 0 {
			c.targets = addPhrases(c.targets, phrasesOf(cl[:k]))
			cls, verb, obj := afterModal(cl[k+1:])
			if verb != "" {
				c.verbs = append(c.verbs, verb)
			}
			switch cls {
			case "keep":
				c.constraints = addPhrase(c.constraints, obj)
			case "use":
				c.uses = addPhrase(c.uses, obj)
			case "":
			default:
				c.additions = addPhrase(c.additions, obj)
			}
			if cls != "" && cls != "use" {
				last = cls
			}
		}
	}
	return c
}

func (c clauses) empty() bool {
	return len(c.targets)+len(c.additions)+len(c.constraints)+len(c.uses)+len(c.verbs) == 0
}

// testClause is the thing under test when words open with a test ("a
// test that X", "tests for X", "a table-driven test covering X"): the
// words after the first link word following a test word in the first
// four words.
func testClause(words []string) ([]string, bool) {
	for i, w := range words {
		if i > 3 {
			break
		}
		if !testWords.get()[low(w)] {
			continue
		}
		for j := i + 1; j < len(words); j++ {
			if testLinkWords.get()[low(words[j])] {
				return words[j+1:], true
			}
		}
		return nil, true
	}
	return nil, false
}

// splitUse cuts a clause's words at their first use word: the words
// before it, and the phrase after it as what the change takes up.
func splitUse(words []string) ([]string, Phrase) {
	for i, w := range words {
		if useWords.get()[low(w)] {
			return words[:i], phraseOf(words[i+1:])
		}
	}
	return words, Phrase{}
}

// afterModal is the class, word and object of the verb a modal governs:
// the first verb-class or use word, "never" or "not" before it making it
// a keep of what the change leaves alone (the object Excluded, W-M1.35)
// unless that verb is itself keep-class ("should not leave the lock file
// behind"): the opposite, a kept thing that bars nothing.
func afterModal(words []string) (string, string, Phrase) {
	keep := false
	for i, w := range words {
		w = low(w)
		if w == "never" || w == "not" {
			keep = true
			continue
		}
		cls := verbClass.get()[w]
		if useWords.get()[w] {
			cls = "use"
		}
		if cls != "" {
			obj := phraseOf(words[i+1:])
			if keep && cls != "keep" {
				cls, obj.Excluded = "keep", true
			}
			return cls, w, obj
		}
	}
	return "", "", Phrase{}
}

// negatedKeep reads a keep-class clause whose head negates the verb after
// it ("never touch the store", "do not embed the summary"): that verb is a
// verb of the clause, as after a modal, and the thing after it is left
// alone, an exclusion. A negated keep-class verb ("do not keep the old
// lock file", "never leave the lock file behind") means the opposite, work
// on the thing, and keeps the class's shape, a kept thing that bars
// nothing (W-M1.35). The verb, the rest without it, and whether the thing
// is excluded.
func negatedKeep(head string, rest []string) (string, []string, bool) {
	i := -1
	switch {
	case head == "never":
		i = 0
	case head == "do" && len(rest) > 0 && low(rest[0]) == "not":
		i = 1
	}
	if i < 0 || i >= len(rest) || verbClass.get()[low(rest[i])] == "keep" {
		return "", rest, false
	}
	return low(rest[i]), append(append([]string{}, rest[:i]...), rest[i+1:]...), true
}

// clausesOf is the sentence's clauses: its words as written, trimmed of
// quotes and brackets, split at commas and before an "and", "then" or
// "also" that a verb or a place word follows.
func clausesOf(s string) [][]string {
	var out [][]string
	var cur []string
	close := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
	}
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, "\"'`()[]{}<>*")
		comma := strings.HasSuffix(w, ",")
		w = strings.TrimSuffix(w, ",")
		if w == "" {
			continue
		}
		cur = append(cur, w)
		if comma {
			close()
		}
	}
	close()
	var split [][]string
	for _, cl := range out {
		for len(cl) > 0 && joinWords.get()[low(cl[0])] {
			cl = cl[1:]
		}
		start := 0
		for i := 1; i+1 < len(cl); i++ {
			if joinWords.get()[low(cl[i])] && (verbClass.get()[low(cl[i+1])] != "" || placeWords.get()[low(cl[i+1])]) {
				split = append(split, cl[start:i])
				start = i + 1
			}
		}
		split = append(split, cl[start:])
	}
	return split
}

// splitPlace is the clause's object as the thing it names and the place
// it names: the words before and after the last place word ahead of the
// first clause word (a place inside "so the count shows in the line" is
// the consequence's, not the change's; one after "with" still is), each
// cut at its first boundary word.
func splitPlace(words []string, cls string) (thing, place []Phrase) {
	p := -1
	for i, w := range words {
		if conditionWords.get()[low(w)] || clauseOpeners.get()[low(w)] {
			break
		}
		if isPlace(low(w), cls) {
			p = i
		}
	}
	if p < 0 {
		return phrasesOf(words), nil
	}
	return phrasesOf(words[:p]), phrasesOf(words[p+1:])
}

// bareVerbAt is where a causative's object ends: at the first verb-class
// word after a noun phrase of two words or more that no determiner
// opens ("let the obligation ledger carry the turn window", "make the
// budgets generator skip packages"); a verb-class word after a
// determiner is a noun ("the stop line").
func bareVerbAt(words []string) int {
	for i := 2; i < len(words); i++ {
		if verbClass.get()[low(words[i])] != "" && !determinerWords.get()[low(words[i-1])] && !boundaryWords.get()[low(words[i-1])] {
			return i
		}
		if boundaryWords.get()[low(words[i])] {
			break
		}
	}
	return len(words)
}

// splitExclusion is the words before the first exclusion word; as
// constraints, the thing after it less the gerund that carries it; and
// the carrier words themselves: "without changing the log format" leaves
// the log format alone and "without changing" is aside, while a word in
// -ing with nothing after it is the thing ("without logging").
func splitExclusion(words []string) ([]string, []Phrase, []string) {
	for i, w := range words {
		if !exclusionWords.get()[low(w)] {
			continue
		}
		tail, aside := words[i+1:], words[i:i+1]
		if len(tail) > 1 && strings.HasSuffix(low(tail[0]), "ing") {
			tail, aside = tail[1:], words[i:i+2]
		}
		cons := phrasesOf(tail)
		for k := range cons {
			cons[k].Excluded = true
		}
		return words[:i], cons, aside
	}
	return words, nil, nil
}

// phrasesOf is the words as phrases: one, or at a possessive ("the seed
// counter's duplicate of the check") the owner first and the owned thing
// after it, the owner marked. The owner is the thing the words are in;
// the owned part says which of its parts.
func phrasesOf(words []string) []Phrase {
	for k, w := range words {
		if k+1 >= len(words) {
			break
		}
		if base, ok := possessive(w); ok {
			owner := append(append([]string{}, words[:k]...), base)
			first := phraseOf(owner)
			if len(first.Tokens) == 0 {
				continue
			}
			first.Owner = true
			rest := phraseOf(words[k+1:])
			rest.Definite = rest.Definite || first.Definite
			if len(rest.Tokens) == 0 {
				return []Phrase{first}
			}
			return []Phrase{first, rest}
		}
	}
	p := phraseOf(words)
	if len(p.Tokens) == 0 {
		return nil
	}
	return []Phrase{p}
}

// possessive is the word without its possessive ending, when it has one.
func possessive(w string) (string, bool) {
	for _, end := range []string{"'s", "\u2019s", "s'"} {
		if strings.HasSuffix(w, end) && len(w) > len(end)+1 {
			return strings.TrimSuffix(w, end), true
		}
	}
	return w, false
}

func isPlace(w, cls string) bool { return placeWords.get()[w] || (w == "to" && cls == "add") }

// modalAt is the index of the clause's modal when its subject precedes it.
func modalAt(cl []string) int {
	for i, w := range cl {
		if modalWords.get()[low(w)] {
			return i
		}
	}
	return -1
}

// phraseOf is the words up to their first boundary word as a phrase:
// definite when they open with a definite word or "already" follows them;
// an anchor when "already" follows a definite phrase or it opens with an
// existing-surface word; none when they open with a condition word, and
// an infinitive ("to name") or a particle ("up") ends it.
func phraseOf(words []string) Phrase {
	pronoun := ""
	for len(words) > 0 && (boundaryWords.get()[low(words[0])] || pronounWords.get()[low(words[0])]) {
		if conditionWords.get()[low(words[0])] {
			return Phrase{}
		}
		if pronounWords.get()[low(words[0])] {
			pronoun = low(words[0])
		}
		words = words[1:]
	}
	if len(words) > 0 && clauseWords.get()[low(words[0])] {
		return Phrase{}
	}
	first := 0
	for first < len(words) && stopWords.get()[low(words[first])] && !determinerWords.get()[low(words[first])] {
		first++
	}
	if first == len(words) {
		return Phrase{}
	}
	if determinerWords.get()[low(words[first])] {
		pronoun = ""
	}
	end := len(words)
	for i, w := range words {
		lw := low(w)
		if boundaryWords.get()[lw] || placeWords.get()[lw] || joinWords.get()[lw] || (lw == "to" && i+1 < len(words) && verbClass.get()[low(words[i+1])] != "") {
			end = i
			break
		}
		if i > first && determinerWords.get()[lw] && determinerWords.get()[low(words[first])] && len(Tokens(strings.Join(words[:i], " "))) > 0 {
			end = i
			break
		}
	}
	for end > 0 && particleWords.get()[low(words[end-1])] {
		end--
	}
	if end == 0 {
		return Phrase{}
	}
	p := Phrase{Tokens: uniq(Tokens(strings.Join(words[:end], " "))), Count: 1, Pronoun: pronoun}
	if len(p.Tokens) == 0 {
		return Phrase{}
	}
	if last := strings.TrimRight(low(words[end-1]), ".,:;!?'\")"); len(p.Tokens) > 1 && strings.HasSuffix(last, "ly") {
		if t := Tokens(last); len(t) == 1 && t[0] == p.Tokens[len(p.Tokens)-1] {
			p.Adverb = t[0]
		}
	}
	already := end < len(words) && low(words[end]) == "already"
	p.Definite = definiteWords.get()[low(words[first])] || already
	p.Anchor = (p.Definite && already) || anchorWords.get()[low(words[first])]
	return p
}

func addPhrase(ps []Phrase, p Phrase) []Phrase {
	if len(p.Tokens) == 0 {
		return ps
	}
	for i := range ps {
		if strings.Join(ps[i].Tokens, " ") == strings.Join(p.Tokens, " ") {
			ps[i].Definite = ps[i].Definite || p.Definite
			ps[i].Anchor = ps[i].Anchor || p.Anchor
			ps[i].Owner = ps[i].Owner || p.Owner
			return ps
		}
	}
	return append(ps, p)
}

// addPhrases merges one sentence's phrases into the frame's, counting a
// phrase once per sentence.
func addPhrases(ps, add []Phrase) []Phrase {
	for _, p := range add {
		found := false
		for i := range ps {
			if strings.Join(ps[i].Tokens, " ") == strings.Join(p.Tokens, " ") {
				ps[i].Count++
				ps[i].Definite = ps[i].Definite || p.Definite
				ps[i].Anchor = ps[i].Anchor || p.Anchor
				found = true
			}
		}
		if !found {
			ps = append(ps, p)
		}
	}
	return ps
}

// orderPhrases sorts stably by count, then definite, keeping first-named order.
func orderPhrases(ps []Phrase) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && before(ps[j], ps[j-1]); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

func before(a, b Phrase) bool {
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return a.Definite && !b.Definite
}

// property is whether the phrase names a property of a thing rather than
// the thing: its head is a property word and other words own it ("the
// spec file format"; "the format" alone owns nothing) (W-M1.34).
func (p Phrase) property() bool {
	return len(p.Tokens) > 1 && propertyWords.get()[p.Tokens[len(p.Tokens)-1]]
}

// String is the phrase's tokens with its marks, for Explain.
func (p Phrase) String() string {
	s := "[" + strings.Join(p.Tokens, " ") + "]"
	if p.Definite {
		s += " definite"
	}
	if p.Owner {
		s += " owner"
	}
	if p.Anchor {
		s += " anchor"
	}
	if p.Count > 1 {
		s += " x" + strconv.Itoa(p.Count)
	}
	return s
}
