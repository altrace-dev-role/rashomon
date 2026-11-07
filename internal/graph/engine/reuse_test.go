package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestReuse_ASymptomWithNoUsePhraseClaimsNoReuse (W-M1.22, AC-M1.5): a
// symptom sentence names its subject and nothing to take up, so the
// brief carries no reuse line. Before the rule, any in-unit file whose
// callable held a prompt word was admitted (here put_session on
// "session ttl", with no use phrase, no addition and no call from an
// addition's code): 81 of the 117 reuse lines on the W-M1.19 set were
// such lines against a '-' truth. A reuse line states its basis or is
// not emitted; a hit inside the unit alone ranks claims for Explain.
func TestReuse_ASymptomWithNoUsePhraseClaimsNoReuse(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "sessions never expire after the ttl passes.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	r := b.Roles
	if r.Unit != graph.UnitID("py", "services/auth") || r.File != graph.FileID("services/auth/auth/sessions.py") {
		t.Fatalf("roles %+v", r)
	}
	if len(r.Frame.Uses) != 0 || len(r.Frame.Additions) != 0 {
		t.Fatalf("frame %+v, want a symptom with no use or addition phrase", r.Frame)
	}
	if len(r.Reuse) != 0 {
		t.Errorf("reuse %+v, want none: no phrase names a thing to take up", r.Reuse)
	}
	if len(r.ReuseClaims) == 0 || r.ReuseClaims[0].File != graph.FileID("services/auth/auth/cache.py") || !r.ReuseClaims[0].In {
		t.Errorf("claims %+v, want the in-unit hit kept for Explain", r.ReuseClaims)
	}
	for _, l := range b.Lines[:b.Kept] {
		if l.Role == RoleReuse {
			t.Errorf("kept a reuse line %+v", l)
		}
	}
	if strings.Contains(b.Text(), "reuse '") {
		t.Errorf("brief carries a reuse line:\n%s", b.Text())
	}
}

// TestReuse_AUsePhraseKeepsTheInUnitFile: the same unit, with a use word
// naming the cache's ttl, keeps the cache module as the reuse: the basis
// is the use phrase, not the unit membership.
func TestReuse_AUsePhraseKeepsTheInUnitFile(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry honor the cache ttl.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	r := b.Roles
	if len(r.Frame.Uses) != 1 {
		t.Fatalf("frame %+v, want one use phrase", r.Frame)
	}
	if len(r.Reuse) != 1 || r.Reuse[0].File != graph.FileID("services/auth/auth/cache.py") {
		t.Errorf("reuse %+v, want the cache module alone", r.Reuse)
	}
	if len(r.ReuseClaims) == 0 || r.ReuseClaims[0].Use == 0 {
		t.Errorf("claims %+v, want a use-phrase basis on the first", r.ReuseClaims)
	}
}
