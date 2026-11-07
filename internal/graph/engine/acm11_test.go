package engine

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// AC-M1.1 (PLAN section 8): over an indexed tree, the brief's unit line
// names the pre-named unit in at least four of five prompts (the same
// share, rounded up, when the file holds more rows) and no brief whose
// unit line is right holds more than one off-task line (W-M1.17: a brief
// on the wrong unit fails clause 1, and its lines are not read twice).
// Clause 3 (S3): every unit line states its basis (named, unit-name or
// coverage) and a claim by word coverage names the ladder's runner-up when
// it had one, so no wrong unit line is delivered as a bare fact; the
// right and wrong counts per basis are logged (the disclosure
// calibration). RASHOMON_ACM11_DIR is the indexed root (its
// .rashomon/graph.jsonl), RASHOMON_ACM11_PROMPTS one or more prompt files
// separated by ':' (test/live/autosteer/acm11/prompts.tsv, edge.tsv),
// each gated on its own, and the test skips without them. A unit of
// 'a;b' is any of them, '-' is right when the brief holds no unit line
// (the fallback below is not read for it); an allow of '*' leaves
// clause 2 unjudged for the row (no allow list was pre-registered); a
// '\n' in a prompt is a newline. The unit the brief names is the unit
// line's when there is one, else the unit containing the first line's key
// (what the hook reports as the finding's Unit today), so the reading
// before the roles stage and after it is one number. Zero model cost;
// the prompts stay fixed.
func TestACM11_UnitLineNamesThePreNamedUnit(t *testing.T) {
	root, prompts := os.Getenv("RASHOMON_ACM11_DIR"), os.Getenv("RASHOMON_ACM11_PROMPTS")
	if root == "" || prompts == "" {
		t.Skip("RASHOMON_ACM11_DIR and RASHOMON_ACM11_PROMPTS not set")
	}
	g, rev := loadACM11(t, root)
	calib := map[string]*[2]int{}
	for _, path := range strings.Split(prompts, ":") {
		rows := readTSV(t, path, 4)
		hits, offTaskBriefs, undisclosed := 0, 0, 0
		for _, r := range rows {
			id, want, allow, prompt := r[0], strings.Split(r[1], ";"), strings.Split(r[2], ","), strings.ReplaceAll(r[3], "\\n", "\n")
			b := Build(g, rev, prompt, Options{IDF: true})
			got := briefUnit(g, rev, b)
			basis, runner, has := unitDisclosure(b)
			ok := slices.Contains(want, got) && got != "" || slices.Contains(want, "-") && !has
			if os.Getenv("RASHOMON_ACM11_DUMP") != "" {
				for _, l := range b.Lines {
					t.Logf("DUMP %s\t%s\t%s\t%s", r[0], l.Role, l.Key, strings.ReplaceAll(l.Text, "\n", " "))
				}
			}
			if ok {
				hits++
			}
			var off []string
			if ok && got != "" && r[2] != "*" {
				off = offTask(b, got, allow)
				if len(off) > 1 {
					offTaskBriefs++
				}
			}
			if has {
				if basis == "" || basis == slot.BasisCoverage && runner == "" && ladderRunnerUp(b.Roles) {
					undisclosed++
				}
				if calib[basis] == nil {
					calib[basis] = &[2]int{}
				}
				if ok {
					calib[basis][0]++
				} else {
					calib[basis][1]++
				}
			}
			t.Logf("%s: unit %q want %q %v; basis %q runner-up %q; off-task %d %v\n%s\n%s", id, got, r[1], ok, basis, runner, len(off), off, b.Text(), b.Explain())
		}
		gate := (len(rows)*4 + 4) / 5
		t.Logf("AC-M1.1 %s: unit line correct %d of %d (gate %d); briefs with the unit right and more than one off-task line %d (gate 0); unit lines without a basis or a coverage claim without its runner-up %d (gate 0)", path, hits, len(rows), gate, offTaskBriefs, undisclosed)
		if hits < gate || offTaskBriefs > 0 || undisclosed > 0 {
			t.Errorf("AC-M1.1 not met on %s: %d of %d units, %d briefs over the off-task line, %d undisclosed unit lines", path, hits, len(rows), offTaskBriefs, undisclosed)
		}
	}
	keys := make([]string, 0, len(calib))
	for k := range calib {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "  %-10s right %3d wrong %3d\n", k, calib[k][0], calib[k][1])
	}
	t.Logf("disclosure calibration (unit lines, right and wrong by basis):\n%s", sb.String())
}

// unitDisclosure is the kept unit line's basis and runner-up, and whether
// the brief holds a unit line.
func unitDisclosure(b Brief) (basis, runner string, has bool) {
	for _, l := range b.Lines[:b.Kept] {
		if u, ok := l.Slot.Unit(); ok {
			return u.Basis, string(u.RunnerUp), true
		}
	}
	return "", "", false
}

