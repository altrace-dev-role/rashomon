package engine

import (
	"path"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The ladder tests (W-M1.12): one synthetic repository per rule the
// AC-M1.1 prompts exercised, each the shape of one prompt, so a rule's
// break is named by the prompt shape it would miss. The repositories are
// built by synth below, as the index writes one: a unit node and a
// containing edge per file, a declaring edge per symbol, calls between
// symbols.

const synLang = "py"

// synFile is one file of a synthetic repository: its unit, its path and
// its symbols.
type synFile struct {
	unit, path string
	syms       []synSym
}

// synSym is a symbol by kind (func, const), name and doc sentence.
type synSym struct{ kind, name, doc string }

// synth builds a store at C1 from files and the proven and possible calls
// between their symbols, each end as "path#name".
func synth(t *testing.T, files []synFile, calls, mayCalls [][2]string) *graph.Store {
	t.Helper()
	rev := graph.RevCommit(graphtest.Commit)
	prov := graph.Prov{Extractor: "synth", Version: "1"}
	s := graph.NewStore()
	ids := map[string]graph.ID{}
	units := map[string]bool{}
	assert := func(src graph.ID, pred graph.Pred, dst graph.ID) {
		s.AssertEdge(graph.Edge{Src: src, Pred: pred, Dst: dst, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	}
	for _, f := range files {
		unit := graph.UnitID(synLang, f.unit)
		if !units[f.unit] {
			units[f.unit] = true
			s.AssertNode(graph.Node{ID: unit, Kind: "unit", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
		}
		file := graph.FileID(f.path)
		s.AssertNode(graph.Node{ID: file, Kind: "file", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
		assert(unit, graph.Contains, file)
		base := strings.TrimSuffix(path.Base(f.path), path.Ext(f.path))
		for _, sym := range f.syms {
			id := graph.SymID(synLang, f.unit, sym.kind, base+"."+sym.name)
			ids[f.path+"#"+sym.name] = id
			s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov,
				Attrs: map[string]string{"symkind": sym.kind, "doc": sym.doc}})
			assert(file, graph.Declares, id)
		}
	}
	edges := func(pairs [][2]string, pred graph.Pred) {
		for _, p := range pairs {
			src, dst := ids[p[0]], ids[p[1]]
			if src == "" || dst == "" {
				t.Fatalf("synth: unknown symbol in %v", p)
			}
			assert(src, pred, dst)
		}
	}
	edges(calls, graph.Calls)
	edges(mayCalls, graph.MayCall)
	return s
}

func fn(name string) synSym { return synSym{kind: "func", name: name} }

// record asserts a knowledge node of kind applying to each target, a
// symbol as "path#name" or a file as its path.
func record(t *testing.T, s *graph.Store, id, kind string, to ...string) {
	t.Helper()
	rev := graph.RevCommit(graphtest.Commit)
	s.AssertNode(graph.Node{ID: graph.ID(id), Kind: kind, Tier: graph.Declared, Conf: 1, Rev: rev,
		Attrs: map[string]string{"clause": "the clause of " + id, "source": "docs/" + id + ".md"}})
	for _, target := range to {
		dst := graph.FileID(target)
		if p, name, ok := strings.Cut(target, "#"); ok {
			base := strings.TrimSuffix(path.Base(p), path.Ext(p))
			dst = graph.SymID(synLang, strings.Split(p, "/")[0], "func", base+"."+name)
		}
		if _, ok := s.Node(dst, rev); !ok {
			t.Fatalf("record %s: no node %s", id, dst)
		}
		s.AssertEdge(graph.Edge{Src: graph.ID(id), Pred: graph.AppliesTo, Dst: dst, Tier: graph.Declared, Conf: 1, Rev: rev})
	}
}

func roles(t *testing.T, g *graph.Store, prompt string) Roles {
	t.Helper()
	b := Build(g, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	return b.Roles
}

func files(rfs []RoleFile) []graph.ID {
	var out []graph.ID
	for _, rf := range rfs {
		out = append(out, rf.File)
	}
	return out
}

func sameIDs(got, want []graph.ID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// A constant whose name covers the whole phrase is a label for the thing,
// not the thing (the ST12 shape: a pattern constant named after the
// continuity briefing): it covers the phrase without completing a claim,
// so its unit's claim is full, never single, and loses the tie to the
// unit whose callables cover the same tokens.
func TestUnits_ALabelCoversButCompletesNoClaim(t *testing.T) {
	g := synth(t, []synFile{
		{"cmd", "cmd/main.py", []synSym{fn("continuity_state"), fn("resume_briefing")}},
		{"pattern", "pattern/pattern.py", []synSym{{kind: "const", name: "CONTINUITY_BRIEFING"}}},
	}, nil, nil)
	picks, _ := units(t, g, "the continuity briefing should name the open requirements")
	if len(picks) != 2 || picks[0].Path != "cmd" || picks[0].Class != ClassFull || !picks[0].Callable || picks[0].Refused != "" {
		t.Fatalf("picks %+v", picks)
	}
	if p := picks[1]; p.Path != "pattern" || p.Class != ClassFull || p.Callable || p.Precision != 0 || p.Covered != p.Of {
		t.Errorf("label claim %+v, want a full, non-callable claim over the whole phrase", p)
	}
}

// Two units whose callables cover the same token of the phrase (the PG12
// shape: "integrity" named in verify and in render) are settled, for a
// one-word phrase, by how much of the rest of the prompt the unit names,
// then by the precision of the covering name; what still ties is refused.
func TestUnits_ASameTokenTieGoesToPrecisionThenMass(t *testing.T) {
	g := synth(t, []synFile{
		{"verify", "verify/integrity.py", []synSym{fn("integrity"), fn("flags")}},
		{"check", "check/check.py", []synSym{fn("integrity")}},
		{"render", "render/render.py", []synSym{fn("render_integrity_marker_text")}},
	}, nil, nil)
	picks, _ := units(t, g, "show the integrity flags in the integrity report")
	if len(picks) != 3 || picks[0].Refused != "" {
		t.Fatalf("picks %+v", picks)
	}
	for i, want := range []string{"verify", "check", "render"} {
		if picks[i].Path != want || picks[i].Class != ClassSingle {
			t.Errorf("pick %d %+v, want %s single", i+1, picks[i], want)
		}
	}
	if picks[0].Mass <= picks[1].Mass || picks[1].Precision <= picks[2].Precision {
		t.Errorf("ladder: %+v", picks)
	}
	picks, _ = units(t, g, "show the markers in the integrity report")
	if len(picks) != 3 || picks[0].Refused != "" || picks[0].Path != "render" {
		t.Fatalf("mass word in the loose name: picks %+v, want render by the marker it names", picks)
	}
	picks, _ = units(t, g, "fix the integrity report")
	if len(picks) != 3 || picks[0].Refused != "ambiguous: 'integrit' names code in 2 units" || picks[2].Path != "render" {
		t.Fatalf("no mass word: picks %+v, want the two precise names refused and render last", picks)
	}
}

// A symbol outside the unit is a reuse claim when the unit already
// calls it, when its name holds a word of what the request says to use,
// or when its name claims the addition (the HD9 shape: the instruction
// loader in store claims "instruction files loaded"; a doc that mentions
// the words, or a name holding one lesser word of the phrase, is a
// neighbour and never a claim). The line goes to the claim with a basis
// alone (W-M1.22): the dependency the unit already calls, hit on one
// lesser word, ranks as a claim for Explain and is no line.
func TestReuse_OutsideTheUnitNeedsAUseAClaimOrADependency(t *testing.T) {
	g := synth(t, []synFile{
		{"brief", "brief/brief.py", []synSym{fn("brief")}},
		{"store", "store/record.py", []synSym{fn("load_instructions")}},
		{"store", "store/reader.py", []synSym{fn("read_files")}},
		{"reports", "reports/daily.py", []synSym{{kind: "func", name: "daily", doc: "counts the instruction files loaded today"}}},
		{"tools", "tools/loader.py", []synSym{{kind: "func", name: "load_rows", doc: "reads rows from a file"}}},
	}, [][2]string{{"brief/brief.py#brief", "store/reader.py#read_files"}}, nil)
	r := roles(t, g, "Report which instruction files were actually loaded, in the brief.")
	if r.Unit != graph.UnitID(synLang, "brief") || r.File != graph.FileID("brief/brief.py") {
		t.Fatalf("roles %+v", r)
	}
	want := []graph.ID{graph.FileID("store/record.py"), graph.FileID("store/reader.py")}
	if !sameIDs(files(r.Reuse), want[:1]) {
		t.Errorf("reuse %v, want %v (the claim alone; the dependency has no basis)", files(r.Reuse), want[:1])
	}
	if len(r.Reuse) == 0 || r.Reuse[0].Basis != ReuseByAddition {
		t.Errorf("reuse %+v, want the addition claim as its basis", r.Reuse)
	}
	if len(r.ReuseClaims) != 2 || r.ReuseClaims[0].File != want[0] || r.ReuseClaims[1].File != want[1] {
		t.Errorf("claims %+v, want the claim, then the dependency", r.ReuseClaims)
	}
	for _, cl := range r.ReuseClaims {
		switch cl.File {
		case want[0]:
			if cl.Add <= 0 {
				t.Errorf("the claim carries no addition mass: %+v", cl)
			}
		case want[1]:
			if cl.Add != 0 {
				t.Errorf("one lesser word is no claim: %+v", cl)
			}
		default:
			t.Errorf("%s admitted on a doc mention or a lesser word: %+v", cl.File, cl)
		}
	}
}

// The symbol the addition's existing code calls ranks first (the T1
// shape: the hook's blast radius calls the platform's callers, so
// callers.py is the thing to reuse over the two files that name blast
// radius themselves), and among those the more precise name ranks
// higher; the file consuming the target is the consumer, never a reuse.
func TestReuse_TheAdditionsCodeRanksWhatItCallsFirst(t *testing.T) {
	g := synth(t, []synFile{
		{"platform", "platform/read.py", []synSym{fn("platform_read")}},
		{"platform", "platform/callers.py", []synSym{fn("callers")}},
		{"hook", "hook/blast.py", []synSym{fn("blast_radius")}},
		{"render", "render/render.py", []synSym{fn("blast_radius_binding")}},
		{"cmd", "cmd/report.py", []synSym{fn("report_platform")}},
	}, [][2]string{
		{"hook/blast.py#blast_radius", "platform/callers.py#callers"},
		{"cmd/report.py#report_platform", "platform/read.py#platform_read"},
	}, nil)
	r := roles(t, g, "Add blast radius to the platform read, counting the proven callers.")
	if r.Unit != graph.UnitID(synLang, "platform") || r.Why != ClassQualified || r.File != graph.FileID("platform/read.py") {
		t.Fatalf("roles %+v", r)
	}
	want := []graph.ID{graph.FileID("platform/callers.py"), graph.FileID("hook/blast.py"), graph.FileID("render/render.py")}
	if !sameIDs(files(r.Reuse), want) {
		t.Fatalf("reuse %v, want %v", files(r.Reuse), want)
	}
	if r.Reuse[0].Basis != ReuseByCall || r.Reuse[1].Basis != ReuseByAddition || r.Reuse[2].Basis != ReuseByAddition {
		t.Errorf("bases %q %q %q, want call, addition, addition", r.Reuse[0].Basis, r.Reuse[1].Basis, r.Reuse[2].Basis)
	}
	cl := map[graph.ID]ReuseClaim{}
	for _, c := range r.ReuseClaims {
		cl[c.File] = c
	}
	if c := cl[want[0]]; !c.ViaAdd || c.Add != 0 || !c.In {
		t.Errorf("callers %+v, want via the addition's code, no addition mass, inside", c)
	}
	if a, b := cl[want[1]], cl[want[2]]; a.ViaAdd || b.ViaAdd || a.Add <= 0 || a.Add != b.Add || a.Mass <= b.Mass {
		t.Errorf("claimants %+v %+v, want equal addition mass and the precise name heavier", a, b)
	}
	if !sameIDs(files(r.Consumers), []graph.ID{graph.FileID("cmd/report.py")}) {
		t.Errorf("consumers %v", files(r.Consumers))
	}
}

// When nothing outside the unit reaches a target symbol, the consumer is
// the unit's proven caller (the ST12 shape: a main package nothing calls
// has no consumer line; a file that calls another symbol of the unit
// takes the role); a possible call through an interface is no "consumed
// by". A possible call to the target itself is a consumer: the target is
// what the change touches.
func TestConsumers_FallBackToAProvenCallerOfTheUnit(t *testing.T) {
	repo := []synFile{
		{"cmd", "cmd/continuity.py", []synSym{fn("continuity_briefing")}},
		{"cmd", "cmd/main.py", []synSym{fn("main")}},
		{"cmd", "cmd/detach.py", []synSym{fn("detach")}},
		{"tools", "tools/run.py", []synSym{fn("run")}},
		{"plugins", "plugins/iface.py", []synSym{fn("dispatch")}},
	}
	calls := [][2]string{{"tools/run.py#run", "cmd/main.py#main"}}
	may := [][2]string{{"plugins/iface.py#dispatch", "cmd/detach.py#detach"}}
	prompt := "the continuity briefing should name the open requirements"
	r := roles(t, synth(t, repo, calls, may), prompt)
	run, iface := graph.FileID("tools/run.py"), graph.FileID("plugins/iface.py")
	if r.Unit != graph.UnitID(synLang, "cmd") || !sameIDs(files(r.Consumers), []graph.ID{run}) || !sameIDs(r.UnitConsumers, []graph.ID{run}) {
		t.Fatalf("roles %+v", r)
	}
	may = append(may, [2]string{"plugins/iface.py#dispatch", "cmd/continuity.py#continuity_briefing"})
	r = roles(t, synth(t, repo, calls, may), prompt)
	if !sameIDs(files(r.Consumers), []graph.ID{iface}) || !sameIDs(r.UnitConsumers, []graph.ID{iface, run}) {
		t.Errorf("a possible call to the target: consumers %v, unit %v", files(r.Consumers), r.UnitConsumers)
	}
}

// A phrase that would open with a condition word (when, if, until, after)
// is no phrase (the PL12 shape: "warn when an edit changes a file" adds
// no edit, it says when to warn), after a verb and after a modal alike.
func TestFrame_AConditionOpensNoPhrase(t *testing.T) {
	cases := []struct{ prompt, absent, target, constraint string }{
		{"Warn when an edit changes a file that no tracked build configuration covers.", "edit", "", ""},
		{"Keep the cache warm until the session ends.", "session", "", "cache warm"},
		{"Delete the sessions after the ttl expires.", "ttl", "sessions", ""},
		{"The warning should report if the file is untracked.", "file", "warning", ""},
	}
	for _, c := range cases {
		f := Parse(c.prompt)
		all := append(append(append(append([]Phrase{}, f.Targets...), f.Additions...), f.Constraints...), f.Uses...)
		for _, p := range all {
			if strings.Contains(" "+words(p)+" ", " "+tokensOf(c.absent)+" ") {
				t.Errorf("%q: phrase %s holds %q; frame %+v", c.prompt, p, c.absent, f)
			}
		}
		if c.target != "" && (len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf(c.target)) {
			t.Errorf("%q: targets %v, want [%s]", c.prompt, f.Targets, tokensOf(c.target))
		}
		if c.constraint != "" && !hasPhrase(f.Constraints, tokensOf(c.constraint)) {
			t.Errorf("%q: constraints %v, want [%s]", c.prompt, f.Constraints, tokensOf(c.constraint))
		}
	}
}

// A knowledge line names the file it was found through: the decision
// found through the unit's other file is reported on that file. A record
// on a reuse outside the unit is the reuse line's to show ("PF-HD1
// applies" on ledger.py) and no knowledge line (W-M1.23), so it is never
// reported on the alphabetically first file it also applies to (ST12's
// shape: PF-HD1 on ledger.go was reported on steerstep.go).
func TestKnowledge_ALineNamesTheFileItWasFoundThrough(t *testing.T) {
	build := func(withDecision bool) *graph.Store {
		s := synth(t, []synFile{
			{"cmd", "cmd/continuity.py", []synSym{fn("continuity_briefing")}},
			{"cmd", "cmd/storeopen.py", []synSym{fn("open_store")}},
			{"ledger", "ledger/ledger.py", []synSym{fn("open_ledger")}},
			{"hook", "hook/steerstep.py", []synSym{fn("step")}},
		}, [][2]string{
			{"cmd/continuity.py#continuity_briefing", "cmd/storeopen.py#open_store"},
			{"cmd/continuity.py#continuity_briefing", "ledger/ledger.py#open_ledger"},
		}, nil)
		record(t, s, "req:PF-HD1", "requirement", "hook/steerstep.py", "ledger/ledger.py#open_ledger")
		if withDecision {
			record(t, s, "dec:D155", "decision", "cmd/storeopen.py")
		}
		return s
	}
	prompt := "the continuity briefing should name the requirements still open"
	r := roles(t, build(true), prompt)
	want := []graph.ID{graph.FileID("cmd/storeopen.py"), graph.FileID("ledger/ledger.py")}
	if r.Path != "cmd" || !sameIDs(files(r.Reuse), want) {
		t.Fatalf("roles %+v", r)
	}
	if len(r.Knowledge) != 1 || r.Knowledge[0] != "dec:D155" {
		t.Fatalf("knowledge %v, want the unit's own decision alone", r.Knowledge)
	}
	if r.KnowledgeVia["dec:D155"] != graph.FileID("cmd/storeopen.py") {
		t.Errorf("via %v", r.KnowledgeVia)
	}
	b := Build(build(false), graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true})
	line := "reuse 'ledger/ledger.py' (holds the addition's code) declares 'open_ledger'; PF-HD1 applies"
	if !strings.Contains(b.Text(), line) {
		t.Errorf("brief lacks %q:\n%s", line, b.Text())
	}
	if strings.Contains(b.Text(), "PF-HD1 applies to 'hook/steerstep.py'") {
		t.Errorf("the reuse's record is reported on a file it was not found through:\n%s", b.Text())
	}
}
