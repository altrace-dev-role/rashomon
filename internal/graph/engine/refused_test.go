package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
)

// refusedTree indexes two Python units in which host/billing.py, which
// declares refund_ledger, void_invoices and faster_checkout, is refused
// (its bracket at line 5 never closes): the store holds none of its
// names, and the header counts it. host/host.py reads the invoices table,
// a name that no unit's claim holds.
func refusedTree(t *testing.T) (*graph.Store, graph.Header) {
	t.Helper()
	root := t.TempDir()
	for rel, text := range map[string]string{
		"host/host.py":    "def canonical_name(user):\n    return user.strip()\n\n\ndef load_rows(db):\n    return db.execute(\"SELECT id, total FROM invoices WHERE paid = 0\")\n",
		"host/billing.py": "def refund_ledger(entry):\n    return entry\n\n\ndef void_invoices(rows:\n    return rows\n\n\ndef faster_checkout(cart):\n    return cart\n",
		"spend/mode.py":   "def fast_mode(x):\n    return x\n\n\ndef fast_mode_pricing(x):\n    return fast_mode(x)\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := index.Config{Units: []index.Unit{
		{Path: "host", Modules: []string{"host"}, Lang: "py"},
		{Path: "spend", Modules: []string{"spend"}, Lang: "py"},
	}}
	r, err := index.Build(root, "c1", cfg)
	if err != nil {
		t.Fatalf("index.Build = %v; want host/billing.py refused and the index written", err)
	}
	if r.Header.Refused != 1 || len(r.Header.RefusedFiles) != 1 || r.Header.RefusedFiles[0].Path != "host/billing.py" {
		t.Fatalf("header refused %d %+v; want host/billing.py alone", r.Header.Refused, r.Header.RefusedFiles)
	}
	return r.Store, r.Header
}

// withheldMark is what Explain and the evidence file print in place of an
// absence while the index refused one file.
const withheldMark = "withheld: the index refused 1 file(s)"

// TestBrief_AnAbsenceOverARefusedFileIsWithheld (note (a) e2, form ii):
// over an index that refused a file, every reason that says the index
// holds no such thing may be false, since the refused file declares the
// thing; while the header counts a refused file, Explain and the evidence
// file print the withheld mark in place of each, one carrier class per
// case: a named word's reason (names.go), the claim's reason (claim.go),
// a pick's refusal (units.go), and the pronoun's refusal (roles.go). The
// precondition, the same brief with no refused count, shows the reason
// reached, so a case cannot pass by missing its site; the prompt brief is
// the same bytes either way, since it carries no reason. Break: print the
// reason whatever the count.
func TestBrief_AnAbsenceOverARefusedFileIsWithheld(t *testing.T) {
	g, h := refusedTree(t)
	rev := graph.RevCommit(h.Rev.Commit)
	for _, tc := range []struct {
		class, prompt string
		// reason is a fragment of the class A reason Explain prints;
		// evidence is the reason's class the evidence file prints, "" when
		// the reason reaches Explain's names line alone.
		reason, evidence string
	}{
		{"named word", "fix host/billing.py so refunds round down", "host/billing.py is not in the index", ""},
		{"claim reason", "speed up the refund ledger", "no unit names the target", "no unit names the target"},
		{"pick refusal", "speed up the invoices", "no unit names the head '", "no unit names the head '"},
		{"pronoun refusal", "make it faster", "' stands for a thing no clause names", "refused: '"},
	} {
		t.Run(tc.class, func(t *testing.T) {
			open := engine.Build(g, rev, tc.prompt, engine.Options{IDF: true})
			if !strings.Contains(open.Explain(), tc.reason) {
				t.Fatalf("precondition: with no refused count Explain does not carry %q:\n%s", tc.reason, open.Explain())
			}
			if tc.evidence != "" && !strings.Contains(open.Evidence(), tc.evidence) {
				t.Fatalf("precondition: with no refused count the evidence does not carry %q:\n%s", tc.evidence, open.Evidence())
			}
			b := engine.Build(g, rev, tc.prompt, engine.Options{IDF: true, Refused: h.Refused})
			if ex := b.Explain(); strings.Contains(ex, tc.reason) || !strings.Contains(ex, withheldMark) {
				t.Errorf("Explain over a refused file carries %q or lacks %q:\n%s", tc.reason, withheldMark, ex)
			}
			if ev := b.Evidence(); tc.evidence != "" && (strings.Contains(ev, tc.evidence) || !strings.Contains(ev, withheldMark)) {
				t.Errorf("the evidence over a refused file carries %q or lacks %q:\n%s", tc.evidence, withheldMark, ev)
			}
			if b.Text() != open.Text() {
				t.Errorf("the prompt brief changed with the refused count:\n%s\nwant\n%s", b.Text(), open.Text())
			}
		})
	}
}
