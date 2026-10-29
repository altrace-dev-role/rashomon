# VER-1.7: the claim comparator (internal/verify/claims.go, claimtext.go,
# claimlex.go). One break per rule of 20.2.5 and per Gate A ruling: the strip, the split, the
# lexicon's windows and filters, the gates a class maps to and its verdict,
# strictness, the cut, the quote, the MaxClaims selection, WroteRelevant and
# D43's charset. The kill row is the named test's.
C = "internal/verify/claims.go"
T = "internal/verify/claimtext.go"
L = "internal/verify/claimlex.go"
F = "internal/verify/finding.go"

# Step 1: the window.
m("VER-1.7 the tail is read before the head", C,
  '\tfor _, w := range [...][]byte{head, tail} {\n', '\tfor _, w := range [...][]byte{tail, head} {\n', "TestVER61_")
m("VER-1.7 the cut falls inside a rune", T,
  '\tfor i > 0 && !utf8.RuneStart(b[i]) {\n', '\tfor i > MaxClaimMessageBytes && !utf8.RuneStart(b[i]) {\n', "TestVER61_")
m("VER-1.7 a window of exactly the cap is cut", T,
  '\tif len(b) <= MaxClaimMessageBytes {\n\t\treturn b, false\n', '\tif len(b) < MaxClaimMessageBytes {\n\t\treturn b, false\n', "TestVER61_")
m("VER-1.7 the sanitiser keeps every rune", T,
  '\t\tif unicode.IsGraphic(r) || unicode.IsSpace(r) {\n\t\t\tout = utf8.AppendRune(out, r)\n',
  '\t\tif unicode.IsGraphic(r) || unicode.IsSpace(r) || r > 0 {\n\t\t\tout = utf8.AppendRune(out, r)\n', "TestVER55_")
m("VER-1.7 the sanitiser's ASCII path keeps a control", T,
  "\t\t\tif ' ' <= c && c < 0x7f || '\\t' <= c && c <= '\\r' {\n", "\t\t\tif ' ' <= c && c < 0x7f || c <= '\\r' {\n", "TestVER55_")

# Step 2: the strip.
m("VER-1.7 the code-fence strip dropped", T, '\t\t\tfenced = !fenced\n', '\t\t\tfenced = false\n', "TestVER55_")
m("VER-1.7 the quote-line strip dropped", T,
  '\t\tcase fenced, bytes.HasPrefix(lead, []byte(">")), isDelivered(lead, delivered):\n',
  '\t\tcase fenced, isDelivered(lead, delivered):\n', "TestVER55_")
m("VER-1.7 the inline-code strip dropped", T,
  "cutURLs(cutSpans(cutSpans(out[n:], '`', 0), '\"', maxQuotedRunes))",
  "cutURLs(cutSpans(out[n:], '\"', maxQuotedRunes))", "TestVER55_")
m("VER-1.7 the double-quote strip dropped", T,
  "cutURLs(cutSpans(cutSpans(out[n:], '`', 0), '\"', maxQuotedRunes))",
  "cutURLs(cutSpans(out[n:], '`', 0))", "TestVER55_")
m("VER-1.7 a quoted span of any length is cut", T,
  '\t\t\tif end >= 0 && (max == 0 || utf8.RuneCount(b[r+1:r+1+end]) <= max) {\n',
  '\t\t\tif end >= 0 {\n', "TestVER55_")
m("VER-1.7 a quoted span of exactly the bound stays", T,
  ' || utf8.RuneCount(b[r+1:r+1+end]) <= max) {\n', ' || utf8.RuneCount(b[r+1:r+1+end]) < max) {\n', "TestVER55_")
m("VER-1.7 the URL strip dropped", T,
  "len(cutURLs(cutSpans(cutSpans(out[n:], '`', 0), '\"', maxQuotedRunes)))",
  "len(cutSpans(cutSpans(out[n:], '`', 0), '\"', maxQuotedRunes))", "TestVER55_")
