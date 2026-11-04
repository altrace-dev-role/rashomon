package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Depth is how many hops the neighborhood reaches from the seeds.
const Depth = 2

// Brief is the pipeline's whole result for one prompt: what seeded, what
// was reached, how it ranked, what was kept, and the text delivered.
type Brief struct {
	Rev          graph.Rev
	Prompt       string
	Tokens       []string
	Weights      map[string]float64
	Scorer       string
	Seeds        []Seed
	Rows         []ScopeRow
	Neighborhood int
	Candidates   []Candidate
	Lines        []Line
	Decisions    []Decision
	Elapsed      time.Duration
}

// Build runs Scope, Traverse, Rank and Select over g at rev for prompt.
func Build(g *graph.Store, rev graph.Rev, prompt string, opt Options) Brief {
	start := time.Now()
	b := Brief{Rev: rev, Prompt: prompt, Tokens: uniq(Tokens(prompt)), Scorer: "tf"}
	if opt.IDF {
		b.Scorer = "idf"
	}
	b.Seeds, b.Rows, b.Weights = Scope(g, rev, prompt, opt)
	ids := make([]graph.ID, len(b.Seeds))
	for i, s := range b.Seeds {
		ids[i] = s.ID
	}
	nb := Traverse(g, ids, Depth, rev)
	b.Neighborhood = len(nb.Distance)
	b.Candidates = Rank(g, nb, b.Seeds)
	b.Lines, b.Decisions = Select(g, rev, b.Candidates)
	byID := map[graph.ID]Candidate{}
	for _, c := range b.Candidates {
		byID[c.ID] = c
	}
	for i := range b.Lines {
		b.Lines[i].Text = lineText(g, rev, b.Lines[i], byID)
	}
	b.Elapsed = time.Since(start)
	return b
}

// Empty is whether nothing was selected.
func (b Brief) Empty() bool { return len(b.Lines) == 0 }

