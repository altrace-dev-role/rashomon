package slot

import (
	"errors"
	"slices"
	"strings"
)

// The unit line and the roles (PLAN 5.2, W-M1.12): the brief names the
// unit that owns the task, the abstraction to reuse, the non-local
// consumers, and the decisions, lessons and requirements that apply. The
// unit is the fourth fact kind of a line; a code line carries a role word
// (RoleReuse, RoleConsumer) or none, which is the target's own file. The
// words equal steer's graph kinds (source.go), pinned there since slot
// cannot import steer.

// UnitFact is the unit that owns the task: its path, how the unit was
// claimed (Basis, one of the three words below; empty renders as before),
// the runner-up unit when the claim is by word coverage or a candidate's,
// the symbols of it the request names, the files outside it that consume
// them, and the knowledge that applies to the unit as a whole. Band is how
// far the claim is stated (BandStated, BandCandidate; empty renders as
// stated), and Unknown the kinds of role line the brief withheld, in the
// Unknown words below.
type UnitFact struct {
	Path      RelPath
	Basis     string
	Band      string
	RunnerUp  RelPath
	Names     []Ident
	Consumers []RelPath
	Lessons   []KnowledgeID
	Unknown   []string
}

// The basis of a unit claim (S3, AC-M1.1 clause 3), so a unit line is
// never a bare fact: the request named the unit's path, a name declared
// in it or a knowledge id that applies to it; the request held the unit's
// own name; or the unit's words covered the request's target phrase, the
// claim the ladder settles by score, whose line names the runner-up.
const (
	BasisNamed    = "named"
	BasisUnitName = "unit-name"
	BasisCoverage = "coverage"
)

// The bands a line is delivered in (W-M1.38, abstention by evidence): a
// line of a kind whose delivered precision is at least 0.8 is stated, from
// 0.5 a candidate, which a unit line says beside its runner-up; below 0.5
// the line is withheld and never reaches a slot. Empty renders as stated.
const (
	BandStated    = "stated"
	BandCandidate = "candidate"
)

// The kinds of role line a unit line names as not known: the target
// file, the reuse, the consumers and the knowledge that applies. The
// last two say "of the change" and "to the change", so the clause is not
// read against the unit's own consumers and records the line lists.
const (
	UnknownTarget    = "target"
	UnknownReuse     = RoleReuse
	UnknownConsumer  = RoleConsumer
	UnknownKnowledge = "knowledge"
)

// unknownWords is each Unknown kind as the line says it, in the order the
// line lists them; the unit comes first and only a not-known line names
// it (graph_notknown.go).
var unknownWords = []struct{ kind, text string }{
	{UnknownUnit, "the unit to change"},
	{UnknownTarget, "the file to change"},
	{UnknownReuse, "the code to reuse"},
	{UnknownConsumer, "the consumers of the change"},
	{UnknownKnowledge, "the records that apply to the change"},
}

// The roles a code line carries beside none.
const (
	RoleReuse    = "reuse"
	RoleConsumer = "consumer"
)

// The basis of a reuse claim (W-M1.25, AC-M1.5), so a reuse line is never
// a bare fact: the request named the file's code after a use word; the
// file holds the code the addition phrase names; or the addition's
// existing code calls the file's code.
const (
	ReuseByUse      = "use"
	ReuseByAddition = "addition"
	ReuseByCall     = "call"
)

// KnowledgeRequirement is the kind of a product requirement row (the truth
// table): what the repository declares it owes, applied to the code that
// evidences it.
const KnowledgeRequirement = "requirement"

// maxLineConsumers caps the consumer files a unit line lists.
const maxLineConsumers = 2

// NewUnitLine is a line for f.
func NewUnitLine(f UnitFact) GraphLine { return GraphLine{unit: &f} }

// Unit is the line's unit fact and whether it is a unit line.
func (l GraphLine) Unit() (UnitFact, bool) {
	if l.unit == nil {
		return UnitFact{}, false
	}
	return *l.unit, true
}

// knowledgeKind is whether k is a kind a knowledge line names.
func knowledgeKind(k string) bool {
	return k == KnowledgeLesson || k == KnowledgeDecision || k == KnowledgeRequirement
}

// validRole is whether r is a role a code line carries.
func validRole(r string) error {
	if r != "" && r != RoleReuse && r != RoleConsumer {
		return errors.New("slot: code line has a role outside reuse and consumer")
	}
	return nil
}

// validBasis is whether b is empty or one of the three reuse words on a
// reuse line.
func validBasis(role, b string) error {
	if b == "" {
		return nil
	}
	if role != RoleReuse {
		return errors.New("slot: a basis on a code line that is not a reuse")
	}
	if b != ReuseByUse && b != ReuseByAddition && b != ReuseByCall {
		return errors.New("slot: reuse line has a basis outside use, addition and call")
	}
	return nil
}

// validBand is whether b is empty or one of the two band words.
func validBand(b string) error {
	if b != "" && b != BandStated && b != BandCandidate {
		return errors.New("slot: line has a band outside stated and candidate")
	}
	return nil
}

