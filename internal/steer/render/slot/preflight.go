package slot

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The preflight note's slot types (internal/intent): lists that may be empty
// and then read "none", because a preflight line prints whether or not its
// source had anything to say. What a repository or a spec names is quoted
// and screened; an item that fails a check a repository can fail is left out
// and counted, never rendered, and the counts the producer passes (items
// given earlier, rules an untrusted spec holds, gates learned) are numbers,
// never text from their source.

// cut keeps the first maxListedPaths items and counts the rest into more.
func cut[T any](items []T, more int) ([]T, int) {
	if len(items) > maxListedPaths {
		return items[:maxListedPaths], more + len(items) - maxListedPaths
	}
	return items, more
}

// listText is a preflight list as it renders: the listed items joined by sep
// and "and {k} more", or "{k} left out" when none is listed but some were
// counted, then each non-empty tail; "none" when there is nothing at all.
func listText(items []string, sep string, more int, tails ...string) string {
	var parts []string
	switch {
	case len(items) > 0 && more > 0:
		parts = append(parts, strings.Join(items, sep)+" and "+strconv.Itoa(more)+" more")
	case len(items) > 0:
		parts = append(parts, strings.Join(items, sep))
	case more > 0:
		parts = append(parts, strconv.Itoa(more)+" left out")
	}
	for _, t := range tails {
		if t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

// countTail is "{n} {words}", or empty for no n.
func countTail(n int, words string) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n) + " " + words
}

// negative refuses a list whose counts are below their floors.
func negative(slot string, counts ...int) error {
	for _, n := range counts {
		if n < 0 {
			return fmt.Errorf("slot: %s has a negative count", slot)
		}
	}
	return nil
}

// trustOf checks that e is a value of EnumTrust, not of another table.
func trustOf(slot string, e Enum) error {
	if e.table != EnumTrust {
		return fmt.Errorf("slot: %s trust is not from the trust table", slot)
	}
	return e.Validate()
}

// PathSet is the paths a preflight line names: Path's checks, at most three
// listed, a secret-shaped path left out and counted, and the paths an earlier
// note of the session named as a count. Unlike PathList it may be empty.
type PathSet struct {
	paths   []Path
	more    int
	earlier int
}

// NewPathSet lists paths, with more the named paths beyond them and earlier
// the paths an earlier note of the session already gave.
func NewPathSet(paths []Path, more, earlier int) PathSet {
	return PathSet{paths: slices.Clone(paths), more: more, earlier: earlier}
}

func (s PathSet) Paths() []Path { return slices.Clone(s.paths) }
func (s PathSet) More() int     { return s.more }
func (s PathSet) Earlier() int  { return s.earlier }

func (s PathSet) Validate() error {
	if err := negative("path set", s.more, s.earlier); err != nil {
		return err
	}
	for _, p := range s.paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s PathSet) Text() string {
	shown, more := cut(s.paths, s.more)
	texts := make([]string, len(shown))
	for i, p := range shown {
		texts[i] = p.Text()
	}
	return listText(texts, ", ", more, countTail(s.earlier, "given earlier"))
}

// bind leaves out and counts a secret-shaped path; a path of any other wrong
// shape refuses the set, as it does PathList.
func (s PathSet) bind() (Value, error) {
	if err := negative("path set", s.more, s.earlier); err != nil {
		return nil, err
	}
	kept, more := make([]Path, 0, len(s.paths)), s.more
	for _, p := range s.paths {
		err := p.Validate()
		switch {
		case errors.Is(err, ErrSecretShaped):
			more++
		case err != nil:
			return nil, err
		default:
			kept = append(kept, p)
		}
	}
	kept, more = cut(kept, more)
	return PathSet{paths: kept, more: more, earlier: s.earlier}, nil
}

// RuleRef is one spec rule a preflight line names: its id, the trust of the
// spec that holds it, and its why, which renders only when Approved passes.
type RuleRef struct {
	ID    string
	Trust Enum
	Why   Approved
}

// specRuleIDRE is the spec's rule id pattern (spec.idString).
var specRuleIDRE = &lazyRegexp{pattern: `^[a-z0-9][a-z0-9._-]{0,47}$`}

func (r RuleRef) idErr() error {
	if err := charset("rule id", r.ID, specRuleIDRE); err != nil {
		return err
	}
	return screened("rule id", r.ID)
}

func (r RuleRef) text() string {
	s := quoted(r.ID) + " (" + r.Trust.Text() + ")"
	if r.Why != "" && r.Why.Validate() == nil {
		s += " " + r.Why.Text()
	}
	return s
}

// RuleRefs is the rules a preflight line names, at most three listed, then
// the rules an earlier note gave and those an untrusted spec holds, which
// are counted and never quoted.
type RuleRefs struct {
	items    []RuleRef
	more     int
	earlier  int
	withheld int
}

// NewRuleRefs lists items, with more the applying rules beyond them, earlier
// those an earlier note gave and withheld those an untrusted spec holds.
func NewRuleRefs(items []RuleRef, more, earlier, withheld int) RuleRefs {
	return RuleRefs{items: slices.Clone(items), more: more, earlier: earlier, withheld: withheld}
}

