package engine

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// An agent noun in -iser or -izer is an inflected form of its stem the
// way -er is ("canonicaliser" through canonical, as "installer" through
// install; W-M1.32, E5): the index name opens the word and what the word
// adds is an inflection, not a word.
func TestDerive_AnAgentNounInIserOpensItsStem(t *testing.T) {
	g := synth(t, []synFile{
		{"host", "host/host.py", []synSym{fn("canonical_name"), fn("canonical")}},
		{"slot", "slot/fact.py", []synSym{fn("hostname_literal")}},
	}, nil, nil)
	c := scan(g, graph.RevCommit(graphtest.Commit))
	want := Tokens("canonical")[0]
	got, _ := c.derive(Tokens("canonicaliser"), nil)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("derived %v, want [%s]: iser is an agent suffix like er", got, want)
	}
	got, _ = c.derive(Tokens("tokenizer"), nil)
	if len(got) != 1 || got[0] != "tokenizer" {
		t.Fatalf("derived %v, want [tokenizer] unchanged: no index name opens it", got)
	}
}
