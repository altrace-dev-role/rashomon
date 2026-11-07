package slot

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The graph brief's slot types (card GRAPH-SLOT1): one line of the brief
// the repository graph makes for a request is a set of structured fields,
// and a template renders it as one sentence. What a repository names (a
// path, a symbol, a table, a column, a module) renders inside a quote; an
// item a check refuses is left out and counted; a knowledge record's clause
// is Approved's and its id a KnowledgeID. The record form (Record) holds a
// path as its keyed digest and each list as its count, so at record scope
// v4 the steer line carries no path and no name.

// RelPath is a path relative to the hook's working directory: at most
// maxParentHops leading "../" segments, then every Path rule. The brief
// spans the repositories of one workspace, and a path the agent can open
// from where it stands is the one it acts on (evidence 11-04: agents read a
// path of another repository as outside their sandbox).
type RelPath string

const maxParentHops = 4

func (p RelPath) Validate() error {
	s := string(p)
	hops := 0
	for strings.HasPrefix(s, "../") {
		s, hops = s[3:], hops+1
	}
	if hops > maxParentHops {
		return fmt.Errorf("slot: relative path climbs more than %d directories", maxParentHops)
	}
	return Path(s).Validate()
}

func (p RelPath) Text() string         { return quoted(string(p)) }
func (p RelPath) bind() (Value, error) { return checked(p, p.Validate()) }

// Ident is a symbol, module, table or column name as the graph holds it:
// one to eight identifier segments joined by dots, screened as a symbol is
// (SymbolFact). A repository names it, so it renders inside a quote.
type Ident string

var identRE = &lazyRegexp{pattern: `^[A-Za-z_][A-Za-z0-9_]{0,63}(\.[A-Za-z_][A-Za-z0-9_]{0,63}){0,7}$`}

func (i Ident) Validate() error {
	if err := charset("identifier", string(i), identRE); err != nil {
		return err
	}
	return screened("identifier", string(i))
}

func (i Ident) Text() string         { return quoted(string(i)) }
func (i Ident) bind() (Value, error) { return checked(i, i.Validate()) }

// KnowledgeID is a knowledge record's id as the repository graph holds it
// (les:INC-212, dec:ADR-0007): LessonID's shape with upper case admitted,
// and LessonID's screens, since it is model-visible and unquoted. It
// renders without its kind prefix, after the kind's own word.
type KnowledgeID string

var knowledgeIDRE = &lazyRegexp{pattern: `^[A-Za-z0-9][A-Za-z0-9._:@-]{0,63}$`}

func (k KnowledgeID) Validate() error {
	s := string(k)
	if err := charset("knowledge id", s, knowledgeIDRE); err != nil {
		return err
	}
	if err := screened("knowledge id", s); err != nil {
		return err
	}
	if err := RefuseHosts(s); err != nil {
		return err
	}
	if urlSchemeRE.get().MatchString(s) {
		return errors.New("slot: knowledge id holds a URL scheme")
	}
	if w, ok := firstWordOf(s, approvedBanned); ok {
		return fmt.Errorf("slot: knowledge id holds the banned word %q", w)
	}
	return nil
}

// Text is the id after its kind prefix: INC-212 for les:INC-212.
func (k KnowledgeID) Text() string {
	s := string(k)
	if i := strings.IndexByte(s, ':'); i > 0 {
		return s[i+1:]
	}
	return s
}

func (k KnowledgeID) bind() (Value, error) { return checked(k, k.Validate()) }

// Revision is the graph revision a brief was read at: a commit's short
// hash, or an overlay revision such as C1+ov16.
type Revision string

var revisionRE = &lazyRegexp{pattern: `^[A-Za-z0-9][A-Za-z0-9+._-]{0,40}$`}

func (r Revision) Validate() error {
	if err := charset("revision", string(r), revisionRE); err != nil {
		return err
	}
	return screened("revision", string(r))
}

func (r Revision) Text() string         { return string(r) }
func (r Revision) bind() (Value, error) { return checked(r, r.Validate()) }

// TableRef is a table a line's code writes or reads, with the columns the
// statement names.
type TableRef struct {
	Table Ident
	Cols  []Ident
}

// ConstFact is a constant a line's file declares, with its integer value.
type ConstFact struct {
	Name  Ident
	Value int
}

// CodeFact is a file of the brief: what it declares, what that code calls,
// writes, reads and imports, and the knowledge that applies to it. Role is
// the part it plays in the request (RoleReuse, RoleConsumer) or "" for the
// target's own file and a related file. Basis is why a reuse file is the
// thing to reuse (one of the three ReuseBy words; empty renders as
// before) and is refused on any other role.
type CodeFact struct {
	Path    RelPath
	Role    string
	Basis   string
	Names   []Ident
	Consts  []ConstFact
	Calls   []Ident
	Writes  []TableRef
	Reads   []TableRef
	Imports []Ident
	Lessons []KnowledgeID
}