func (r RuleRefs) Items() []RuleRef { return slices.Clone(r.items) }
func (r RuleRefs) More() int        { return r.more }
func (r RuleRefs) Earlier() int     { return r.earlier }
func (r RuleRefs) Withheld() int    { return r.withheld }

func (r RuleRefs) Validate() error {
	if err := negative("rule refs", r.more, r.earlier, r.withheld); err != nil {
		return err
	}
	for _, it := range r.items {
		if err := trustOf("rule", it.Trust); err != nil {
			return err
		}
		if err := it.idErr(); err != nil {
			return err
		}
	}
	return nil
}

func (r RuleRefs) Text() string {
	shown, more := cut(r.items, r.more)
	texts := make([]string, len(shown))
	for i, it := range shown {
		texts[i] = it.text()
	}
	return listText(texts, "; ", more, countTail(r.earlier, "given earlier"), countTail(r.withheld, "not shown, spec untrusted"))
}

// bind refuses a trust outside the trust table (the producer's error), leaves
// out and counts an id the spec's pattern or the secret checks refuse, and
// drops a why Approved refuses, so the bound list holds no text that may not
// render.
func (r RuleRefs) bind() (Value, error) {
	if err := negative("rule refs", r.more, r.earlier, r.withheld); err != nil {
		return nil, err
	}
	kept, more := make([]RuleRef, 0, len(r.items)), r.more
	for _, it := range r.items {
		if err := trustOf("rule", it.Trust); err != nil {
			return nil, err
		}
		if it.idErr() != nil {
			more++
			continue
		}
		if it.Why != "" && it.Why.Validate() != nil {
			it.Why = ""
		}
		kept = append(kept, it)
	}
	kept, more = cut(kept, more)
	return RuleRefs{items: kept, more: more, earlier: r.earlier, withheld: r.withheld}, nil
}

// GateRef is one gate a preflight line names: its command words and the
// trust of what declared it.
type GateRef struct {
	Words Gate
	Trust Enum
}

// GateRefs is the gates a preflight line names, at most three listed, then
// how many gate families earlier sessions made required; learned is -1 when
// the project has no index, which reads as none.
type GateRefs struct {
	items   []GateRef
	more    int
	learned int
}

// NewGateRefs lists items, with more the gates beyond them and learned the
// families earlier sessions made required (-1: no index).
func NewGateRefs(items []GateRef, more, learned int) GateRefs {
	return GateRefs{items: slices.Clone(items), more: more, learned: learned}
}

func (g GateRefs) Items() []GateRef { return slices.Clone(g.items) }
func (g GateRefs) More() int        { return g.more }
func (g GateRefs) Learned() int     { return g.learned }

func (g GateRefs) Validate() error {
	if err := negative("gate refs", g.more, g.learned+1); err != nil {
		return err
	}
	for _, it := range g.items {
		if err := trustOf("gate", it.Trust); err != nil {
			return err
		}
		if err := it.Words.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (g GateRefs) Text() string {
	shown, more := cut(g.items, g.more)
	texts := make([]string, len(shown))
	for i, it := range shown {
		texts[i] = it.Words.Text() + " (" + it.Trust.Text() + ")"
	}
	return listText(texts, "; ", more, countTail(g.learned, "learned from earlier sessions"))
}

// bind refuses a trust outside the trust table and leaves out and counts a
// gate Gate refuses: its words come from a spec or the index.
func (g GateRefs) bind() (Value, error) {
	if err := negative("gate refs", g.more, g.learned+1); err != nil {
		return nil, err
	}
	kept, more := make([]GateRef, 0, len(g.items)), g.more
	for _, it := range g.items {
		if err := trustOf("gate", it.Trust); err != nil {
			return nil, err
		}
		if it.Words.Validate() != nil {
			more++
			continue
		}
		kept = append(kept, it)
	}
	kept, more = cut(kept, more)
	return GateRefs{items: kept, more: more, learned: g.learned}, nil
}

// EnumList is values of one closed table, each at most once, joined by "; "
// because a value may hold a comma; "none" when empty.
type EnumList struct {
	table  EnumTable
	values []string
}

// NewEnumList lists values of table; Validate checks them.
func NewEnumList(table EnumTable, values []string) EnumList {
	return EnumList{table: table, values: slices.Clone(values)}
}

func (l EnumList) Values() []string { return slices.Clone(l.values) }

func (l EnumList) Validate() error {
	for i, v := range l.values {
		if err := NewEnum(l.table, v).Validate(); err != nil {
			return err
		}
		if slices.Contains(l.values[:i], v) {
			return errors.New("slot: enum list names a value twice")
		}
	}
	return nil
}

func (l EnumList) Text() string {
	if len(l.values) == 0 {
		return "none"
	}
	return strings.Join(l.values, "; ")
}

func (l EnumList) bind() (Value, error) { return checked(l, l.Validate()) }
