package verify

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The claim comparator (spec 20.2.5; tech spec 2.2.5): the one place a claim
// in the agent's message meets the record. Claims is pure: the message
// windows and the gates at a point in, the verdicts out, no state between
// calls. It decides neither delivery (2.2), silence (2.4 reads LastIsQuestion
// and the recording) nor the finding's kind (tosteer's claimKinds). The steps,
// numbered as 20.2.5 numbers them:
//
//  1. Each window is cut to MaxClaimMessageBytes on a rune boundary and
//     sanitised as report's sanitizeMessage does: runes neither graphic nor
//     space are dropped. Its 64K-rune cap is the byte cap here, since a window
//     of 64 KiB holds at most 64K runes.
//  2. Removed: fenced code blocks, lines whose first non-space byte is '>',
//     and a line that is "※" and a delivered steer line, byte for byte, all
//     tested on the raw line; then, inside the lines kept, inline code spans,
//     double-quoted spans of up to 200 runes, and URLs.
//  3. Sentences end after '.', '!' or '?' before a space or the end, and at a
//     newline. A sentence ending in '?' or opening with a claimSkips entry is
//     no claim.
//  4. and 5. The lexicon and its filters, claimlex.go.
//  6. and 7. A class maps to the required gates of its kind (done: every
//     required gate; an advisory gate never enters, D38); the verdict is
//     VerdictOf their states, no_gate_known with none.
//  8. A claim is strict when neither its sentence nor the next holds a
//     disclosure outside an accepted predicate's words (Gate A Q3, Q8) and no
//     window was cut.
//
// The tables are versioned with RulesVersion (claimlex.go). Two v0 deviations
// from 20.2.5: the cut is read from the capture's own flag, a tail being set,
// not from a 128 KiB total the payload does not carry; and delivered is nil
// until 2.2 stores the lines it delivers, so step 2's quoted-line rule
// removes nothing yet.
//
// No text is kept, by construction: ClaimScan's fields are closed values,
// slot ids taken from the gates, counts, and one-way HMACs; none is typed to
// hold the message. TestVER57 and FuzzVER55 guard against a future field;
// they are not the proof.

// MaxClaimMessageBytes bounds each window (ENG-STD Task 1.7); the capture
// already bounds them, this is the defence.
const MaxClaimMessageBytes = 64 << 10

// MaxClaims bounds the verdicts a scan keeps: a 64 KiB head of one repeated
// claim would otherwise give some 4,000 (Gate A Q6).
const MaxClaims = 64

// ClaimScan is what the comparator returns (2.2.5): the verdicts and the two
// message facts the Silence table reads.
type ClaimScan struct {
	// Claims is non-nil, in sentence order and then ClaimClasses order. Past
	// MaxClaims the worst are kept, by Rank of the verdict (no_gate_known
	// after green), then strict before loose, then position.
	Claims         []ClaimVerdict `json:"claims"`
	LastIsQuestion bool           `json:"last_is_question"` // the message's last sentence ends in '?'
	Cut            bool           `json:"cut"`              // a window lost text: every claim is loose
	ClaimsOmitted  int            `json:"claims_omitted"`   // verdicts past MaxClaims
}

// Claims compares the claims of a message's head and tail windows with gates
// (spec 20.2.5). wrote is whether the window holds a relevant write
// (WroteRelevant): done fires only then. key is the install key the quotes
// are HMACed under; delivered are the steer lines delivered this session.
func Claims(head, tail, key []byte, gates []GateState, wrote bool, source ClaimSource, delivered [][]byte) ClaimScan {
	scan := ClaimScan{Claims: []ClaimVerdict{}, Cut: len(tail) > 0}
	var texts []string
	for _, w := range [...][]byte{head, tail} {
		w, cut := capWindow(w)
		scan.Cut = scan.Cut || cut
		texts = appendSentences(texts, string(stripWindow(sanitize(w), delivered)))
	}
	if n := len(texts); n > 0 {
		scan.LastIsQuestion = strings.HasSuffix(texts[n-1], "?")
	}
	found := compare(texts, gates, wrote, scan.Cut)
	kept := keep(found)
	scan.ClaimsOmitted = len(found) - len(kept)

	mac := hmac.New(sha256.New, key)
	var q QuoteHMAC
	for i, c := range kept {
		if i == 0 || c.sentence != kept[i-1].sentence {
			q = quote(mac, texts[c.sentence])
		}
		scan.Claims = append(scan.Claims, ClaimVerdict{Source: source, Class: claimLexicon[c.rule].class,
			Strict: c.strict, Verdict: c.verdict, Gates: slices.Clone(c.gates), QuoteHMAC: q})
	}
	return scan
}

// WroteRelevant reports whether run holds, at or before seq at, a write
// gateStates counts against a gate: a classified write with a path whose
// class is Relevant (Gate A Q1). 2.4 passes it to Claims for the window.
func WroteRelevant(run *store.Run, at int64) bool {
	if run == nil {
		return false
	}
	var buf []write
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if d.Seq > at {
			continue
		}
		buf, _ = appendWrites(buf[:0], nil, d)
		for _, w := range buf {
			if w.Path != nil && Relevant(w.Class) {
				return true
			}
		}
	}
	return false
}

