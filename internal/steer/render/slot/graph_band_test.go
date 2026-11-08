package slot

import (
	"slices"
	"strings"
	"testing"
)

// TestGU5_ACandidateSaysSoAndTheUnitLineNamesWhatIsNotKnown (W-M1.38,
// abstention by evidence): a unit line in the candidate band says so
// before its basis and names its runner-up, the pair the brief stands on;
// a stated line renders as a line with no band; a role line in the
// candidate band says so after its path or id, before a reuse basis; the
// unit line closes with the kinds the brief withheld, after a semicolon
// and in a fixed order; a band outside the two, or an unknown kind outside
// the four or named twice, refuses the line; the record form keeps the
// band and the kinds and never the runner-up's path.
func TestGU5_ACandidateSaysSoAndTheUnitLineNamesWhatIsNotKnown(t *testing.T) {
	for _, c := range []struct {
		name string
		line GraphLine
		want string
	}{
		{"named, stated", NewUnitLine(UnitFact{Path: "internal/safe", Basis: BasisNamed, Band: BandStated, Names: []Ident{"Guard"}}),
			"unit 'internal/safe' (named in the request) declares 'Guard'"},
		{"coverage, stated", NewUnitLine(UnitFact{Path: "internal/store", Basis: BasisCoverage, Band: BandStated, RunnerUp: "internal/safe"}),
			"unit 'internal/store' (by word coverage; runner-up 'internal/safe')"},
		{"unit-name, candidate", NewUnitLine(UnitFact{Path: "internal/hook", Basis: BasisUnitName, Band: BandCandidate, RunnerUp: "internal/steer"}),
			"unit 'internal/hook' (a candidate by its name; runner-up 'internal/steer')"},
		{"coverage, candidate, no runner-up", NewUnitLine(UnitFact{Path: "internal/store", Basis: BasisCoverage, Band: BandCandidate}),
			"unit 'internal/store' (a candidate by word coverage)"},
		{"named, candidate", NewUnitLine(UnitFact{Path: "internal/safe", Basis: BasisNamed, Band: BandCandidate, RunnerUp: "internal/store"}),
			"unit 'internal/safe' (a candidate named in the request; runner-up 'internal/store')"},
		{"not known, in order", NewUnitLine(UnitFact{Path: "internal/store", Basis: BasisCoverage, Band: BandCandidate, RunnerUp: "internal/safe",
			Names: []Ident{"barrierSync"}, Lessons: []KnowledgeID{"dec:D145"}, Unknown: []string{UnknownKnowledge, UnknownTarget, UnknownReuse}}),
			"unit 'internal/store' (a candidate by word coverage; runner-up 'internal/safe') declares 'barrierSync'; D145 applies; not known: the file to change, the code to reuse, the records that apply to the change"},
		{"not known alone", NewUnitLine(UnitFact{Path: "internal/safe", Basis: BasisNamed, Band: BandStated, Unknown: []string{UnknownReuse}}),
			"unit 'internal/safe' (named in the request); not known: the code to reuse"},
		{"not known, all four", NewUnitLine(UnitFact{Path: "internal/safe", Basis: BasisUnitName, Band: BandCandidate, Consumers: []RelPath{"cmd/run.go"},
			Unknown: []string{UnknownConsumer, UnknownKnowledge, UnknownReuse, UnknownTarget}}),
			"unit 'internal/safe' (a candidate by its name) is consumed by 'cmd/run.go'; not known: the file to change, the code to reuse, the consumers of the change, the records that apply to the change"},
		{"consumer, candidate", NewCodeLine(CodeFact{Path: "jobs/purge_sessions.py", Role: RoleConsumer, Band: BandCandidate, Calls: []Ident{"purge"}}),
			"consumer 'jobs/purge_sessions.py' (a candidate) calls 'purge'"},
		{"reuse, candidate", NewCodeLine(CodeFact{Path: "store/record.py", Role: RoleReuse, Basis: ReuseByUse, Band: BandCandidate}),
			"reuse 'store/record.py' (a candidate; named to use)"},
		{"target, stated", NewCodeLine(CodeFact{Path: "store/record.py", Band: BandStated, Names: []Ident{"load"}}),
			"'store/record.py' declares 'load'"},
		{"target, candidate", NewCodeLine(CodeFact{Path: "store/record.py", Band: BandCandidate, Names: []Ident{"load"}}),
			"'store/record.py' (a candidate) declares 'load'"},
		{"decision, candidate", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeDecision, ID: "dec:ADR-0007", Band: BandCandidate, Path: "services/orgs",
			Clause: "org settings are read through one accessor"}),
			"decision ADR-0007 (a candidate) applies to 'services/orgs': decided 'org settings are read through one accessor'"},
		{"lesson, stated", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-212", Band: BandStated, Clause: "the purge predicate moves with the lifetime"}),
			"lesson INC-212: rule 'the purge predicate moves with the lifetime'"},
	} {
		v, err := Bind(c.line)
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		if got := v.Text(); got != c.want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, c.want)
		}
	}
	// Each refused line beside its twin that binds, so a refusal is for
	// the defect named and nothing else on the line.
	for _, c := range []struct {
		name      string
		bad, good GraphLine
	}{
		{"unit band outside the two", NewUnitLine(UnitFact{Path: "internal/store", Band: "likely"}), NewUnitLine(UnitFact{Path: "internal/store", Band: BandCandidate})},
		{"unit band withheld", NewUnitLine(UnitFact{Path: "internal/store", Band: "withheld"}), NewUnitLine(UnitFact{Path: "internal/store", Band: BandStated})},
		{"unknown kind outside the four", NewUnitLine(UnitFact{Path: "internal/store", Unknown: []string{"unit"}}),
			NewUnitLine(UnitFact{Path: "internal/store", Unknown: []string{UnknownConsumer}})},
		{"unknown kind twice", NewUnitLine(UnitFact{Path: "internal/store", Unknown: []string{UnknownReuse, UnknownReuse}}),
			NewUnitLine(UnitFact{Path: "internal/store", Unknown: []string{UnknownReuse, UnknownKnowledge}})},
		{"code band outside the two", NewCodeLine(CodeFact{Path: "a.py", Role: RoleConsumer, Band: "withheld"}),
			NewCodeLine(CodeFact{Path: "a.py", Role: RoleConsumer, Band: BandCandidate})},
		{"knowledge band outside the two", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-1", Band: "withheld"}),
			NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-1", Band: BandCandidate})},
		{"keyed unit with a runner-up path", GraphLine{keyed: true, unit: &UnitFact{RunnerUp: "internal/safe"}},
			GraphLine{keyed: true, unit: &UnitFact{Basis: BasisCoverage, Band: BandCandidate}}},
		{"keyed unit with a band outside the two", GraphLine{keyed: true, unit: &UnitFact{Band: "likely"}},
			GraphLine{keyed: true, unit: &UnitFact{Band: BandStated}}},
		{"keyed unit with an unknown kind outside the four", GraphLine{keyed: true, unit: &UnitFact{Unknown: []string{"unit"}}},
			GraphLine{keyed: true, unit: &UnitFact{Unknown: []string{UnknownTarget}}}},
		{"keyed code with a band outside the two", GraphLine{keyed: true, code: &CodeFact{Role: RoleConsumer, Band: "likely"}},
			GraphLine{keyed: true, code: &CodeFact{Role: RoleConsumer, Band: BandCandidate}}},
		{"keyed knowledge with a band outside the two", GraphLine{keyed: true, know: &KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-1", Band: "likely"}},
			GraphLine{keyed: true, know: &KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-1", Band: BandCandidate}}},
	} {
		if _, err := Bind(c.bad); err == nil {
			t.Errorf("%s: bound", c.name)
		}
		if c.bad.Validate() == nil {
			t.Errorf("%s: valid", c.name)
		}
		if _, err := Bind(c.good); err != nil {
			t.Errorf("%s: the twin is refused: %v", c.name, err)
		}
	}
	digest := func(string) string { return strings.Repeat("ab", 32) }
	v, err := Bind(NewUnitLine(UnitFact{Path: "internal/store", Basis: BasisCoverage, Band: BandCandidate, RunnerUp: "internal/safe",
		Unknown: []string{UnknownTarget}}))
	if err != nil {
		t.Fatal(err)
	}
	rec := v.(GraphLine).Record(digest)
	if u, ok := rec.Unit(); !ok || u.Band != BandCandidate || u.RunnerUp != "" || !slices.Equal(u.Unknown, []string{UnknownTarget}) {
		t.Errorf("record form unit %+v (want the band and the kinds kept, no runner-up path)", u)
	}
	if err := rec.Validate(); err != nil {
		t.Errorf("record form unit: %v", err)
	}
	if got := rec.Text(); got != "a unit on record; not known: the file to change" {
		t.Errorf("record text %q", got)
	}
	cv, err := Bind(NewCodeLine(CodeFact{Path: "jobs/purge_sessions.py", Role: RoleConsumer, Band: BandCandidate}))
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := cv.(GraphLine).Record(digest).Code(); !ok || f.Band != BandCandidate {
		t.Errorf("record form code %+v (want the band kept)", f)
	}
	kv, err := Bind(NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeDecision, ID: "dec:ADR-0007", Band: BandCandidate}))
	if err != nil {
		t.Fatal(err)
	}
	if k, ok := kv.(GraphLine).Record(digest).Knowledge(); !ok || k.Band != BandCandidate {
		t.Errorf("record form knowledge %+v (want the band kept)", k)
	}
}