// Text is what the model receives.
func (b Brief) Text() string {
	if b.Empty() {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "rashomon graph (rev %s): the code your request touches, by the repository's dependency graph. %d node(s) matched, %d reached, %d line(s) kept.\n",
		b.Rev, len(b.Seeds), b.Neighborhood, len(b.Lines))
	for _, l := range b.Lines {
		sb.WriteString("- " + l.Text + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// lineText renders one line: the path or table, then each member with its
// relations, then the knowledge that applies.
func lineText(g Reader, rev graph.Rev, l Line, byID map[graph.ID]Candidate) string {
	var parts []string
	know := map[graph.ID]bool{}
	var knowIDs []graph.ID
	for _, m := range l.Members {
		c := byID[m]
		if t := memberText(g, rev, c); t != "" {
			parts = append(parts, t)
		}
		for _, k := range c.Knowledge {
			if !know[k] {
				know[k] = true
				knowIDs = append(knowIDs, k)
			}
		}
	}
	if n, ok := g.Node(l.Key, rev); ok && IsKnowledge(n.Kind) {
		return knowledgeText(g, rev, n)
	}
	head := display(l.Key)
	text := head + ": " + strings.Join(parts, "; ")
	if len(parts) == 0 {
		text = head
	}
	var names []string
	for _, k := range knowIDs {
		if n, ok := g.Node(k, rev); ok {
			names = append(names, strings.TrimPrefix(string(k), n.Kind+":"))
		}
	}
	if len(names) > 0 {
		text += " (see " + strings.Join(names, ", ") + ")"
	}
	if len(text) > MaxLineBytes {
		text = text[:MaxLineBytes-3] + "..."
	}
	return text
}

// knowledgeText is a lesson's or decision's own line: its words, then
// what it applies to.
func knowledgeText(g Reader, rev graph.Rev, n graph.Node) string {
	name := strings.TrimPrefix(string(n.ID), n.Kind+":")
	words := n.Attrs["text"]
	if !strings.HasPrefix(words, name) {
		words = name + ": " + words
	}
	var to []string
	for _, e := range g.Out(n.ID, rev, graph.AppliesTo) {
		to = append(to, qualified(e.Dst))
	}
	if len(to) > 0 {
		words += " [applies to " + strings.Join(to, ", ") + "]"
	}
	return words
}

// memberText is one candidate's words on its line.
func memberText(g Reader, rev graph.Rev, c Candidate) string {
	n, ok := g.Node(c.ID, rev)
	if !ok {
		return ""
	}
	switch c.Kind {
	case "sym":
		if n.Attrs["symkind"] == "const" {
			return short(c.ID) + "=" + n.Attrs["value"]
		}
		if rel := relations(g, rev, c.ID); rel != "" {
			return short(c.ID) + n.Attrs["sig"] + " " + rel
		}
		return short(c.ID) + n.Attrs["sig"]
	case "file":
		return relations(g, rev, c.ID)
	case "table":
		return tableText(g, rev, c.ID)
	}
	return ""
}

// relations are a node's outgoing calls, reads, writes and references, as
// " calls a, b; writes t (cols)".
func relations(g Reader, rev graph.Rev, id graph.ID) string {
	groups := map[graph.Pred][]string{}
	for _, e := range g.Out(id, rev, graph.Calls, graph.Reads, graph.Writes, graph.References) {
		s := short(e.Dst)
		if cols := e.Attrs["cols"]; cols != "" && (e.Pred == graph.Reads || e.Pred == graph.Writes) {
			s += " (" + cols + ")"
		}
		if e.Pred == graph.References {
			s += " from " + module(e.Dst)
		}
		groups[e.Pred] = append(groups[e.Pred], s)
	}
	var parts []string
	for _, p := range []graph.Pred{graph.Calls, graph.Writes, graph.Reads, graph.References} {
		if len(groups[p]) > 0 {
			word := string(p)
			if p == graph.References {
				word = "imports"
			}
			parts = append(parts, word+" "+strings.Join(groups[p], ", "))
		}
	}
	return strings.Join(parts, "; ")
}

// tableText names who writes and reads a table.
func tableText(g Reader, rev graph.Rev, id graph.ID) string {
	var w, r []string
	for _, e := range g.In(id, rev, graph.Writes, graph.Reads) {
		s := qualified(e.Src)
		if e.Pred == graph.Writes {
			w = append(w, s)
		} else {
			r = append(r, s)
		}
	}
	sort.Strings(w)
	sort.Strings(r)
	var parts []string
	if len(w) > 0 {
		parts = append(parts, "written by "+strings.Join(w, ", "))
	}
	if len(r) > 0 {
		parts = append(parts, "read by "+strings.Join(r, ", "))
	}
	return strings.Join(parts, "; ")
}

// display is a line head: a file's path, a table's name.
func display(id graph.ID) string {
	s := string(id)
	switch {
	case strings.HasPrefix(s, "file:"):
		return strings.TrimPrefix(s, "file:")
	case strings.HasPrefix(s, "tbl:"):
		return "table " + strings.TrimPrefix(s, "tbl:")
	}
	return s
}

// qualified is a symbol's dotted name, a file's path, a table's name.
func qualified(id graph.ID) string {
	s := string(id)
	if strings.HasPrefix(s, "sym:") {
		parts := strings.Split(s, ":")
		return parts[len(parts)-1]
	}
	return display(id)
}

// short is a symbol's last name segment.
func short(id graph.ID) string {
	q := qualified(id)
	if i := strings.LastIndex(q, "."); i >= 0 && strings.HasPrefix(string(id), "sym:") {
		return q[i+1:]
	}
	return q
}

// module is the dotted module of a symbol.
func module(id graph.ID) string {
	q := qualified(id)
	if i := strings.LastIndex(q, "."); i >= 0 {
		return q[:i]
	}
	return q
}

// Explain is the evidence tables: tokens, scope rows, candidates with
// every term, and the budget decisions.
func (b Brief) Explain() string {
	var sb strings.Builder
	var toks []string
	for _, t := range b.Tokens {
		toks = append(toks, fmt.Sprintf("%s(%.2f)", t, b.Weights[t]))
	}
	fmt.Fprintf(&sb, "prompt: %q\nscorer: %s  tokens: %s\nrev: %s  elapsed: %s\n", b.Prompt, b.Scorer, strings.Join(toks, " "), b.Rev, b.Elapsed)
	fmt.Fprintf(&sb, "\nscope (%d scored, %d seeds; floor %.2f, max %d):\n", len(b.Rows), len(b.Seeds), SeedFloor, MaxSeeds)
	for _, r := range b.Rows {
		mark := " "
		if r.Seed {
			mark = "*"
		}
		fmt.Fprintf(&sb, "%s %6.2f  %-8s %s  [%s]\n", mark, r.Score, r.Kind, r.ID, strings.Join(r.Hits, " "))
	}
	fmt.Fprintf(&sb, "\nrank (%d reached at depth %d):\n", b.Neighborhood, Depth)
	for _, c := range b.Candidates {
		fmt.Fprintf(&sb, "%6.2f  d=%d base=%.2f lex=%.2f know=%d  %-8s %s  via %s\n", c.Score, c.Distance, c.Base, c.Lex, len(c.Knowledge), c.Kind, c.ID, c.Via)
	}
	fmt.Fprintf(&sb, "\nselect (budget %d lines x %d bytes):\n", MaxLines, MaxLineBytes)
	for _, d := range b.Decisions {
		fmt.Fprintf(&sb, "line %d  %-22s %s\n", d.Line, d.Why, d.ID)
	}
	return sb.String()
}
