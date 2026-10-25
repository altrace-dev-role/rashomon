package pgraph

import (
	"math"
	"strings"
	"testing"
)

func TestPL3_ScoreCountsDistinctTriples(t *testing.T) {
	a, b, c := tri("x", MayCall, "1"), tri("x", MayCall, "2"), tri("x", MayCall, "3")
	for _, k := range []struct {
		name      string
		got, want []Triple
		acc       Accuracy
		p, r      float64
	}{
		{"all true", []Triple{a, b}, []Triple{a, b}, Accuracy{2, 2, 2}, 1, 1},
		{"spurious edge lowers precision only", []Triple{a, b, c}, []Triple{a, b}, Accuracy{3, 2, 2}, 2.0 / 3, 1},
		{"missed edge lowers recall only", []Triple{a}, []Triple{a, b}, Accuracy{1, 2, 1}, 1, 0.5},
		{"duplicates count once", []Triple{a, a, b}, []Triple{a, a}, Accuracy{2, 1, 1}, 0.5, 1},
		{"disjoint", []Triple{a}, []Triple{b}, Accuracy{1, 1, 0}, 0, 0},
	} {
		acc := Score(k.got, k.want)
		if acc != k.acc || acc.Precision() != k.p || acc.Recall() != k.r {
			t.Errorf("%s: %+v p=%v r=%v; want %+v p=%v r=%v", k.name, acc, acc.Precision(), acc.Recall(), k.acc, k.p, k.r)
		}
	}
	// No claims and no truth are undefined, never 0 and never 1.
	e := Score(nil, []Triple{a})
	if !math.IsNaN(e.Precision()) || e.Recall() != 0 {
		t.Errorf("empty answer: p=%v r=%v", e.Precision(), e.Recall())
	}
	if e := Score([]Triple{a}, nil); e.Precision() != 0 || !math.IsNaN(e.Recall()) {
		t.Errorf("empty truth: p=%v r=%v", e.Precision(), e.Recall())
	}
}

// A figure is never printed without the counts it is a ratio of and the
// corpus's size, and an undefined one says so instead of printing NaN.
func TestPL3_DescribeCarriesCountsAndCorpusSize(t *testing.T) {
	for _, k := range []struct {
		acc    Accuracy
		corpus string
		want   string
	}{
		{Accuracy{Got: 38, Want: 14, Hit: 14}, "dispatch-v1", "precision 0.368 (14 of 38 CHA edges are true), recall 1.000 (14 of 14 labelled edges found), corpus dispatch-v1: 14 labelled edges"},
		{Accuracy{Got: 0, Want: 3, Hit: 0}, "demo-v1", "precision undefined (0 of 0 CHA edges are true), recall 0.000 (0 of 3 labelled edges found), corpus demo-v1: 3 labelled edges"},
		{Accuracy{Got: 2, Want: 0, Hit: 0}, "demo-v2", "precision 0.000 (0 of 2 CHA edges are true), recall undefined (0 of 0 labelled edges found), corpus demo-v2: 0 labelled edges"},
	} {
		if got := k.acc.Describe(k.corpus); got != k.want {
			t.Errorf("Describe(%+v, %q) = %q, want %q", k.acc, k.corpus, got, k.want)
		}
	}
}

const goodLabels = "pgraph-labels v0\ncorpus demo-v1\n# a comment\nfunc:a.F\tmethod:a.T.M\nfunc:a.G\tmethod:a.T.M\n"

func TestPL3_ParseLabels(t *testing.T) {
	l, err := ParseLabels([]byte(goodLabels))
	if err != nil || l.Corpus != "demo-v1" || len(l.Truth) != 2 {
		t.Fatalf("%+v %v", l, err)
	}
	if l.Truth[0] != (Triple{"func:a.F", MayCall, "method:a.T.M"}) {
		t.Errorf("first label %v", l.Truth[0])
	}
	for name, in := range map[string]string{
		"empty":             "",
		"bad header":        "pgraph-labels v9\ncorpus c\n",
		"no corpus line":    "pgraph-labels v0\nfunc:a.F\tmethod:a.T.M\n",
		"unnamed corpus":    "pgraph-labels v0\ncorpus \n",
		"no final newline":  strings.TrimSuffix(goodLabels, "\n"),
		"one field":         "pgraph-labels v0\ncorpus c\nfunc:a.F\n",
		"three fields":      "pgraph-labels v0\ncorpus c\na\tb\tc\n",
		"empty field":       "pgraph-labels v0\ncorpus c\na\t\n",
		"duplicate label":   "pgraph-labels v0\ncorpus c\na\tb\na\tb\n",
		"nul byte":          "pgraph-labels v0\ncorpus c\na\x00\tb\n",
		"invalid utf8":      "pgraph-labels v0\ncorpus c\na\t\xff\n",
		"corpus with a tab": "pgraph-labels v0\ncorpus a\tb\n",
		"over the cap":      "pgraph-labels v0\ncorpus c\n" + strings.Repeat("#", MaxLabelBytes),
	} {
		if _, err := ParseLabels([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A header alone with a corpus and no labels is a valid, empty set.
	if l, err := ParseLabels([]byte("pgraph-labels v0\ncorpus c\n")); err != nil || len(l.Truth) != 0 {
		t.Errorf("empty label set: %+v %v", l, err)
	}
	if _, err := ParseLabels([]byte("pgraph-labels v0\ncorpus c\n" + strings.Repeat("#\n", MaxLabelLines+1))); err == nil {
		t.Error("over the line cap accepted")
	}
}

// Every id in the hand-written label file names a symbol the extractor knows,
// so a typo cannot silently lower recall.
func TestPL3_LabelsNameRealSymbols(t *testing.T) {
	lab := dispatchLabels(t)
	known := map[string]bool{}
	for _, x := range Extract(loadT(t, dispatchRoot)) {
		known[x.Subject], known[x.Object] = true, true
	}
	for _, x := range lab.Truth {
		if x.Predicate != MayCall {
			t.Errorf("label predicate %v", x.Predicate)
		}
		if !known[x.Subject] || !known[x.Object] {
			t.Errorf("label names an unknown symbol: %v", x)
		}
	}
}

func FuzzParseLabels(f *testing.F) {
	f.Add([]byte(goodLabels))
	f.Add([]byte("pgraph-labels v0\ncorpus c\n"))
	f.Add([]byte("pgraph-labels v0\ncorpus c\na\tb\n"))
	f.Add([]byte(""))
	f.Add([]byte("pgraph-labels v0\ncorpus \x00\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		keep := append([]byte(nil), in...)
		l, err := ParseLabels(in)
		for i := range in { // the parse must not alias its input
			in[i] ^= 0xff
		}
		if err != nil {
			return
		}
		if len(l.Truth) > len(keep) {
			t.Fatal("more labels than input bytes")
		}
		seen := map[Triple]bool{}
		for _, x := range l.Truth {
			if err := x.check(); err != nil || x.Predicate != MayCall || seen[x] {
				t.Fatalf("bad label %v: %v", x, err)
			}
			seen[x] = true
			if !strings.Contains(string(keep), x.Subject) || !strings.Contains(string(keep), x.Object) {
				t.Fatalf("label %v not from the input", x)
			}
		}
		if checkField("corpus", l.Corpus) != nil {
			t.Fatalf("corpus %q", l.Corpus)
		}
	})
}
