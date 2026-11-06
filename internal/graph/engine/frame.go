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
// what the request names as existing elsewhere. The frame lives in memory
// and Explain alone prints its words (D160).

// Phrase is a run of the prompt's words as one thing it names: its tokens
// (Tokens), whether the prompt marks it as existing (the, existing,
// current, its, our, this, or "already" after it), and how many sentences
// name it.
type Phrase struct {
	Tokens   []string
	Definite bool
	Count    int
}

// Frame is what the prompt asks: the places it names as the target of the
// change, best first (named by most sentences, then definite, then first
// named), the things it adds or shows, what it keeps, what it says to use,
// and the verbs it used to say so (as tokens: what the prompt does, not a
// thing it names).
type Frame struct {
	Targets, Additions, Constraints, Uses []Phrase
	Verbs                                 []string
}

// Parse reads the frame of prompt.
func Parse(prompt string) Frame {
	var f Frame
	var verbs []string
	for _, sentence := range strings.FieldsFunc(prompt, func(r rune) bool { return strings.ContainsRune(".!?;:\n", r) }) {
		c := parseSentence(sentence)
		f.Targets = addPhrases(f.Targets, c.targets)
		f.Additions = addPhrases(f.Additions, c.additions)
		f.Constraints = addPhrases(f.Constraints, c.constraints)
		f.Uses = addPhrases(f.Uses, c.uses)
		verbs = append(verbs, c.verbs...)
	}
	orderPhrases(f.Targets)
	f.Verbs = uniq(Tokens(strings.Join(verbs, " ")))
	return f
}

// The verb classes: what a clause opening with the verb does with its
// object. An add adds the object somewhere, an edit changes the object, a
// show reports the object somewhere, a keep leaves the object as it is.
var verbClass = map[string]string{
	"add": "add", "wire": "add", "support": "add", "extend": "add", "expose": "add", "introduce": "add", "attach": "add",
	"implement": "add", "create": "add", "build": "add", "write": "add", "record": "add", "warn": "add", "emit": "add",
	"include": "add", "provide": "add", "integrate": "add", "register": "add", "install": "add", "give": "add",
	"fix": "edit", "change": "edit", "update": "edit", "refactor": "edit", "rename": "edit", "move": "edit", "remove": "edit",
	"replace": "edit", "make": "edit", "let": "edit", "ensure": "edit", "drop": "edit", "delete": "edit", "split": "edit",
	"merge": "edit", "stop": "edit", "allow": "edit", "refuse": "edit", "teach": "edit", "tighten": "edit", "clean": "edit",
	"harden": "edit", "migrate": "edit", "rewrite": "edit", "simplify": "edit", "correct": "edit", "handle": "edit",
	"use": "edit", "convert": "edit", "switch": "edit", "turn": "edit", "set": "edit", "reset": "edit", "restore": "edit",
	"read": "edit", "load": "edit", "parse": "edit", "route": "edit", "pass": "edit", "run": "edit", "check": "edit",
	"validate": "edit", "verify": "edit", "resolve": "edit", "bind": "edit", "cut": "edit", "fold": "edit", "retire": "edit",
	"report": "show", "show": "show", "print": "show", "surface": "show", "display": "show", "render": "show", "name": "show",
	"list": "show", "log": "show", "count": "show", "deliver": "show", "return": "show",
	"keep": "keep", "preserve": "keep", "leave": "keep", "maintain": "keep", "retain": "keep", "never": "keep", "do": "keep",
}

// placeWords open the phrase that names where the change lands; "to" only
// after an add (add X to Y), since elsewhere it opens an infinitive.
var placeWords = map[string]bool{
	"in": true, "into": true, "on": true, "onto": true, "under": true, "inside": true, "within": true, "at": true,
	"through": true, "across": true, "wherever": true, "where": true, "beside": true, "alongside": true, "next": true,
}

// boundaryWords end a phrase; the conditionWords among them open a
// condition (when, if, unless), so a phrase that would start with one is
// no phrase: "warn when an edit changes a file" adds no "edit changes a
// file", it says when to warn.
var boundaryWords = map[string]bool{
	"that": true, "which": true, "when": true, "whenever": true, "already": true, "so": true, "because": true,
	"unless": true, "until": true, "if": true, "with": true, "using": true, "as": true, "not": true, "but": true,
	"before": true, "after": true, "instead": true, "rather": true, "once": true, "while": true,
}

var conditionWords = map[string]bool{
	"when": true, "whenever": true, "if": true, "unless": true, "until": true, "once": true, "while": true,
	"because": true, "before": true, "after": true,
}

// joinWords join clauses; pronounWords stand for a thing another clause names.
var joinWords = map[string]bool{"and": true, "then": true, "also": true, "or": true}

var pronounWords = map[string]bool{"it": true, "them": true, "they": true, "these": true, "those": true, "which": true, "what": true}

// useWords open the phrase naming what the change takes up: the thing to
// reuse, which the request presumes exists.
var useWords = map[string]bool{
	"respect": true, "respecting": true, "honor": true, "honour": true, "honoring": true, "honouring": true,
	"consult": true, "consulting": true, "obey": true, "reuse": true, "reusing": true, "leverage": true,
	"via": true, "using": true, "invoke": true, "invoking": true, "follow": true, "following": true,
}

var modalWords = map[string]bool{"should": true, "must": true, "shall": true, "needs": true, "has": true, "ought": true}

