package engine

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestTokens_CamelCase (W-M1.5, R3): a Go identifier splits where its case
// changes, an initialism (HTTP, JSON, HMAC) is a word of its own whose
// last capital starts the next word, and a lower-case s after an
// initialism is its plural, not a word; prompt words and snake_case names
// split the same way, so a prompt reaches a Go name through the stems.
func TestTokens_CamelCase(t *testing.T) {
	for in, want := range map[string][]string{
		"PlatformRead":                  {"platform", "read"},
		"readVouch":                     {"read", "vouch"},
		"HTTPServer":                    {"http", "server"},
		"ReadJSONFile":                  {"read", "json", "file"},
		"HMACKey":                       {"hmac", "key"},
		"SessionID":                     {"session", "id"},
		"URLs":                          {"urls"},
		"ErrNotSynced":                  {"err", "sync"},
		"MaxLogBytes":                   {"max", "log", "byte"},
		"read_snapshot":                 {"read", "snapshot"},
		"the proven fan-in of a symbol": {"proven", "fan", "symbol"},
	} {
		if got := Tokens(in); !slices.Equal(got, want) {
			t.Errorf("Tokens(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestScope_DocMatches (W-M1.5, S2): a prompt word found only in a
// symbol's doc sentence scores the symbol, at the summary's weight and
// not the name's, and a symbol with no doc and no other hit scores nothing.
func TestScope_DocMatches(t *testing.T) {
	s := graph.NewStore()
	rev := graph.RevCommit("c1")
	prov := graph.Prov{Extractor: "test", Version: "1"}
	callers := graph.ID("sym:go:internal/platform:func:Callers")
	locate := graph.ID("sym:go:internal/platform:func:Locate")
	s.AssertNode(graph.Node{ID: callers, Kind: "sym", Tier: graph.Proven, Conf: 1, Rev: rev, Prov: prov,
		Attrs: map[string]string{"symkind": "func", "doc": "Callers is the proven fan-in of a symbol."}})
	s.AssertNode(graph.Node{ID: locate, Kind: "sym", Tier: graph.Proven, Conf: 1, Rev: rev, Prov: prov,
		Attrs: map[string]string{"symkind": "func"}})
	n, _ := s.Node(callers, rev)
	if f := FieldsOf(n); !slices.Contains(f.Summary, "fan") || slices.Contains(f.Name, "fan") || !slices.Equal(f.Name, []string{"caller"}) {
		t.Errorf("FieldsOf(Callers) = %+v; want the doc's words in Summary and only the name in Name", f)
	}
	for _, opt := range []Options{{}, {IDF: true}} {
		_, rows, _ := Scope(s, rev, "report the proven fan-in of every symbol", opt)
		var hit *ScopeRow
		for i := range rows {
			switch rows[i].ID {
			case callers:
				hit = &rows[i]
			case locate:
				t.Errorf("idf=%v: Locate scored %v with no doc and no hit", opt.IDF, rows[i].Score)
			}
		}
		if hit == nil || hit.Score <= 0 || !slices.Equal(hit.Hits, []string{"fan", "proven", "symbol"}) {
			t.Errorf("idf=%v: Callers row = %+v; want a score from hits fan, proven, symbol", opt.IDF, hit)
		}
	}
}
