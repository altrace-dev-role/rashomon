package engine

import "strings"

// The question's subject: what a verbless or question clause is about,
// moved from frame.go whole (W-M1.33, LAW10 file ceiling).

// subjectOf is the thing a verbless clause names: a symptom's subject
// ("the report prints a row twice when" names the report printing), a
// question's after its question word and auxiliary ("where is the
// deadline enforced" names the deadline), and when the question word is
// the subject itself ("what decides whether a lesson is proven", "who
// calls the installer") the verb's complement (W-M1.32, H7).
func subjectOf(cl []string) []Phrase {
	question := len(cl) > 0 && questionWords.get()[low(cl[0])]
	if question {
		alone := low(cl[0]) == "what" || low(cl[0]) == "who"
		cl = cl[1:]
		if len(cl) > 0 && auxiliaryWords.get()[low(cl[0])] {
			cl, alone = cl[1:], false
		}
		if alone && len(cl) > 1 && thirdPerson(cl[0], cl[1]) {
			return phrasesOf(complement(cl[1:]))
		}
	}
	return phrasesOf(cl[:predicateAt(cl, question)])
}

// thirdPerson is whether w, with next after it, is a verb in the third
// person: a known verb's -s form ("decides"), or an -s word an object or
// a clause follows ("calls the", "owns whether").
func thirdPerson(w, next string) bool {
	lw, ln := low(w), low(next)
	if !strings.HasSuffix(lw, "s") || strings.HasSuffix(lw, "ss") {
		return false
	}
	return verbClass.get()[singular(lw)] != "" || objectWords.get()[ln] || conditionWords.get()[ln] || ln == "that"
}

// complement is what a verb is about, as the thing: its words less a
// clause opener (whether, if, that) and the clause's own auxiliary
// ("whether a lesson is proven" is the lesson proven).
func complement(words []string) []string {
	if len(words) > 0 && (conditionWords.get()[low(words[0])] || low(words[0]) == "that") {
		words = words[1:]
	}
	out := make([]string, 0, len(words))
	for _, w := range words {
		if !auxiliaryWords.get()[low(w)] {
			out = append(out, w)
		}
	}
	return out
}

// predicateAt is where a clause's subject ends: at an auxiliary or a
// modal ("is over budget", "should report"), at a word in the third
// person ("records", "counts", "drops") that a determiner or a pronoun
// follows ("the panic barrier records the panic value") or that is a
// verb of a known class with words after it ("the budget store drops
// rows"), and in a question at the bare verb after the subject ("how
// does the harness decide"), the subject being at least one word. A
// clause with none is its subject whole.
func predicateAt(cl []string, question bool) int {
	for i, w := range cl {
		lw := low(w)
		if i == 0 {
			continue
		}
		if auxiliaryWords.get()[lw] || modalWords.get()[lw] || question && verbClass.get()[lw] != "" && !determinerWords.get()[low(cl[i-1])] {
			return i
		}
		third := i+1 < len(cl) && strings.HasSuffix(lw, "s") && !strings.HasSuffix(lw, "ss")
		if third && (objectWords.get()[low(cl[i+1])] || verbClass.get()[singular(lw)] != "") {
			return i
		}
	}
	return len(cl)
}

// singular is a third-person verb's base form: reads, drops, fixes,
// carries.
func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ches"), strings.HasSuffix(w, "shes"), strings.HasSuffix(w, "xes"), strings.HasSuffix(w, "sses"):
		return w[:len(w)-2]
	}
	return strings.TrimSuffix(w, "s")
}