var definiteWords = map[string]bool{"the": true, "existing": true, "current": true, "its": true, "our": true, "this": true, "these": true, "those": true}

// clauses is one sentence's phrases by class; verbs is the verb-class
// words that opened a clause or followed its modal.
type clauses struct {
	targets, additions, constraints, uses []Phrase
	verbs                                 []string
}

// parseSentence reads one sentence's clauses.
func parseSentence(s string) clauses {
	var c clauses
	last := ""
	for _, cl := range clausesOf(s) {
		if len(cl) == 0 {
			continue
		}
		if useWords[cl[0]] {
			c.uses = addPhrase(c.uses, phraseOf(cl[1:]))
			continue
		}
		if cls := verbClass[cl[0]]; cls != "" {
			last = cls
			c.verbs = append(c.verbs, cl[0])
			rest, use := splitUse(cl[1:])
			c.uses = addPhrase(c.uses, use)
			thing, place := splitPlace(rest, cls)
			switch cls {
			case "add", "show":
				c.additions, c.targets = addPhrase(c.additions, thing), addPhrase(c.targets, place)
			case "edit":
				c.targets = addPhrase(addPhrase(c.targets, thing), place)
			case "keep":
				c.constraints = addPhrase(c.constraints, thing)
			}
			continue
		}
		if last != "" && isPlace(cl[0], last) {
			rest, use := splitUse(cl[1:])
			c.uses = addPhrase(c.uses, use)
			c.targets = addPhrase(c.targets, phraseOf(rest))
			continue
		}
		if k := modalAt(cl); k > 0 {
			c.targets = addPhrase(c.targets, phraseOf(cl[:k]))
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

// splitUse cuts a clause's words at their first use word: the words
// before it, and the phrase after it as what the change takes up.
func splitUse(words []string) ([]string, Phrase) {
	for i, w := range words {
		if useWords[w] {
			return words[:i], phraseOf(words[i+1:])
		}
	}
	return words, Phrase{}
}

// afterModal is the class, word and object of the verb a modal governs:
// the first verb-class or use word, "never" or "not" before it making it
// a keep.
func afterModal(words []string) (string, string, Phrase) {
	keep := false
	for i, w := range words {
		if w == "never" || w == "not" {
			keep = true
			continue
		}
		cls := verbClass[w]
		if useWords[w] {
			cls = "use"
		}
		if cls != "" {
			if keep {
				cls = "keep"
			}
			return cls, w, phraseOf(words[i+1:])
		}
	}
	return "", "", Phrase{}
}

// clausesOf is the sentence's clauses: its words, lower case, trimmed of
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
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, "\"'`()[]{}<>")
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
		for len(cl) > 0 && joinWords[cl[0]] {
			cl = cl[1:]
		}
		start := 0
		for i := 1; i+1 < len(cl); i++ {
			if joinWords[cl[i]] && (verbClass[cl[i+1]] != "" || placeWords[cl[i+1]]) {
				split = append(split, cl[start:i])
				start = i + 1
			}
		}
		split = append(split, cl[start:])
	}
	return split
}

// splitPlace is the clause's object as the thing it names and the place
// it names: the words before and after the last place word, each cut at
// its first boundary word.
func splitPlace(words []string, cls string) (thing, place Phrase) {
	p := -1
	for i, w := range words {
		if isPlace(w, cls) {
			p = i
		}
	}
	if p < 0 {
		return phraseOf(words), Phrase{}
	}
	return phraseOf(words[:p]), phraseOf(words[p+1:])
}

func isPlace(w, cls string) bool { return placeWords[w] || (w == "to" && cls == "add") }

// modalAt is the index of the clause's modal when its subject precedes it.
func modalAt(cl []string) int {
	for i, w := range cl {
		if modalWords[w] {
			return i
		}
	}
	return -1
}

// phraseOf is the words up to their first boundary word as a phrase:
// definite when they open with a definite word or "already" follows them;
// none when they open with a condition word.
func phraseOf(words []string) Phrase {
	for len(words) > 0 && (boundaryWords[words[0]] || pronounWords[words[0]]) {
		if conditionWords[words[0]] {
			return Phrase{}
		}
		words = words[1:]
	}
	end := len(words)
	for i, w := range words {
		if boundaryWords[w] || placeWords[w] || joinWords[w] {
			end = i
			break
		}
	}
	if end == 0 {
		return Phrase{}
	}
	p := Phrase{Tokens: uniq(Tokens(strings.Join(words[:end], " "))), Count: 1}
	if len(p.Tokens) == 0 {
		return Phrase{}
	}
	p.Definite = definiteWords[words[0]] || (end < len(words) && words[end] == "already")
	return p
}

func addPhrase(ps []Phrase, p Phrase) []Phrase {
	if len(p.Tokens) == 0 {
		return ps
	}
	for i := range ps {
		if strings.Join(ps[i].Tokens, " ") == strings.Join(p.Tokens, " ") {
			ps[i].Definite = ps[i].Definite || p.Definite
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

// String is the phrase's tokens with its marks, for Explain.
func (p Phrase) String() string {
	s := "[" + strings.Join(p.Tokens, " ") + "]"
	if p.Definite {
		s += " definite"
	}
	if p.Count > 1 {
		s += " x" + strconv.Itoa(p.Count)
	}
	return s
}
