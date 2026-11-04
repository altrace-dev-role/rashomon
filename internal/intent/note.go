package intent

import (
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The finding's keys: the first prompt's note carries the brief, every
// later one is a note; ReadDelivered reads the brief's key back.
const (
	keyBrief = "brief"
	keyNote  = "note"
)

// Finding is the request's task-preflight finding, or false when the
// request has nothing new to say. The first note of a session carries the
// brief when b has anything in it, whatever the request names. Otherwise a
// request that named no path and no identifier gets none, and one that did
// gets a note only when a path, rule, gate, symbol or evidence kind is new
// to the session or the request was cut. The model is sent only the lines
// that list something; the brief, the evidence and the unknowns print to the
// person (TOKEN-BUDGET1). Paths and rules given earlier are counted;
// gates and evidence print in full. The note steps down to fit the briefing
// cap: whys dropped first, then one item per list.
func Finding(in Intent, d Delivered, b *Brief, afterSeq int64) (steer.Finding, bool) {
	return FindingKeyed(in, d, b, afterSeq, nil)
}

// FindingKeyed is Finding for a line recorded at record scope v4 (card
// PREFLIGHT1b). digest, when not nil, is the keyed digest of a path
// (PathDigest): the delivered set holds digests, so a candidate path is
// digested before the "given earlier" check, and the finding's Record is the
// message the steer line holds, each path list as its count and its keyed
// digests. The message that prints is Finding's, unchanged. A nil digest is
// scope v5: the line records what printed. A switch from v4 to v5 in the
// middle of a session can announce a path once more, since the digests the
// v4 lines hold do not match the paths a v5 check looks for; that costs one
// repeated path in one note and is harmless.
func FindingKeyed(in Intent, d Delivered, b *Brief, afterSeq int64, digest func(string) string) (steer.Finding, bool) {
	f, ok, _ := FindingRiding(in, d, b, afterSeq, digest, nil)
	return f, ok
}

// Ride is an approved lesson a later prompt's note carries as its last line
// (LEARN-RIDE1): its id and its text, both the project's ledger's.
type Ride struct{ ID, Text string }

// FindingRiding is FindingKeyed with lesson, when not nil, as the last line of a
// later prompt's note (LEARN-RIDE1, R13 D1), so a lesson arrives on the
// prompt it is about, one note still and inside the note's cap. The lesson
// rides at the step the note alone prints at, dropping the unknowns line if
// it must; a note that cannot hold it there is the note alone, and rode is
// false, so the lesson waits for a prompt that carries no note. A lesson
// never steps the note down (#197 F1, R55). The first prompt's note has no
// lesson form and carries none. At record scope v4 the line is kept as
// it printed: the lesson's text is the ledger's, not the request's.
func FindingRiding(in Intent, d Delivered, b *Brief, afterSeq int64, digest func(string) string, lesson *Ride) (f steer.Finding, ok, rode bool) {
	first := !d.BriefGiven && b.has()
	var paths []string
	var rules []Rule
	earlierPaths, earlierRules := 0, 0
	seen := func(p string) bool {
		if digest != nil {
			return d.Paths[digest(p)]
		}
		return d.Paths[p]
	}
	for _, p := range in.Paths {
		if seen(p) {
			earlierPaths++
		} else {
			paths = append(paths, p)
		}
	}
	for _, r := range in.Rules {
		if d.Rules[r.ID] {
			earlierRules++
		} else {
			rules = append(rules, r)
		}
	}
	fresh := len(paths) > 0 || len(rules) > 0 || len(in.Symbols) > 0
	for _, g := range in.Gates {
		fresh = fresh || !d.Gates[g.Words]
	}
	for _, k := range in.Evidence {
		fresh = fresh || !d.Evidence[k]
	}
	if !first && (!in.Named || !fresh && !in.Truncated) {
		return steer.Finding{}, false, false
	}

	unknown := in.Unknown
	if first {
		if b.Module == "" {
			unknown = append(unknown[:len(unknown):len(unknown)], unknownNoModule)
		}
		if b.WalkCapped {
			unknown = append(unknown[:len(unknown):len(unknown)], unknownCapped)
		}
	}
	source := "inferred from the request"
	if in.FromTask {
		source = "task file"
	}
	key := keyNote
	if first {
		key = keyBrief
	}
	prio, _ := pattern.Priority(pattern.TaskPreflight)
	// The symbols line, when the graph proved any, is the last thing the
	// note gives up: every step is tried with it, then every step without.
	withSymbols := []bool{false}
	if len(in.Symbols) > 0 {
		withSymbols = []bool{true, false}
	}
	for _, ws := range withSymbols {
		for step := range 3 {
			n := -1 // every item
			if step == 2 {
				n = 1
			}
			pf := render.TaskPreflightBinding{
				Paths:    pathSet(paths, n, earlierPaths),
				Rules:    ruleRefs(rules, n, earlierRules, in.Withheld, step == 0),
				Gates:    gateRefs(in.Gates, n, in.Learned),
				Source:   slot.NewEnum(slot.EnumEvidenceSource, source),
				Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, in.Evidence),
				Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, unknown),
			}
			var sf *slot.SymbolFacts
			if ws {
				f := symbolFacts(in.Symbols, n)
				sf = &f
			}
			// With symbols, the unknowns line goes before the symbols do.
			m, rec, err := note(pf, sf, first, b, n, true, nil, digest)
			if err != nil && sf != nil {
				m, rec, err = note(pf, sf, first, b, n, false, nil, digest)
			}
			if err != nil {
				continue
			}
			// The note prints at the first step it renders alone. A lesson
			// rides at that step, its unknowns line dropped if it must be, or
			// not at all: the rules a person trusted keep their whys, and the
			// lesson waits a prompt (#197 F1, R55).
			if lesson != nil && !first {
				for _, unknowns := range [...]bool{true, false} {
					if lm, lrec, err := note(pf, sf, first, b, n, unknowns, lesson, digest); err == nil {
						m, rec, rode = lm, lrec, true
						break
					}
				}
			}
			return steer.Finding{Pattern: pattern.TaskPreflight, Key: key, Priority: prio, MaxLevel: steer.LevelNudge,
				StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: afterSeq}, true, rode
		}
	}
	return steer.Finding{}, false, false
}

