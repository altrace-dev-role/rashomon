package verify

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// claimVocabulary is every byte the scan may hold that is not an HMAC: the
// field names and the closed values, each class with both strictness values
// and every verdict over allKinds' ids. A piece of the input found in the
// scan and not in here came from the input.
func claimVocabulary() []byte {
	var cs []ClaimVerdict
	ids := []SlotID{}
	for _, g := range allKinds() {
		ids = append(ids, SlotID(g.Gate))
	}
	verdicts := []Verdict{NoGateKnown}
	for _, s := range States {
		verdicts = append(verdicts, Verdict(s))
	}
	for _, class := range ClaimClasses {
		for _, v := range verdicts {
			for _, strict := range []bool{true, false} {
				cs = append(cs, ClaimVerdict{Source: SourceFinalMessage, Class: class, Strict: strict, Verdict: v,
					Gates: ids, QuoteHMAC: QuoteHMAC(QuoteHMACPrefix + strings.Repeat("0", 64))})
			}
		}
	}
	out, _ := json.Marshal([]ClaimScan{{Claims: cs, LastIsQuestion: true, Cut: true}, {Claims: []ClaimVerdict{}}})
	return out
}

// FuzzVER55_ClaimsNeverPanicAndKeepNoText: any two windows give a scan that
// holds no piece of either longer than eight bytes, every value from its
// closed set, at most MaxClaims verdicts and under 64 KiB marshalled (card
// 1.7 item 9). A regression guard behind the structural guarantee, not its
// proof: ClaimScan has no field typed to hold text.
func FuzzVER55_ClaimsNeverPanicAndKeepNoText(f *testing.F) {
	for _, r := range ver55Rows {
		f.Add([]byte(r.msg), []byte(nil))
	}
	f.Add([]byte("All tests pass. "+strings.Repeat("Done. ", 200)), []byte("Lint is clean, but one warning."))
	vocab := claimVocabulary()
	ids := []SlotID{}
	for _, g := range allKinds() {
		ids = append(ids, SlotID(g.Gate))
	}
	f.Fuzz(func(t *testing.T, head, tail []byte) {
		s := Claims(head, tail, claimKey, allKinds(), true, SourceFinalMessage, nil)
		out, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) >= 64<<10 || s.Claims == nil || len(s.Claims) > MaxClaims || s.ClaimsOmitted < 0 {
			t.Fatalf("scan of %d bytes, %d claims, %d omitted", len(out), len(s.Claims), s.ClaimsOmitted)
		}
		for _, c := range s.Claims {
			known := c.Verdict == NoGateKnown || slices.Contains(States, State(c.Verdict))
			if c.Source != SourceFinalMessage || !slices.Contains(ClaimClasses, c.Class) || !known || !c.QuoteHMAC.Valid() {
				t.Fatalf("a value outside its closed set: %+v", c)
			}
			for _, id := range c.Gates {
				if !slices.Contains(ids, id) {
					t.Fatalf("gate %q is no fixture id", id)
				}
			}
		}
		pieces := map[string]bool{}
		for i := 0; i+9 <= len(out); i++ {
			pieces[string(out[i:i+9])] = true
		}
		for _, in := range [][]byte{head, tail} {
			for i := 0; i+9 <= len(in); i++ {
				if w := in[i : i+9]; pieces[string(w)] && !bytes.Contains(vocab, w) {
					t.Fatalf("the scan carries %q of the input", w)
				}
			}
		}
	})
}