// sentence is one sentence as the comparator reads it. The per-word slices
// are read once, so no rule rescans a clause per match: a 64 KiB sentence of
// one repeated claim stays linear. compare holds two at a time, the claim's
// sentence and the next, and reuses their slices for the next pair.
type sentence struct {
	text    string   // as split and trimmed: what the quote digests
	words   []string // words(text)
	clauses []int    // clauses[i] is the index of the first word of word i's clause
	negated []bool   // negated[i]: a negation stands before word i in its clause
	covered []bool   // covered[i]: word i is in an accepted predicate
	spans   []span   // the accepted predicates, lexicon order; none when skipped
	disc    int8     // memo of disclosed: 0 unread, 1 no, 2 yes
}

// span is one accepted predicate: its lexicon row and the words it covers.
type span struct{ rule, from, to int }

// read tokenises text into s, reusing s's slices, and accepts its
// predicates (steps 3 to 5).
func (s *sentence) read(text string, wrote bool) {
	s.text, s.disc = text, 0
	s.words, s.clauses = appendWords(s.words[:0], s.clauses[:0], text)
	s.negated = appendNegations(s.negated[:0], s.words, s.clauses)
	s.covered = append(s.covered[:0], make([]bool, len(s.words))...)
	s.spans = s.spans[:0]
	if skipped(text, s.words) {
		return
	}
	s.spans = accept(s.spans, s, wrote)
	for _, x := range s.spans {
		for k := x.from; k < x.to; k++ {
			s.covered[k] = true
		}
	}
}

// disclosed reports, once per read, a disclosure in s outside its accepted
// predicates' words (Gate A Q3, Q8).
func (s *sentence) disclosed() bool {
	if s.disc == 0 {
		s.disc = 1
		if disclosed(s.words, s.covered) {
			s.disc = 2
		}
	}
	return s.disc == 2
}

// skipped reports a sentence that is no claim: a question, or advice.
func skipped(text string, ws []string) bool {
	if strings.HasSuffix(text, "?") {
		return true
	}
	for _, sk := range claimSkips {
		if phraseAt(ws, 0, sk) {
			return true
		}
	}
	return false
}

// accept appends the predicates of s that the lexicon and step 5 accept, in
// lexicon order (steps 4 and 5).
func accept(dst []span, s *sentence, wrote bool) []span {
	for ri := range claimLexicon {
		r := &claimLexicon[ri]
		if r.class == ClaimDone && !wrote {
			continue
		}
		for p := range s.words {
			if r.class == ClaimDone && s.clauses[p] != 0 {
				break // done is read in the first clause only
			}
			for _, pred := range r.predicates {
				if phraseAt(s.words, p, pred) && r.admits(s, p, len(pred)) {
					dst = append(dst, span{ri, p, p + len(pred)})
				}
			}
		}
	}
	return dst
}

// admits applies the subject rule (done has none: accept reads it in the
// first clause only) and step 5's filters to the predicate s.words[p:p+n].
// A modal inside the subject's own words is none (Gate A Q9).
func (r *claimRule) admits(s *sentence, p, n int) bool {
	ws := s.words
	sFrom, sTo := p, p
	if r.class != ClaimDone {
		var ok bool
		if sFrom, sTo, ok = r.subject(ws, p); !ok {
			return false
		}
	}
	return !s.negated[p] && !modalBefore(ws, p, sFrom, sTo) && !slices.Contains(ws[p+n:min(len(ws), p+n+2)], "yet")
}

// subject is the nearest subject that ends at or before ws[p] and starts
// within the four words before it (one-sided, Gate A Q4).
func (r *claimRule) subject(ws []string, p int) (from, to int, ok bool) {
	for s := p - 1; s >= max(0, p-4); s-- {
		if r.number && isNumber(ws[s]) {
			return s, s + 1, true
		}
		for _, sub := range r.subjects {
			if s+len(sub) <= p && phraseAt(ws, s, sub) {
				return s, s + len(sub), true
			}
		}
	}
	return 0, 0, false
}

// isNumber reports a word of digits, or of digits, '/' and digits ("42/42").
func isNumber(w string) bool {
	a, b, slash := strings.Cut(w, "/")
	return digits(a) && (!slash || digits(b))
}