// symbolFacts is the symbols slot for in's proven symbols, the first n (every
// one for n < 0) listed.
func symbolFacts(syms []Symbol, n int) slot.SymbolFacts {
	shown, more := listed(syms, n)
	items := make([]slot.SymbolFact, len(shown))
	for i, s := range shown {
		items[i] = slot.SymbolFact{Name: s.Name, Dir: slot.Path(s.Dir), Callers: s.Callers, Packages: s.Packages}
	}
	return slot.NewSymbolFacts(items, more)
}

// note is the message one rung of FindingRiding's ladder prints and, at
// record scope v4 (digest not nil), the message its steer line holds. sf,
// when not nil, is the symbols line, which takes the unknowns' place where
// the six-line cap leaves no room for both: on the first prompt and beside
// a lesson.
func note(pf render.TaskPreflightBinding, sf *slot.SymbolFacts, first bool, b *Brief, n int, unknowns bool, r *Ride, digest func(string) string) (m, rec render.Message, err error) {
	var binding any = later(pf, sf, unknowns, r)
	if first {
		binding = firstBinding(pf, sf, b, n, pathSet(b.Instructions, n, 0), pathSet(b.Manifests, n, 0))
	}
	if m, err = render.New(pattern.TaskPreflight, binding); err != nil || digest == nil {
		return m, rec, err
	}
	// The record form is m's binding with each path list as a count and
	// keyed digests, 64 hex by construction, and a count is never longer
	// than the paths it counts, so it renders wherever m did and this
	// error is not returned after m rendered (#170 F7,
	// TestPF1w_RecordFormNeverStepsTheNoteDown). A riding lesson's line is
	// the same in both.
	rec, err = render.New(pattern.TaskPreflight, recordBinding(pf, sf, first, b, n, unknowns, r, digest))
	return m, rec, err
}

// firstBinding is the first prompt's note binding, with the symbols line in
// the unknowns' place when sf is not nil.
func firstBinding(pf render.TaskPreflightBinding, sf *slot.SymbolFacts, b *Brief, n int, instructions, manifests slot.PathSet) any {
	if sf != nil {
		return render.TaskPreflightFirstSymbolsBinding{
			Instructions: instructions, Manifests: manifests, GoDirs: slot.Count(b.GoDirs),
			Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source, Evidence: pf.Evidence, Symbols: *sf,
		}
	}
	return render.TaskPreflightFirstBinding{
		Instructions: instructions, Manifests: manifests, GoDirs: slot.Count(b.GoDirs),
		Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source, Evidence: pf.Evidence, Unknown: pf.Unknown,
	}
}

