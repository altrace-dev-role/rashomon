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
// read from anywhere but the binary, and nothing here demotes a line.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// realCaptureName is the one committed capture a fixture may replay whole:
// the Claude Code 2.1.280 session under test/fixtures/claude-code.
const realCaptureName = "claude-code/2.1.280"

// detectorCorpus is the fixtures' and the sheet's directory, relative to this
// package.
const detectorCorpus = "../corpus/detectors"

// detectorDigest is the part of `rashomon digest` the detectors' fire
// conditions are read from.
type detectorDigest struct {
	Coverage struct {
		State string `json:"state"`
	} `json:"coverage"`
	Declarations struct {
		WithoutExecution []json.RawMessage `json:"without_execution"`
	} `json:"declarations"`
	Unknown        bool `json:"unknown"`
	SilentFailures struct {
		Fires bool `json:"fires"`
	} `json:"silent_failures"`
	MaskedRuns struct {
		Fires bool `json:"fires"`
	} `json:"masked_runs"`
	TestBending struct {
		TestsOnlyThenGreen        []json.RawMessage `json:"tests_only_then_green"`
		TestsOnlyThenGreenOmitted int               `json:"tests_only_then_green_omitted"`
		Flaky                     []json.RawMessage `json:"flaky"`
		FlakyOmitted              int               `json:"flaky_omitted"`
	} `json:"test_bending"`
	Truncated bool `json:"truncated"`
}

// detectorSpec is one entry of the inventory. fired reads the structured
// digest, which is what a rewording of the line leaves alone; sentence reads
// the line itself, and a disagreement between the two fails the fixture, so a
// change to either surfaces here. A detector with no sentence is report-only.
type detectorSpec struct {
	id       string
	fired    func(detectorDigest) bool
	sentence func(line string) bool
}

func lineHas(s string) func(string) bool {
	return func(line string) bool { return strings.Contains(line, s) }
}

func lineMatches(re string) func(string) bool {
	r := regexp.MustCompile(re)
	return func(line string) bool { return r.MatchString(line) }
}

// detectorInventory is every detector that puts a judgement in front of a
// user, in the order the end-of-turn line states them, then the report-only
// ones. Its ids are the directory names under the corpus, the rows of
// label-rules.tsv and the sheet's detector column.
var detectorInventory = []detectorSpec{
	{"digest_unknown", func(d detectorDigest) bool { return d.Unknown }, lineHas("digest unknown (")},
	{"coverage_unverified", func(d detectorDigest) bool { return !d.Unknown && d.Coverage.State != "verified" }, lineHas("coverage unverified (")},
	{"digest_truncated", func(d detectorDigest) bool { return d.Truncated }, lineHas("digest truncated (")},
	{"silent_failure", func(d detectorDigest) bool { return d.SilentFailures.Fires }, lineMatches(`\d+ recorded failures?`)},
	{"masked_run", func(d detectorDigest) bool { return d.MaskedRuns.Fires }, lineHas("pass claimed; exit status of ")},
	{"tests_only_then_green", func(d detectorDigest) bool {
		return len(d.TestBending.TestsOnlyThenGreen)+d.TestBending.TestsOnlyThenGreenOmitted > 0
	}, lineHas("test command failed, then the only recorded edits were to files named like tests, then it passed")},
	{"flaky_pair", func(d detectorDigest) bool { return len(d.TestBending.Flaky)+d.TestBending.FlakyOmitted > 0 },
		lineHas("same test command had both outcomes with no recorded file edit between")},
	{"declaration_without_execution", func(d detectorDigest) bool { return len(d.Declarations.WithoutExecution) > 0 },
		lineMatches(`\d+ declarations? without recorded execution`)},
	// Report-only: sections of `rashomon report` that state a finding and have
	// no sentence on the end-of-turn line. No fixtures yet; each row of the
	// table says so.
	{"novel_destination", nil, nil},
	{"executed_differently_from_declared", nil, nil},
}

func detectorSpecByID(id string) (detectorSpec, bool) {
	for _, d := range detectorInventory {
		if d.id == id {
			return d, true
		}
	}
	return detectorSpec{}, false
}

func knownDetector(id string) bool { _, ok := detectorSpecByID(id); return ok }

// fixtureReplay is what one fixture produced.
type fixtureReplay struct {
	line    string
	spoke   bool
	digest  detectorDigest
	rawJSON string
}

// fixtureProblems holds a replay to its fixture: the label must match what
// the detector did, and the structured digest and the printed line must agree
// that it did it. It returns whether the detector fired and what is wrong.
func fixtureProblems(spec detectorSpec, f detectorFixture, r fixtureReplay) (fired bool, problems []string) {
	fired = spec.fired(r.digest)
	switch {
	case isFireLabel(f.Label) && !fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s did not fire: the label presupposes a fire. Either the detector's behaviour changed or the fixture is wrong.\nline: %q\ndigest: %s",
			f.Label, spec.id, r.line, r.rawJSON))
	case !isFireLabel(f.Label) && fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s fired.\nline: %q\ndigest: %s", f.Label, spec.id, r.line, r.rawJSON))
	}
	if spec.sentence != nil && fired != (r.spoke && spec.sentence(r.line)) {
		problems = append(problems, fmt.Sprintf("the digest says fired=%v for %s but the printed line (%q, printed %v) disagrees: the line's wording changed, or the digest's fire condition did. Update detectorInventory with the new wording only after reading what the line now says.",
			fired, spec.id, r.line, r.spoke))
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
		if !ruled[d.id] {
			missing = append(missing, d.id)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no label rule for %v: a person cannot label what no rule defines", missing)
	}
}
