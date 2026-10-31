package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestLU1_TheBitIsOmittedWhenFalse (5.14b): location_unread is the one key of
// a call record that is absent where its fact is false, so a call whose
// location was read costs no byte, and a record before schema 9, which has no
// such key, reads false. Break: drop omitempty, and every declaration grows by
// a key; or rename the key, and the report reads none.
func TestLU1_TheBitIsOmittedWhenFalse(t *testing.T) {
	plain, err := json.Marshal(Declaration{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "location_unread") {
		t.Errorf("a declaration whose location was read carries the key: %s", plain)
	}
	unread, err := json.Marshal(Declaration{LocationUnread: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unread), `"location_unread":true`) {
		t.Errorf("an unread location is not recorded: %s", unread)
	}
	var back Declaration
	if err := json.Unmarshal(unread, &back); err != nil || !back.LocationUnread {
		t.Errorf("the bit does not read back: %v, %+v", err, back.LocationUnread)
	}
	var old Declaration
	if err := json.Unmarshal(plain, &old); err != nil || old.LocationUnread {
		t.Errorf("a record with no key reads %v, %v; want false", old.LocationUnread, err)
	}
}
