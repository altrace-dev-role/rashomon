package consent

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// shortInput is a machine whose home directory is 40 characters, the
// longest TestCSH1 holds the ceiling for, at the default places.
func shortInput() ScreenInput {
	home := "/Users/" + strings.Repeat("h", 33)
	return ScreenInput{
		Version:      "1.2.0",
		SettingsPath: home + "/.claude/settings.json",
		Executable:   home + "/.local/bin/rashomon",
		InstallID:    "3f2a9c1e5b7d40a8c6e1f9b2d3a4c5e6",
		StoreRoot:    home + "/.local/state/rashomon",
	}
}

func renderShort(t *testing.T, in ScreenInput) string {
	t.Helper()
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Short(inv, in)
	if err != nil {
		t.Fatalf("Short: %v", err)
	}
	return string(b)
}

// TestCSH1_TheShortScreenFitsOneTerminal (CONSENT-SHORT1): the screen a yes
// is asked under is one default terminal, 24 rows of at most 80 columns,
// for a 40-character home with and without the CHANGED SINCE line, and
// stays within 80 columns with paths long enough to need cutting. On main
// it is the five sections, 146 lines (B260). Break: let a paragraph or a
// command join the screen past the ceiling, or raise MaxShortLines, and this
// fails.
func TestCSH1_TheShortScreenFitsOneTerminal(t *testing.T) {
	if MaxShortLines != 24 {
		t.Errorf("MaxShortLines is %d; the ceiling is a default terminal's 24 rows", MaxShortLines)
	}
	older := record(ViaWatchTTY, disclose.ScopeVersion-1, steer.DisclosureVersion-1, RecordScopeV4)
	older.BinaryVersion = "1.2.0-alpha.1"
	for name, prior := range map[string]*Record{"fresh": nil, "over an older scope": &older} {
		t.Run(name, func(t *testing.T) {
			in := shortInput()
			in.Prior = prior
			screen := renderShort(t, in)
			lines := strings.Split(strings.TrimSuffix(screen, "\n"), "\n")
			if len(lines) > MaxShortLines {
				t.Errorf("the screen is %d lines, past the ceiling of %d:\n%s", len(lines), MaxShortLines, screen)
			}
			for i, l := range lines {
				if n := utf8.RuneCountInString(l); n > Width {
					t.Errorf("line %d is %d columns: %q", i+1, n, l)
				}
			}
		})
	}
	in := testInput()
	in.Prior = &older
	for i, l := range strings.Split(renderShort(t, in), "\n") {
		if n := utf8.RuneCountInString(l); n > Width {
			t.Errorf("with long paths, line %d is %d columns: %q", i+1, n, l)
		}
	}
}

// TestCSH1_TheShortScreenSaysWhatAYesRecordsNeverStoresAndTurnsOff: the
// screen carries every CiteShort paragraph verbatim (what a yes records,
// what is never stored, the notes), names the settings file with its entry
// count and the record's place, lists each off switch and the --full
// pointer, renders the same bytes twice (text_sha256 is reproducible by
// --show), and opens with CHANGED SINCE only over an older scope. Every
// statement is the inventory's, so a short screen cannot drift from the
// five sections it sums up.
func TestCSH1_TheShortScreenSaysWhatAYesRecordsNeverStoresAndTurnsOff(t *testing.T) {
	in := shortInput()
	screen := renderShort(t, in)
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]bool{}
	for _, p := range inv.Paragraphs {
		if p.Cite != disclose.CiteShort {
			continue
		}
		owners[p.Owner] = true
		if !strings.Contains(flat(screen), flat(p.Text)) {
			t.Errorf("the screen does not carry %s's short paragraph: %q", p.Owner, p.Text)
		}
	}
	for _, o := range []string{"record", "verify", "steer"} {
		if !owners[o] {
			t.Errorf("the inventory has no short paragraph of %s's", o)
		}
	}
	for _, want := range []string{"RECORDED ON THIS MACHINE for each tool call", "NEVER STORED:", "NOTES TO YOUR AGENT",
		fmt.Sprintf("%s holds %d hook entries", in.SettingsPath, len(install.Events())), in.StoreRoot,
		"changes nothing until you type y."} {
		if !strings.Contains(flat(screen), want) {
			t.Errorf("the screen does not say %q:\n%s", want, screen)
		}
	}
	for _, c := range ShortCommands {
		if !strings.Contains(screen, indent+c.Does+" ") || !strings.Contains(screen, " "+c.Run+"\n") {
			t.Errorf("the screen does not list %q -> %q", c.Does, c.Run)
		}
	}
	for _, run := range []string{"rashomon steer off --user", "rashomon pause", "rashomon purge", "rashomon consent --full"} {
		found := false
		for _, c := range ShortCommands {
			found = found || c.Run == run
		}
		if !found {
			t.Errorf("the short screen's commands lack %q", run)
		}
	}
	if again := renderShort(t, in); again != screen {
		t.Error("the same input rendered two different short screens; --show could not reproduce text_sha256")
	}
	if strings.Contains(screen, "CHANGED SINCE") {
		t.Error("a fresh machine's short screen shows CHANGED SINCE")
	}

	alpha1 := record(ViaWatchTTY, 4, 2, RecordScopeV4)
	alpha1.BinaryVersion = "1.2.0-alpha.1"
	in.Prior = &alpha1
	want := fmt.Sprintf("CHANGED SINCE YOU LAST AGREED (1.2.0-alpha.1, scope 4): now scope %d.", disclose.ScopeVersion)
	if s := renderShort(t, in); !strings.Contains(flat(s), want) {
		t.Errorf("over alpha.1's consent the short screen does not say %q:\n%s", want, s)
	}
	current := record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5)
	in.Prior = &current
	if s := renderShort(t, in); strings.Contains(s, "CHANGED SINCE") {
		t.Error("a consent for this scope still shows CHANGED SINCE")
	}

	var kept []disclose.Paragraph
	for _, p := range inv.Paragraphs {
		if p.Cite != disclose.CiteShort {
			kept = append(kept, p)
		}
	}
	inv.Paragraphs = kept
	if _, err := Short(inv, shortInput()); err == nil {
		t.Error("a short screen rendered from an inventory with no short paragraphs")
	}
}