func digits(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

// appendNegations appends, for each word of ws, whether a negation stands
// before it in its clause (cs, step 5): a negator or a word ending n't, not
// since ended by "and" or "but". Read before a predicate's first word, so the
// "no" of "no errors" never negates its own match (Gate A Q5).
func appendNegations(dst []bool, ws []string, cs []int) []bool {
	neg := false
	for i, w := range ws {
		if cs[i] == i {
			neg = false // a clause opens
		}
		dst = append(dst, neg)
		switch {
		case w == "and" || w == "but":
			neg = false
		case slices.Contains(claimNegators[:], w) || strings.HasSuffix(w, "n't"):
			neg = true
		}
	}
	return dst
}

// modalBefore reports a modal, future or conditional word, or one ending
// 'll, in the three words before ws[p], outside ws[sFrom:sTo].
func modalBefore(ws []string, p, sFrom, sTo int) bool {
	for i := max(0, p-3); i < p; i++ {
		if (i < sFrom || i >= sTo) && (slices.Contains(claimModals[:], ws[i]) || strings.HasSuffix(ws[i], "'ll")) {
			return true
		}
	}
	return false
}

// phraseAt reports whether ws holds phrase at i; no word and no phrase is
// empty, so the first bytes compare before the strings do.
func phraseAt(ws []string, i int, phrase []string) bool {
	return i+len(phrase) <= len(ws) && ws[i][0] == phrase[0][0] && ws[i] == phrase[0] && slices.Equal(ws[i+1:i+len(phrase)], phrase[1:])
}

// candidate is one verdict before the MaxClaims cut and the quote.
type candidate struct {
	pos, sentence, rule int
	strict              bool
	verdict             Verdict
	gates               []SlotID
}

// mapping is a class's verdict and gate ids over the gates (steps 6 and 7),
// read once per class.
type mapping struct {
	read    bool
	verdict Verdict
	ids     []SlotID
}

// compare is the verdict of each sentence's classes, sentence order then
// lexicon order (steps 3 to 8), reading the sentences in one pass.
func compare(texts []string, gates []GateState, wrote, cut bool) []candidate {
	var found []candidate
	var maps [len(claimLexicon)]mapping
	var cur, next sentence
	if len(texts) > 0 {
		cur.read(texts[0], wrote)
	}
	for i := range texts {
		last := i+1 == len(texts)
		if !last {
			next.read(texts[i+1], wrote)
		}
		for j, sp := range cur.spans {
			if j > 0 && cur.spans[j-1].rule == sp.rule {
				continue // one verdict per sentence and class
			}
			m := &maps[sp.rule]
			if !m.read {
				*m = mapGates(&claimLexicon[sp.rule], gates)
			}
			strict := !cut && !cur.disclosed() && (last || !next.disclosed())
			found = append(found, candidate{len(found), i, sp.rule, strict, m.verdict, m.ids})
		}
		cur, next = next, cur
	}
	return found
}

// mapGates is rule's verdict over the required gates of its kind, every
// required gate for done; ids are theirs within the slot charset, sorted, at
// most MaxPaths, never nil. An id outside the charset is a defect: it is
// dropped and its state still counts.
func mapGates(r *claimRule, gates []GateState) mapping {
	var states []State
	ids := []SlotID{}
	for _, g := range gates {
		if !g.Required || r.class != ClaimDone && g.Kind != r.kind {
			continue
		}
		states = append(states, g.State)
		if id := SlotID(g.Gate); id.Valid() {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return mapping{read: true, verdict: VerdictOf(states), ids: ids[:min(len(ids), MaxPaths)]}
}

// disclosed reports a claimDisclosures entry in ws none of whose words is
// covered. A one-word entry matches any word it opens ("fail" in "failures",
// Gate A Q7); a phrase matches whole words.
func disclosed(ws []string, covered []bool) bool {
	for k, w := range ws {
		for _, d := range claimDisclosures {
			if w[0] != d[0][0] {
				continue // a cheap first test: most words open no entry
			}
			hit := len(d) == 1 && strings.HasPrefix(ws[k], d[0]) || len(d) > 1 && phraseAt(ws, k, d)
			if hit && !slices.Contains(covered[k:k+len(d)], true) {
				return true
			}
		}
	}
	return false
}

// keep is found cut to MaxClaims: the worst by Rank of the verdict, with
// no_gate_known after green, then strict before loose, then position; kept
// in position order (Gate A Q6).
func keep(found []candidate) []candidate {
	if len(found) <= MaxClaims {
		return found
	}
	kept := slices.Clone(found)
	slices.SortFunc(kept, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(verdictRank(a.verdict), verdictRank(b.verdict)),
			cmp.Compare(looseRank(a.strict), looseRank(b.strict)), cmp.Compare(a.pos, b.pos))
	})
	kept = kept[:MaxClaims]
	slices.SortFunc(kept, func(a, b candidate) int { return cmp.Compare(a.pos, b.pos) })
	return kept
}

// verdictRank is Rank over a verdict: no_gate_known, which no gate stands
// behind, after green.
func verdictRank(v Verdict) int {
	if v == NoGateKnown {
		return len(Order)
	}
	return Rank(State(v))
}

func looseRank(strict bool) int {
	if strict {
		return 0
	}
	return 1
}

// quote is QuoteHMACPrefix and the hex HMAC of sentence under mac's key.
func quote(mac hash.Hash, sentence string) QuoteHMAC {
	mac.Reset()
	mac.Write([]byte(sentence))
	return QuoteHMAC(QuoteHMACPrefix + hex.EncodeToString(mac.Sum(nil)))
}
