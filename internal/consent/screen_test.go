package consent

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

var sectionHeadings = []string{
	"1. HOOK ENTRIES",
	"2. WHAT IS RECORDED",
	"3. WHAT RASHOMON ADDS TO YOUR AGENT'S CONTEXT",
	"4. WHAT RASHOMON LEARNS",
	"5. WHAT YOU WILL SEE",
}

func testInput() ScreenInput {
	long := "/Users/someone/with/a/rather/long/home/directory/that/goes/on/and/on"
	return ScreenInput{
		Version:      "1.2.0",
		SettingsPath: long + "/.claude/settings.json",
		Executable:   long + "/.local/bin/rashomon-with-a-name-long-enough-to-need-cutting-at-the-width",
		InstallID:    "3f2a9c1e5b7d40a8c6e1f9b2d3a4c5e6",
		StoreRoot:    long + "/.local/state/rashomon",
	}
}

func render(t *testing.T, in ScreenInput) string {
	t.Helper()
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Screen(inv, in)
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	return string(b)
}

// flat is s with every run of white space one space, so a paragraph wrapped
// at any width compares equal to its inventory text.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

// sectionOf returns the text of section n (1-based) of a rendered screen.
func sectionOf(t *testing.T, screen string, n int) string {
	t.Helper()
	start := strings.Index(screen, sectionHeadings[n-1])
	if start < 0 {
		t.Fatalf("screen has no %q heading", sectionHeadings[n-1])
	}
	end := len(screen)
	if n < len(sectionHeadings) {
		if e := strings.Index(screen, sectionHeadings[n]); e > start {
			end = e
		}
	}
	return screen[start:end]
}

// TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty: the screen
// is rendered from the inventory: five sections in order, every installed
// event and its time limit named in section 1, every layer paragraph carried
// verbatim into the section it cites, the seven commands at the end, and no
// line past 80 characters -- with paths long enough to need cutting
// (Technical spec 7.2.3).
func TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty(t *testing.T) {
	in := testInput()
	screen := render(t, in)

	for i, line := range strings.Split(screen, "\n") {
		if n := utf8.RuneCountInString(line); n > Width {
			t.Errorf("line %d is %d characters: %q", i+1, n, line)
		}
	}

	last := -1
	for _, h := range sectionHeadings {
		at := strings.Index(screen, h)
		if at < 0 || at < last {
			t.Fatalf("section %q missing or out of order", h)
		}
		last = at
	}
	numbered := 0
	for _, line := range strings.Split(screen, "\n") {
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' {
			numbered++
		}
	}
	if numbered != 5 {
		t.Errorf("screen has %d numbered sections, want 5", numbered)
	}

	one := sectionOf(t, screen, 1)
	for _, e := range install.Events {
		if !strings.Contains(flat(one), " "+e) {
			t.Errorf("section 1 does not name the %s entry", e)
		}
	}
	if !strings.Contains(flat(one), fmt.Sprintf("gets %d entries", len(install.Events))) {
		t.Errorf("section 1 does not give the entry count %d", len(install.Events))
	}
	if !strings.Contains(flat(one), flat(limits())) || !strings.Contains(limits(), fmt.Sprintf("%d s", install.Timeout)) ||
		!strings.Contains(limits(), fmt.Sprintf("%d s for %s", install.RecapTimeout, install.EventStop)) {
		t.Errorf("section 1 does not give each entry's time limit: %q", limits())
	}
	if !strings.Contains(flat(one), "Each can delay Claude by at most its limit; after that Claude Code carries on.") {
		t.Error("section 1 does not say a hook delays Claude by at most its limit")
	}
	joined := strings.ReplaceAll(flat(one), " ", "")
	for _, want := range []string{in.SettingsPath, in.Executable, install.Marker + in.InstallID} {
		if !strings.Contains(joined, strings.ReplaceAll(want, " ", "")) {
			t.Errorf("section 1 does not carry %q", want)
		}
	}
	if !strings.Contains(flat(sectionOf(t, screen, 2)), flat(in.StoreRoot)[:20]) {
		t.Error("section 2 does not name the store root")
	}

	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	cites := map[string]int{
		disclose.CiteSection1: 1, disclose.CiteSection2: 2, disclose.CiteSection3: 3,
		disclose.CiteSection4: 4, disclose.CiteSection5: 5,
	}
	// Which section each layer's text belongs in is the screen's layout
	// (Archive 80.3.3), not something the inventory may move: a paragraph
	// cited into another section changes these counts.
	wantPerSection := map[int]int{1: 3, 2: 5, 3: 4, 4: 3, 5: 2}
	gotPerSection := map[int]int{}
	carried := 0
	for _, p := range inv.Paragraphs {
		n, ok := cites[p.Cite]
		if !ok {
			continue
		}
		gotPerSection[n]++
		body := sectionOf(t, screen, n)
		if strings.Contains(p.Text, "\n") {
			for _, l := range strings.Split(p.Text, "\n") {
				if !strings.Contains(body, indent+l+"\n") {
					t.Errorf("section %d does not carry %s's line %q verbatim", n, p.Owner, l)
				}
			}
		} else if !strings.Contains(flat(body), flat(p.Text)) {
			t.Errorf("section %d does not carry %s's paragraph verbatim: %q", n, p.Owner, p.Text)
		}
		carried++
	}
	if carried == 0 {
		t.Fatal("premise: the inventory has no layer paragraphs, so the screen carries nothing")
	}
	for n, want := range wantPerSection {
		if gotPerSection[n] != want {
			t.Errorf("section %d carries %d paragraphs, want %d", n, gotPerSection[n], want)
		}
	}

	five := sectionOf(t, screen, 5)
	commands := 0
	for _, c := range Commands {
		if strings.Contains(five, indent+c.Does+" ") && strings.Contains(five, " "+c.Run+"\n") {
			commands++
		} else {
			t.Errorf("section 5 does not list %q -> %q", c.Does, c.Run)
		}
	}
	if len(Commands) != 7 || commands != 7 {
		t.Errorf("the screen lists %d of %d commands; the spec names seven", commands, len(Commands))
	}

	if again := render(t, in); again != screen {
		t.Error("the same input rendered two different screens; text_sha256 could not be reproduced by --show")
	}
}