// TableFact is a table of the brief with the code that writes and reads it.
type TableFact struct {
	Table   Ident
	Writers []Ident
	Readers []Ident
	Lessons []KnowledgeID
}

// KnowledgeFact is a lesson, decision or requirement of the brief: its id,
// the file it points the agent at, its title (for a lesson, the failure it
// was drawn from), its one clause (the rule), and the document it came
// from. Path, Title, Clause and Source may each be empty.
type KnowledgeFact struct {
	Kind   string
	ID     KnowledgeID
	Path   RelPath
	Title  Approved
	Clause Approved
	Source Path
}

// The knowledge kinds a line names.
const (
	KnowledgeLesson   = "lesson"
	KnowledgeDecision = "decision"
)

// Counts is how many items each list of a line holds beyond those listed,
// left out by a check or cut at the list's cap; in the record form, every
// item, since the record lists none.
type Counts struct {
	Names, Consts, Calls, Writes, Reads, Imports, Lessons, Writers, Readers, Consumers int
}

// The list caps of one line: three names, callers or applying records, two
// tables each way, two imports and two constants, three columns per table.
const (
	maxLineIdents  = 3
	maxLineTables  = 2
	maxLineImports = 2
	maxLineConsts  = 2
	maxLineCols    = 3
)

// GraphLine is one line of the brief: a code, table, knowledge or unit
// fact, or nothing, which a sparse template leaves out. Build one with
// NewCodeLine, NewTableLine, NewKnowledgeLine or NewUnitLine; Record is its
// record form.
type GraphLine struct {
	code  *CodeFact
	table *TableFact
	know  *KnowledgeFact
	unit  *UnitFact
	more  Counts
	// The record form (record scope v4): the path's and the source's keyed
	// digests and every list as its count.
	keyed        bool
	digest       string
	sourceDigest string
}

// NewCodeLine is a line for f.
func NewCodeLine(f CodeFact) GraphLine { return GraphLine{code: &f} }

// NewTableLine is a line for f.
func NewTableLine(f TableFact) GraphLine { return GraphLine{table: &f} }

// NewKnowledgeLine is a line for f.
func NewKnowledgeLine(f KnowledgeFact) GraphLine { return GraphLine{know: &f} }

// Code, Table and Knowledge are the line's fact and whether it is of that
// kind; in the record form (Record) the fact's lists are empty and its
// paths "".
func (l GraphLine) Code() (CodeFact, bool) {
	if l.code == nil {
		return CodeFact{}, false
	}
	return *l.code, true
}

func (l GraphLine) Table() (TableFact, bool) {
	if l.table == nil {
		return TableFact{}, false
	}
	return *l.table, true
}

func (l GraphLine) Knowledge() (KnowledgeFact, bool) {
	if l.know == nil {
		return KnowledgeFact{}, false
	}
	return *l.know, true
}

// Kind is the line's kind: code, table, unit, the knowledge kind, or ""
// for an empty line.
func (l GraphLine) Kind() string {
	switch {
	case l.code != nil:
		return "code"
	case l.table != nil:
		return "table"
	case l.know != nil:
		return l.know.Kind
	case l.unit != nil:
		return "unit"
	}
	return ""
}

// More is the counts beyond the listed items.
func (l GraphLine) More() Counts { return l.more }

// Keyed reports the record form; Digest and SourceDigest are its keyed
// digests of the path and the source, "" where there was none.
func (l GraphLine) Keyed() bool          { return l.keyed }
func (l GraphLine) Digest() string       { return l.digest }
func (l GraphLine) SourceDigest() string { return l.sourceDigest }

// Lists reports a line that stands for anything.
func (l GraphLine) Lists() bool {
	return l.code != nil || l.table != nil || l.know != nil || l.unit != nil
}

// Validate checks the line as it is: at most one fact, every item passing
// its check, counts at or above zero, and a record form that lists nothing.
func (l GraphLine) Validate() error {
	if err := l.oneFact(); err != nil {
		return err
	}
	if err := l.more.validate(); err != nil {
		return err
	}
	if l.keyed {
		return l.validateKeyed()
	}
	switch {
	case l.code != nil:
		return l.code.validate()
	case l.table != nil:
		return l.table.validate()
	case l.know != nil:
		return l.know.validate()
	case l.unit != nil:
		return l.unit.validate()
	}
	return nil
}

func (m Counts) validate() error {
	return negative("graph line", m.Names, m.Consts, m.Calls, m.Writes, m.Reads, m.Imports, m.Lessons, m.Writers, m.Readers, m.Consumers)
}

