package spend

import (
	"bytes"
	"fmt"
	"io"
	"math/bits"
	"strconv"
	"strings"
)

// Text renders the summary for a terminal.
//
// Every line that carries a dollar figure is under a header that names the
// price snapshot and says the figure is an estimate at API list prices, and
// the second line says a plan subscription is not billed per token. Those
// two sentences are not decoration: without them the output reads as a bill.
func Text(w io.Writer, s *Summary) error {
	var b bytes.Buffer

	fmt.Fprintf(&b, "SPEND  last %d days · %s · est. %s at API list prices (%s)\n",
		s.Days, countOf(s.Sessions, "session"), headline(s.Total), s.Pricing.Snapshot)
	fmt.Fprintf(&b, "       %s\n", s.Pricing.Note)
	if s.ExtraAttempts.Responses > 0 {
		fmt.Fprintf(&b, "       the total excludes extra attempts (%s tokens, cost unknown)\n", thousands(s.ExtraAttempts.Tokens.Total()))
	}
	switch {
	case s.Read.Files == 0 && s.Read.FilesBeforeWindow > 0:
		fmt.Fprintf(&b, "       no Claude Code transcript was written in the last %d days (%s last written before that %s not read)\n",
			s.Days, countOf(s.Read.FilesBeforeWindow, "older transcript"), wasWere(s.Read.FilesBeforeWindow))
	case s.Read.Files == 0:
		b.WriteString("       no Claude Code transcripts were found to read\n")
	}

	if s.Responses > 0 {
		fmt.Fprintf(&b, "%-14s%s\n", "by agent", agentLine(s))
		fmt.Fprintf(&b, "%-14s%s\n", "by model", modelLine(s))
		fmt.Fprintf(&b, "%-14soutput %s   input %s   cache write %s   cache read %s\n", "by kind",
			money(s.ByKind.Output), money(s.ByKind.Input), money(s.ByKind.CacheWrite), money(s.ByKind.CacheRead))
		fmt.Fprintf(&b, "%-14s%s\n", "cache expiry", coldLine(s.CacheExpiry))
		fmt.Fprintf(&b, "%-14s%s\n", "refusals", refusalLine(s.Refusals))
		fmt.Fprintf(&b, "%-14s%s\n", "retries", attemptsLine(s.ExtraAttempts))
		for i, line := range sessionLines(s.PerSession) {
			label := ""
			if i == 0 {
				label = "by session"
			}
			fmt.Fprintf(&b, "%-14s%s\n", label, line)
		}
	} else if s.Refusals.WithoutUsage > 0 {
		fmt.Fprintf(&b, "%-14s%s\n", "refusals", refusalLine(s.Refusals))
	}

	if line := silentLine(s); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
	}

	// Beside any refusal or extra attempt, the list says what it leaves out:
	// no line there is not "nothing to save" on classifier hits.
	classifier := s.Refusals.Responses+s.Refusals.WithoutUsage+s.ExtraAttempts.Responses > 0
	if len(s.Savings) > 0 || classifier {
		b.WriteString("\n")
		label := "savings"
		for _, sv := range s.Savings {
			fmt.Fprintf(&b, "%-14s%s\n", label, savingLine(sv))
			label = ""
		}
		if classifier {
			fmt.Fprintf(&b, "%-14snot computed: what refusals and fallback routing (classifier hits) cost that could be saved\n", label)
		}
	}

	if s.Read.UnreadableDirs > 0 {
		fmt.Fprintf(&b, "\nnote: %s under projects/ could not be read; the transcripts in %s are not counted\n",
			countOf(s.Read.UnreadableDirs, "folder"), itThem(s.Read.UnreadableDirs))
	}
	if s.Read.UnparsedUsageLines > 0 {
		fmt.Fprintf(&b, "\nnote: %s that may carry usage could not be read (a malformed field, a negative or implausibly large count, or no message id), so %s not counted\n",
			countOf(s.Read.UnparsedUsageLines, "transcript line"), itThem(s.Read.UnparsedUsageLines)+" "+isAre(s.Read.UnparsedUsageLines))
	}
	if s.Read.UnreadableFiles > 0 {
		fmt.Fprintf(&b, "\nnote: %s could not be read to the end; what they hold past that point is not counted\n",
			countOf(s.Read.UnreadableFiles, "transcript file"))
	}
	if s.Read.FutureDatedResponses > 0 {
		fmt.Fprintf(&b, "\nnote: %s carried a timestamp after this run, which is not in the last %d days, so they are not counted\n",
			countOf(s.Read.FutureDatedResponses, "response"), s.Days)
	}
	if s.Read.UndatedResponses > 0 {
		fmt.Fprintf(&b, "\nnote: %s carried no timestamp and could not be placed in the window, so they are not counted\n",
			countOf(s.Read.UndatedResponses, "response"))
	}
	if s.FastMode.Responses > 0 {
		fmt.Fprintf(&b, "\nnote: %s ran in fast mode, which bills at a premium; %s %s\n",
			countOf(s.FastMode.Responses, "response"), itThem(s.FastMode.Responses)+" "+isAre(s.FastMode.Responses), s.FastMode.Pricing)
	}
	fmt.Fprintf(&b, "\nout of scope: fast mode's premium, Batch and partner (Bedrock, Vertex) pricing; long-context premiums; web-search fees ($10 per 1,000 searches)\n")

	_, err := w.Write(b.Bytes())
	return err
}