func (f UnitFact) validate() error {
	if err := f.Path.Validate(); err != nil {
		return err
	}
	if err := f.validDisclosure(); err != nil {
		return err
	}
	if err := validIdents(f.Names); err != nil {
		return err
	}
	for _, c := range f.Consumers {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return validIDs(f.Lessons)
}

// validDisclosure is whether the basis is one of the three words or empty,
// the band one of the two or empty, each unknown kind one of the four and
// named once, and the runner-up, when named, passes the path check.
func (f UnitFact) validDisclosure() error {
	if f.Basis != "" && f.Basis != BasisNamed && f.Basis != BasisUnitName && f.Basis != BasisCoverage {
		return errors.New("slot: unit line has a basis outside named, unit-name and coverage")
	}
	if err := validBand(f.Band); err != nil {
		return err
	}
	if err := validUnknown(f.Unknown); err != nil {
		return err
	}
	if f.RunnerUp != "" {
		return f.RunnerUp.Validate()
	}
	return nil
}

// bindUnit leaves out and counts what a check refuses and cuts each list
// at its cap; the unit's path, a basis outside the three words, a band
// outside the two, an unknown kind outside the four and a runner-up path
// that fails a check refuse the line.
func (l GraphLine) bindUnit() (Value, error) {
	u, m := *l.unit, l.more
	if err := u.Path.Validate(); err != nil {
		return nil, err
	}
	if err := u.validDisclosure(); err != nil {
		return nil, err
	}
	u.Names, m.Names = keptIdents(u.Names, m.Names, maxLineIdents)
	u.Consumers, m.Consumers = keptPaths(u.Consumers, m.Consumers, maxLineConsumers)
	u.Lessons, m.Lessons = keptIDs(u.Lessons, m.Lessons)
	return GraphLine{unit: &u, more: m}, nil
}

func keptPaths(items []RelPath, more, cap int) ([]RelPath, int) {
	kept := make([]RelPath, 0, len(items))
	for _, p := range items {
		if p.Validate() != nil {
			more++
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) > cap {
		return kept[:cap], more + len(kept) - cap
	}
	return kept, more
}

// unitText is "unit 'internal/platform' (by word coverage; runner-up
// 'internal/verify') declares 'Read', 'Callers'; is consumed by
// 'cmd/rashomon/report_platform.go'; D145 applies; not known: the code to
// reuse", the basis clause in one of its forms or absent, or "a unit on
// record" with counts in the record form; the kinds not known close it,
// after a semicolon even when nothing else is listed, so the clause is
// never read as the unit's own.
func (l GraphLine) unitText() string {
	u, m := *l.unit, l.more
	head := "unit " + u.Path.Text()
	if b := basisText(u.Basis, u.Band, u.RunnerUp); b != "" {
		head += " " + b
	}
	if l.keyed {
		head = "a unit on record"
	}
	var parts []string
	if p := phrase("declares", identTexts(u.Names), m.Names, "names"); p != "" {
		parts = append(parts, p)
	}
	if p := phrase("is consumed by", pathTexts(u.Consumers), m.Consumers, "files"); p != "" {
		parts = append(parts, p)
	}
	if p := applies(u.Lessons, m.Lessons); p != "" {
		parts = append(parts, p)
	}
	text := head
	if len(parts) > 0 {
		text += " " + strings.Join(parts, "; ")
	}
	if p := notKnown(u.Unknown); p != "" {
		text += "; " + p
	}
	return text
}

// notKnown is "not known: the file to change, the code to reuse" for the
// kinds the brief withheld, in unknownWords' order, or "" for none.
func notKnown(kinds []string) string {
	var texts []string
	for _, w := range unknownWords {
		if slices.Contains(kinds, w.kind) {
			texts = append(texts, w.text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	return "not known: " + strings.Join(texts, ", ")
}

// validUnknown is whether each kind is one of the four role kinds and
// named once: the kinds of role line a brief withheld.
func validUnknown(kinds []string) error {
	for i, k := range kinds {
		if k == UnknownUnit || unknownText(k) == "" || slices.Contains(kinds[:i], k) {
			return errors.New("slot: line names an unknown kind outside the four, or twice")
		}
	}
	return nil
}

// unknownText is how a line says kind k is not known, or "" for a kind
// outside the five.
func unknownText(k string) string {
	for _, w := range unknownWords {
		if w.kind == k {
			return w.text
		}
	}
	return ""
}

// basisText is the unit line's disclosure clause: how the unit was
// claimed, and for a claim by word coverage the runner-up when there was
// one. A candidate says so before its basis and always names its
// runner-up, the pair the brief stands on: "(a candidate by its name;
// runner-up 'internal/verify')".
func basisText(basis, band string, runnerUp RelPath) string {
	if band == BandCandidate {
		s := "(a candidate"
		switch basis {
		case BasisNamed:
			s += " named in the request"
		case BasisUnitName:
			s += " by its name"
		case BasisCoverage:
			s += " by word coverage"
		}
		if runnerUp != "" {
			s += "; runner-up " + runnerUp.Text()
		}
		return s + ")"
	}
	switch basis {
	case BasisNamed:
		return "(named in the request)"
	case BasisUnitName:
		return "(by its name)"
	case BasisCoverage:
		if runnerUp != "" {
			return "(by word coverage; runner-up " + runnerUp.Text() + ")"
		}
		return "(by word coverage)"
	}
	return ""
}

// codeClause is a code line's disclosure clause: "(a candidate)" for a
// line in the candidate band, its reuse basis after it ("(a candidate;
// named to use)"), or the reuse basis alone as before.
func codeClause(band, basis string) string {
	b := strings.TrimSuffix(strings.TrimPrefix(reuseBasisText(basis), "("), ")")
	switch {
	case band == BandCandidate && b != "":
		return "(a candidate; " + b + ")"
	case band == BandCandidate:
		return "(a candidate)"
	case b != "":
		return "(" + b + ")"
	}
	return ""
}

// reuseBasisText is the reuse line's disclosure clause: why its file is
// the thing to reuse.
func reuseBasisText(basis string) string {
	switch basis {
	case ReuseByUse:
		return "(named to use)"
	case ReuseByAddition:
		return "(holds the addition's code)"
	case ReuseByCall:
		return "(called by the addition's code)"
	}
	return ""
}

func pathTexts(items []RelPath) []string {
	out := make([]string, len(items))
	for i, p := range items {
		out[i] = p.Text()
	}
	return out
}
