package pgraph

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

// Caps on what ParseLabels will read.
const (
	// MaxLabelBytes bounds a whole label file.
	MaxLabelBytes = 4 << 20
	// MaxLabelLines bounds its lines, comments included.
	MaxLabelLines = 100_000
)

// Accuracy is a count of facts against a hand-labelled truth, as sets of
// distinct triples: Got facts claimed, Want facts true, Hit in both. A ratio
// of these is never quoted without them or without the corpus (Describe).
type Accuracy struct{ Got, Want, Hit int }

// Score compares a producer's facts with the labelled truth. Time is
// O(len(got) + len(want)).
func Score(got, want []Triple) Accuracy {
	g, w := map[Triple]bool{}, map[Triple]bool{}
	for _, x := range got {
		g[x] = true
	}
	for _, x := range want {
		w[x] = true
	}
	a := Accuracy{Got: len(g), Want: len(w)}
	for x := range g {
		if w[x] {
			a.Hit++
		}
	}
	return a
}

// Precision is the share of claimed facts that are true. It is NaN when
// nothing was claimed: a figure over no claims is undefined, never 0 or 1.
// The figure says nothing without its counts and its corpus's size; Describe
// prints all three, and is what a published figure goes through.
func (a Accuracy) Precision() float64 { return ratio(a.Hit, a.Got) }

// Recall is the share of true facts that were claimed; NaN when there is no
// truth. Want is the size of the labelled corpus it was scored against.
func (a Accuracy) Recall() float64 { return ratio(a.Hit, a.Want) }

// Describe is the one way a figure is printed: precision and recall each with
// the counts they are a ratio of, then the corpus by name and its size in
// labelled edges, for example
//
//	precision 0.368 (14 of 38 CHA edges are true), recall 1.000 (14 of 14 labelled edges found), corpus dispatch-v1: 14 labelled edges
//
// An undefined ratio prints as "undefined", with its counts. a must be the
// Score of a producer's facts against the corpus's own labels, so that Want
// is the corpus's size.
func (a Accuracy) Describe(corpus string) string {
	return fmt.Sprintf("precision %s (%d of %d CHA edges are true), recall %s (%d of %d labelled edges found), corpus %s: %d labelled edges",
		fixed3(a.Precision()), a.Hit, a.Got, fixed3(a.Recall()), a.Hit, a.Want, corpus, a.Want)
}

func fixed3(x float64) string {
	if math.IsNaN(x) {
		return "undefined"
	}
	return fmt.Sprintf("%.3f", x)
}

func ratio(n, d int) float64 {
	if d == 0 {
		return math.NaN()
	}
	return float64(n) / float64(d)
}

// LabelHeader is the first line of a label file.
const LabelHeader = "pgraph-labels v0"

// Labels is a hand-labelled truth set for one corpus: may-call edges that
// someone read from the source and wrote down.
type Labels struct {
	// Corpus names the program the labels describe; a published figure is
	// published with it.
	Corpus string
	// Truth holds the labelled edges, as MayCall triples, in file order.
	Truth []Triple
}

// ParseLabels reads a label file: LabelHeader, a line "corpus NAME", then one
// edge per line as "caller<TAB>target" with symbol ids; lines starting with
// "#" are comments. Strict: a repeated edge, a missing field, or a field
// outside the stored-field alphabet is an error. Caps are checked before
// parsing, and the result holds copies, not slices of b.
func ParseLabels(b []byte) (Labels, error) {
	var l Labels
	if len(b) > MaxLabelBytes {
		return l, fmt.Errorf("pgraph: labels are %d bytes, cap %d", len(b), MaxLabelBytes)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return l, fmt.Errorf("pgraph: labels are empty or lack a final newline")
	}
	seen := map[Triple]bool{}
	for n := 1; len(b) > 0; n++ {
		if n > MaxLabelLines {
			return Labels{}, fmt.Errorf("pgraph: labels are over %d lines", MaxLabelLines)
		}
		i := bytes.IndexByte(b, '\n')
		line := string(b[:i])
		b = b[i+1:]
		switch {
		case n == 1:
			if line != LabelHeader {
				return Labels{}, fmt.Errorf("pgraph: bad label header")
			}
		case n == 2:
			name, ok := strings.CutPrefix(line, "corpus ")
			if !ok {
				return Labels{}, fmt.Errorf("pgraph: line 2 is not a corpus name")
			}
			if err := checkField("corpus", name); err != nil {
				return Labels{}, err
			}
			l.Corpus = name
		case strings.HasPrefix(line, "#"):
		default:
			f := strings.Split(line, "\t")
			if len(f) != 2 {
				return Labels{}, fmt.Errorf("pgraph: line %d: %d fields, want 2", n, len(f))
			}
			t := Triple{f[0], MayCall, f[1]}
			if err := t.check(); err != nil {
				return Labels{}, fmt.Errorf("pgraph: line %d: %w", n, err)
			}
			if seen[t] {
				return Labels{}, fmt.Errorf("pgraph: line %d repeats a label", n)
			}
			seen[t] = true
			l.Truth = append(l.Truth, t)
		}
	}
	if l.Corpus == "" {
		return Labels{}, fmt.Errorf("pgraph: labels name no corpus")
	}
	return l, nil
}
