package directive

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// MaxLines is the most quotes the briefing holds under its header: the
// spec's five lines (tech 4.2.4 :1255), each one directive, a correction
// with the words it restates.
const MaxLines = 5

// Briefing is the continuity briefing at the compaction b: every directive in
// ds on a line before b, so lost to it, quoted newest first (quotesBefore).
// It keeps the newest whole and drops the oldest whole, five lines at most
// and inside the briefing cap, the header counting what it dropped as
// earlier and the words the line law refused as withheld. digest is the
// keyed digest of the person's words (intent.SaidDigest under the install
// key): the finding's Record holds each quote as its digest and byte length,
// never the words (D160), so with no digest there is no briefing. Its key is
// b's uuid, so one compaction's briefing is given once and the next
// compaction's again (D-b). It stands at priority 0, the own-channel
// pattern's, admitted at SessionStart alone (steer's ownChannel).
func Briefing(ds []Directive, b Boundary, digest func(string) string, afterSeq int64) (steer.Finding, bool) {
	if digest == nil || b.UUID == "" {
		return steer.Finding{}, false
	}
	qs, withheld := quotesBefore(ds, b.Line)
	for n := min(MaxLines, len(qs)); n >= 1; n-- {
		m, err := render.New(pattern.ContinuityBriefing, binding(qs[:n], len(qs)-n, withheld, nil))
		if err != nil {
			continue
		}
		rec, err := render.New(pattern.ContinuityBriefing, binding(qs[:n], len(qs)-n, withheld, digest))
		if err != nil {
			continue
		}
		return steer.Finding{Pattern: pattern.ContinuityBriefing, Key: b.UUID, MaxLevel: steer.LevelNudge,
			StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: afterSeq}, true
	}
	return steer.Finding{}, false
}

// quote is one line of the briefing: a directive's words and, for a
// correction, the words of the directive it restates, or "".
type quote struct{ said, restates string }

// quotesBefore is the briefing's lines from ds, newest first: every directive
// on a line before line. A correction carries the words of the directive it
// restates when that one is among them, which then has no line of its own;
// when those words break the line law the correction stands alone. withheld
// counts the directives whose own words break it.
func quotesBefore(ds []Directive, line int) (qs []quote, withheld int) {
	byLine := make(map[int]Directive, len(ds))
	var live []Directive
	for _, d := range ds {
		if d.Line > 0 && d.Line < line {
			byLine[d.Line] = d
			live = append(live, d)
		}
	}
	slices.SortStableFunc(live, func(a, b Directive) int { return b.Line - a.Line })
	carried := map[int]bool{}
	for _, d := range live {
		if carried[d.Line] {
			continue
		}
		q := quote{said: d.Said}
		if e, ok := byLine[d.Restates]; ok && d.Kind == KindCorrection && d.Restates < d.Line {
			q.restates = e.Said
		}
		if q.restates != "" && slot.NewSaid(q.said, q.restates).Validate() != nil {
			q.restates = ""
		}
		if slot.NewSaid(q.said, "").Validate() != nil {
			withheld++
			continue
		}
		if q.restates != "" {
			carried[d.Restates] = true
		}
		qs = append(qs, q)
	}
	return qs, withheld
}

// binding is the briefing's binding for qs: the words, or with digest not
// nil the record form, each quote's keyed digest and byte length.
func binding(qs []quote, earlier, withheld int, digest func(string) string) any {
	s := make([]slot.Said, len(qs))
	for i, q := range qs {
		s[i] = slot.NewSaid(q.said, q.restates)
		if digest != nil {
			s[i] = keyed(q, digest)
		}
	}
	e, w := slot.Count(earlier), slot.Count(withheld)
	switch len(s) {
	case 1:
		return render.ContinuitySaid1Binding{Earlier: e, Withheld: w, Said1: s[0]}
	case 2:
		return render.ContinuitySaid2Binding{Earlier: e, Withheld: w, Said1: s[0], Said2: s[1]}
	case 3:
		return render.ContinuitySaid3Binding{Earlier: e, Withheld: w, Said1: s[0], Said2: s[1], Said3: s[2]}
	case 4:
		return render.ContinuitySaid4Binding{Earlier: e, Withheld: w, Said1: s[0], Said2: s[1], Said3: s[2], Said4: s[3]}
	}
	return render.ContinuitySaid5Binding{Earlier: e, Withheld: w, Said1: s[0], Said2: s[1], Said3: s[2], Said4: s[3], Said5: s[4]}
}

// keyed is q's record form.
func keyed(q quote, digest func(string) string) slot.Said {
	if q.restates == "" {
		return slot.NewSaidDigest(digest(q.said), len(q.said), "", 0)
	}
	return slot.NewSaidDigest(digest(q.said), len(q.said), digest(q.restates), len(q.restates))
}
