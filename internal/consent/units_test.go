package consent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// unitRecord is the record `rashomon consent <unit>` appends.
func unitRecord(u Unit) Record {
	r := record(ViaUnitTTY, 0, 0, "")
	r.Unit, r.TTY = u, true
	return r
}

func has(t *testing.T, root string, u Unit) bool {
	t.Helper()
	ok, err := Has(root, u)
	if err != nil {
		t.Fatalf("Has(%s): %v", u, err)
	}
	return ok
}

// TestTR1b_RecordIsConsentedByInstall: an act of consent that shows the
// install screen -- watch, consent, a plugin enable -- grants record; a notice
// or a steer switch does not, and no file is no consent. The via rows each
// have a value the rule accepts and one it refuses.
func TestTR1b_RecordIsConsentedByInstall(t *testing.T) {
	for _, tc := range []struct {
		via  Via
		want bool
	}{
		{ViaWatchTTY, true}, {ViaWatchYes, true}, {ViaConsentTTY, true},
		{ViaConsentYes, true}, {ViaPluginEnable, true},
		{ViaFirstRunNotice, false}, {ViaUpgradeNotice, false}, {ViaSteerOn, false},
	} {
		t.Run(string(tc.via), func(t *testing.T) {
			root := t.TempDir()
			mustAppend(t, root, record(tc.via, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
			if got := has(t, root, UnitRecord); got != tc.want {
				t.Errorf("Has(record) after one %s record = %v, want %v", tc.via, got, tc.want)
			}
		})
	}

	t.Run("no file", func(t *testing.T) {
		if has(t, t.TempDir(), UnitRecord) {
			t.Error("Has(record) with no consent.json = true")
		}
	})

	// v-compat: a consent.json written before units existed, byte for byte
	// as that build wrote it (thirteen keys, no unit), still reads as record
	// granted, and grants nothing else.
	t.Run("a pre-units consent.json", func(t *testing.T) {
		root := t.TempDir()
		old := `{"v":1,"scope_version":3,"record_scope":"v5","steer_disclosure":2,"steer":"on","text_sha256":"` +
			strings.Repeat("a", 64) + `","binary_version":"0.9.0","binary_sha256":"` + strings.Repeat("b", 64) +
			`","via":"watch_tty","tty":true,"plugin_option_steering":null,"invocation_id":"` +
			strings.Repeat("c", 32) + `","at":1760000000000}` + "\n"
		if err := os.WriteFile(Path(root), []byte(old), 0o600); err != nil {
			t.Fatal(err)
		}
		if !has(t, root, UnitRecord) {
			t.Error("a consent.json from before units does not read as record granted")
		}
		for _, u := range Units[1:] {
			if has(t, root, u) {
				t.Errorf("a consent.json from before units grants %s", u)
			}
		}
	})

	t.Run("an unreadable file grants nothing and says why", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(Path(root), 0o700); err != nil {
			t.Fatal(err)
		}
		if ok, err := Has(root, UnitRecord); ok || err == nil {
			t.Errorf("Has on a directory = %v, %v; want false and an error", ok, err)
		}
	})

	t.Run("a unit outside the closed set is refused", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		if ok, err := Has(root, Unit("share.everything")); ok || err == nil {
			t.Errorf("Has(unknown unit) = %v, %v; want false and an error", ok, err)
		}
	})
}

// TestTR1c_OtherUnitsAreOffUntilAsked: after an install every unit but record
// is off; a unit's own grant turns on that unit and no other; a grant with no
// install under it grants nothing.
func TestTR1c_OtherUnitsAreOffUntilAsked(t *testing.T) {
	others := Units[1:]
	if len(others) != 4 {
		t.Fatalf("premise: %d units besides record, want 4", len(others))
	}

	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
	for _, u := range others {
		if has(t, root, u) {
			t.Errorf("Has(%s) after install alone = true; every unit but record is off until asked", u)
		}
	}

	for _, asked := range others {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		mustAppend(t, root, unitRecord(asked))
		for _, u := range Units {
			want := u == asked || u == UnitRecord
			if got := has(t, root, u); got != want {
				t.Errorf("after consent %s: Has(%s) = %v, want %v", asked, u, got, want)
			}
		}
	}

	t.Run("a grant with no install under it grants nothing", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, unitRecord(UnitBodies))
		if has(t, root, UnitBodies) || has(t, root, UnitRecord) {
			t.Error("a unit grant with no install on file turned something on")
		}
	})

	t.Run("a unit on a record that is not a unit grant grants nothing", func(t *testing.T) {
		root := t.TempDir()
		r := record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5)
		r.Unit = UnitBodies
		if err := Append(root, r); err == nil {
			t.Error("Append accepted a unit on a record that is not a unit grant")
		}
		// The same line put in the file by hand is skipped like a torn one.
		line, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(Path(root), append(line, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if has(t, root, UnitBodies) || has(t, root, UnitRecord) {
			t.Error("a hand-edited unit on a watch record turned something on")
		}
	})
}