// TestCSH1_TheStoredListsSayWhereTheyAreKept: keeping is not reporting
// (owner, 11-01 10:4x PST). The heading over each list of what rashomon
// keeps says the list stays on this machine, so "stored" never reads as
// stored by someone else: the full screen's STORED paragraph and the short
// screen's RECORDED one. NEVER SENT BY RASHOMON, not NEVER SENT: the notes
// the list keeps word for word reach the model provider through Claude
// Code. Break: restore either bare heading.
func TestCSH1_TheStoredListsSayWhereTheyAreKept(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	heads := map[string]string{
		disclose.CiteSection2: "STORED ON THIS MACHINE, NEVER SENT BY RASHOMON: for each tool call, ",
		disclose.CiteShort:    "RECORDED ON THIS MACHINE for each tool call, ",
	}
	found := map[string]bool{}
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, "STORED:") || strings.HasPrefix(p.Text, "RECORDED for") {
			t.Errorf("a %s paragraph keeps a bare heading: %.60q", p.Cite, p.Text)
		}
		if want, ok := heads[p.Cite]; ok && p.Owner == disclose.OwnerRecord && strings.HasPrefix(p.Text, want) {
			found[p.Cite] = true
		}
	}
	for cite, want := range heads {
		if !found[cite] {
			t.Errorf("no record paragraph of %s opens %q", cite, want)
		}
	}
}

// TestCSH1_CoversIsTheTwoReadersOnRecordsInHand: Covers, which the plugin
// route asks before it writes a consent, answers what ScopeCovered and
// Current answer from the file, each a highest value on its own. Break: read
// only the latest record, or only one of the two numbers, and a row here
// disagrees with the readers the hooks use.
func TestCSH1_CoversIsTheTwoReadersOnRecordsInHand(t *testing.T) {
	cur, older := disclose.ScopeVersion, disclose.ScopeVersion-1
	disc, olderDisc := steer.DisclosureVersion, steer.DisclosureVersion-1
	unit := record(ViaConsentTTY, 0, 0, "")
	unit.Unit = Units[0]
	for _, tc := range []struct {
		name string
		recs []Record
		want bool
	}{
		{"no records", nil, false},
		{"this build's", []Record{record(ViaWatchTTY, cur, disc, RecordScopeV5)}, true},
		{"an older scope", []Record{record(ViaWatchTTY, older, disc, RecordScopeV4)}, false},
		{"an older disclosure", []Record{record(ViaWatchTTY, cur, olderDisc, RecordScopeV5)}, false},
		{"alpha.1's", []Record{record(ViaWatchTTY, 4, 2, RecordScopeV4)}, false},
		{"a unit answer alone", []Record{unit}, false},
		{"each number on its own record", []Record{
			record(ViaWatchTTY, cur, olderDisc, RecordScopeV5), record(ViaSteerOn, older, disc, RecordScopeV5)}, true},
		{"this build's, then an older one", []Record{
			record(ViaPluginEnable, cur, disc, RecordScopeV5), record(ViaWatchYes, older, olderDisc, RecordScopeV4)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Covers(tc.recs); got != tc.want {
				t.Errorf("Covers = %v, want %v", got, tc.want)
			}
			root := t.TempDir()
			for _, r := range tc.recs {
				if err := Append(root, r); err != nil {
					t.Fatal(err)
				}
			}
			scope, err := ScopeCovered(root)
			if err != nil {
				t.Fatal(err)
			}
			steerCur, err := Current(root)
			if err != nil {
				t.Fatal(err)
			}
			if Covers(tc.recs) != (scope && steerCur) {
				t.Errorf("Covers = %v, the readers say scope %v and steer %v", Covers(tc.recs), scope, steerCur)
			}
		})
	}
}
