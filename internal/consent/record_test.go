package consent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// record is a record every reader counts, for the tests to vary.
func record(via Via, scope, disclosure int, recordScope string) Record {
	return Record{
		V: RecordVersion, ScopeVersion: scope, RecordScope: recordScope,
		SteerDisclosure: disclosure, Steer: SteerOn, TextSHA256: strings.Repeat("a", 64),
		BinaryVersion: "test", BinarySHA256: strings.Repeat("b", 64), Via: via,
		InvocationID: strings.Repeat("c", 32), At: 1760000000000,
	}
}

type readers struct {
	current bool
	scope   string
	covered bool
}

func readAll(t *testing.T, root string) readers {
	t.Helper()
	cur, err := Current(root)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	scope, err := RecordScope(root)
	if err != nil {
		t.Fatalf("RecordScope: %v", err)
	}
	cov, err := ScopeCovered(root)
	if err != nil {
		t.Fatalf("ScopeCovered: %v", err)
	}
	return readers{cur, scope, cov}
}

func mustAppend(t *testing.T, root string, r Record) {
	t.Helper()
	if err := Append(root, r); err != nil {
		t.Fatalf("Append(%s): %v", r.Via, err)
	}
}

// TestTRU48_OneConsentFileThreeReaders: one consent.json, one record per
// line, read by STEER's Current, RECORD's RecordScope and TRUST's
// ScopeCovered; a missing file and a file with no line that parses are no
// consent at all three; a torn last line is skipped and the lines before it
// read (Technical spec 7.2.3).
func TestTRU48_OneConsentFileThreeReaders(t *testing.T) {
	none := readers{current: false, scope: RecordScopeV4, covered: false}

	t.Run("missing file is no consent", func(t *testing.T) {
		if got := readAll(t, t.TempDir()); got != none {
			t.Errorf("readers on no file = %+v, want %+v", got, none)
		}
	})

	t.Run("no line that parses is no consent", func(t *testing.T) {
		root := t.TempDir()
		body := "not json\n{\"v\":2,\"via\":\"watch_tty\",\"scope_version\":9,\"steer_disclosure\":9}\n" +
			"{\"v\":1,\"via\":\"by_hand\",\"scope_version\":9,\"steer_disclosure\":9}\n"
		if err := os.WriteFile(Path(root), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readAll(t, root); got != none {
			t.Errorf("readers on unparseable lines = %+v, want %+v", got, none)
		}
	})

	t.Run("one line per record, three readers", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchYes, disclose.ScopeVersion, 0, RecordScopeV5))
		if got, want := readAll(t, root), (readers{false, RecordScopeV5, true}); got != want {
			t.Errorf("after watch_yes = %+v, want %+v", got, want)
		}
		mustAppend(t, root, record(ViaFirstRunNotice, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		if got, want := readAll(t, root), (readers{true, RecordScopeV5, true}); got != want {
			t.Errorf("after first_run_notice = %+v, want %+v", got, want)
		}
		body, err := os.ReadFile(Path(root))
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(body, []byte{'\n'}); n != 2 {
			t.Errorf("two appends left %d lines:\n%s", n, body)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(Path(root))
			if err != nil {
				t.Fatal(err)
			}
			if mode := info.Mode().Perm(); mode != 0o600 {
				t.Errorf("consent.json mode %o, want 600", mode)
			}
		}
	})

	t.Run("an older scope does not cover this build", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion-1, steer.DisclosureVersion, RecordScopeV4))
		if got, want := readAll(t, root), (readers{true, RecordScopeV4, false}); got != want {
			t.Errorf("after an older-scope consent = %+v, want %+v", got, want)
		}
	})

	t.Run("a downgrade keeps consent; the latest record_scope wins", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		mustAppend(t, root, record(ViaUpgradeNotice, disclose.ScopeVersion-1, steer.DisclosureVersion-1, RecordScopeV4))
		if got, want := readAll(t, root), (readers{true, RecordScopeV4, true}); got != want {
			t.Errorf("after a lower record = %+v, want %+v", got, want)
		}
	})

	t.Run("a torn last line is skipped and the lines before it read", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		f, err := os.OpenFile(Path(root), os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(`{"v":1,"scope_version":3,"via":"upgr`); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if got, want := readAll(t, root), (readers{true, RecordScopeV5, true}); got != want {
			t.Errorf("with a torn last line = %+v, want %+v", got, want)
		}
		// The next append starts a line of its own rather than completing
		// the torn one, so it is read.
		mustAppend(t, root, record(ViaConsentTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV4))
		recs, err := Read(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 2 || recs[1].Via != ViaConsentTTY {
			t.Errorf("after the torn line, Read = %+v, want watch_tty then consent_tty", recs)
		}
	})

	t.Run("every via parses", func(t *testing.T) {
		root := t.TempDir()
		for _, v := range Vias {
			mustAppend(t, root, record(v, disclose.ScopeVersion, 0, RecordScopeV5))
		}
		recs, err := Read(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != len(Vias) {
			t.Fatalf("Read returned %d records for %d vias", len(recs), len(Vias))
		}
		for i, v := range Vias {
			if recs[i].Via != v {
				t.Errorf("record %d via %q, want %q", i, recs[i].Via, v)
			}
		}
		if err := Append(root, record("by_hand", 3, 0, RecordScopeV5)); err == nil {
			t.Error("Append accepted a via outside the closed set")
		}
	})

	t.Run("an unreadable file is no consent and says why", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(Path(root), 0o700); err != nil {
			t.Fatal(err)
		}
		if ok, err := Current(root); ok || err == nil {
			t.Errorf("Current on a directory = %v, %v; want false and an error", ok, err)
		}
		if ok, err := ScopeCovered(root); ok || err == nil {
			t.Errorf("ScopeCovered on a directory = %v, %v; want false and an error", ok, err)
		}
		if s, err := RecordScope(root); s != RecordScopeV4 || err == nil {
			t.Errorf("RecordScope on a directory = %q, %v; want v4 and an error", s, err)
		}
	})
}

