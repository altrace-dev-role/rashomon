package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ovMaxList is the most names a list of the overview prints before "and N
// more"; ovLabelCol is the column where a knowledge or activity row's text
// starts.
const (
	ovMaxList  = 8
	ovLabelCol = 19
)

// ovLen is the columns s takes: its runes, since the view's glyphs and rules
// are one column each and more than one byte.
func ovLen(s string) int { return utf8.RuneCountInString(s) }

// ovPad is n spaces, or none when n is not positive.
func ovPad(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// ovShort is the first eight printable runes of an id.
func ovShort(id string) string {
	r := []rune(printable(id))
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// yIes ends "famil" for n families.
func yIes(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func namesName(n int) string {
	if n == 1 {
		return "names"
	}
	return "name"
}

func itThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

var ovWords = [...]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
	"eleven", "twelve"}

// ovWord is n as a word up to twelve and in digits above.
func ovWord(n int) string {
	if n >= 0 && n < len(ovWords) {
		return ovWords[n]
	}
	return strconv.Itoa(n)
}

// ovCap is s with its first rune upper case.
func ovCap(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// ovAnd is "a", "a and b" or "a, b and c".
func ovAnd(xs []string) string {
	if len(xs) < 2 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

// ovWrap breaks s at spaces into lines of at most width columns; a word
// longer than width keeps a line of its own.
func ovWrap(s string, width int) []string { return ovFill(strings.Fields(s), width) }

// ovItems is a list of items joined by " · ", broken only between items,
// with the dot kept at the end of the line it closes.
func ovItems(items []string, width int) []string {
	toks := make([]string, len(items))
	for i, it := range items {
		toks[i] = it
		if i < len(items)-1 {
			toks[i] += " ·"
		}
	}
	return ovFill(toks, width)
}

// ovFill joins tokens by single spaces into lines of at most width columns.
func ovFill(toks []string, width int) []string {
	var lines []string
	cur := ""
	for _, t := range toks {
		switch {
		case cur == "":
			cur = t
		case ovLen(cur)+1+ovLen(t) <= width:
			cur += " " + t
		default:
			lines = append(lines, cur)
			cur = t
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// ovSeqs is "#a, #b" for the seqs.
func ovSeqs(seqs []int64) string {
	parts := make([]string, len(seqs))
	for i, s := range seqs {
		parts[i] = "#" + strconv.FormatInt(s, 10)
	}
	return strings.Join(parts, ", ")
}

// ovIDs is the printable ids joined by ", ", at most ovMaxList of them.
func ovIDs(ids []string) string {
	var parts []string
	for i, id := range ids {
		if i == ovMaxList {
			parts = append(parts, fmt.Sprintf("and %d more", len(ids)-ovMaxList))
			break
		}
		parts = append(parts, printable(id))
	}
	return strings.Join(parts, ", ")
}

// ovNotes is "N notes: " and each title with its count, in the order first
// sent.
func ovNotes(ds []AutosteerNote) string {
	var titles []string
	count := map[string]int{}
	for _, d := range ds {
		t := printable(d.Title)
		if t == "" {
			t = printable(d.Pattern)
		}
		if count[t] == 0 {
			titles = append(titles, t)
		}
		count[t]++
	}
	parts := make([]string, len(titles))
	for i, t := range titles {
		parts[i] = fmt.Sprintf("%d %s", count[t], pluralTitle(t, count[t]))
	}
	return fmt.Sprintf("%d note%s: %s", len(ds), plural(len(ds)), ovAnd(parts))
}

// ovNoteSeqs is the seqs the notes were sent at, or "".
func ovNoteSeqs(ds []AutosteerNote) string {
	var seqs []int64
	for _, d := range ds {
		if d.Seq > 0 {
			seqs = append(seqs, d.Seq)
		}
	}
	return ovSeqs(seqs)
}

// ovImprovement is the improvement row: always unknown, with the mistakes
// recorded as caught and, when read, those found later. A nil block is not
// read, never zero.
func ovImprovement(m *AutosteerMistakes) string {
	if m == nil {
		return "Unknown. The mistake counts were not read here."
	}
	n := len(m.Caught)
	s := "Unknown. 0 mistakes were recorded as caught, which does not mean none occurred"
	if n > 0 {
		s = fmt.Sprintf("Unknown. %d mistake%s %s recorded as caught%s", n, plural(n), wasWere(n), ovCallsAt(m.Caught))
	}
	if m.Later != nil {
		k := len(m.Later)
		s += fmt.Sprintf("; %d %s found later%s", k, wasWere(k), ovCallsAt(m.Later))
	}
	if n > 0 {
		return s + "; no record measures the final result."
	}
	return s + "."
}

// ovCallsAt is " (call #a)" or " (calls #a, #b)" for the mistakes' seqs,
// or "" when none has one.
func ovCallsAt(ms []AutosteerMistake) string {
	var seqs []int64
	for _, m := range ms {
		if m.Seq > 0 {
			seqs = append(seqs, m.Seq)
		}
	}
	switch len(seqs) {
	case 0:
		return ""
	case 1:
		return " (call " + ovSeqs(seqs) + ")"
	}
	return " (calls " + ovSeqs(seqs) + ")"
}

// ovCount is one name of a count list.
type ovCount struct {
	name string
	n    int
}

// ovSorted is m with each key through name, keys that print alike summed,
// the largest count first and then by name.
func ovSorted(m map[string]int, name func(string) string) []ovCount {
	sum := map[string]int{}
	for k, n := range m {
		sum[name(k)] += n
	}
	cs := make([]ovCount, 0, len(sum))
	for k, n := range sum {
		cs = append(cs, ovCount{k, n})
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].n != cs[j].n {
			return cs[i].n > cs[j].n
		}
		return cs[i].name < cs[j].name
	})
	return cs
}

// ovNameCounts is each count as "name n", at most ovMaxList, then "and k
// more".
func ovNameCounts(cs []ovCount) []string {
	var out []string
	for i, c := range cs {
		if i == ovMaxList {
			out = append(out, fmt.Sprintf("and %d more", len(cs)-ovMaxList))
			break
		}
		out = append(out, fmt.Sprintf("%s %d", c.name, c.n))
	}
	return out
}

// krow is a knowledge or activity row: the label, its text wrapped from
// column 19.
func (v *ovView) krow(label, text string) { v.klines(label, ovWrap(text, ovWidth-ovLabelCol)) }

// krows is a row of items joined by " · ", or "none".
func (v *ovView) krows(label string, items []string) {
	if len(items) == 0 {
		items = []string{"none"}
	}
	v.klines(label, ovItems(items, ovWidth-ovLabelCol))
}

func (v *ovView) klines(label string, lines []string) {
	for i, l := range lines {
		if i == 0 {
			v.line(v.paint(ovDim, label) + ovPad(max(ovLabelCol-ovLen(label), 1)) + l)
			continue
		}
		v.line(ovPad(ovLabelCol) + l)
	}
}
