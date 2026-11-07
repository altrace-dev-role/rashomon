package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// Depth is how many hops the neighborhood reaches from the seeds.
const Depth = 2

// Brief is the pipeline's whole result for one prompt: what seeded, what
// was reached, how it ranked, what was kept, and the message delivered.
type Brief struct {
	Rev          graph.Rev
	Prompt       string
	Tokens       []string
	Weights      map[string]float64
	Scorer       string
	Seeds        []Seed
	Rows         []ScopeRow
	Roles        Roles
	Neighborhood int
	Candidates   []Candidate
	Lines        []Line
	Decisions    []Decision
	// Message is the brief as the model receives it: the task-preflight
	// graph template over the lines' slots (GRAPH-SLOT1), rendered under
	// the briefing cap; the zero Message when no line rendered.
	Message render.Message
	// Kept is how many lines Message carries, in Lines' order.
	Kept    int
	Elapsed time.Duration
}

// Options selects the scorer and the path form. IDF weights each prompt
// token by ln((N+1)/df), N the nodes scored and df the nodes holding the
// token, so a word on half the graph counts for little; off, every token
// weighs 1. Rel maps a path as the graph holds it, relative to the index
// root, to the form the brief prints, relative to the hook's cwd; nil
// prints the graph's form.
type Options struct {
	IDF bool
	Rel func(string) string
	// Lessons is the ablation arm (buildLessons): knowledge nodes matched
	// by the prompt's words alone, no traversal, no rank.
	Lessons bool
	// Lexicon is reserved for the proposer layer (AD16, Part B1): a term's
	// expansions, written out of hook by `rashomon graph enrich` and read
	// here as data. Build reads none of it until AC-P1 adopts it
	// (TestOptions_LexiconIsReserved).
	Lexicon map[string][]string
}

func (o Options) rel() func(string) string {
	if o.Rel == nil {
		return func(p string) string { return p }
	}
	return o.Rel
}

// Build runs Scope, the roles stage (Parse, FindRoles), Traverse, Rank
// and Select over g at rev for prompt, puts the role lines before the
// related lines Select kept, binds each line's facts into its slot, and
// renders the brief, dropping the last line until the message fits the
// briefing cap.
func Build(g *graph.Store, rev graph.Rev, prompt string, opt Options) Brief {
	if opt.Lessons {
		return buildLessons(g, rev, prompt, opt)
	}
	start := time.Now()
	b := Brief{Rev: rev, Prompt: prompt, Tokens: uniq(Tokens(prompt)), Scorer: "tf"}
	if opt.IDF {
		b.Scorer = "idf"
	}
	c := scan(g, rev)
	c.idf = opt.IDF
	b.Seeds, b.Rows, b.Weights = c.scope(prompt, opt)
	rel := opt.rel()
	outside := Outside(g, rev, rel)
	b.Roles = FindRoles(g, c, b.Rows, b.Weights, Parse(prompt), Names(g, c, prompt, b.Tokens, b.Weights), b.Tokens, outside)
	ids := make([]graph.ID, len(b.Seeds))
	for i, s := range b.Seeds {
		ids[i] = s.ID
	}
	nb := Traverse(g, ids, Depth, rev)
	b.Neighborhood = len(nb.Distance)
	b.Candidates = Rank(g, nb, b.Seeds, outside)
	related, decisions := Select(g, rev, b.Candidates)
	byID := map[graph.ID]Candidate{}
	for _, c := range b.Candidates {
		byID[c.ID] = c
	}
	b.Lines, b.Decisions = merge(g, rev, b.Roles, related, decisions, byID)
	for i := range b.Lines {
		b.Lines[i].Slot = lineFact(g, rev, b.Lines[i], byID, rel)
	}
	b.bind()
	b.deliver()
	b.Elapsed = time.Since(start)
	return b
}

// bind binds each line's slot; a line the slot refuses is dropped with a
// decision naming the refusal, and the lines that bound close up.
func (b *Brief) bind() {
	kept := b.Lines[:0]
	for i, l := range b.Lines {
		v, err := slot.Bind(l.Slot)
		if err != nil {
			b.Decisions = append(b.Decisions, Decision{ID: l.Key, Line: i + 1, Why: "refused: " + err.Error()})
			continue
		}
		l.Slot = v.(slot.GraphLine)
		l.Text = v.Text()
		kept = append(kept, l)
	}
	b.Lines = kept
}

