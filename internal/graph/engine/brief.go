package engine

import (
	"fmt"
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
}

func (o Options) rel() func(string) string {
	if o.Rel == nil {
		return func(p string) string { return p }
	}
	return o.Rel
}

// Build runs Scope, Traverse, Rank and Select over g at rev for prompt,
// binds each selected line's facts into its slot, and renders the brief,
// dropping the last line until the message fits the briefing cap.
func Build(g *graph.Store, rev graph.Rev, prompt string, opt Options) Brief {
	if opt.Lessons {
		return buildLessons(g, rev, prompt, opt)
	}
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
	rel := opt.rel()
	b.Candidates = Rank(g, nb, b.Seeds, Outside(g, rev, rel))
	b.Lines, b.Decisions = Select(g, rev, b.Candidates)
	byID := map[graph.ID]Candidate{}
	for _, c := range b.Candidates {
		byID[c.ID] = c
	}
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

// Explain is the evidence tables: tokens, scope rows, candidates with
// every term, the budget decisions and each line's sentence.
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