// TestTR1c_UnitRecordsAreRefusedUnlessWellFormed: a unit grant names one unit
// of the closed set other than record, and was answered at a terminal.
func TestTR1c_UnitRecordsAreRefusedUnlessWellFormed(t *testing.T) {
	good := unitRecord(UnitShareOutcomes)
	if err := Append(t.TempDir(), good); err != nil {
		t.Fatalf("a well-formed unit grant was refused: %v", err)
	}
	for name, mut := range map[string]func(*Record){
		"no unit":      func(r *Record) { r.Unit = "" },
		"unknown unit": func(r *Record) { r.Unit = "share.everything" },
		"record unit":  func(r *Record) { r.Unit = UnitRecord },
		"no terminal":  func(r *Record) { r.TTY = false },
	} {
		r := good
		mut(&r)
		if err := Append(t.TempDir(), r); err == nil {
			t.Errorf("Append accepted a unit grant with %s", name)
		}
	}
}

// TestTR1c_UnitGrantsDoNotMoveTheOtherReaders: a unit grant is not a consent
// to the screen, so it does not make the scope covered, raise the steer
// disclosure, change the record scope, or become the record CHANGED SINCE
// compares against.
func TestTR1c_UnitGrantsDoNotMoveTheOtherReaders(t *testing.T) {
	root := t.TempDir()
	watch := record(ViaWatchTTY, disclose.ScopeVersion-1, 0, RecordScopeV4)
	mustAppend(t, root, watch)
	before := readAll(t, root)
	mustAppend(t, root, unitRecord(UnitBodies))
	if after := readAll(t, root); after != before {
		t.Errorf("readers after a unit grant = %+v, want %+v", after, before)
	}
	recs, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	last, ok := LastAccepted(recs)
	if !ok || last.Via != ViaWatchTTY {
		t.Errorf("LastAccepted after a unit grant = %+v, %v; want the watch record", last, ok)
	}
}

// TestTR1c_CompactionKeepsEveryUnitGrant: compaction keeps the latest record
// per (via, scope_version) and so would keep one unit grant of several; it
// must keep the latest grant of each unit, or compaction would turn units off.
func TestTR1c_CompactionKeepsEveryUnitGrant(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
	mustAppend(t, root, unitRecord(UnitBodies))
	mustAppend(t, root, unitRecord(UnitShareOutcomes))
	mustAppend(t, root, unitRecord(UnitBodies))
	if err := compact(root); err != nil {
		t.Fatal(err)
	}
	for _, u := range []Unit{UnitRecord, UnitBodies, UnitShareOutcomes} {
		if !has(t, root, u) {
			t.Errorf("after compaction Has(%s) = false", u)
		}
	}
	for _, u := range []Unit{UnitShareLessons, UnitShareBodies} {
		if has(t, root, u) {
			t.Errorf("after compaction Has(%s) = true, never asked", u)
		}
	}
}

// TestTR1c_EveryUnitButRecordHasANotice: the first-run notice for a unit is
// closed text, fits the screen width, and is empty for record and for a name
// outside the closed set.
func TestTR1c_EveryUnitButRecordHasANotice(t *testing.T) {
	if Notice(UnitRecord) != "" || Notice(Unit("nope")) != "" {
		t.Error("Notice for record or an unknown unit is not empty")
	}
	seen := map[string]Unit{}
	for _, u := range Units[1:] {
		n := Notice(u)
		if n == "" {
			t.Errorf("Notice(%s) is empty", u)
			continue
		}
		if prev, dup := seen[n]; dup {
			t.Errorf("Notice(%s) equals Notice(%s)", u, prev)
		}
		seen[n] = u
		if !strings.HasSuffix(n, "\n") || !strings.Contains(n, "rashomon consent "+string(u)) {
			t.Errorf("Notice(%s) does not end in a line feed or name its unit:\n%s", u, n)
		}
		for _, line := range strings.Split(strings.TrimSuffix(n, "\n"), "\n") {
			if c := utf8.RuneCountInString(line); c > Width {
				t.Errorf("Notice(%s) has a line of %d characters: %q", u, c, line)
			}
		}
	}
}

// TestTR1c_ParseUnitIsExact: the names the plan gives parse; case, spacing
// and prefixes do not.
func TestTR1c_ParseUnitIsExact(t *testing.T) {
	for _, s := range []string{"record", "bodies", "share.outcomes", "share.lessons", "share.bodies"} {
		if u, ok := ParseUnit(s); !ok || string(u) != s {
			t.Errorf("ParseUnit(%q) = %q, %v", s, u, ok)
		}
	}
	for _, s := range []string{"", "Bodies", "share", "share.", "record ", "share.outcomes\n", "share/outcomes"} {
		if _, ok := ParseUnit(s); ok {
			t.Errorf("ParseUnit(%q) parsed", s)
		}
	}
}

// TestTR1b_PreUnitsRecordsMarshalAsTheyAlwaysDid: a record that grants no unit
// carries no unit key, so the bytes of every record written before units
// existed, and of every watch or consent record since, are unchanged; a unit
// grant carries one.
func TestTR1b_PreUnitsRecordsMarshalAsTheyAlwaysDid(t *testing.T) {
	plain, err := json.Marshal(record(ViaWatchTTY, 3, 2, RecordScopeV5))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), `"unit"`) {
		t.Errorf("a watch record carries a unit key: %s", plain)
	}
	grant, err := json.Marshal(unitRecord(UnitBodies))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(grant), `"unit":"bodies"`) {
		t.Errorf("a unit grant lacks its unit key: %s", grant)
	}
}