// deliver renders the bound lines through the task-preflight graph
// template, from all of them down to one, and keeps the first that
// renders; a line the ladder stepped past is dropped with a decision.
func (b *Brief) deliver() {
	for n := len(b.Lines); n > 0; n-- {
		m, err := render.New(pattern.TaskPreflight, b.binding(n, nil))
		if err != nil {
			continue
		}
		b.Message, b.Kept = m, n
		break
	}
	for i := b.Kept; i < len(b.Lines); i++ {
		b.Decisions = append(b.Decisions, Decision{ID: b.Lines[i].Key, Line: i + 1, Why: "dropped: over the brief's cap"})
	}
}

// binding is the graph template's binding over the first n lines, in their
// record form when digest is set.
func (b Brief) binding(n int, digest func(string) string) render.TaskPreflightGraphBinding {
	lines := make([]slot.GraphLine, 0, n)
	for _, l := range b.Lines[:n] {
		if digest != nil {
			lines = append(lines, l.Slot.Record(digest))
		} else {
			lines = append(lines, l.Slot)
		}
	}
	return render.NewGraphBinding(slot.Revision(b.Rev.String()), slot.Count(len(b.Seeds)), slot.Count(b.Neighborhood), lines)
}

// Record is the message the steer line records at record scope v4: the
// kept lines in their record form, paths and sources as keyed digests and
// lists as counts, rendered through the same template.
func (b Brief) Record(digest func(string) string) (render.Message, error) {
	if b.Empty() {
		return render.Message{}, render.ErrNothing
	}
	return render.New(pattern.TaskPreflight, b.binding(b.Kept, digest))
}

// Empty is whether nothing rendered.
func (b Brief) Empty() bool { return b.Kept == 0 }

// Text is what the model receives.
func (b Brief) Text() string { return b.Message.Text() }

// Explain is the evidence tables for the person who typed the prompt: the
// prompt, its tokens with their weights, the scope rows with the words
// each matched, the candidates with every term, the budget decisions and
// each line's sentence. The typed verb prints it; the hook path writes
// Evidence.
func (b Brief) Explain() string {
	var toks []string
	for _, t := range b.Tokens {
		toks = append(toks, fmt.Sprintf("%s(%.2f)", t, b.Weights[t]))
	}
	return fmt.Sprintf("prompt: %q\nscorer: %s  tokens: %s\n", b.Prompt, b.Scorer, strings.Join(toks, " ")) + b.tables(true)
}

// roles is the frame and the unit claims, with the phrases' words when
// hits is set and their counts otherwise, then the roles by id.
func (b Brief) roles(hits bool) string {
	r := b.Roles
	var sb strings.Builder
	if hits {
		fmt.Fprintf(&sb, "\nnames: %s\n", namedWords(r.Named))
		if len(r.Derived) > 0 {
			fmt.Fprintf(&sb, "derived: %s\n", derivedWords(r.Derived))
		}
		fmt.Fprintf(&sb, "frame: targets %s; additions %s; constraints %s; uses %s; verbs %s\n", phrases(r.Frame.Targets), phrases(r.Frame.Additions), phrases(r.Frame.Constraints), phrases(r.Frame.Uses), r.Frame.Verbs)
		fmt.Fprintf(&sb, "units (phrase [%s], fallback %v; floor %.2f):\n", strings.Join(r.Target, " "), r.Fallback, UnitFloor)
	} else {
		fmt.Fprintf(&sb, "\nframe: targets %d; additions %d; constraints %d; uses %d; verbs %d\n", len(r.Frame.Targets), len(r.Frame.Additions), len(r.Frame.Constraints), len(r.Frame.Uses), len(r.Frame.Verbs))
		fmt.Fprintf(&sb, "units (phrase of %d tokens, fallback %v; floor %.2f):\n", len(r.Target), r.Fallback, UnitFloor)
	}
	for _, p := range r.Picks {
		kind := "label"
		if p.Callable {
			kind = "callable"
		}
		fmt.Fprintf(&sb, "  %-10s %5.2f of %5.2f  %-32s nodes %-3d %-8s prec %.2f mass %5.2f  %s\n", p.Class, p.Covered, p.Of, p.Path, len(p.Nodes), kind, p.Precision, p.Mass, p.Refused)
	}
	if n := len(r.ReuseClaims); n > 0 {
		fmt.Fprintf(&sb, "reuse claims (%d; use, add, mass; via the addition's code; in unit; called outside):\n", n)
	}
	for i, cl := range r.ReuseClaims {
		if i == 8 {
			fmt.Fprintf(&sb, "  ... %d more\n", len(r.ReuseClaims)-i)
			break
		}
		fmt.Fprintf(&sb, "  use %6.2f add %6.2f mass %6.2f  via %-5v in %-5v out %-5v nodes %-3d %s\n", cl.Use, cl.Add, cl.Mass, cl.ViaAdd, cl.In, cl.Reused, cl.Nodes, cl.File)
	}
	fmt.Fprintf(&sb, "roles (%s): unit %s; file %s; targets %d; reuse %s; consumers %s; knowledge %s\n",
		r.Why, r.Unit, r.File, len(r.Targets), roleFiles(r.Reuse), roleFiles(r.Consumers), joinIDs(r.Knowledge))
	return sb.String()
}

