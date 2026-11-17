package verify

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The ledger's turn rows (W-M3.1; PG13's first window, PG3, PG5, PG7). A
// Stop's ledger is its session rows, unchanged, then the rows its own turn
// owes over the window TurnWindow gives the Stop's prompt id: per required
// gate, in snap.Gates order,
//   - gate_after_write, where the session row is owed and the turn holds a
//     relevant named write the gate covers. It reads as the session row: the
//     window ends at the point, so the gate's last covered write is the
//     turn's, and a green after it reads the same over either.
//   - watched_red_then_green, where the family's last red at the point is a
//     call of the turn (its seq at or after From), read as the session row.
//   - gate_after_write once per unit the turn wrote under the gate, Unit
//     UnitOf's id, in unit id order: the gate's table with that unit's
//     writes alone counted (unitView).
//
// A gate's turn rows read unknown where its session rows do. Only a Stop owes
// turn rows, and an empty window owes none; with UnitOf nil no row is about a
// unit. Ids are TurnObligationID's, so two Stops in one turn share them and
// two turns never do. Past MaxTurnUnitRows unit rows the rest are cut, in
// that order, and counted.

// turnObligations is the turn rows of a Stop at snap's point in promptID's
// turn (TurnWindow), and how many unit rows the cap cut; none at any other
// point. c is the classification snap came from; root is the Stop's project
// (StopInput.Root), under which a write's path is placed before unitOf reads
// it (placed). Cost: one pass over the declarations and one over the runs,
// then per required gate one over the writes and a sort of its units:
// O(D + R + G x W log W).
func turnObligations(snap Snapshot, c *classified, run *store.Run, promptID, root string, unitOf UnitOf) ([]Obligation, int) {
	if snap.Point.Kind != PointStop {
		return nil, 0
	}
	w := TurnWindow(run, promptID, snap.Point.Seq)
	if w.Empty() {
		return nil, 0
	}
	l := ledger{scope: ScopeTurn, evidence: make([]int64, 0, 8)}
	reds := watches(c.runs, w.To)
	kept, cut := 0, 0
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if !g.Required {
			continue
		}
		source, known := obligationSource(g.Source)
		start := len(l.rows)
		wrote, units, byUnit := turnWrites(g, c.writes, w, root, unitOf)
		if wrote && (g.State != Absent || g.LastWrite != nil) {
			l.fromTable(g, source, GateAfterWrite, gateAfterWriteTable)
			l.turnRow(g, w, "")
		}
		if r, ok := reds[g.Family]; ok && r.red.Seq >= w.From {
			state, ev, n := watchedRow(r, c.writes, g.root)
			l.add(g, source, WatchedRedThenGreen, state, ev[:n]...)
			l.turnRow(g, w, "")
		}
		for _, u := range units {
			v, owed := unitView(g, byUnit[u], c.unnamed, w.To, snap.Point.Tree)
			switch {
			case !owed:
				continue
			case kept == MaxTurnUnitRows:
				cut++
				continue
			}
			kept++
			l.fromTable(&v, source, GateAfterWrite, gateAfterWriteTable)
			l.turnRow(g, w, u)
		}
		if snap.Recording != Complete || Rank(g.State) < 0 || !known {
			for j := start; j < len(l.rows); j++ {
				l.rows[j].State = ObligationUnknown
			}
		}
	}
	return l.rows, cut
}

// turnWrites reads the relevant named writes g covers at or before w.To:
// whether one is in w; with unitOf set, the units one in w was made in,
// ascending; and each such unit's writes at or before w.To, in seq order,
// those before w included, which unitView reads. unitOf reads each path
// placed under root (placed); a path placed nowhere, or in no unit (""),
// makes no unit row.
func turnWrites(g *GateState, writes []write, w Window, root string, unitOf UnitOf) (bool, []string, map[string][]write) {
	wrote := false
	var units []string
	var byUnit map[string][]write
	var turned map[string]bool
	for i := range writes {
		x := &writes[i]
		if x.Seq > w.To || x.Path == nil || !Relevant(x.Class) || !covers(g.root, x) {
			continue
		}
		in := x.Seq >= w.From
		wrote = wrote || in
		if unitOf == nil {
			continue
		}
		u := ""
		if rel, ok := placed(root, x); ok {
			u = unitOf(rel)
		}
		if u == "" {
			continue
		}
		if byUnit == nil {
			byUnit, turned = make(map[string][]write), make(map[string]bool)
		}
		if in && !turned[u] {
			turned[u] = true
			units = append(units, u)
		}
		byUnit[u] = append(byUnit[u], *x)
	}
	slices.Sort(units)
	return wrote, units, byUnit
}