// headline is the total as the header states it. A wholly unpriced total is
// "cost unknown", never "$0.00".
func headline(c Cost) string {
	if !c.Wholly() {
		return fmt.Sprintf("cost unknown (%s tokens on %s no rate is known for)", thousands(c.UnpricedTokens), countOf(c.Unpriced, "response"))
	}
	if c.Known() {
		return usd(c.Nano)
	}
	return fmt.Sprintf("%s, plus %s tokens on %s whose cost is unknown", usd(c.Nano), thousands(c.UnpricedTokens), countOf(c.Unpriced, "response"))
}

// money renders a cost in a breakdown: its priced part, plus a note of any
// unpriced remainder. Unpriced is never shown as $0, and neither is a
// breakdown with no response behind it: "none" says there was nothing, where
// "$0.00" beside an unknown would invite reading the two as the same kind of
// answer.
func money(c Cost) string {
	if c.Priced == 0 && c.Unpriced == 0 {
		return "none"
	}
	if !c.Wholly() {
		return "unknown"
	}
	if c.Known() {
		return usd(c.Nano)
	}
	return usd(c.Nano) + " + unknown"
}

// usd renders nanodollars. A positive amount under a cent is "<$0.01": it
// is not zero, and rounding it to "$0.00" would say it was.
func usd(nano int64) string {
	if nano > 0 && nano < 1e7 {
		return "<$0.01"
	}
	return "$" + strconv.FormatFloat(float64(nano)/1e9, 'f', 2, 64)
}

// agentLine is main against subagent spend. The shares are printed only when
// both sides are wholly priced: a share of the priced part alone, beside an
// unknown, would call the unpriced side's share 0% whatever its tokens.
func agentLine(s *Summary) string {
	main, sub := s.ByAgent.Main, s.ByAgent.Subagents
	if main.Nano+sub.Nano == 0 || !main.Known() || !sub.Known() {
		return fmt.Sprintf("main %s   subagents %s", money(main), money(sub))
	}
	pm, ps := shares(main.Nano, sub.Nano)
	return fmt.Sprintf("main %s (%s)   subagents %s (%s)", money(main), pm, money(sub), ps)
}

// shares is two amounts' percentages of their sum, summing to exactly 100.
//
// Each share rounded on its own summed to 101% whenever both sat on a half
// (99.5 and 0.5), so the percentages are the floors with the leftover point
// given to the larger remainder (largest remainder; a tie goes to the first).
// And a share that rounds to 0 while its amount is not zero is "<1%", its
// complement ">99%": printing 0% beside a non-zero figure says it was nothing.
//
// In 128-bit arithmetic (percent): a*100 overflowed an int64 past about 9e16
// nanodollars, and shares(1e17, 1) printed "-84%".
func shares(a, b int64) (string, string) {
	total := uint64(a) + uint64(b)
	pa, ra := percent(uint64(a), total)
	pb, rb := percent(uint64(b), total)
	if pa+pb < 100 {
		if ra >= rb {
			pa++
		} else {
			pb++
		}
	}
	label := func(p uint64, n int64) string {
		switch {
		case p == 0 && n > 0:
			return "<1%"
		case p == 100 && uint64(n) < total:
			return ">99%"
		}
		return fmt.Sprintf("%d%%", p)
	}
	return label(pa, a), label(pb, b)
}

