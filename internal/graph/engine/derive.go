package engine

import "strings"

// derive reads the target tokens the index has no name for through the
// names it has (S2, W-M1.14): a token one index name token opens (installer
// through install, canonicalisation through canonical) is that name, the
// shorter of the two at least four letters and the other at least five,
// and the longest such name wins; a token that opens an index name
// (classif through classifier) is that name when it is five letters or
// more. Shorter prefixes are coincidence (scan and scanner share four
// letters, so do stop and store), and so is a compound: what the longer
// word adds must be an inflection (installer, generator, classif beside
// classifi), not a word (workflow through work, leftmost through left).
// The readings are returned for disclosure.
func (c corpus) derive(target []string, into map[string]string) ([]string, map[string]string) {
	out := make([]string, 0, len(target))
	for _, t := range target {
		if c.nameDF[t] > 0 || len(t) < 4 {
			out = append(out, t)
			continue
		}
		if d, ok := into[t]; ok {
			out = append(out, d)
			continue
		}
		best := ""
		for v := range c.nameDF {
			if len(v) < 4 || v == t {
				continue
			}
			opens := strings.HasPrefix(t, v) && len(t) >= 5 && inflects(t[len(v):])
			opened := strings.HasPrefix(v, t) && len(t) >= 5 && inflects(v[len(t):])
			if !opens && !opened {
				continue
			}
			if best == "" || derivedBefore(t, v, best) {
				best = v
			}
		}
		if best == "" {
			out = append(out, t)
			continue
		}
		if into == nil {
			into = map[string]string{}
		}
		into[t] = best
		out = append(out, best)
	}
	return out, into
}

// inflects is whether what the longer word adds to the shorter is an
// inflection: a suffix word, or a tail of one or two letters the
// tokenizer's stemming leaves (classif beside classifi). Anything longer
// outside the list is another word.
func inflects(rest string) bool { return len(rest) <= 2 || suffixWords.get()[rest] }

// suffixWords is what an inflected or derived form adds to its stem, as
// the tokenizer leaves it: the agent (installer, generator), the act
// (isation, ment), the quality (ness, ity), the participle and the
// adjective. A remainder outside the list is another word.
var suffixWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"er": true, "or": true, "ers": true, "ors": true, "ier": true, "iers": true, "ar": true, "ant": true, "ent": true,
		"ance": true, "ence": true, "ion": true, "ions": true, "ation": true, "ations": true, "isation": true, "ization": true,
		"ment": true, "ments": true, "ness": true, "ist": true, "ists": true, "ity": true, "al": true, "ive": true, "ic": true,
		"ful": true, "less": true, "able": true, "ible": true, "ing": true, "ed": true, "ly": true, "s": true, "es": true, "ies": true,
		"ise": true, "ize": true, "ised": true, "ized": true, "ising": true, "izing": true, "iser": true, "izer": true, "isers": true, "izers": true,
	}
}}

// weighDerived gives each derived name the weight the scorer would give
// it as a prompt word (its idf, or one), in a copy of weights: the
// prompt's own word weighs nothing, being in no name, and a name that
// stands in for it must weigh what it names or the ladder reads it as
// covered for free.
func (c corpus) weighDerived(weights map[string]float64, derived map[string]string) map[string]float64 {
	var out map[string]float64
	for _, d := range derived {
		if weights[d] > 0 {
			continue
		}
		if out == nil {
			out = make(map[string]float64, len(weights)+len(derived))
			for k, v := range weights {
				out[k] = v
			}
			weights = out
		}
		weights[d] = c.weights([]string{d}, c.idf)[d]
	}
	return weights
}

// derivedBefore orders two readings of t: a name t opens before a name
// that opens t; among names that open t the longer; among names t opens
// the shorter; then the first by letter.
func derivedBefore(t, a, b string) bool {
	ao, bo := strings.HasPrefix(t, a), strings.HasPrefix(t, b)
	if ao != bo {
		return ao
	}
	if len(a) != len(b) {
		if ao {
			return len(a) > len(b)
		}
		return len(a) < len(b)
	}
	return a < b
}