// unitView is g as gateTable reads it with one unit's writes alone counted
// (W-M3.1), mine, that unit's relevant named writes g covers at or before
// at, in seq order; and whether it owes a gate_after_write row, as
// obligations owes one. The rows the table decides by the last run alone
// (backgrounded, red, unknown, masked with no write after it) read as g's,
// but for the two that count writes: a masked last run is stale only where
// the unit was written after it, and an empty run, or none, owes only where
// the unit was written since. Where g is decided by its writes and its tree
// (written since, moved, green), the unit is stale where it was written since
// the green, else its tree is bound over its own paths (bind), so a named
// write to another unit after the green moves none of its. Reason stays g's,
// and no row reads it.
func unitView(g *GateState, mine []write, unnamed []int64, at int64, tree map[string]*string) (GateState, bool) {
	v := *g
	ref := g.LastRun
	if g.LastGreen != nil {
		ref = g.LastGreen
	}
	var counted []write
	for _, x := range mine {
		if ref == nil || x.Seq > ref.Seq {
			counted = append(counted, x)
		}
	}
	v.LastWrite = lastWrite(counted)
	switch {
	case g.State == Stale && g.Reason == MaskedThenWrites:
		if !slices.ContainsFunc(counted, func(x write) bool { return x.Seq > g.LastRun.Seq }) {
			v.State = MaskedInconclusive
		}
	case g.State == Green, g.State == Stale && (g.Reason == WritesAfterGreen || g.Reason == StaleForeign):
		bound := boundUnknown
		if g.LastGreen != nil && g.LastRun != nil && g.LastGreen.Seq == g.LastRun.Seq {
			bound = bind(mine, unnamed, g.root, g.LastGreen.Seq, at, tree)
		}
		v.State = Green
		if len(counted) > 0 || bound == boundOwn || bound == boundForeign {
			v.State = Stale
		}
	}
	return v, v.State != Absent || v.LastWrite != nil
}

// turnRow gives the row add last appended its turn id over w, and its unit.
func (l *ledger) turnRow(g *GateState, w Window, unit string) {
	o := &l.rows[len(l.rows)-1]
	o.ID, o.Unit = TurnObligationID(o.Kind, idFamily(g), w, unit), unit
}

// placed is w's path relative to root, the Stop's project, the form UnitOf
// reads (W-M3.3). A path relativise left relative is in its call's project,
// so it is placed by where that project sits against root, as covers reads
// it: the path itself where the project is root; under the project's
// directory where the project is inside root; with root's directory taken
// off where root is inside the project and the path runs into it. An
// absolute path is cut to root. false where no such place exists: an
// absolute path outside root, a path that climbs out, a project the record
// does not say or that is neither inside root nor holds it. With root "" the
// path is read as recorded, as before W-M3.3.
func placed(root string, w *write) (string, bool) {
	p := *w.Path
	if root == "" {
		return p, true
	}
	root = slashed(root)
	if filepath.IsAbs(p) {
		return under(p, root)
	}
	project := w.project()
	if project == nil || *project == "" || p == ".." || strings.HasPrefix(p, "../") {
		return "", false
	}
	proj := slashed(*project)
	if proj == root {
		return p, true
	}
	if sub, in := under(proj, root); in {
		return sub + "/" + p, true
	}
	if rel, in := under(root, proj); in && strings.HasPrefix(p, rel+"/") {
		return p[len(rel)+1:], true
	}
	return "", false
}
