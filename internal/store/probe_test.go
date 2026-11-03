package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestH52_TheProbeBitIsOmittedWhenFalse (issue #52): probe is absent where
// its fact is false, as launch is, so a line that is no probe costs no byte,
// and a record before schema 10, which has no such key, reads false and its
// probe counts as any failure does; the shape itself never carries it.
// Break: drop omitempty, rename the key, or write the shape's copy.
func TestH52_TheProbeBitIsOmittedWhenFalse(t *testing.T) {
	plain, err := json.Marshal(Declaration{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "probe") {
		t.Errorf("a declaration that is no probe carries the key: %s", plain)
	}
	d := Declaration{Probe: true}
	d.Shape.Probe = true
	probed, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(probed), `"probe":true`) != 1 {
		t.Errorf("a probe is not recorded once, as the declaration's key: %s", probed)
	}
	var back Declaration
	if err := json.Unmarshal(probed, &back); err != nil || !back.Probe {
		t.Errorf("the bit does not read back: %v, %+v", err, back.Probe)
	}
}
