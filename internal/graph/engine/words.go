package engine

// The frame's word classes (W-M1.12): the lists Parse reads a sentence
// by, data the rules in frame.go consult. A word is in one class by what
// it does in a request, not by what it is: "test" opens a test clause,
// "to" is a place only after an add. The lists are read in lower case.

// The verb classes: what a clause opening with the verb does with its
// object. An add adds the object somewhere, an edit changes the object, a
// show reports the object somewhere, a keep leaves the object as it is.
var verbClass = &lazySet[string]{f: func() map[string]string {
	return map[string]string{
		"say": "show", "mention": "show", "tell": "show", "carry": "show", "yield": "show",
		"produce": "show", "note": "show", "reflect": "show", "decide": "edit", "hold": "edit",
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
		"reduce": "edit", "lower": "edit", "shrink": "edit", "speed": "edit", "trim": "edit", "optimise": "edit", "optimize": "edit",
		"improve": "edit", "cache": "edit", "bump": "edit", "upgrade": "edit", "downgrade": "edit", "pin": "edit", "unpin": "edit",
		"enable": "edit", "disable": "edit", "guard": "edit", "cap": "edit", "limit": "edit", "bound": "edit", "skip": "edit",
		"audit": "edit", "review": "edit", "land": "edit", "rebase": "edit", "port": "edit", "extract": "edit", "inline": "edit",
		"hoist": "edit", "dedupe": "edit", "deduplicate": "edit", "unify": "edit", "consolidate": "edit", "wrap": "edit",
		"unwrap": "edit", "patch": "edit", "tune": "edit", "align": "edit", "prevent": "edit", "gate": "edit", "enforce": "edit",
		"require": "edit", "stub": "edit", "mock": "edit", "retry": "edit", "batch": "edit", "throttle": "edit", "sort": "edit",
		"filter": "edit", "dedup": "edit", "rework": "edit", "redo": "edit", "address": "edit", "investigate": "edit",
		"debug": "edit", "diagnose": "edit", "trace": "edit", "profile": "edit", "measure": "edit", "benchmark": "edit",
		"test": "edit", "cover": "edit", "document": "edit", "describe": "edit", "explain": "edit", "summarize": "edit",
		"summarise": "edit", "inspect": "edit", "examine": "edit", "revisit": "edit", "tidy": "edit", "finish": "edit",
		"complete": "edit", "continue": "edit", "resume": "edit", "repair": "edit", "restructure": "edit", "reorganize": "edit",
		"swap": "edit", "relocate": "edit",
		"report": "show", "show": "show", "print": "show", "surface": "show", "display": "show", "render": "show", "name": "show",
		"list": "show", "log": "show", "count": "show", "deliver": "show", "return": "show",
		"keep": "keep", "preserve": "keep", "leave": "keep", "maintain": "keep", "retain": "keep", "never": "keep", "do": "keep",
	}
}}

// destinationWords are the closed list of constructions whose last phrase
// names the existing thing the change takes its object onto (W-M1.28):
// replace or swap X with Y; move, migrate, port or relocate X onto or to
// Y; switch or convert X to use or read Y; consolidate, fold or merge X
// into Y; route X through Y. Each verb maps to the words that open its
// destination. The list is the construction, written before any reading
// of the development sets, and no word joins it to fit a row.
var destinationWords = &lazySet[[][]string]{f: func() map[string][][]string {
	with, onto, to, into := [][]string{{"with"}}, [][]string{{"onto"}, {"to"}}, [][]string{{"to", "use"}, {"to", "read"}}, [][]string{{"into"}}
	return map[string][][]string{
		"replace": with, "swap": with, "move": onto, "migrate": onto, "port": onto, "relocate": onto,
		"switch": to, "convert": to, "consolidate": into, "fold": into, "merge": into, "route": {{"through"}},
	}
}}

// placeWords open the phrase that names where the change lands; "to" only
// after an add (add X to Y), since elsewhere it opens an infinitive.
var placeWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"in": true, "into": true, "on": true, "onto": true, "under": true, "inside": true, "within": true, "at": true,
		"through": true, "across": true, "wherever": true, "where": true, "beside": true, "alongside": true, "next": true,
	}
}}

// boundaryWords end a phrase; the conditionWords among them open a
// condition (when, if, unless), so a phrase that would start with one is
// no phrase: "warn when an edit changes a file" adds no "edit changes a
// file", it says when to warn.
var boundaryWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"that": true, "which": true, "when": true, "whenever": true, "already": true, "so": true, "because": true,
		"unless": true, "until": true, "if": true, "with": true, "using": true, "as": true, "not": true, "but": true,
		"before": true, "after": true, "instead": true, "rather": true, "once": true, "while": true, "covering": true,
		"including": true, "excluding": true, "regarding": true, "about": true, "without": true, "whose": true, "than": true,
		"whether": true,
	}
}}

var conditionWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"when": true, "whenever": true, "if": true, "unless": true, "until": true, "once": true, "while": true,
		"because": true, "before": true, "after": true, "where": true, "how": true, "whether": true,
	}
}}

// joinWords join clauses; pronounWords stand for a thing another clause names.
var joinWords = &lazySet[bool]{f: func() map[string]bool { return map[string]bool{"and": true, "then": true, "also": true, "or": true} }}

var pronounWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"it": true, "them": true, "these": true, "those": true, "which": true, "what": true}
}}

// propertyWords head a constraint that preserves a property of the thing
// the work is on ("without changing the spec file format": the format
// stays, the work is spec's), so the phrase's other words name where the
// work is. A constraint headed by any other word names the thing left
// alone. The stems of format, schema, output, layout, contract,
// interface, api, behaviour, behavior, signature and encoding, written
// as Tokens makes them and held to it by test (an initializer makes no
// call, RES-2) (W-M1.34, O3).
var propertyWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"format": true, "schema": true, "output": true, "layout": true, "contract": true, "interfac": true,
		"api": true, "behaviour": true, "behavior": true, "signatur": true, "encod": true,
	}
}}

// exclusionWords open what a change leaves alone ("without changing the
// log format", "except the hook tests"): a constraint, at the sentence's
// head or after the object (W-M1.33, O7).
var exclusionWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"without": true, "except": true, "excluding": true}
}}

// useWords open the phrase naming what the change takes up: the thing to
// reuse, which the request presumes exists.
var useWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"respect": true, "respecting": true, "honor": true, "honour": true, "honoring": true, "honouring": true,
		"consult": true, "consulting": true, "obey": true, "reuse": true, "reusing": true, "leverage": true,
		"via": true, "using": true, "invoke": true, "invoking": true, "follow": true, "following": true,
	}
}}

var modalWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"should": true, "must": true, "shall": true, "needs": true, "has": true, "ought": true}
}}

var definiteWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"the": true, "existing": true, "current": true, "its": true, "our": true, "this": true, "these": true, "those": true}
}}

// anchorWords mark a definite phrase as the surface the change lands on.
var anchorWords = &lazySet[bool]{f: func() map[string]bool { return map[string]bool{"existing": true, "current": true} }}

// questionWords open a question; the words after them (and an auxiliary)
// are its subject. particleWords are verb particles ("clean up"), no
// thing of their own. testWords open a test clause.
var questionWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"how": true, "what": true, "where": true, "why": true, "who": true, "which": true, "when": true, "does": true, "is": true, "are": true, "can": true, "could": true, "will": true, "would": true, "did": true}
}}

var auxiliaryWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"does": true, "do": true, "did": true, "is": true, "are": true, "was": true, "were": true, "can": true, "could": true, "should": true, "would": true, "will": true}
}}

var particleWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"up": true, "off": true, "out": true, "down": true, "back": true, "away": true, "over": true}
}}

var testWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"test": true, "tests": true, "coverage": true, "benchmark": true, "benchmarks": true}
}}

// testLinkWords join a test clause to the thing under test.
var testLinkWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"for": true, "that": true, "of": true, "covering": true, "over": true, "on": true, "against": true, "proving": true, "showing": true}
}}

// determinerWords open a noun phrase; a second one inside a phrase that
// opened with one opens another ("the digest carry the turn's time" ends
// at the second "the"); a phrase that opened without one is a verb
// phrase and stays whole ("purging the sessions").
var determinerWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"the": true, "a": true, "an": true, "its": true, "their": true, "our": true, "this": true, "these": true, "those": true,
		"every": true, "each": true, "any": true, "some": true, "no": true, "zero": true,
	}
}}

// objectWords open a verb's object: determiners and object pronouns.
var objectWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"the": true, "a": true, "an": true, "its": true, "their": true, "our": true, "this": true, "these": true, "those": true,
		"each": true, "every": true, "no": true, "all": true, "any": true, "some": true, "zero": true, "it": true, "them": true,
		"us": true, "me": true, "one": true, "two": true, "three": true, "more": true, "fewer": true, "nothing": true,
	}
}}

// clauseOpeners open a clause about the thing, not a place of the change.
var clauseOpeners = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"so": true, "that": true, "which": true, "whose": true, "instead": true, "rather": true}
}}

// causativeWords take an object and a bare verb for what it should do.
var causativeWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"make": true, "let": true, "have": true, "ensure": true, "force": true, "allow": true, "teach": true, "help": true, "keep": true, "get": true, "stop": true}
}}

// clauseWords open a clause, not a thing: a subject pronoun ("where we
// left off" names no code).
var clauseWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"we": true, "i": true, "you": true, "he": true, "she": true, "they": true, "someone": true, "anyone": true, "everyone": true, "nobody": true}
}}
