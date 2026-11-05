package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestLA1_TheLaunchBitIsOmittedWhenFalse (LAUNCH-ANY1): launch is absent
// where its fact is false, as location_unread is, so a line that launches
// nothing costs no byte, and a record before schema 10, which has no such
// key, reads false; the shape itself never carries it. Break: drop
// omitempty, rename the key, or write the shape's copy.
func TestLA1_TheLaunchBitIsOmittedWhenFalse(t *testing.T) {
	plain, err := json.Marshal(Declaration{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "launch") {
		t.Errorf("a declaration that launches nothing carries the key: %s", plain)
	}
	d := Declaration{Launch: true}
	d.Shape.Launch = true
	launched, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(launched), `"launch":true`) != 1 {
		t.Errorf("a launch is not recorded once, as the declaration's key: %s", launched)
	}
	var back Declaration
	if err := json.Unmarshal(launched, &back); err != nil || !back.Launch {
		t.Errorf("the bit does not read back: %v, %+v", err, back.Launch)
	}
	if LaunchSchemaVersion != 10 {
		t.Errorf("LaunchSchemaVersion %d: the bit landed at 10", LaunchSchemaVersion)
	}
	if CallSchemaVersion != LaunchSchemaVersion {
		t.Errorf("CallSchemaVersion %d: the writer stamps the bit's version, %d", CallSchemaVersion, LaunchSchemaVersion)
	}
}
