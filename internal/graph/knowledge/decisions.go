package knowledge

import "strings"

// Decision lines are a notes file's "- D161 (when, who): text" entries,
// wrapped onto the lines after them, and its "## D66 (when, who): title"
// headings followed by a paragraph. A word after the id (D161 amended,
// D149 addendum) keys the entry on its own (D161-amended).

// ParseDecisions is the decisions of a notes file: a dash entry's first
// sentence is its clause and the whole entry its text; a heading's text is
// its title and clause and the paragraph under it its text (a heading that
// ends at its parenthesis takes the paragraph's first sentence). The count
// of unparsed lines is always zero: a line that is not a decision is prose.
func ParseDecisions(file string, text []byte) ([]Row, int) {
	lines := strings.Split(string(text), "\n")
	var rows []Row
	seen := map[string]int{}
	for i := 0; i < len(lines); i++ {
		if id, rest, ok := decisionAt(lines[i], "- D"); ok && rest != "" {
			body := rest
			for i+1 < len(lines) && continues(lines[i+1]) {
				i++
				body += " " + strings.TrimSpace(lines[i])
			}
			rows = append(rows, decision(file, unique(seen, id), id, firstSentence(body), body))
			continue
		}
		if id, rest, ok := decisionAt(lines[i], "## D"); ok {
			para := paragraphAfter(lines, i+1)
			title := rest
			if title == "" {
				title = firstSentence(para)
			}
			if para == "" {
				para = rest
			}
			if title != "" {
				rows = append(rows, decision(file, unique(seen, id), id, title, para))
			}
		}
	}
	return rows, 0
}

func decision(file, key, id, title, text string) Row {
	return Row{ID: key, Kind: KindDecision, Title: title, Clause: title, Text: text, Source: Source{File: file, Row: id}, Status: StatusCandidate, Tier: TierInferred}
}

// decisionAt reads "<prefix><digits>[ words] (<when>)[: <rest>]" and
// returns the id (D161, or D161-amended with the words) and the rest, ""
// when the parenthesis ends the line.
func decisionAt(line, prefix string) (id, rest string, ok bool) {
	s, ok := strings.CutPrefix(line, prefix)
	if !ok {
		return "", "", false
	}
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	if n == 0 {
		return "", "", false
	}
	id = "D" + s[:n]
	before, after, ok := strings.Cut(s[n:], "(")
	if !ok {
		return "", "", false
	}
	if words := strings.Fields(before); len(words) > 0 {
		id += "-" + strings.ToLower(strings.Join(words, "-"))
	}
	if _, rest, ok = strings.Cut(after, "):"); ok {
		return id, strings.TrimSpace(rest), true
	}
	if i := strings.LastIndexByte(after, ')'); i >= 0 && strings.TrimSpace(after[i+1:]) == "" {
		return id, "", true
	}
	return "", "", false
}

// continues is a line that wraps the entry before it: not blank, not a
// new entry, not a heading.
func continues(line string) bool {
	return strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "#")
}

// paragraphAfter is the first paragraph at or after lines[i], its lines
// joined by a space, stopping at a heading or an entry.
func paragraphAfter(lines []string, i int) string {
	var para []string
	for ; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(lines[i], "#") || strings.HasPrefix(lines[i], "- ") {
			break
		}
		para = append(para, t)
	}
	return strings.Join(para, " ")
}

// firstSentence is s up to and including its first period followed by a
// space, or all of s.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
