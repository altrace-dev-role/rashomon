package graphtest

import (
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Synthetic builds a store of about records records shaped like a
// repository: files declaring functions and constants, calls between
// functions, table reads and writes, lessons on tables, at commit C1.
// Names come from a twenty-word vocabulary (session, org, user, invoice,
// token, cache, expire, timeout, login, plan, seat, key, report, purge,
// settings, notify, email, queue, audit, limit) so a prompt's words hit a
// realistic fraction. It is the K7 fixture (engine's hook-path latency) and
// the index of the prompt's graph-read RSS case (test/perf), so the two
// measure one shape.
func Synthetic(records int) *graph.Store {
	vocab := []string{"session", "org", "user", "invoice", "token", "cache", "expire", "timeout", "login", "plan", "seat", "key", "report", "purge", "settings", "notify", "email", "queue", "audit", "limit"}
	rev := graph.RevCommit(Commit)
	s := graph.NewStore()
	prov := graph.Prov{Extractor: "synthetic", Version: "1"}
	perFile := 12
	files := records / (perFile*3 + 2)
	var syms []graph.ID
	for f := 0; f < files; f++ {
		unit := "services/" + vocab[f%len(vocab)]
		mod := vocab[f%len(vocab)] + "." + vocab[(f/len(vocab))%len(vocab)] + strconv.Itoa(f)
		path := unit + "/" + vocab[(f/len(vocab))%len(vocab)] + strconv.Itoa(f) + ".py"
		file := graph.FileID(path)
		s.AssertNode(graph.Node{ID: file, Kind: "file", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov, Attrs: map[string]string{"module": mod}})
		for i := 0; i < perFile; i++ {
			name := vocab[(f+i)%len(vocab)] + "_" + vocab[(f*7+i*3)%len(vocab)] + strconv.Itoa(i)
			id := graph.SymID("py", unit, "func", mod+"."+name)
			s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov,
				Attrs: map[string]string{"symkind": "func", "sig": "(" + vocab[(i*5+f)%len(vocab)] + "_id, " + vocab[(i*11+f)%len(vocab)] + ")", "line": strconv.Itoa(i * 9)}})
			s.AssertEdge(graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
			if len(syms) > 0 {
				s.AssertEdge(graph.Edge{Src: id, Pred: graph.Calls, Dst: syms[(f*perFile+i*31)%len(syms)], Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov})
			}
			if i%4 == 0 {
				tbl := graph.TableID("public." + vocab[(f+i)%len(vocab)] + "s")
				s.AssertNode(graph.Node{ID: tbl, Kind: "table", Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov})
				pred := graph.Reads
				if i%8 == 0 {
					pred = graph.Writes
				}
				s.AssertEdge(graph.Edge{Src: id, Pred: pred, Dst: tbl, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov, Attrs: map[string]string{"cols": "org_id,expires_at"}})
			}
			syms = append(syms, id)
		}
		cid := graph.SymID("py", unit, "const", mod+"."+vocab[f%len(vocab)]+"_TTL")
		s.AssertNode(graph.Node{ID: cid, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov, Attrs: map[string]string{"symkind": "const", "value": "3600"}})
		s.AssertEdge(graph.Edge{Src: file, Pred: graph.Declares, Dst: cid, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	}
	for i := 0; i < len(vocab); i++ {
		les := graph.ID("les:INC-" + strconv.Itoa(100+i))
		s.AssertNode(graph.Node{ID: les, Kind: "lesson", Tier: graph.Declared, Conf: 1, Rev: rev, Attrs: map[string]string{"title": "incident on " + vocab[i], "text": "INC: the " + vocab[i] + " table is purged nightly; change its predicate with any lifetime change."}})
		s.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: graph.TableID("public." + vocab[i] + "s"), Tier: graph.Declared, Conf: 1, Rev: rev})
	}
	return s
}