// ladderRunnerUp is whether the ladder held a second, unrefused pick on
// another unit than the one it settled on.
func ladderRunnerUp(r Roles) bool {
	return len(r.Picks) > 1 && r.Picks[1].Refused == "" && r.Picks[1].ID != r.Picks[0].ID
}

// TestACM11_Replay is the development set (test/live/autosteer/acm11/
// replay.py): every row is a commit subject as the prompt and the packages
// its non-test Go files touched as the truth; the rate is how often the
// brief's unit is one of them, over every row and over the rows whose
// subject names no path. RASHOMON_ACM11_REPLAY names the file. A number
// from this set is a development reading, never the acceptance.
// RASHOMON_ACM11_ROWS=1 logs every row's unit, so two runs can be diffed.
func TestACM11_Replay(t *testing.T) {
	root, replay := os.Getenv("RASHOMON_ACM11_DIR"), os.Getenv("RASHOMON_ACM11_REPLAY")
	if root == "" || replay == "" {
		t.Skip("RASHOMON_ACM11_DIR and RASHOMON_ACM11_REPLAY not set")
	}
	g, rev := loadACM11(t, root)
	rows := readTSV(t, replay, 4)
	var all, allHit, bare, bareHit, none int
	var misses []string
	for _, r := range rows {
		truth, namesPath, prompt := strings.Split(r[1], ";"), r[2] == "1", r[3]
		b := Build(g, rev, prompt, Options{IDF: true})
		got := briefUnit(g, rev, b)
		hit := got != "" && slices.Contains(truth, got)
		all++
		if hit {
			allHit++
		}
		if got == "" {
			none++
		}
		if !namesPath {
			bare++
			if hit {
				bareHit++
			}
		}
		if !hit && len(misses) < 25 {
			misses = append(misses, r[0]+" got "+got+" want "+r[1]+" :: "+prompt)
		}
		if os.Getenv("RASHOMON_ACM11_ROWS") != "" {
			t.Logf("row %s got %s want %s", r[0], got, r[1])
		}
	}
	t.Logf("replay: unit@1 %d of %d (%.1f%%); bare subjects %d of %d (%.1f%%); no unit %d", allHit, all, rate(allHit, all), bareHit, bare, rate(bareHit, bare), none)
	t.Logf("first misses:\n%s", strings.Join(misses, "\n"))
}

func rate(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func loadACM11(t *testing.T, root string) (*graph.Store, graph.Rev) {
	t.Helper()
	g, h, err := graph.LoadFile(root)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", root, err)
	}
	return g, graph.RevCommit(h.Rev.Commit)
}

// readTSV is the file's rows with at least n tab-separated fields, comment
// and blank lines dropped.
func readTSV(t *testing.T, path string, n int) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", n)
		if len(fields) < n {
			t.Fatalf("%s: row with %d fields: %q", path, len(fields), line)
		}
		rows = append(rows, fields)
	}
	if len(rows) == 0 {
		t.Fatalf("%s: no rows", path)
	}
	return rows
}

// briefUnit is the unit path the brief names: its unit line's, else the
// unit containing its first line's key.
func briefUnit(g Reader, rev graph.Rev, b Brief) string {
	for _, l := range b.Lines[:b.Kept] {
		if u, ok := l.Slot.Unit(); ok {
			return string(u.Path)
		}
	}
	if b.Kept == 0 {
		return ""
	}
	return unitPathOf(g, rev, b.Lines[0].Key)
}

// unitPathOf walks the containing and declaring edges up to a unit, or for
// a knowledge node through what it applies to, within four steps.
func unitPathOf(g Reader, rev graph.Rev, id graph.ID) string {
	for step := 0; step < 4 && id != ""; step++ {
		n, ok := g.Node(id, rev)
		if !ok {
			return ""
		}
		if n.Kind == "unit" {
			return unitPath(id)
		}
		next := graph.ID("")
		if in := g.In(id, rev, graph.Contains, graph.Declares); len(in) > 0 {
			next = in[0].Src
		} else if out := g.Out(id, rev, graph.AppliesTo); len(out) > 0 {
			next = out[0].Dst
		}
		id = next
	}
	return ""
}

// offTask is the kept lines whose path lies outside the unit and every
// allowed prefix; a table line or a knowledge line with no path is on
// task.
func offTask(b Brief, unit string, allow []string) []string {
	var out []string
	for _, l := range b.Lines[:b.Kept] {
		p := linePath(l)
		if p == "" || strings.HasPrefix(p, unit+"/") || p == unit {
			continue
		}
		ok := false
		for _, a := range allow {
			ok = ok || (a != "" && strings.HasPrefix(p, a))
		}
		if !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func linePath(l Line) string {
	if c, ok := l.Slot.Code(); ok {
		return string(c.Path)
	}
	if u, ok := l.Slot.Unit(); ok {
		return string(u.Path)
	}
	if k, ok := l.Slot.Knowledge(); ok {
		return string(k.Path)
	}
	return ""
}
