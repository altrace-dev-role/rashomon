package acceptance

// Detector precision, the replay half: each fixture's run is fed to the built
// binary as the hook payloads it would have sent, the Stop path prints its
// end-of-turn line, `digest` reports which detectors fired, and the label the
// fixture carries is held to what happened. Then the per-detector table.
//
// Read the table with:
//
//	env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT go test -tags perf \
//	    ./test/acceptance -run '^TestDetectorPrecision_Table$' -count=1 -v
//
// The table itself is in detector_replay_perf_test.go, built with -tags perf.
//
// This file measures and never changes a detector: no detector's behaviour is
// read from anywhere but the binary, and nothing here demotes a line. Which
// detector fired is read from the digest's structured fields through the code
// identifier (report.DetectorID), never from the printed wording.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// realCaptureName is the one committed capture a fixture may replay whole:
// the Claude Code 2.1.280 session under test/fixtures/claude-code.
const realCaptureName = "claude-code/2.1.280"

// detectorCorpus is the fixtures' and the sheet's directory, relative to this
// package.
const detectorCorpus = "../corpus/detectors"

// detectorSpec is one entry of the inventory. The id is the detector's code
// identifier (report.DetectorID), and it is the only thing a detector is told
// apart by here: whether it fired is asked of digest.Digest.Fired, the same
// condition the end-of-turn line is rendered from, never read out of the
// printed sentence. Task 2.4 rewrites the sentences; nothing in this harness
// may notice. A report-only detector has no digest condition and no fixtures
// yet.
type detectorSpec struct {
	id         report.DetectorID
	reportOnly bool
}

// name is the id as the corpus spells it: fixture directory, rule row, sheet
// column.
func (s detectorSpec) name() string { return string(s.id) }

// fired reports whether the detector fired on a replayed turn's digest.
func (s detectorSpec) fired(d *digest.Digest) bool { return d.Fired(s.id) }

// detectorInventory is every detector that puts a judgement in front of a
// user, in the order report.DetectorIDs states them: the digest-level ones in
// the order the end-of-turn line gives them, then the two that exist only in
// `rashomon report`. Its ids are the directory names under the corpus, the
// rows of label-rules.tsv and the sheet's detector column; the catalogue's
// steer pattern ids are not part of it, because five of the ten have none yet.
var detectorInventory = func() []detectorSpec {
	var out []detectorSpec
	for _, id := range report.DetectorIDs() {
		out = append(out, detectorSpec{
			id:         id,
			reportOnly: id == report.DetectorNovelDestination || id == report.DetectorExecutedDifferentlyFromDeclared,
		})
	}
	return out
}()

func detectorSpecByID(id string) (detectorSpec, bool) {
	for _, d := range detectorInventory {
		if d.name() == id {
			return d, true
		}
	}
	return detectorSpec{}, false
}

func knownDetector(id string) bool { _, ok := detectorSpecByID(id); return ok }

// fixtureReplay is what one fixture produced.
type fixtureReplay struct {
	// line and spoke are what the Stop path printed. They are kept to show a
	// person what the fire looked like in a failure message, as data; no
	// verdict is drawn from them.
	line    string
	spoke   bool
	digest  digest.Digest
	rawJSON string
}

// fixtureProblems holds a replay to its fixture: the label must match what
// the detector did. It returns whether the detector fired and what is wrong.
// The printed line is quoted in a problem for the reader and decides nothing:
// a reworded sentence changes no result here.
func fixtureProblems(spec detectorSpec, f detectorFixture, r fixtureReplay) (fired bool, problems []string) {
	fired = spec.fired(&r.digest)
	switch {
	case isFireLabel(f.Label) && !fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s did not fire: the label presupposes a fire. Either the detector's behaviour changed or the fixture is wrong.\nline: %q\ndigest: %s",
			f.Label, spec.name(), r.line, r.rawJSON))
	case !isFireLabel(f.Label) && fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s fired.\nline: %q\ndigest: %s", f.Label, spec.name(), r.line, r.rawJSON))
	}
	return fired, problems
}

// TestDetectorPrecision_InventoryMatchesCorpus: the inventory, the fixture
// directories and the label rules are one list. A directory with no detector,
// a detector with no rule, or a rule with no detector is a drift.
func TestDetectorPrecision_InventoryMatchesCorpus(t *testing.T) {
	ents, err := os.ReadDir(detectorCorpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if ent.IsDir() && !knownDetector(ent.Name()) {
			t.Errorf("directory %s is no detector of the inventory", ent.Name())
		}
	}
	b, err := os.ReadFile(filepath.Join(detectorCorpus, "label-rules.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if want := "detector\tfires_when\ttrue_positive_when\tfalse_positive_when"; lines[0] != want {
		t.Fatalf("label-rules.tsv header = %q, want %q", lines[0], want)
	}
	ruled := map[string]bool{}
	for _, l := range lines[1:] {
		cols := strings.Split(l, "\t")
		if len(cols) != 4 {
			t.Errorf("label-rules.tsv row %q has %d columns, want 4", l, len(cols))
			continue
		}
		if !knownDetector(cols[0]) {
			t.Errorf("label-rules.tsv names %s, which is no detector of the inventory", cols[0])
		}
		for _, c := range cols[1:] {
			if strings.TrimSpace(c) == "" {
				t.Errorf("label-rules.tsv row for %s has an empty cell", cols[0])
			}
		}
		ruled[cols[0]] = true
	}
	var missing []string
	for _, d := range detectorInventory {
		if !ruled[d.name()] {
			missing = append(missing, d.name())
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no label rule for %v: a person cannot label what no rule defines", missing)
	}
}

// TestDetectorPrecision_InventoryIsTheTenIds: the inventory is exactly the ten
// detectors, spelled as the corpus spells them, in code's order. The strings
// are written out here on purpose: they are fixture directory names, so a
// constant renamed in code must fail this test rather than quietly orphan the
// corpus. No steer pattern id appears: the harness runs without the detector
// to pattern mapping, and a detector with no catalogue id is reported by its
// own.
func TestDetectorPrecision_InventoryIsTheTenIds(t *testing.T) {
	want := []string{
		"digest_unknown", "coverage_unverified", "digest_truncated", "silent_failure",
		"masked_run", "tests_only_then_green", "flaky_pair", "declaration_without_execution",
		"novel_destination", "executed_differently_from_declared",
	}
	var got []string
	for _, d := range detectorInventory {
		got = append(got, d.name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("inventory ids = %v, want %v", got, want)
	}
}