m("VER-1.7 a delivered line matches by prefix", T,
  '\t\tif bytes.Equal(text, d) {\n', '\t\tif bytes.HasPrefix(text, d) {\n', "TestVER56_")
m("VER-1.7 a bare mark excludes everything", T,
  '\trest, _ := bytes.CutPrefix(lead, []byte("※"))\n',
  '\trest, ok := bytes.CutPrefix(lead, []byte("※"))\n\tif ok {\n\t\treturn true\n\t}\n', "TestVER56_")

# Step 3: the split and the skips.
m("VER-1.7 a dot before a word ends a sentence", T,
  '; i+1 == len(s) || unicode.IsSpace(r) {\n', '; i+1 == len(s) || unicode.IsSpace(r) || r > 0 {\n', "TestVER62_")
m("VER-1.7 a newline ends no sentence", T, "\t\tcase '\\n':\n\t\t\tend = i\n", "\t\tcase '\\n':\n", "TestVER62_")
m("VER-1.7 LastIsQuestion reads the first sentence", C,
  '\t\tscan.LastIsQuestion = strings.HasSuffix(texts[n-1], "?")\n', '\t\tscan.LastIsQuestion = strings.HasSuffix(texts[0], "?")\n', "TestVER62_")
m("VER-1.7 the question skip dropped", C,
  '\tif strings.HasSuffix(text, "?") {\n\t\treturn true\n\t}\n', '', "TestVER55_")
m("VER-1.7 the Please skip dropped", L, '{{"please"}, {"run"},', '{{"run"},', "TestVER55_")

# Steps 4 and 5: the lexicon's windows and filters.
m("VER-1.7 digits are separators", T,
  "\treturn unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/'", "\treturn unicode.IsLetter(r) || r == '/'",
  "TestVER55_|TestVER55b_")
m("VER-1.7 the number subject dropped", L,
  '{class: ClaimTestsPass, kind: "test", number: true,', '{class: ClaimTestsPass, kind: "test",', "TestVER55_")
m("VER-1.7 the subject window is unbounded", C,
  '\tfor s := p - 1; s >= max(0, p-4); s-- {\n', '\tfor s := p - 1; s >= 0; s-- {\n', "TestVER55_")
m("VER-1.7 the subject may follow the predicate", C,
  '\tfor s := p - 1; s >= max(0, p-4); s-- {\n\t\tif r.number && isNumber(ws[s]) {\n\t\t\treturn s, s + 1, true\n\t\t}\n\t\tfor _, sub := range r.subjects {\n\t\t\tif s+len(sub) <= p && phraseAt(ws, s, sub) {\n',
  '\tfor s := min(len(ws)-1, p+4); s >= max(0, p-4); s-- {\n\t\tif r.number && isNumber(ws[s]) {\n\t\t\treturn s, s + 1, true\n\t\t}\n\t\tfor _, sub := range r.subjects {\n\t\t\tif s != p && phraseAt(ws, s, sub) {\n',
  "TestVER55_")
m("VER-1.7 done in any clause", C, ' && s.clauses[p] != 0 {\n\t\t\t\tbreak', ' && s.clauses[p] < 0 {\n\t\t\t\tbreak', "TestVER55_")
m("VER-1.7 one verdict per sentence and class dropped", C,
  '\t\t\tif j > 0 && cur.spans[j-1].rule == sp.rule {\n', '\t\t\tif j < 0 && cur.spans[j-1].rule == sp.rule {\n', "TestVER55_")
