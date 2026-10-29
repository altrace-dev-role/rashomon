package disclose

import (
	"slices"
	"testing"
)

// TestTR1j_InventoryCarriesTheBuiltUnitsParagraphs: the inventory's unit
// paragraphs are the screens of the units a command in this release uses,
// bodies and share.outcomes, in that order, owned by TRUST and citing PF-SH1;
// none is an install screen paragraph, so the install text_sha256 and
// ScopeVersion do not move with them (card 4.2).
func TestTR1j_InventoryCarriesTheBuiltUnitsParagraphs(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var units []string
	for _, u := range inv.Units {
		units = append(units, u.Unit)
		if u.Owner != OwnerTrust || u.Cite != CitePFSH1 || len(u.Text) < 80 {
			t.Errorf("unit paragraph %+v: want owner %q, cite %q and its text", u, OwnerTrust, CitePFSH1)
		}
		if slices.ContainsFunc(inv.Paragraphs, func(p Paragraph) bool { return p.Text == u.Text }) {
			t.Errorf("the %s paragraph is on the install screen too", u.Unit)
		}
	}
	if !slices.Equal(units, []string{"bodies", "share.outcomes"}) {
		t.Errorf("unit paragraphs for %v, want bodies and share.outcomes", units)
	}
}