// namedWords is R0's reading for Explain: each word, its kind and what
// it resolved to or why not.
func namedWords(n Named) string {
	if len(n.Words) == 0 {
		return "none"
	}
	out := make([]string, len(n.Words))
	for i, w := range n.Words {
		s := w.Word + " (" + w.Kind + ")"
		if w.Unit != "" {
			s += " -> " + w.Unit
		}
		if w.Node != "" {
			s += " " + string(w.Node)
		}
		if w.Why != "" {
			s += ": " + w.Why
		}
		out[i] = s
	}
	return strings.Join(out, "; ")
}

// derivedWords is the S2 readings for Explain, token>name, in order.
func derivedWords(d map[string]string) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		keys[i] = k + ">" + d[k]
	}
	return strings.Join(keys, " ")
}

func phrases(ps []Phrase) string {
	if len(ps) == 0 {
		return "none"
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return strings.Join(out, " ")
}

func roleFiles(rfs []RoleFile) string {
	ids := make([]graph.ID, len(rfs))
	for i, rf := range rfs {
		ids[i] = rf.File
	}
	return joinIDs(ids)
}

func joinIDs(ids []graph.ID) string {
	if len(ids) == 0 {
		return "none"
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return strings.Join(out, ", ")
}

// Evidence is Explain with nothing of the prompt: no prompt line, no
// token and no matched word on a scope row, only their counts. D160 reads
// the prompt in memory and stores it nowhere, so what the hook path writes
// under the store is the selection alone: the ids scored, reached, chosen
// and rendered, which the steer line carries too.
func (b Brief) Evidence() string {
	return fmt.Sprintf("scorer: %s  tokens: %d\n", b.Scorer, len(b.Tokens)) + b.tables(false)
}

// tables is the rev line and the scope, rank, select and render tables;
// hits says whether a scope row names the prompt words it matched or
// counts them.
func (b Brief) tables(hits bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "rev: %s  elapsed: %s\n", b.Rev, b.Elapsed)
	fmt.Fprintf(&sb, "\nscope (%d scored, %d seeds; floor %.2f, max %d):\n", len(b.Rows), len(b.Seeds), SeedFloor, MaxSeeds)
	for _, r := range b.Rows {
		mark := " "
		if r.Seed {
			mark = "*"
		}
		if hits {
			fmt.Fprintf(&sb, "%s %6.2f  %-8s %s  [%s]\n", mark, r.Score, r.Kind, r.ID, strings.Join(r.Hits, " "))
		} else {
			fmt.Fprintf(&sb, "%s %6.2f  %-8s %s  %d\n", mark, r.Score, r.Kind, r.ID, len(r.Hits))
		}
	}
	sb.WriteString(b.roles(hits))
	fmt.Fprintf(&sb, "\nrank (%d reached at depth %d):\n", b.Neighborhood, Depth)
	for _, c := range b.Candidates {
		reach := 0
		if c.Reach {
			reach = 1
		}
		fmt.Fprintf(&sb, "%6.2f  d=%d base=%.2f lex=%.2f know=%d reach=%d  %-8s %s  via %s\n", c.Score, c.Distance, c.Base, c.Lex, len(c.Knowledge), reach, c.Kind, c.ID, c.Via)
	}
	fmt.Fprintf(&sb, "\nselect (budget %d lines):\n", MaxLines)
	for _, d := range b.Decisions {
		fmt.Fprintf(&sb, "line %d  %-22s %s\n", d.Line, d.Why, d.ID)
	}
	fmt.Fprintf(&sb, "\nrender (%d of %d lines kept, %d bytes, template %s):\n", b.Kept, len(b.Lines), len(b.Text()), b.Message.Template())
	for i, l := range b.Lines {
		mark := " "
		if i >= b.Kept {
			mark = "-"
		}
		fmt.Fprintf(&sb, "%s line %d  %s  (%s)\n", mark, i+1, l.Text, l.Key)
	}
	return sb.String()
}