m("VER-1.7 the negator dropped", C, '\treturn !s.negated[p] && ', '\treturn ', "TestVER55_")
m("VER-1.7 and and but end no negation", C, '\t\tcase w == "and" || w == "but":\n', '\t\tcase w == "":\n', "TestVER55_")
m("VER-1.7 the negator reads the predicate's own first word", C,
  '\t\tdst = append(dst, neg)\n\t\tswitch {\n\t\tcase w == "and" || w == "but":\n\t\t\tneg = false\n\t\tcase slices.Contains(claimNegators[:], w) || strings.HasSuffix(w, "n\'t"):\n\t\t\tneg = true\n\t\t}\n',
  '\t\tswitch {\n\t\tcase w == "and" || w == "but":\n\t\t\tneg = false\n\t\tcase slices.Contains(claimNegators[:], w) || strings.HasSuffix(w, "n\'t"):\n\t\t\tneg = true\n\t\t}\n\t\tdst = append(dst, neg)\n',
  "TestVER55_")
m("VER-1.7 a negation runs past its clause", C, '\t\tif cs[i] == i {\n', '\t\tif cs[i] == i && i < 0 {\n', "TestVER55_")
m("VER-1.7 the modal window is four words", C,
  '\tfor i := max(0, p-3); i < p; i++ {\n', '\tfor i := max(0, p-4); i < p; i++ {\n', "TestVER55_")
m("VER-1.7 a modal inside the subject counts", C,
  '\t\tif (i < sFrom || i >= sTo) && (slices.Contains', '\t\tif (slices.Contains', "TestVER55_")
m("VER-1.7 the yet window dropped", C,
  ' && !slices.Contains(ws[p+n:min(len(ws), p+n+2)], "yet")\n', '\n', "TestVER55_")
m("VER-1.7 the yet window is three words", C,
  'ws[p+n:min(len(ws), p+n+2)], "yet")', 'ws[p+n:min(len(ws), p+n+3)], "yet")', "TestVER55_")

# Steps 6 and 7: the gates and the verdict.
m("VER-1.7 done fires without a write", C,
  '\t\tif r.class == ClaimDone && !wrote {\n\t\t\tcontinue\n\t\t}\n', '', "TestVER55_|TestVER59_")
m("VER-1.7 a non-required gate counts", C,
  '\t\tif !g.Required || r.class != ClaimDone && g.Kind != r.kind {\n', '\t\tif r.class != ClaimDone && g.Kind != r.kind {\n', "TestVER58_")
m("VER-1.7 done maps to its own kind", C,
  '\t\tif !g.Required || r.class != ClaimDone && g.Kind != r.kind {\n', '\t\tif !g.Required || g.Kind != r.kind {\n', "TestVER55_|TestVER58_")
m("VER-1.7 unknown never reaches the verdict", C,
  '\t\tstates = append(states, g.State)\n', '\t\tif g.State != Unknown {\n\t\t\tstates = append(states, g.State)\n\t\t}\n', "TestVER58_")
m("VER-1.7 no_gate_known reads as green", C,
  'verdict: VerdictOf(states),', 'verdict: VerdictOf(append(states, Green)),', "TestVER58_")
m("VER-1.7 Gates nil with no gate", C, '\tids := []SlotID{}\n\tfor _, g := range gates {\n', '\tvar ids []SlotID\n\tfor _, g := range gates {\n', "TestVER58_")
m("VER-1.7 Gates unsorted", C, '\tslices.Sort(ids)\n\treturn mapping{', '\treturn mapping{', "TestVER58_")
m("VER-1.7 the MaxPaths cap dropped", C, 'ids: ids[:min(len(ids), MaxPaths)]}', 'ids: ids}', "TestVER58_")
m("VER-1.7 an id outside the charset kept", C,
  '\t\tif id := SlotID(g.Gate); id.Valid() {\n', '\t\tif id := SlotID(g.Gate); id != "" {\n', "TestVER58_")
m("VER-1.7 D43 reverted: '@' outside the slot charset", F,
  'return idForm(string(s), "._:@-") }', 'return idForm(string(s), "._:-") }', "TestVER54d_|TestVER58_")

# Step 8: strictness.
m("VER-1.7 the next sentence's disclosure dropped", C,
  '\t\t\tstrict := !cut && !cur.disclosed() && (last || !next.disclosed())\n', '\t\t\tstrict := !cut && !cur.disclosed()\n',
  "TestVER55_|TestVER60_")
