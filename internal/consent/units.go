package consent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// UnitRecord is the unit every install grants: recording (PF-TR3).
const UnitRecord = "record"

// Units are the consent units beyond recording (PRD PF-SH1, card 4.2), each
// off until the user answers its own screen, `rashomon consent <unit>`. A
// unit with no paragraph in the inventory has no command in this release and
// cannot be granted.
var Units = []string{"bodies", "share.outcomes", "share.lessons", "share.bodies"}

// Has reports whether root's consent on file grants unit. UnitRecord is
// granted by any install record, which every install writes. Any other unit
// is its latest record in file order, the order the ledger is appended in, so
// the user's last answer stands whatever the clock said when it was given:
// off after a grant withdraws it and a grant after off restores it. A unit
// with no record is off; a name that is no unit is an error, never false.
func Has(root, unit string) (bool, error) {
	if unit != UnitRecord && !slices.Contains(Units, unit) {
		return false, fmt.Errorf("consent: %q is no unit; the units are %s", unit, strings.Join(Units, ", "))
	}
	recs, err := Read(root)
	if err != nil {
		return false, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		switch r := recs[i]; {
		case unit == UnitRecord && r.Unit == "":
			return true, nil
		case r.Unit == unit:
			return r.UnitOn, nil
		}
	}
	return false, nil
}

// UnitScreen renders one consent unit's screen: the install screen's opening
// line, the unit's name and its paragraph from the inventory, every line at
// most Width characters. A unit record's text_sha256 hashes these bytes; the
// install screen and its text_sha256 do not include them.
func UnitScreen(inv disclose.Inventory, unit string, in ScreenInput) ([]byte, error) {
	i := slices.IndexFunc(inv.Units, func(u disclose.UnitParagraph) bool { return u.Unit == unit })
	if i < 0 {
		return nil, fmt.Errorf("consent: the inventory has no paragraph for the unit %q", unit)
	}
	var w screenWriter
	w.wrap("", "", "rashomon "+in.Version+" asks before it changes anything. Nothing below is written until you type y.")
	w.blank()
	w.line(strings.ToUpper(unit))
	w.wrap(indent, indent, inv.Units[i].Text)
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}
