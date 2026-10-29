package consent

import (
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// unitRecord is a unit's answer as `rashomon consent <unit>` appends it: no
// scope, no record scope, no steer disclosure, at ms.
func unitRecord(unit string, on bool, at int64) Record {
	r := record(ViaConsentYes, 0, 0, "")
	r.Steer, r.Unit, r.UnitOn, r.At = "", unit, on, at
	return r
}

func has(t *testing.T, root, unit string) bool {
	t.Helper()
	ok, err := Has(root, unit)
	if err != nil {
		t.Fatalf("Has(%q): %v", unit, err)
	}
	return ok
}

// TestTR1b_RecordIsConsentedByInstall: an install record alone, a --yes one
// at steer_disclosure 0 among them, grants record and no other unit; a name
// that is no unit is an error, never false.
func TestTR1b_RecordIsConsentedByInstall(t *testing.T) {
	root := t.TempDir()
	if has(t, root, UnitRecord) {
		t.Error("no consent on file grants record")
	}
	mustAppend(t, root, record(ViaWatchYes, disclose.ScopeVersion, 0, RecordScopeV5))
	if !has(t, root, UnitRecord) {
		t.Error("a watch --yes record does not grant record")
	}
	for _, u := range Units {
		if has(t, root, u) {
			t.Errorf("an install record grants %s", u)
		}
	}
	for _, bad := range []string{"", "share", "Bodies", "share.outcome"} {
		if ok, err := Has(root, bad); err == nil || ok {
			t.Errorf("Has(%q) = %v, %v; want an error", bad, ok, err)
		}
	}
}

// TestTR1c_OtherUnitsAreOffUntilAsked: a bodies grant grants bodies and
// nothing else, record included only by the install record before it.
func TestTR1c_OtherUnitsAreOffUntilAsked(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, 2, RecordScopeV5))
	mustAppend(t, root, unitRecord("bodies", true, 2))
	if !has(t, root, "bodies") {
		t.Error("a bodies grant does not grant bodies")
	}
	for _, u := range []string{"share.outcomes", "share.lessons", "share.bodies"} {
		if has(t, root, u) {
			t.Errorf("a bodies grant grants %s", u)
		}
	}
	if !has(t, root, UnitRecord) {
		t.Error("a unit record hid the install record's record grant")
	}
}

// TestTR1d_UnitOffIsTheLatestWord: grant, off, grant reads true, false,
// true; and the latest record in the file is the latest word even when its
// clock reads earlier than the one before it.
func TestTR1d_UnitOffIsTheLatestWord(t *testing.T) {
	root := t.TempDir()
	for i, on := range []bool{true, false, true} {
		mustAppend(t, root, unitRecord("share.outcomes", on, int64(10+i)))
		if got := has(t, root, "share.outcomes"); got != on {
			t.Errorf("after answer %d (%v): Has = %v", i+1, on, got)
		}
	}
	mustAppend(t, root, unitRecord("share.outcomes", false, 1))
	if has(t, root, "share.outcomes") {
		t.Error("an off appended last, with an earlier clock, did not withdraw the grant")
	}
}

// TestTR1g_CompactionKeepsEachUnitsLatest: two units answered by the same
// via both keep their latest answer through compaction at the cap.
func TestTR1g_CompactionKeepsEachUnitsLatest(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, 2, RecordScopeV5))
	mustAppend(t, root, unitRecord("bodies", true, 2))
	mustAppend(t, root, unitRecord("share.outcomes", true, 3))
	for i, grew := 0, false; ; i++ {
		info, err := os.Stat(Path(root))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > CapBytes {
			t.Fatal("consent.json passed the cap without compacting")
		}
		if grew && info.Size() < 4096 {
			break // compacted
		}
		grew = grew || info.Size() > CapBytes/2
		r := record(ViaFirstRunNotice, disclose.ScopeVersion, 0, RecordScopeV5)
		r.At = int64(100 + i)
		mustAppend(t, root, r)
	}
	if !has(t, root, "bodies") || !has(t, root, "share.outcomes") {
		recs, _ := Read(root)
		t.Errorf("after compaction: bodies %v, share.outcomes %v; want both granted: %+v",
			has(t, root, "bodies"), has(t, root, "share.outcomes"), recs)
	}
}

// TestTR1h_UnitAnswerIsNoInstallConsent: a unit record claims nothing the
// install readers read. One that names a scope, a record scope or a steer
// disclosure does not parse, so Append refuses it; a valid one after an
// install consent of an older scope leaves ScopeCovered false and the prior
// the install record, so watch and consent still show what changed.
func TestTR1h_UnitAnswerIsNoInstallConsent(t *testing.T) {
	root := t.TempDir()
	for name, r := range map[string]Record{
		"scope":            func() Record { r := unitRecord("bodies", true, 1); r.ScopeVersion = disclose.ScopeVersion; return r }(),
		"record scope":     func() Record { r := unitRecord("bodies", true, 1); r.RecordScope = RecordScopeV5; return r }(),
		"steer disclosure": func() Record { r := unitRecord("bodies", true, 1); r.SteerDisclosure = 2; return r }(),
		"unknown unit":     unitRecord("share", true, 1),
	} {
		if err := Append(root, r); err == nil {
			t.Errorf("Append took a unit record naming a %s", name)
		}
	}
	if _, err := os.Stat(Path(root)); !os.IsNotExist(err) {
		t.Fatalf("a refused unit record left consent.json behind (%v)", err)
	}
	older := record(ViaWatchTTY, disclose.ScopeVersion-1, 2, RecordScopeV5)
	mustAppend(t, root, older)
	mustAppend(t, root, unitRecord("bodies", true, older.At+1))
	if got := readAll(t, root); got.covered {
		t.Error("a unit answer covered the install scope the user was not shown")
	}
	recs, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if prior, ok := LastAccepted(recs); !ok || prior.Unit != "" || prior.ScopeVersion != older.ScopeVersion {
		t.Errorf("LastAccepted = %+v, %v; want the older install record", prior, ok)
	}
}

// TestTR1i_UnitScreenShowsItsParagraph: a built unit's screen is the opening
// line, the unit's name and its paragraph, every line within Width; the two
// share units with no builder have no screen.
func TestTR1i_UnitScreenShowsItsParagraph(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"bodies", "share.outcomes"} {
		b, err := UnitScreen(inv, u, ScreenInput{Version: "test"})
		if err != nil {
			t.Fatalf("UnitScreen(%s): %v", u, err)
		}
		s := string(b)
		if !strings.HasPrefix(s, "rashomon test asks before it changes anything.") || !strings.Contains(s, "\n"+strings.ToUpper(u)+"\n") {
			t.Errorf("%s's screen does not open as the install screen does and name the unit:\n%s", u, s)
		}
		for _, line := range strings.Split(s, "\n") {
			if n := len([]rune(line)); n > Width {
				t.Errorf("%s's screen has a line of %d characters: %q", u, n, line)
			}
		}
		if strings.Contains(s, "1. HOOK ENTRIES") {
			t.Errorf("%s's screen carries the install screen:\n%s", u, s)
		}
	}
	for _, u := range []string{"share.lessons", "share.bodies"} {
		if _, err := UnitScreen(inv, u, ScreenInput{Version: "test"}); err == nil {
			t.Errorf("UnitScreen(%s) rendered a screen for a unit nothing builds", u)
		}
	}
}
