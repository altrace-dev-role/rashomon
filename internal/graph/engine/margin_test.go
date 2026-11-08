package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The ladder's margins (W-M1.24): for every brief over the development
// sets, the key that settled its unit and the margin at that key over the
// runner-up, right and wrong units apart, and every unit the catch-all
// cmd/rashomon took or was owed. RASHOMON_ACM11_MARGINS=1 runs it over
// RASHOMON_ACM11_PROMPTS and RASHOMON_ACM15_CLASSES (both halves, the
// acceptance half spent); the fresh set (classes2.tsv) is read once at the
// frozen engine and is refused here by name. A reading, never a gate.
func TestLadder_MarginTable(t *testing.T) {
	root := os.Getenv("RASHOMON_ACM11_DIR")
	if root == "" || os.Getenv("RASHOMON_ACM11_MARGINS") == "" {
		t.Skip("RASHOMON_ACM11_DIR and RASHOMON_ACM11_MARGINS not set")
	}
	classes := os.Getenv("RASHOMON_ACM15_CLASSES")
	var files []string
	for _, p := range append(strings.Split(os.Getenv("RASHOMON_ACM11_PROMPTS"), ":"), classes) {
		if p == "" {
			continue
		}
		if filepath.Base(p) == "classes2.tsv" {
			t.Fatalf("%s: the fresh set is read once, at the frozen engine (TestACM15_FreshTable), and is never development data", p)
		}
		files = append(files, p)
	}
	g, rev := loadACM11(t, root)
	type tally struct {
		right, wrong int
		margins      []string
	}
	keys := map[string]*tally{}
	var catchAll []string
	for _, path := range files {
		n := 4
		if path == classes {
			n = 9
		}
		seen := map[string]int{}
		for _, r := range readTSV(t, path, n) {
			id, want, prompt, label := r[0], r[1], r[3], filepath.Base(path)
			if n == 9 {
				want, prompt = r[2], r[8]
				label += " " + halfOf(seen[r[1]])[:3]
				seen[r[1]]++
			}
			wants := strings.Split(want, ";")
			b := Build(g, rev, strings.ReplaceAll(prompt, "\\n", "\n"), Options{IDF: true})
			got := briefUnit(g, rev, b)
			_, _, has := unitDisclosure(b)
			ok := slices.Contains(wants, got) && got != "" || slices.Contains(wants, "-") && !has
			key, margin := settledBy(b.Roles)
			if keys[key] == nil {
				keys[key] = &tally{}
			}
			mark := "W"
			if ok {
				mark = "R"
				keys[key].right++
			} else {
				keys[key].wrong++
			}
			if margin != "" {
				keys[key].margins = append(keys[key].margins, mark+" "+id+" "+margin)
			}
			truth := "truth not in the ladder"
			for i, p := range b.Roles.Picks {
				if slices.Contains(wants, p.Path) {
					truth = fmt.Sprintf("truth #%d %s", i, pickText(p))
					break
				}
			}
			var top []string
			for i, p := range b.Roles.Picks {
				if i == 3 {
					break
				}
				top = append(top, pickText(p))
			}
			line := fmt.Sprintf("%s %s %s: want %s got %q (ladder %q) key %s [%s]; %s; picks %s", mark, label, id, want, got, b.Roles.Path, key, margin, truth, strings.Join(top, " | "))
			t.Log(line)
			if b.Roles.Path == "cmd/rashomon" || slices.Contains(wants, "cmd/rashomon") {
				catchAll = append(catchAll, line)
			}
		}
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, k := range names {
		fmt.Fprintf(&sb, "  %-15s right %3d wrong %3d\n", k, keys[k].right, keys[k].wrong)
		for _, m := range keys[k].margins {
			fmt.Fprintf(&sb, "      %s\n", m)
		}
	}
	t.Logf("settled by (key, right, wrong; the margin at the key per row):\n%s", sb.String())
	t.Logf("the catch-all cmd/rashomon, taken or owed:\n%s", strings.Join(catchAll, "\n"))
}

// settledBy is the key that settled the roles' unit and the margin at it
// over the runner-up: a claim by a name (named, unit-name, anchor, record),
// the class, the coverage within one class (as a share of the phrase's
// weight), settle's tie-break on the same tokens (callable, mass,
// precision) or the call direction; a refusal is keyed by its reason.
func settledBy(r Roles) (string, string) {
	if len(r.Picks) == 0 {
		return "no-claim", ""
	}
	p := r.Picks[0]
	switch {
	case strings.HasPrefix(p.Refused, "below the floor"):
		return "refused-floor", fmt.Sprintf("%.2f of %.2f", p.Covered, p.Of)
	case strings.HasPrefix(p.Refused, "ambiguous"):
		return "refused-tie", p.Refused
	case p.Refused != "":
		return "refused-other", p.Refused
	case strings.HasPrefix(r.Why, "anchor "):
		return "anchor", ""
	case strings.Contains(r.Why, "tie settled by"):
		return "record", ""
	case p.Class == ClassNamed:
		if _, over, ok := strings.Cut(r.Why, " over "); ok && strings.Contains(r.Why, " applies to ") {
			return "named", "record over " + over
		}
		return "named", ""
	case p.Class == ClassUnit:
		return "unit-name", ""
	}
	if runnerUp(r.Picks) == "" {
		return "sole", ""
	}
	q := r.Picks[1]
	if classRank(p.Class) != classRank(q.Class) {
		return "class", fmt.Sprintf("%s over %s, coverage gap %.2f", p.Class, q.Class, (p.Covered-q.Covered)/p.Of)
	}
	if p.Covered != q.Covered {
		return "coverage", fmt.Sprintf("%s %.2f", p.Class, (p.Covered-q.Covered)/p.Of)
	}
	n := 1
	for n < len(r.Picks) && r.Picks[n].Class == p.Class && r.Picks[n].Covered == p.Covered {
		n++
	}
	single := p.Class != ClassPartial && len(p.Tokens) == 1
	if sameTokens(r.Picks[:n]) && nameBefore(p, q, single) {
		switch {
		case p.Callable != q.Callable:
			return "tie-callable", p.Class
		case single && p.Mass != q.Mass:
			return "tie-mass", fmt.Sprintf("%s single-token %.2f vs %.2f", p.Class, p.Mass, q.Mass)
		case p.Precision != q.Precision:
			return "tie-precision", fmt.Sprintf("%s %.2f vs %.2f", p.Class, p.Precision, q.Precision)
		}
		return "tie-mass", fmt.Sprintf("%s %.2f vs %.2f", p.Class, p.Mass, q.Mass)
	}
	return "call-direction", fmt.Sprintf("%s, %d tie", p.Class, n)
}

func pickText(p UnitPick) string {
	return fmt.Sprintf("%s %s %.2f/%.2f call=%v prec %.2f mass %.2f", p.Path, p.Class, p.Covered, p.Of, p.Callable, p.Precision, p.Mass)
}