// TestTRU2_ScreenVariantsComeFromTheInventory: an older consent opens the
// screen with the CHANGED SINCE block, rendered from the inventory's
// template; a consent for this scope, and none at all, do not; a machine
// with no store shows the placeholder instead of an install id.
func TestTRU2_ScreenVariantsComeFromTheInventory(t *testing.T) {
	in := testInput()
	older := record(ViaWatchTTY, disclose.ScopeVersion-1, 0, RecordScopeV4)
	older.BinaryVersion = "1.1.0"
	in.Prior = &older
	screen := render(t, in)
	want := fmt.Sprintf("CHANGED SINCE YOU LAST AGREED (1.1.0, scope %d): this build asks for scope %d.",
		disclose.ScopeVersion-1, disclose.ScopeVersion)
	if !strings.Contains(flat(screen), want) {
		t.Errorf("an older consent does not open the screen with %q:\n%s", want, screen)
	}
	if strings.Index(screen, "CHANGED SINCE") > strings.Index(screen, sectionHeadings[0]) {
		t.Error("the CHANGED SINCE block comes after section 1")
	}

	current := record(ViaWatchTTY, disclose.ScopeVersion, 0, RecordScopeV5)
	in.Prior = &current
	if s := render(t, in); strings.Contains(s, "CHANGED SINCE") {
		t.Error("a consent for this scope still shows the CHANGED SINCE block")
	}
	in.Prior = nil
	if s := render(t, in); strings.Contains(s, "CHANGED SINCE") {
		t.Error("no consent at all shows the CHANGED SINCE block")
	}

	in.InstallID = ""
	fresh := render(t, in)
	if !strings.Contains(flat(fresh), install.Marker+" "+NewInstallPlaceholder) {
		t.Errorf("a machine with no store does not show the placeholder id:\n%s", fresh)
	}

	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var kept []disclose.Paragraph
	for _, p := range inv.Paragraphs {
		if p.Cite != disclose.CiteChangedSince {
			kept = append(kept, p)
		}
	}
	inv.Paragraphs = kept
	in.Prior = &older
	if _, err := Screen(inv, in); err == nil {
		t.Error("a screen that needs the CHANGED SINCE block rendered without its template")
	}
}

// TestTRU5_TextHashIsOfTheBytesShown: text_sha256 is the SHA-256 of the
// exact bytes, so one changed byte changes it.
func TestTRU5_TextHashIsOfTheBytesShown(t *testing.T) {
	screen := []byte(render(t, testInput()))
	h := TextSHA256(screen)
	if len(h) != 64 {
		t.Fatalf("TextSHA256 = %q, want 64 hex characters", h)
	}
	if got := TextSHA256(bytes.Clone(screen)); got != h {
		t.Error("the same bytes hash differently")
	}
	changed := bytes.Clone(screen)
	changed[len(changed)/2] ^= 1
	if TextSHA256(changed) == h {
		t.Error("a changed byte leaves text_sha256 the same")
	}
}
