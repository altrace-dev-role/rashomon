package slot

import (
	"errors"
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
// the runner-up unit when the claim is by word coverage, the symbols of it
// the request names, the files outside it that consume them, and the
// knowledge that applies to the unit as a whole.
type UnitFact struct {
	Path      RelPath
	Basis     string
	RunnerUp  RelPath
	Names     []Ident
	Consumers []RelPath
	Lessons   []KnowledgeID
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

// validDisclosure is whether the basis is one of the three words or empty
// and the runner-up, when named, passes the path check.
func (f UnitFact) validDisclosure() error {
	if f.Basis != "" && f.Basis != BasisNamed && f.Basis != BasisUnitName && f.Basis != BasisCoverage {
		return errors.New("slot: unit line has a basis outside named, unit-name and coverage")
	}
	if f.RunnerUp != "" {
		return f.RunnerUp.Validate()
	}
	return nil
}

// bindUnit leaves out and counts what a check refuses and cuts each list
// at its cap; the unit's path, a basis outside the three words and a
// runner-up path that fails a check refuse the line.
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
// 'cmd/rashomon/report_platform.go'; D145 applies", the basis clause in
// one of its three forms or absent, or "a unit on record" with counts in
// the record form.
func (l GraphLine) unitText() string {
	u, m := *l.unit, l.more
	head := "unit " + u.Path.Text()
	if b := basisText(u.Basis, u.RunnerUp); b != "" {
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
	if len(parts) == 0 {
		return head
	}
	return head + " " + strings.Join(parts, "; ")
}

// basisText is the unit line's disclosure clause: how the unit was
// claimed, and for a claim by word coverage the runner-up when there was
// one.
func basisText(basis string, runnerUp RelPath) string {
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