// oneFact refuses a line holding two facts.
func (l GraphLine) oneFact() error {
	n := 0
	for _, set := range []bool{l.code != nil, l.table != nil, l.know != nil, l.unit != nil} {
		if set {
			n++
		}
	}
	if n > 1 {
		return errors.New("slot: graph line holds more than one fact")
	}
	return nil
}

func (l GraphLine) validateKeyed() error {
	for _, d := range []string{l.digest, l.sourceDigest} {
		if d != "" && !digestShaped(d) {
			return errors.New("slot: graph line digest is not 64 lowercase hex")
		}
	}
	switch {
	case l.code != nil:
		c := *l.code
		if c.Path != "" || len(c.Names)+len(c.Consts)+len(c.Calls)+len(c.Writes)+len(c.Reads)+len(c.Imports) > 0 {
			return errors.New("slot: keyed graph line holds a path or a name")
		}
		if err := validRole(c.Role); err != nil {
			return err
		}
		if err := validBasis(c.Role, c.Basis); err != nil {
			return err
		}
		return validIDs(c.Lessons)
	case l.table != nil:
		t := *l.table
		if t.Table != "" || len(t.Writers)+len(t.Readers) > 0 {
			return errors.New("slot: keyed graph line holds a table or a name")
		}
		return validIDs(t.Lessons)
	case l.know != nil:
		k := *l.know
		if k.Path != "" || k.Source != "" {
			return errors.New("slot: keyed graph line holds a path")
		}
		return k.validate()
	case l.unit != nil:
		u := *l.unit
		if u.Path != "" || len(u.Names)+len(u.Consumers) > 0 {
			return errors.New("slot: keyed graph line holds a path or a name")
		}
		return validIDs(u.Lessons)
	}
	return nil
}