// TestTRU48_CompactionKeepsEveryReadersAnswer: at the 64 KiB cap the file is
// compacted to the latest record per via and per scope version, by temporary
// file, sync and rename; every reader answers as it did before, and nothing
// is left beside the file.
func TestTRU48_CompactionKeepsEveryReadersAnswer(t *testing.T) {
	root := t.TempDir()
	// The highest disclosure comes first, so only the rule that keeps it
	// stops a later record of the same via and scope pushing it out.
	mustAppend(t, root, record(ViaFirstRunNotice, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion-1, 0, RecordScopeV4))
	for i := 0; ; i++ {
		info, err := os.Stat(Path(root))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > CapBytes-1024 {
			break
		}
		r := record(ViaFirstRunNotice, disclose.ScopeVersion, 0, RecordScopeV5)
		r.At = int64(i)
		mustAppend(t, root, r)
	}
	before := readAll(t, root)

	final := record(ViaConsentTTY, disclose.ScopeVersion, 0, RecordScopeV5)
	for i := 0; i < 8; i++ {
		final.At = int64(1<<40 + i)
		mustAppend(t, root, final)
	}
	wantAt := final.At

	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= CapBytes/2 {
		t.Fatalf("consent.json is %d bytes after passing the cap; it was not compacted", info.Size())
	}
	if got := readAll(t, root); got != before {
		t.Errorf("readers after compaction = %+v, want %+v as before", got, before)
	}
	recs, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	olderWatch := 0
	for _, r := range recs {
		if r.Via == ViaWatchTTY && r.ScopeVersion == disclose.ScopeVersion-1 {
			olderWatch++
		}
	}
	if olderWatch != 1 {
		t.Errorf("compaction kept %d watch_tty records of the older scope, want its one: %+v", olderWatch, recs)
	}
	newest := recs[len(recs)-1]
	if newest.Via != ViaConsentTTY || newest.At != wantAt {
		t.Errorf("the newest record after compaction is %+v, want the final consent_tty", newest)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if !slices.Equal(left, []string{store.FileConsent, store.FileRecordScope}) {
		t.Errorf("store root after compaction holds %q, want consent.json and its record-scope marker alone", left)
	}
}

// TestTRU48_DisclosureVersionIsPinned: a bump holds every note until the
// user is shown the new text, so it is deliberate.
func TestTRU48_DisclosureVersionIsPinned(t *testing.T) {
	if steer.DisclosureVersion != 2 {
		t.Errorf("steer.DisclosureVersion = %d; a bump holds every note and must be deliberate", steer.DisclosureVersion)
	}
}

// TestTRU48_RecordCarriesTheSpecsKeys: a record marshals to exactly the keys
// Technical spec 7.2.3 lists, and a settings install's plugin option is null.
func TestTRU48_RecordCarriesTheSpecsKeys(t *testing.T) {
	b, err := json.Marshal(record(ViaWatchTTY, 3, 2, RecordScopeV5))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatal(err)
	}
	want := []string{"v", "scope_version", "record_scope", "steer_disclosure", "steer", "text_sha256",
		"binary_version", "binary_sha256", "via", "tty", "plugin_option_steering", "invocation_id", "at"}
	if len(keys) != len(want) {
		t.Errorf("record has %d keys, want %d: %s", len(keys), len(want), b)
	}
	for _, k := range want {
		if _, ok := keys[k]; !ok {
			t.Errorf("record lacks %q: %s", k, b)
		}
	}
	if string(keys["plugin_option_steering"]) != "null" {
		t.Errorf("plugin_option_steering = %s, want null", keys["plugin_option_steering"])
	}
}

// TestTRU23_ConsentWritesAreInTheInventory: every path this package writes
// under a store root is described by a FileRule in disclose.Default().Files.
func TestTRU23_ConsentWritesAreInTheInventory(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
	if err := WriteSDKAnswer(root, true); err != nil {
		t.Fatal(err)
	}
	// A replacement's temporary file exists only between its write and its
	// rename; create one where compact, WriteSDKAnswer and the record-scope
	// marker's write would, to check their rules.
	for _, path := range []string{Path(root), HarnessesPath(root), filepath.Join(root, store.FileRecordScope)} {
		tmp, err := os.CreateTemp(root, "."+filepath.Base(path)+".tmp-*")
		if err != nil {
			t.Fatal(err)
		}
		if err := tmp.Close(); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 6 {
		t.Fatal("premise: the test wrote fewer files than it checks")
	}
	for _, e := range entries {
		if !rootRuleCovers(e.Name(), inv.Files) {
			t.Errorf("consent wrote %q, which disclose.Default().Files does not cover", e.Name())
		}
	}
}

// rootRuleCovers reports whether a store-root file name matches a FileRule
// whose base is the root itself.
func rootRuleCovers(name string, rules []disclose.FileRule) bool {
	for _, r := range rules {
		if r.Base != "" {
			continue
		}
		if ok, _ := filepath.Match(r.Pattern, name); ok {
			return true
		}
	}
	return false
}