m("VER-1.7 the cut ignored", C, '\t\t\tstrict := !cut && ', '\t\t\tstrict := ', "TestVER61_")
m("VER-1.7 a predicate's own words disclose", C,
  '\t\t\tif hit && !slices.Contains(covered[k:k+len(d)], true) {\n', '\t\t\tif hit {\n', "TestVER55_|TestVER60_")
m("VER-1.7 the next sentence's own predicates disclose", C,
  ' && (last || !next.disclosed())\n', ' && (last || !disclosed(next.words, make([]bool, len(next.words))))\n', "TestVER60_")
m("VER-1.7 a one-word disclosure matches the whole word only", C,
  'len(d) == 1 && strings.HasPrefix(ws[k], d[0])', 'len(d) == 1 && ws[k] == d[0]', "TestVER60_")
m("VER-1.7 a phrase disclosure matches its first word", C,
  ' || len(d) > 1 && phraseAt(ws, k, d)\n', ' || len(d) > 1 && ws[k] == d[0]\n', "TestVER60_")

# The quote: HMAC of the sentence under the key, never the sentence.
m("VER-1.7 the quote without the key", C,
  '\tmac := hmac.New(sha256.New, key)\n', '\tmac := sha256.New()\n', "TestVER57_")
IMPORTS["VER-1.7 the quote without the key"] = (C, '\t"crypto/hmac"\n', '')
m("VER-1.7 the sentence written into Gates", C,
  'Gates: slices.Clone(c.gates), QuoteHMAC: q})', 'Gates: append(slices.Clone(c.gates), SlotID(texts[c.sentence])), QuoteHMAC: q})',
  "TestVER57_")

# MaxClaims (Gate A Q6).
m("VER-1.7 the MaxClaims cap dropped", C, '\tif len(found) <= MaxClaims {\n', '\tif len(found) >= 0 {\n', "TestVER61_")
m("VER-1.7 the cap keeps by position only", C,
  '\t\treturn cmp.Or(cmp.Compare(verdictRank(a.verdict), verdictRank(b.verdict)),\n\t\t\tcmp.Compare(looseRank(a.strict), looseRank(b.strict)), cmp.Compare(a.pos, b.pos))\n',
  '\t\treturn cmp.Compare(a.pos, b.pos)\n', "TestVER61_")
m("VER-1.7 the cap keeps the loose first", C,
  'cmp.Compare(looseRank(a.strict), looseRank(b.strict))', 'cmp.Compare(looseRank(b.strict), looseRank(a.strict))', "TestVER61_")
m("VER-1.7 no_gate_known ranks worst", C, '\tif v == NoGateKnown {\n\t\treturn len(Order)\n', '\tif v == NoGateKnown {\n\t\treturn -1\n', "TestVER61_")
m("VER-1.7 no_gate_known ranks with green", C,
  '\tif v == NoGateKnown {\n\t\treturn len(Order)\n', '\tif v == NoGateKnown {\n\t\treturn len(Order) - 1\n', "TestVER61_")

# WroteRelevant (Gate A Q1): gateStates' own predicate.
m("VER-1.7 WroteRelevant counts any class", C,
  '\t\t\tif w.Path != nil && Relevant(w.Class) {\n', '\t\t\tif w.Path != nil {\n', "TestVER59_")
m("VER-1.7 WroteRelevant counts a write it cannot name", C,
  '\t\t\tif w.Path != nil && Relevant(w.Class) {\n', '\t\t\tif Relevant(w.Class) {\n', "TestVER59_")
m("VER-1.7 WroteRelevant reads past the point", C,
  '\t\tif d.Seq > at {\n\t\t\tcontinue\n\t\t}\n\t\tbuf, _ = appendWrites', '\t\tbuf, _ = appendWrites', "TestVER59_")
