package engine

import "strings"

// The sentence's clauses (W-M1.28): the words a clause rule reads, cut at
// commas and joins, where a coordinated list's commas cut nothing. Split
// from frame.go at LAW10's file ceiling.

// clausesOf is the sentence's clauses: its words as written, trimmed of
// quotes and brackets, split at commas and before an "and", "then" or
// "also" that a verb or a place word follows. A coordinated list ("in
// report, seed and spend") is one constituent, so its commas end no
// clause: the clause keeps the list's first item, as a phrase keeps the
// first of two things an "and" joins, and goes on after its last item
// ("Replace the readers in report, seed and spend with the scanner"
// keeps "with the scanner" in the verb's clause) (W-M1.28).
func clausesOf(s string) [][]string {
	var ws []string
	var comma []bool
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, "\"'`()[]{}<>*")
		c := strings.HasSuffix(w, ",")
		w = strings.TrimSuffix(w, ",")
		if w == "" {
			continue
		}
		ws, comma = append(ws, w), append(comma, c)
	}
	var out [][]string
	var cur []string
	close := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
	}
	for i := 0; i < len(ws); i++ {
		cur = append(cur, ws[i])
		if !comma[i] {
			continue
		}
		if end := listEnd(ws, comma, i+1, false); end > 0 {
			if i = end - 1; !comma[i] {
				continue
			}
		}
		close()
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

// listEnd is the end of a coordinated list's later items when the comma
// before ws[i] parts its items, or 0 when that comma ends a clause: items
// closed by commas, then "and" or "or" and the last item, or the last
// item after the "and" of an Oxford comma once an item closed by a comma
// came before it. The last item stands alone: the sentence ends, a comma
// closes it or a word that is no item word follows it ("spend with the
// scanner"). "update report, then fix the seed test" and "for every
// symbol an edit touches, report two numbers" are two clauses each.
func listEnd(ws []string, comma []bool, i int, inList bool) int {
	if i < len(ws) && listJoin(ws[i]) {
		if !inList {
			return 0
		}
		return lastItem(ws, comma, i+1)
	}
	j := itemAt(ws, comma, i)
	switch {
	case j == 0:
		return 0
	case comma[j-1]:
		return listEnd(ws, comma, j, true)
	case j < len(ws) && listJoin(ws[j]):
		return lastItem(ws, comma, j+1)
	}
	return 0
}

// lastItem is the end of the list's last item at ws[i], or 0 when none
// stands alone there.
func lastItem(ws []string, comma []bool, i int) int {
	j := itemAt(ws, comma, i)
	if j == 0 || j < len(ws) && !comma[j-1] && itemWord(ws[j], false) {
		return 0
	}
	return j
}

// itemAt is the end of the list item at ws[i], or 0 when none opens
// there: an optional determiner, then one or two item words, ending at a
// comma. A verb-class word is an item word only after the determiner
// ("the log, the trace and the dump"); "on failure, log and retry" is
// three clauses. A phrase after an item attaches by its preposition
// (W-M1.28): "of" makes the noun's complement, so the item after it is
// this item's own ("the handling of empty files" is one item, and no
// clause goes on with "of empty files"), and an item whose complement
// reads as no item opens none. A destination's preposition after the
// last item is the verb's, so the clause goes on with it ("the lexer and
// the handling of empty files with the line scanner"), as lastItem reads.
func itemAt(ws []string, comma []bool, i int) int {
	det := i < len(ws) && !comma[i] && determinerWords.get()[low(ws[i])]
	j := i
	if det {
		j++
	}
	for n := 0; n < 2 && j < len(ws) && itemWord(ws[j], det && n == 0); n++ {
		if j++; comma[j-1] {
			break
		}
	}
	if j == i || det && j == i+1 {
		return 0
	}
	if j < len(ws) && !comma[j-1] && low(ws[j]) == "of" {
		return itemAt(ws, comma, j+1)
	}
	return j
}

// itemWord is whether w can be a word of a list item: it makes a token
// and is in no word class the clause rules read (a verb-class word only
// as a noun).
func itemWord(w string, noun bool) bool {
	lw := low(w)
	if len(Tokens(w)) == 0 || verbClass.get()[lw] != "" && !noun {
		return false
	}
	for _, class := range [...]*lazySet[bool]{placeWords, boundaryWords, conditionWords, joinWords, pronounWords, useWords,
		modalWords, exclusionWords, clauseWords, determinerWords, particleWords, auxiliaryWords, questionWords, causativeWords} {
		if class.get()[lw] {
			return false
		}
	}
	return true
}

// listJoin is whether w joins a list's last item: "and" or "or".
func listJoin(w string) bool { return low(w) == "and" || low(w) == "or" }
