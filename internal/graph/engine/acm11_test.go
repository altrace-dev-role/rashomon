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
// the prompts stay fixed. Since the 2026-11-08 re-specification (PLAN
// 6.1, "AC-M1.5 re-specified from VISION") a row is judged as gate shape
// 3 judges it (acm11Right): the pre-named unit is owed where the prompt
// names it, and elsewhere the brief is right when it states no wrong unit.
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
			kind, ok := acm11Right(want, prompt, claimOf(b, got))
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
			t.Logf("%s: unit %q want %q %v (%s row); basis %q runner-up %q; off-task %d %v\n%s\n%s", id, got, r[1], ok, kind, basis, runner, len(off), off, b.Text(), b.Explain())
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

// acm11Right is a row's kind and unit verdict as gate shape 3 judges
// them: a no-unit row when the truth is '-', a naming row when the prompt
// names a truth unit (promptNames; AC-M1.1's files carry no target file),
// a prose row otherwise; on a naming or no-unit row the brief names a
// truth unit, or holds no unit line where '-' is allowed (the verdict as
// written), and on a prose row it claims no unit or its claim holds a
// truth (unitRight3).
func acm11Right(want []string, prompt string, c briefClaim) (string, bool) {
	kind := kindProse
	switch {
	case len(want) == 1 && want[0] == "-":
		kind = kindNoUnit
	case promptNames(prompt, want, nil):
		kind = kindNaming
	}
	unitOK := c.Unit != "" && slices.Contains(want, c.Unit) || slices.Contains(want, "-") && !c.Line
	return kind, unitRight3(kind, want, c, unitOK)
}

// TestACM11_RowsAreJudgedAsShape3 (PLAN 6.1, 2026-11-08): a prompt that
// names its unit, by path or by its last segment as a word, owes it and
// gets no credit for a runner-up; a prompt that names none is right with
// no unit claimed or a claim holding a truth; a '-' row owes no unit line.
// Break: judge a prose row by its unit alone, credit a naming row's
// runner-up, or read "restore" as naming store.
func TestACM11_RowsAreJudgedAsShape3(t *testing.T) {
	store := []string{"internal/store"}
	none := briefClaim{}
	stated := func(u string) briefClaim { return briefClaim{Unit: u, Line: true} }
	pair := func(u, r string) briefClaim { return briefClaim{Unit: u, Runner: r, Line: true, Candidate: true} }
	for _, tc := range []struct {
		name, prompt string
		want         []string
		c            briefClaim
		kind         string
		right        bool
	}{
		{"naming by word, none claimed", "Compact the store on close.", store, none, kindNaming, false},
		{"naming by word, stated", "Compact the store on close.", store, stated("internal/store"), kindNaming, true},
		{"naming by path, runner-up only", "Fix internal/store/compact.go.", store, pair("internal/report", "internal/store"), kindNaming, false},
		{"prose, none claimed", "Restore the dropped rows on close.", store, none, kindProse, true},
		{"prose, wrong stated", "Restore the dropped rows on close.", store, stated("internal/report"), kindProse, false},
		{"prose, runner-up holds", "Restore the dropped rows on close.", store, pair("internal/report", "internal/store"), kindProse, true},
		{"prose, candidate alone", "Restore the dropped rows on close.", store, pair("internal/report", ""), kindProse, false},
		{"no-unit, no line", "Rename the changelog heading.", []string{"-"}, none, kindNoUnit, true},
		{"no-unit, a line", "Rename the changelog heading.", []string{"-"}, stated("internal/report"), kindNoUnit, false},
	} {
		kind, right := acm11Right(tc.want, tc.prompt, tc.c)
		if kind != tc.kind || right != tc.right {
			t.Errorf("%s: %s row, right %v; want %s, %v", tc.name, kind, right, tc.kind, tc.right)
		}
	}
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

// TestACM11_ControlPromptsSendNoBrief (W-M1.42's gate, registered
// 2026-11-09 before W-M1.42 is built): a request with no task in it ("stop",
// "continue", "yes do it", a harness notice) gets no brief, since lines
// matched by one common word cost the reader's trust and buy nothing (PLAN
// risk R4). RASHOMON_ACM11_CONTROL names the file
// (test/live/autosteer/acm11/control.tsv); every row's brief must be empty,
// and each one sent is logged with its text. The sets AC-M1.1 reads stay
// as they are: a brief this rule silences there is read row by row in the
// gate, and a row whose truth names a unit is a loss.
func TestACM11_ControlPromptsSendNoBrief(t *testing.T) {
	root, control := os.Getenv("RASHOMON_ACM11_DIR"), os.Getenv("RASHOMON_ACM11_CONTROL")
	if root == "" || control == "" {
		t.Skip("RASHOMON_ACM11_DIR and RASHOMON_ACM11_CONTROL not set")
	}
	g, rev := loadACM11(t, root)
	rows := readTSV(t, control, 4)
	var sent []string
	for _, r := range rows {
		b := Build(g, rev, strings.ReplaceAll(r[3], "\\n", "\n"), Options{IDF: true})
		if !b.Empty() {
			sent = append(sent, r[0])
			t.Logf("%s sent %d line(s) for %q:\n%s", r[0], b.Kept, r[3], b.Text())
		}
	}
	t.Logf("control: %d of %d requests with no task sent a brief (gate 0): %v", len(sent), len(rows), sent)
	if len(sent) > 0 {
		t.Errorf("%d of %d control requests sent a brief: %v", len(sent), len(rows), sent)
	}
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
