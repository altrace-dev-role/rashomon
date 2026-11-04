package directive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// testDigest stands in for intent.SaidDigest: 64 lowercase hex per words.
func testDigest(words string) string {
	sum := sha256.Sum256([]byte(words))
	return hex.EncodeToString(sum[:])
}

// head is the briefing's header line with its two counts.
func head(earlier, withheld int) string {
	return fmt.Sprintf("rashomon record: the person's words from before this session's last compaction, newest first, "+
		"read from the transcript and not kept; %d earlier and %d withheld not shown.", earlier, withheld)
}

// TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved: the briefing at a
// boundary quotes every directive before it and none after, newest first, a
// correction with the words it restates beside it and those words on no line
// of their own. It is continuity-briefing at priority 0, a nudge keyed by the
// boundary's uuid, admitted at SessionStart. Its Record holds each quote's
// keyed digest and byte length and no word of the person's. Break: quote a
// directive after the boundary, oldest first, lose the restated words, key
// by anything but the boundary, or record the words, and this fails.
func TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved(t *testing.T) {
	split := Directive{Kind: KindDirective, Said: "they go vertical, we go horizontal and integration", Line: 3}
	push := Directive{Kind: KindDirective, Said: "no push without my word", Line: 10}
	again := Directive{Kind: KindCorrection, Said: "hes vertical we are horizontal and integration remember?", Line: 20, Restates: 3}
	after := Directive{Kind: KindDirective, Said: "everything is the wednesday delivery", Line: 40}
	f, ok := Briefing([]Directive{split, push, again, after}, Boundary{UUID: "b-2", Line: 30}, testDigest, 7)
	if !ok {
		t.Fatal("no briefing for two directives and a correction before the boundary")
	}
	want := head(0, 0) + "\n'" + again.Said + "', restating '" + split.Said + "'\n'" + push.Said + "'"
	if f.Message.Text() != want {
		t.Errorf("the briefing reads\n%s\nwant\n%s", f.Message.Text(), want)
	}
	if f.Pattern != pattern.ContinuityBriefing || f.Kind != "" || f.Key != "b-2" || f.Priority != 0 ||
		f.MaxLevel != steer.LevelNudge || f.StopForm != steer.StopFormNone || f.AfterSeq != 7 {
		t.Errorf("the finding is %+v", f)
	}
	if err := f.CheckOn(steer.EventSessionStart); err != nil {
		t.Errorf("the finding is not admitted at SessionStart: %v", err)
	}
	rec := head(0, 0) + fmt.Sprintf("\na quote of %d bytes, restating a quote of %d bytes\na quote of %d bytes",
		len(again.Said), len(split.Said), len(push.Said))
	if f.Record.Text() != rec {
		t.Errorf("the record reads\n%s\nwant\n%s", f.Record.Text(), rec)
	}
	for _, d := range []Directive{split, push, again, after} {
		for _, w := range strings.Fields(d.Said) {
			if len(w) > 4 && strings.Contains(f.Record.Text(), w) {
				t.Errorf("the record holds the person's word %q", w)
			}
		}
	}
	wantSlots := map[string][2]string{"said1": {testDigest(again.Said), testDigest(split.Said)}, "said2": {testDigest(push.Said), ""}}
	for _, s := range f.Record.Slots() {
		v, isSaid := s.Value.(slot.Said)
		if !isSaid {
			continue
		}
		if w, ok := wantSlots[s.Name]; !ok || !v.Keyed() || v.Digest() != w[0] || v.RestatesDigest() != w[1] {
			t.Errorf("record slot %s holds %+v, want the keyed digests %v", s.Name, v, w)
		}
		delete(wantSlots, s.Name)
	}
	if len(wantSlots) != 0 {
		t.Errorf("record slots missing: %v", wantSlots)
	}
}

