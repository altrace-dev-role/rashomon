package spend

import (
	"bytes"
	"fmt"
	"io"
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
	}

	if line := silentLine(s); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
	}

	if len(s.Savings) > 0 {
		b.WriteString("\n")
		for i, sv := range s.Savings {
			label := ""
			if i == 0 {
				label = "savings"
			}
			fmt.Fprintf(&b, "%-14s%s\n", label, savingLine(sv))
		}
	}

	if s.Read.UnreadableDirs > 0 {
		fmt.Fprintf(&b, "\nnote: %s under projects/ could not be read; the transcripts in %s are not counted\n",
			countOf(s.Read.UnreadableDirs, "folder"), itThem(s.Read.UnreadableDirs))
	}
	if s.Read.UnparsedUsageLines > 0 {
		fmt.Fprintf(&b, "\nnote: %s that may carry usage could not be read (a malformed field, or a negative or implausibly large count), so %s not counted\n",
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
	fmt.Fprintf(&b, "\nout of scope: fast mode, Batch and partner (Bedrock, Vertex) pricing; long-context premiums\n")

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
	total := main.Nano + sub.Nano
	pct := func(n int64) string {
		if total == 0 || !main.Known() || !sub.Known() {
			return ""
		}
		return fmt.Sprintf(" (%d%%)", (n*100+total/2)/total)
	}
	return fmt.Sprintf("main %s%s   subagents %s%s", money(main), pct(main.Nano), money(sub), pct(sub.Nano))
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
	if r.Responses == 0 {
		return "none (no response ended with stop_reason refusal)"
	}
	return fmt.Sprintf("%s ended in a refusal, %s", countOf(r.Responses, "response"), money(r.Cost))
}

func attemptsLine(a ExtraAttempts) string {
	if a.Responses == 0 {
		return "none (no response carried more than one attempt)"
	}
	return fmt.Sprintf("%s carried %s: %s tokens spent on declined attempts, cost unknown (%s)",
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
	switch {
	case j.Turns == 0:
		b.WriteString("none found (no recorded turn with a failed call ended in a summary that left it out)\n")
	case j.Cost.Priced == 0 && j.Cost.Unpriced == 0:
		fmt.Fprintf(&b, "%s, with no response in the window tied to %s\n", countOf(j.Turns, "turn"), itThem(j.Turns))
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