// percent is n*100/total and its remainder, without overflow: n <= total, so
// the high word of n*100 is below total and Div64 cannot panic.
func percent(n, total uint64) (q, r uint64) {
	hi, lo := bits.Mul64(n, 100)
	return bits.Div64(hi, lo, total)
}

// sessionLines is the costliest sessions, one per line, each with its main
// and subagent split and -- once Join has run -- whether rashomon recorded
// it. Per-session spend was JSON-only, and the sessions the silent-failure
// line does not cover were counted but never named; this is where a reader
// finds both. At most maxNamed; --json lists every session.
func sessionLines(ps []SessionSpend) []string {
	var out []string
	for i, p := range ps {
		if i == maxNamed {
			out = append(out, fmt.Sprintf("and %d more (--json lists every session)", len(ps)-maxNamed))
			break
		}
		total := p.Main
		total.addCost(p.Subagents)
		line := fmt.Sprintf("%s %s (main %s, subagents %s)", p.SessionID, money(total), money(p.Main), money(p.Subagents))
		switch p.Coverage {
		case CoverageNotRecorded:
			line += ", not recorded by rashomon"
		case CoveragePartly:
			line += ", partly recorded by rashomon"
		}
		out = append(out, line)
	}
	return out
}

func modelLine(s *Summary) string {
	parts := make([]string, 0, len(s.ByModel))
	for _, m := range s.ByModel {
		if m.Priced {
			parts = append(parts, fmt.Sprintf("%s %s", m.Model, money(m.Cost)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s tokens, cost unknown (not in the price table)", m.Model, thousands(m.Tokens.Total())))
	}
	return strings.Join(parts, "   ")
}

func coldLine(c CacheExpiry) string {
	if c.Responses == 0 {
		return fmt.Sprintf("none: no cache write followed a gap longer than its TTL (heuristic: %s)", c.Heuristic)
	}
	return fmt.Sprintf("%s re-written after a gap longer than its TTL, on %s (heuristic: %s)",
		money(c.Cost), countOf(c.Responses, "response"), c.Heuristic)
}

func refusalLine(r Refusals) string {
	if r.Responses == 0 && r.WithoutUsage == 0 {
		return "none (no response ended with stop_reason refusal)"
	}
	var parts []string
	if r.Responses > 0 {
		parts = append(parts, fmt.Sprintf("%s ended in a refusal, %s", countOf(r.Responses, "response"), money(r.Cost)))
	}
	switch {
	case r.WithoutUsage == 1:
		parts = append(parts, "1 pre-output refusal was written without usage, so its billing cannot be read")
	case r.WithoutUsage > 1:
		parts = append(parts, fmt.Sprintf("%d pre-output refusals were written without usage, so their billing cannot be read", r.WithoutUsage))
	}
	return strings.Join(parts, "; ")
}

func attemptsLine(a ExtraAttempts) string {
	if a.Responses == 0 {
		return "none (no response carried more than one attempt)"
	}
	return fmt.Sprintf("%s carried %s: %s tokens spent on the extra attempts, cost unknown (%s)",
		countOf(a.Responses, "response"), countOf(a.Attempts, "extra attempt"), thousands(a.Tokens.Total()), a.CostUnknownReason)
}

// silentLine is the only-we-can line. Its two unknown cases say WHY they are
// unknown, and neither prints a figure: a machine with no store, and a store
// that recorded none of these sessions, have checked nothing, and "$0.00
// across 0 turns" would claim they had.
func silentLine(s *Summary) string {
	j := s.SilentFailureTurns
	const lead = "in turns that ended with a failure the summary never mentioned: "
	switch {
	case j.Store == StoreNotConsulted:
		return ""
	case j.Transcripts == 0:
		// Nothing in the window to cover or not: "unknown ... none of the
		// 0 transcripts" was true and said nothing.
		return lead + "none: no transcript in the window\n"
	case j.Store == StoreNone:
		return lead + fmt.Sprintf("unknown\n  (rashomon has recorded nothing on this machine, so none of the %s is covered)\n",
			countOf(j.Transcripts, "transcript")) + notCoveredNames(j)
	case j.CoveredTranscripts == 0:
		return lead + fmt.Sprintf("unknown\n  (rashomon recorded none of the %s, so none is covered)\n",
			countOf(j.Transcripts, "transcript")) + notCoveredNames(j)
	}
	var b strings.Builder
	b.WriteString(lead)
	// Three answers, each saying only what the record supports: no turn fired
	// is a checked none, not "at least none across 0 turns"; turns that fired
	// with no response of theirs in the window are counted without a figure,
	// not "at least none"; and a figure is a floor (Bound), so "at least".
	// A failed turn with no words to judge (Unjudged) is neither: with any,
	// "none" holds only among the turns that were checked, never "no recorded
	// turn ... left it out".
	switch {
	case j.Turns == 0 && j.Unjudged == 0:
		b.WriteString("none found (no recorded turn with a failed call ended in a summary that left it out)\n")
	case j.Turns == 0:
		b.WriteString("none found in the turns that could be checked\n")
	case j.Cost.Priced == 0 && j.Cost.Unpriced == 0:
		fmt.Fprintf(&b, "%s, with no response in the window tied to %s\n", countOf(j.Turns, "turn"), itThem(j.Turns))
	case !j.Cost.Wholly():
		// Every response tied to the turns ran on a model the table does
		// not price: "at least unknown" is no figure.
		fmt.Fprintf(&b, "cost unknown (%s tokens on %s with no known rate) across %s\n",
			thousands(j.Cost.UnpricedTokens), countOf(j.Cost.Unpriced, "response"), countOf(j.Turns, "turn"))
	default:
		fmt.Fprintf(&b, "at least %s across %s\n", money(j.Cost), countOf(j.Turns, "turn"))
	}
	fmt.Fprintf(&b, "  (from rashomon's record; %d of %s %s recorded, so this covers only those",
		j.CoveredTranscripts, countOf(j.Transcripts, "transcript"), wasWere(j.CoveredTranscripts))
	if j.NotCoveredTranscripts > 0 {
		fmt.Fprintf(&b, "; %s in the other %d is not covered", money(j.NotCoveredCost), j.NotCoveredTranscripts)
	}
	b.WriteString(")\n")
	b.WriteString(notCoveredNames(j))
	if j.Unjudged == 1 {
		b.WriteString("  (1 turn with a failed call could not be checked: no final message could be tied to its prompt)\n")
	} else if j.Unjudged > 1 {
		fmt.Fprintf(&b, "  (%d turns with a failed call could not be checked: no final message could be tied to their prompts)\n", j.Unjudged)
	}
	if j.Turns > 0 {
		fmt.Fprintf(&b, "  (%s)\n", j.Bound)
	}
	return b.String()
}

// maxNamed bounds how many not-covered sessions the text names; --json
// names every one.
const maxNamed = 5

// notCoveredNames names the sessions whose transcripts are not covered, so a
// reader can tell which conversations the line says nothing about.
func notCoveredNames(j SilentFailureTurns) string {
	n := j.NotCoveredSessions
	if len(n) == 0 {
		return ""
	}
	shown := n
	if len(shown) > maxNamed {
		shown = shown[:maxNamed]
	}
	more := ""
	if len(n) > len(shown) {
		more = fmt.Sprintf(" and %d more (--json names every one)", len(n)-len(shown))
	}
	return fmt.Sprintf("  (not covered: %s %s%s)\n", sessionWord(len(n)), strings.Join(shown, ", "), more)
}

func sessionWord(n int) string {
	if n == 1 {
		return "session"
	}
	return "sessions"
}

func itThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func savingLine(sv Saving) string {
	switch sv.Kind {
	case SavingColdCache:
		line := fmt.Sprintf("%s of cache re-written after a gap longer than its TTL (heuristic)", money(sv.Cost))
		if sv.Hint == SavingHintLongerTTL {
			line += ": part was written with the 5m TTL, and the 1h TTL keeps a cache across pauses up to an hour"
		}
		return line
	case SavingSilentFailure:
		return fmt.Sprintf("%s bought a \"done\" in turns whose recorded failures the summary never mentioned", money(sv.Cost))
	}
	return money(sv.Cost)
}

func countOf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

// thousands renders n with comma separators.
func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
