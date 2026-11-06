package slot

import (
	"strconv"
	"strings"
)

// The graph line as one sentence (card GRAPH-SLOT1): Text and the
// renderers it dispatches to, one per fact kind (the unit's in
// graph_unit.go), split from graph.go for Law 10. A code line with a role
// opens with the role's word. Every repository name renders inside a quote (quoted); an item
// left out by bind is reported as its count.

// Text is the line as one sentence; "none" for an empty line, which a
// sparse template leaves out before it renders.
func (l GraphLine) Text() string {
	switch {
	case l.code != nil:
		return l.codeText()
	case l.table != nil:
		return l.tableText()
	case l.know != nil:
		return l.knowText()
	case l.unit != nil:
		return l.unitText()
	}
	return "none"
}

func (l GraphLine) codeText() string {
	c, m := *l.code, l.more
	head := c.Path.Text()
	if l.keyed {
		head = "a file on record"
	}
	if c.Role != "" {
		head = c.Role + " " + head
	}
	var parts []string
	if p := phrase("declares", identTexts(c.Names), m.Names, "names"); p != "" {
		parts = append(parts, p)
	}
	for _, k := range c.Consts {
		parts = append(parts, k.Name.Text()+" is "+strconv.Itoa(k.Value))
	}
	if m.Consts > 0 {
		parts = append(parts, "holds "+strconv.Itoa(m.Consts)+" more constants")
	}
	if p := phrase("calls", identTexts(c.Calls), m.Calls, "callees"); p != "" {
		parts = append(parts, p)
	}
	if p := phrase("writes", tableTexts(c.Writes), m.Writes, "tables"); p != "" {
		parts = append(parts, p)
	}
	if p := phrase("reads", tableTexts(c.Reads), m.Reads, "tables"); p != "" {
		parts = append(parts, p)
	}
	if p := phrase("imports", identTexts(c.Imports), m.Imports, "modules"); p != "" {
		parts = append(parts, p)
	}
	if p := applies(c.Lessons, m.Lessons); p != "" {
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return head
	}
	return head + " " + strings.Join(parts, "; ")
}

func (l GraphLine) tableText() string {
	t, m := *l.table, l.more
	head := "table " + t.Table.Text()
	if l.keyed {
		head = "a table on record"
	}
	var parts []string
	if p := phrase("is written by", identTexts(t.Writers), m.Writers, "writers"); p != "" {
		parts = append(parts, p)
	}
	if p := phrase("is read by", identTexts(t.Readers), m.Readers, "readers"); p != "" {
		parts = append(parts, p)
	}
	if p := applies(t.Lessons, m.Lessons); p != "" {
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return head
	}
	return head + " " + strings.Join(parts, "; ")
}

func (l GraphLine) knowText() string {
	k := *l.know
	s := k.Kind + " " + k.ID.Text()
	switch {
	case l.keyed && l.digest != "":
		s += " applies to a file on record"
	case k.Path != "":
		s += " applies to " + k.Path.Text()
	}
	if k.Clause != "" {
		s += ": " + k.Clause.Text()
	}
	switch {
	case l.keyed && l.sourceDigest != "":
		s += "; source on record"
	case k.Source != "":
		s += "; source " + k.Source.Text()
	}
	return s
}

// phrase is "verb a, b and 2 more", "verb 2 nouns" for a list with nothing
// listed and a count, or "" for nothing at all.
func phrase(verb string, texts []string, more int, noun string) string {
	switch {
	case len(texts) > 0 && more > 0:
		return verb + " " + strings.Join(texts, ", ") + " and " + strconv.Itoa(more) + " more"
	case len(texts) > 0:
		return verb + " " + strings.Join(texts, ", ")
	case more > 0:
		return verb + " " + strconv.Itoa(more) + " " + noun
	}
	return ""
}

// applies is "INC-212 applies", "INC-212, ADR-0007 apply", with "and 2
// more" for records beyond the listed, or "2 records apply" for a count
// alone.
func applies(ids []KnowledgeID, more int) string {
	texts := make([]string, len(ids))
	for i, id := range ids {
		texts[i] = id.Text()
	}
	switch {
	case len(texts) == 0 && more == 0:
		return ""
	case len(texts) == 0:
		return strconv.Itoa(more) + " records apply"
	case len(texts) == 1 && more == 0:
		return texts[0] + " applies"
	case more > 0:
		return strings.Join(texts, ", ") + " and " + strconv.Itoa(more) + " more apply"
	}
	return strings.Join(texts, ", ") + " apply"
}

func identTexts(items []Ident) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text()
	}
	return out
}

// tableTexts is each table in its quote, with its columns in theirs:
// "'public.sessions' ('created_at', 'expires_at')".
func tableTexts(items []TableRef) []string {
	out := make([]string, len(items))
	for i, t := range items {
		out[i] = t.Table.Text()
		if len(t.Cols) > 0 {
			out[i] += " (" + strings.Join(identTexts(t.Cols), ", ") + ")"
		}
	}
	return out
}