// later is a later prompt's note binding: pf alone, or with r as its last
// line, with its unknowns line or, when unknowns is false, without it; with
// sf, the symbols line before the unknowns, or before the lesson in the
// unknowns' place.
func later(pf render.TaskPreflightBinding, sf *slot.SymbolFacts, unknowns bool, r *Ride) any {
	switch {
	case sf != nil && r != nil:
		return render.TaskPreflightSymbolsLessonBinding{Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source,
			Evidence: pf.Evidence, Symbols: *sf, Lesson: slot.LessonID(r.ID), Text: slot.Approved(r.Text)}
	case sf != nil && unknowns:
		return render.TaskPreflightSymbolsBinding{Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source,
			Evidence: pf.Evidence, Symbols: *sf, Unknown: pf.Unknown}
	case sf != nil:
		return render.TaskPreflightSymbolsKnownBinding{Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source,
			Evidence: pf.Evidence, Symbols: *sf}
	case r == nil:
		return pf
	case unknowns:
		return render.TaskPreflightLessonBinding{Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source,
			Evidence: pf.Evidence, Unknown: pf.Unknown, Lesson: slot.LessonID(r.ID), Text: slot.Approved(r.Text)}
	}
	return render.TaskPreflightLessonKnownBinding{Paths: pf.Paths, Rules: pf.Rules, Gates: pf.Gates, Source: pf.Source,
		Evidence: pf.Evidence, Lesson: slot.LessonID(r.ID), Text: slot.Approved(r.Text)}
}

// recordBinding is the binding the steer line holds at record scope v4: pf's
// every path list as keyed digests (the same items pf lists at this step),
// the symbols as their count, the rest as it is, a riding lesson's line
// included.
func recordBinding(pf render.TaskPreflightBinding, sf *slot.SymbolFacts, first bool, b *Brief, n int, unknowns bool, r *Ride, digest func(string) string) any {
	keyed := func(s slot.PathSet) slot.PathSet {
		ps := s.Paths()
		ds := make([]string, len(ps))
		for i, p := range ps {
			ds[i] = digest(string(p))
		}
		return slot.NewDigestPathSet(ds, s.More(), s.Earlier())
	}
	pf.Paths = keyed(pf.Paths)
	if sf != nil {
		c := sf.Counted()
		sf = &c
	}
	if !first {
		return later(pf, sf, unknowns, r)
	}
	return firstBinding(pf, sf, b, n, keyed(pathSet(b.Instructions, n, 0)), keyed(pathSet(b.Manifests, n, 0)))
}

// listed is the first n items (every item for n < 0) and the count of the
// rest.
func listed[T any](items []T, n int) ([]T, int) {
	if n < 0 || len(items) <= n {
		return items, 0
	}
	return items[:n], len(items) - n
}

func pathSet(paths []string, n, earlier int) slot.PathSet {
	shown, more := listed(paths, n)
	ps := make([]slot.Path, len(shown))
	for i, p := range shown {
		ps[i] = slot.Path(p)
	}
	return slot.NewPathSet(ps, more, earlier)
}

func ruleRefs(rules []Rule, n, earlier, withheld int, whys bool) slot.RuleRefs {
	shown, more := listed(rules, n)
	refs := make([]slot.RuleRef, len(shown))
	for i, r := range shown {
		refs[i] = slot.RuleRef{ID: r.ID, Trust: slot.NewEnum(slot.EnumTrust, string(r.Trust))}
		if whys {
			refs[i].Why = slot.Approved(r.Why)
		}
	}
	return slot.NewRuleRefs(refs, more, earlier, withheld)
}

func gateRefs(gates []Gate, n, learned int) slot.GateRefs {
	shown, more := listed(gates, n)
	refs := make([]slot.GateRef, len(shown))
	for i, g := range shown {
		refs[i] = slot.GateRef{Words: slot.Gate(g.Words), Trust: slot.NewEnum(slot.EnumTrust, string(g.Trust))}
	}
	return slot.NewGateRefs(refs, more, learned)
}
