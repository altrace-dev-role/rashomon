package render

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

var callerNames = []slot.Ident{
	"shop.billing.checkout.place_order",
	"shop.billing.refund.issue",
	"shop.reports.monthly.build",
	"shop.tasks.nightly.close_books",
	"shop.api.views.InvoiceView.get",
}

const staleClause = "; the graph does not hold the present content of the edited file."

// The texts at 1, 2 and 5 names and at 5 names with 2 more, fresh and
// stale, with their byte lengths counted from the accepted texts.
func TestBlastCallersRender_TextsAndLengths(t *testing.T) {
	cases := []struct {
		n, more     int
		fresh, stal int
	}{
		{1, 0, 144, 208},
		{2, 0, 173, 237},
		{5, 0, 271, 335},
		{5, 2, 282, 346},
	}
	for _, c := range cases {
		list := slot.NewIdentList(callerNames[:c.n], c.more)
		m, err := New(pattern.BlastRadius, BlastCallersBinding{Callers: list, Function: "shop.billing.invoice.total"})
		if err != nil {
			t.Fatalf("%d names, %d more: New = %v", c.n, c.more, err)
		}
		s, err := New(pattern.BlastRadius, BlastCallersStaleBinding{Callers: list, Function: "shop.billing.invoice.total"})
		if err != nil {
			t.Fatalf("%d names, %d more, stale: New = %v", c.n, c.more, err)
		}
		if m.Template() != "blast-callers/v1" || s.Template() != "blast-callers-stale/v1" {
			t.Errorf("Template = %q, %q", m.Template(), s.Template())
		}
		if len(m.Text()) != c.fresh || len(s.Text()) != c.stal {
			t.Errorf("%d names, %d more: %d and %d bytes, want %d and %d", c.n, c.more, len(m.Text()), len(s.Text()), c.fresh, c.stal)
		}
		if want := strings.TrimSuffix(m.Text(), ".") + staleClause; s.Text() != want {
			t.Errorf("stale text = %q, want %q", s.Text(), want)
		}
	}
	m, err := New(pattern.BlastRadius, BlastCallersBinding{
		Callers:  slot.NewIdentList(callerNames[:1], 0),
		Function: "shop.billing.invoice.total",
	})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	want := "rashomon record: the repository graph names 'shop.billing.checkout.place_order' as calling " +
		"'shop.billing.invoice.total', which this call edited."
	if m.Text() != want {
		t.Errorf("Text() = %q, want %q", m.Text(), want)
	}
}

// The record rows hold two counts and no name.
func TestBlastCallersRender_RecordHoldsCountsOnly(t *testing.T) {
	r, err := New(pattern.BlastRadius, BlastCallersRecordBinding{Listed: 5, More: 2})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	s, err := New(pattern.BlastRadius, BlastCallersStaleRecordBinding{Listed: 5, More: 2})
	if err != nil {
		t.Fatalf("New, stale = %v", err)
	}
	want := "rashomon record: the repository graph names callers of the function this call edited: 5 listed, 2 more."
	if r.Text() != want || r.Template() != "blast-callers-record/v1" {
		t.Errorf("record = %q (%q), want %q", r.Text(), r.Template(), want)
	}
	if w := strings.TrimSuffix(want, ".") + staleClause; s.Text() != w || s.Template() != "blast-callers-stale-record/v1" {
		t.Errorf("stale record = %q (%q), want %q", s.Text(), s.Template(), w)
	}
	for _, m := range []Message{r, s} {
		for _, sl := range m.Slots() {
			if _, ok := sl.Value.(slot.Count); !ok {
				t.Errorf("%s: slot %q holds %T, want a count", m.Template(), sl.Name, sl.Value)
			}
		}
	}
}

// Every row passes the statement check, and a list that fails its own
// check refuses the note.
func TestBlastCallersRender_RowsAreStatementsAndABadListRefuses(t *testing.T) {
	rows := templatesBlastCallers()
	if len(rows) != 4 {
		t.Fatalf("%d rows, want 4", len(rows))
	}
	for k, row := range rows {
		if k.pattern != pattern.BlastRadius {
			t.Errorf("%s: pattern %q", row.id, k.pattern)
		}
		if err := CheckTemplate(row.format); err != nil {
			t.Errorf("%s: %v", row.id, err)
		}
	}
	if _, err := New(pattern.BlastRadius, BlastCallersBinding{
		Callers:  slot.NewIdentList(nil, 3),
		Function: "shop.billing.invoice.total",
	}); err == nil {
		t.Error("a list with no name rendered")
	}
}
