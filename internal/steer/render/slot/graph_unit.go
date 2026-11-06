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

// UnitFact is the unit that owns the task: its path, the symbols of it the
// request names, the files outside it that consume them, and the knowledge
// that applies to the unit as a whole.
type UnitFact struct {
	Path      RelPath
	Names     []Ident
	Consumers []RelPath
	Lessons   []KnowledgeID
}

// The roles a code line carries beside none.
const (
	RoleReuse    = "reuse"
	RoleConsumer = "consumer"
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

func (f UnitFact) validate() error {
	if err := f.Path.Validate(); err != nil {
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

// bindUnit leaves out and counts what a check refuses and cuts each list
// at its cap; the unit's path refuses the line.
func (l GraphLine) bindUnit() (Value, error) {
	u, m := *l.unit, l.more
	if err := u.Path.Validate(); err != nil {
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

// unitText is "unit 'internal/platform' declares 'Read', 'Callers'; is
// consumed by 'cmd/rashomon/report_platform.go'; D145 applies", or "a unit
// on record" with counts in the record form.
func (l GraphLine) unitText() string {
	u, m := *l.unit, l.more
	head := "unit " + u.Path.Text()
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

func pathTexts(items []RelPath) []string {
	out := make([]string, len(items))
	for i, p := range items {
		out[i] = p.Text()
	}
	return out
}