// TestCSL1_TheBriefingKeepsTheNewestWholeAndCountsTheRest: five lines at
// most, the newest five, the rest counted as earlier; a briefing over its
// byte cap drops its oldest lines whole, never cuts a quote; words the line
// law refuses are counted as withheld, and a correction whose restated words
// it refuses stands alone. Break: keep the oldest, cut a quote to fit, or
// drop a refused quote without counting it, and this fails.
func TestCSL1_TheBriefingKeepsTheNewestWholeAndCountsTheRest(t *testing.T) {
	names := []string{"one", "two", "three", "four", "five", "six", "seven"}
	var seven []Directive
	for i, n := range names {
		seven = append(seven, Directive{Kind: KindDirective, Said: "standing directive " + n, Line: i + 1})
	}
	f, ok := Briefing(seven, Boundary{UUID: "b", Line: 100}, testDigest, 1)
	want := head(2, 0) + "\n'standing directive seven'\n'standing directive six'\n'standing directive five'\n" +
		"'standing directive four'\n'standing directive three'"
	if !ok || f.Message.Text() != want {
		t.Errorf("seven directives read\n%s\nwant\n%s", f.Message.Text(), want)
	}

	var long []Directive
	for i := 1; i <= 5; i++ {
		words := func(kind string) string {
			s := fmt.Sprintf("%s %d ", kind, i) + strings.Repeat("keep the split across the compaction ", 7)
			return s[:slot.MaxSaidBytes]
		}
		long = append(long, Directive{Kind: KindDirective, Said: words("directive"), Line: i},
			Directive{Kind: KindCorrection, Said: words("correction"), Line: 10 + i, Restates: i})
	}
	f, ok = Briefing(long, Boundary{UUID: "b", Line: 100}, testDigest, 1)
	lines := strings.Split(f.Message.Text(), "\n")
	if !ok || len(lines) != 3 || lines[0] != head(3, 0) || lines[1] != "'"+long[9].Said+"', restating '"+long[8].Said+"'" ||
		lines[2] != "'"+long[7].Said+"', restating '"+long[6].Said+"'" {
		t.Errorf("five long corrections read\n%s\nwant the newest two whole and 3 earlier", f.Message.Text())
	}

	refused := []Directive{
		{Kind: KindDirective, Said: "run `go test` before done", Line: 2},
		{Kind: KindDirective, Said: "keep the split", Line: 4},
		{Kind: KindCorrection, Said: "keep the split, i said", Line: 6, Restates: 2},
	}
	f, ok = Briefing(refused, Boundary{UUID: "b", Line: 10}, testDigest, 1)
	want = head(0, 1) + "\n'keep the split, i said'\n'keep the split'"
	if !ok || f.Message.Text() != want {
		t.Errorf("a refused quote reads\n%s\nwant\n%s", f.Message.Text(), want)
	}
}

// TestCSL1_NoBriefingWithoutAQuoteADigestOrABoundary: nothing before the
// boundary, nothing quotable, no digest to record by, or no boundary uuid to
// key by gives no briefing. Break: brief with nothing to say, or record with
// no digest, and this fails.
func TestCSL1_NoBriefingWithoutAQuoteADigestOrABoundary(t *testing.T) {
	keep := []Directive{{Kind: KindDirective, Said: "keep the split", Line: 4}}
	for name, tc := range map[string]struct {
		ds     []Directive
		b      Boundary
		digest func(string) string
	}{
		"after the boundary": {keep, Boundary{UUID: "b", Line: 3}, testDigest},
		"on the boundary":    {keep, Boundary{UUID: "b", Line: 4}, testDigest},
		"none":               {nil, Boundary{UUID: "b", Line: 10}, testDigest},
		"none quotable":      {[]Directive{{Kind: KindDirective, Said: "run `go test`", Line: 2}}, Boundary{UUID: "b", Line: 10}, testDigest},
		"no digest":          {keep, Boundary{UUID: "b", Line: 10}, nil},
		"no uuid":            {keep, Boundary{Line: 10}, testDigest},
	} {
		if f, ok := Briefing(tc.ds, tc.b, tc.digest, 1); ok {
			t.Errorf("%s: a briefing %q", name, f.Message.Text())
		}
	}
}
