package pgraph

import (
	"fmt"
	"sort"
	"strings"
)

// Triple is the statement a fact makes. Subject and Object are symbol ids as
// symbolID spells them.
type Triple struct {
	Subject   string
	Predicate Predicate
	Object    string
}

func tripleLess(a, b Triple) bool {
	if a.Subject != b.Subject {
		return a.Subject < b.Subject
	}
	if a.Predicate != b.Predicate {
		return a.Predicate.String() < b.Predicate.String()
	}
	return a.Object < b.Object
}

func (t Triple) check() error {
	if t.Predicate < Declares || t.Predicate > ImplementsViaPointer {
		return fmt.Errorf("pgraph: invalid predicate %d", uint8(t.Predicate))
	}
	if err := checkField("subject", t.Subject); err != nil {
		return err
	}
	return checkField("object", t.Object)
}

// Fact is one statement with its provenance. Its fields are unexported and it
// has no setters: a Fact is read-only from the moment the log writes it.
type Fact struct {
	t                         Triple
	tier                      Tier
	config, commit, extractor string
}

func (f Fact) Triple() Triple    { return f.t }
func (f Fact) Tier() Tier        { return f.tier }
func (f Fact) Config() string    { return f.config }
func (f Fact) Commit() string    { return f.commit }
func (f Fact) Extractor() string { return f.extractor }

// factKey is a fact's identity: the statement, the tier and the build
// configuration. Commit and extractor are provenance, not identity.
type factKey struct {
	t      Triple
	tier   Tier
	config string
}

func (f Fact) key() factKey { return factKey{f.t, f.tier, f.config} }

func keyLess(a, b factKey) bool {
	if a.t != b.t {
		return tripleLess(a.t, b.t)
	}
	if a.tier != b.tier {
		return a.tier < b.tier
	}
	return a.config < b.config
}

func (f Fact) producerName() string {
	n, _, _ := strings.Cut(f.extractor, "@")
	return n
}

// Op is a record's kind. A retraction is a tombstone: it removes a fact from
// the live set and leaves the history as it was.
type Op uint8

const (
	Assert Op = iota + 1
	Retract
)

// Record is one entry of the log.
type Record struct {
	op   Op
	fact Fact
}

func (r Record) Op() Op     { return r.op }
func (r Record) Fact() Fact { return r.fact }

// Delta counts what a Reconcile appended.
type Delta struct{ Asserted, Retracted int }

// Log is an append-only sequence of records. Invariants at every step: a fact
// key is asserted only while not live and retracted only while live; live is
// exactly the keys asserted and not since retracted; records are never
// changed or removed. The zero value is an empty log.
type Log struct {
	recs []Record
	live map[factKey]Fact
}

func (l *Log) Len() int { return len(l.recs) }

// Records returns a copy of the history.
func (l *Log) Records() []Record { return append([]Record(nil), l.recs...) }

func (l *Log) apply(r Record) {
	if l.live == nil {
		l.live = map[factKey]Fact{}
	}
	l.recs = append(l.recs, r)
	if r.op == Assert {
		l.live[r.fact.key()] = r.fact
	} else {
		delete(l.live, r.fact.key())
	}
}

// Live returns the live facts in the view's order. O(n log n) in live facts.
func (l *Log) Live() []Fact {
	out := make([]Fact, 0, len(l.live))
	for _, f := range l.live {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return keyLess(out[i].key(), out[j].key()) })
	return out
}

// Reconcile makes the facts p holds under config equal to want: it asserts
// what is missing and retracts what p once held and no longer does. Facts of
// other producers, other tiers and other configs are untouched. Input is
// validated whole before anything is appended, so an error leaves the log as
// it was. Time is O(live + want) plus sorting the delta.
func (l *Log) Reconcile(p Producer, config, commit string, want []Triple) (Delta, error) {
	if p.tier < Proven || p.tier > Proposed {
		return Delta{}, fmt.Errorf("pgraph: producer has no tier")
	}
	if err := checkField("config", config); err != nil {
		return Delta{}, err
	}
	if err := checkField("commit", commit); err != nil {
		return Delta{}, err
	}
	wanted := make(map[factKey]struct{}, len(want))
	for _, t := range want {
		if err := t.check(); err != nil {
			return Delta{}, err
		}
		wanted[factKey{t, p.tier, config}] = struct{}{}
	}
	var add, drop []factKey
	for k := range wanted {
		if _, ok := l.live[k]; !ok {
			add = append(add, k)
		}
	}
	for k, f := range l.live {
		if _, ok := wanted[k]; !ok && f.tier == p.tier && f.config == config && f.producerName() == p.name {
			drop = append(drop, k)
		}
	}
	sort.Slice(add, func(i, j int) bool { return keyLess(add[i], add[j]) })
	sort.Slice(drop, func(i, j int) bool { return keyLess(drop[i], drop[j]) })
	for _, k := range drop {
		l.apply(Record{Retract, Fact{k.t, k.tier, k.config, commit, p.String()}})
	}
	for _, k := range add {
		l.apply(Record{Assert, Fact{k.t, k.tier, k.config, commit, p.String()}})
	}
	return Delta{Asserted: len(add), Retracted: len(drop)}, nil
}

// ViewHeader is the first line of every view.
const ViewHeader = "pgraph-view v0"

// View is the materialized view: the header, then one line per live fact,
// "subject predicate object tier config" tab-separated, in sorted order. It is
// a function of the live set alone, so it carries neither commit nor history
// and two logs that reach the same live set render identical bytes.
func (l *Log) View() []byte {
	var b strings.Builder
	b.WriteString(ViewHeader + "\n")
	for _, f := range l.Live() {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", f.t.Subject, f.t.Predicate, f.t.Object, f.tier, f.config)
	}
	return []byte(b.String())
}