func validIDs(ids []KnowledgeID) error {
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validIdents(items []Ident) error {
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f CodeFact) validate() error {
	if err := f.Path.Validate(); err != nil {
		return err
	}
	if err := validRole(f.Role); err != nil {
		return err
	}
	if err := validBasis(f.Role, f.Basis); err != nil {
		return err
	}
	for _, c := range f.Consts {
		if err := c.Name.Validate(); err != nil {
			return err
		}
		if err := inRange("constant", c.Value, -maxCount, maxCount); err != nil {
			return err
		}
	}
	for _, t := range slices.Concat(f.Writes, f.Reads) {
		if err := t.Table.Validate(); err != nil {
			return err
		}
		if err := validIdents(t.Cols); err != nil {
			return err
		}
	}
	if err := validIdents(slices.Concat(f.Names, f.Calls, f.Imports)); err != nil {
		return err
	}
	return validIDs(f.Lessons)
}

func (f TableFact) validate() error {
	if err := f.Table.Validate(); err != nil {
		return err
	}
	if err := validIdents(slices.Concat(f.Writers, f.Readers)); err != nil {
		return err
	}
	return validIDs(f.Lessons)
}

func (f KnowledgeFact) validate() error {
	if !knowledgeKind(f.Kind) {
		return errors.New("slot: knowledge line has a kind outside lesson, decision and requirement")
	}
	if err := f.ID.Validate(); err != nil {
		return err
	}
	if f.Path != "" {
		if err := f.Path.Validate(); err != nil {
			return err
		}
	}
	for _, a := range []Approved{f.Title, f.Clause} {
		if a != "" {
			if err := a.Validate(); err != nil {
				return err
			}
		}
	}
	if f.Source != "" {
		return f.Source.Validate()
	}
	return nil
}

// bind leaves out and counts an item a check refuses and cuts each list at
// its cap; a path, a table or a knowledge id the line stands on refuses the
// line, which the brief then drops. A knowledge line keeps its id when its
// path, clause or source is refused: each is left out on its own.
func (l GraphLine) bind() (Value, error) {
	if l.keyed || !l.Lists() {
		return checked(l, l.Validate())
	}
	if err := l.oneFact(); err != nil {
		return nil, err
	}
	m := l.more
	if err := m.validate(); err != nil {
		return nil, err
	}
	switch {
	case l.unit != nil:
		return l.bindUnit()
	case l.code != nil:
		c := *l.code
		if err := c.Path.Validate(); err != nil {
			return nil, err
		}
		if err := validRole(c.Role); err != nil {
			return nil, err
		}
		if err := validBasis(c.Role, c.Basis); err != nil {
			return nil, err
		}
		c.Names, m.Names = keptIdents(c.Names, m.Names, maxLineIdents)
		c.Consts, m.Consts = keptConsts(c.Consts, m.Consts)
		c.Calls, m.Calls = keptIdents(c.Calls, m.Calls, maxLineIdents)
		c.Writes, m.Writes = keptTables(c.Writes, m.Writes)
		c.Reads, m.Reads = keptTables(c.Reads, m.Reads)
		c.Imports, m.Imports = keptIdents(c.Imports, m.Imports, maxLineImports)
		c.Lessons, m.Lessons = keptIDs(c.Lessons, m.Lessons)
		return GraphLine{code: &c, more: m}, nil
	case l.table != nil:
		t := *l.table
		if err := t.Table.Validate(); err != nil {
			return nil, err
		}
		t.Writers, m.Writers = keptIdents(t.Writers, m.Writers, maxLineIdents)
		t.Readers, m.Readers = keptIdents(t.Readers, m.Readers, maxLineIdents)
		t.Lessons, m.Lessons = keptIDs(t.Lessons, m.Lessons)
		return GraphLine{table: &t, more: m}, nil
	}
	k := *l.know
	if !knowledgeKind(k.Kind) {
		return nil, errors.New("slot: knowledge line has a kind outside lesson, decision and requirement")
	}
	if err := k.ID.Validate(); err != nil {
		return nil, err
	}
	if k.Path.Validate() != nil {
		k.Path = ""
	}
	if k.Title.Validate() != nil {
		k.Title = ""
	}
	if k.Clause.Validate() != nil {
		k.Clause = ""
	}
	if k.Source.Validate() != nil {
		k.Source = ""
	}
	return GraphLine{know: &k, more: m}, nil
}

func keptIdents(items []Ident, more, cap int) ([]Ident, int) {
	kept := make([]Ident, 0, len(items))
	for _, it := range items {
		if it.Validate() != nil {
			more++
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) > cap {
		return kept[:cap], more + len(kept) - cap
	}
	return kept, more
}

func keptIDs(ids []KnowledgeID, more int) ([]KnowledgeID, int) {
	kept := make([]KnowledgeID, 0, len(ids))
	for _, id := range ids {
		if id.Validate() != nil {
			more++
			continue
		}
		kept = append(kept, id)
	}
	if len(kept) > maxLineIdents {
		return kept[:maxLineIdents], more + len(kept) - maxLineIdents
	}
	return kept, more
}

func keptConsts(items []ConstFact, more int) ([]ConstFact, int) {
	kept := make([]ConstFact, 0, len(items))
	for _, c := range items {
		if c.Name.Validate() != nil || inRange("constant", c.Value, -maxCount, maxCount) != nil {
			more++
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) > maxLineConsts {
		return kept[:maxLineConsts], more + len(kept) - maxLineConsts
	}
	return kept, more
}

func keptTables(items []TableRef, more int) ([]TableRef, int) {
	kept := make([]TableRef, 0, len(items))
	for _, t := range items {
		if t.Table.Validate() != nil {
			more++
			continue
		}
		cols, _ := keptIdents(t.Cols, 0, maxLineCols)
		kept = append(kept, TableRef{Table: t.Table, Cols: cols})
	}
	if len(kept) > maxLineTables {
		return kept[:maxLineTables], more + len(kept) - maxLineTables
	}
	return kept, more
}

// Record is l's record form: every list as its count, the path and the
// source as their keyed digests (digest, intent's PathDigest), the knowledge
// ids, the title, the clause and a code line's role and basis kept, since
// those are the repository's declared records and the engine's reading,
// never the request's words. It is built from the bound line,
// so the counts are what the printed line listed, left out and cut.
func (l GraphLine) Record(digest func(string) string) GraphLine {
	out := GraphLine{keyed: true}
	m := l.more
	switch {
	case l.code != nil:
		c := *l.code
		m.Names += len(c.Names)
		m.Consts += len(c.Consts)
		m.Calls += len(c.Calls)
		m.Writes += len(c.Writes)
		m.Reads += len(c.Reads)
		m.Imports += len(c.Imports)
		out.digest = digest(string(c.Path))
		out.code = &CodeFact{Role: c.Role, Basis: c.Basis, Lessons: slices.Clone(c.Lessons)}
	case l.table != nil:
		t := *l.table
		m.Writers += len(t.Writers)
		m.Readers += len(t.Readers)
		out.table = &TableFact{Lessons: slices.Clone(t.Lessons)}
	case l.know != nil:
		k := *l.know
		if k.Path != "" {
			out.digest = digest(string(k.Path))
		}
		if k.Source != "" {
			out.sourceDigest = digest(string(k.Source))
		}
		out.know = &KnowledgeFact{Kind: k.Kind, ID: k.ID, Title: k.Title, Clause: k.Clause}
	case l.unit != nil:
		u := *l.unit
		m.Names += len(u.Names)
		m.Consumers += len(u.Consumers)
		out.digest = digest(string(u.Path))
		out.unit = &UnitFact{Basis: u.Basis, Lessons: slices.Clone(u.Lessons)}
	default:
		return GraphLine{}
	}
	out.more = m
	return out
}
