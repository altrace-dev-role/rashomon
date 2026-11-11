//go:build perf

package acceptance

// The sweep over both corpora, built only with -tags perf.
//
// It replays every fixture of test/corpus/detectors and test/corpus/lines
// through the real binary, some 70 of them, one fresh machine each: about
// 5 s on a laptop without the race detector, against the sub-second of
// everything else in this package (CI's log prints a package's time, not a
// test's). That is too much for every push of the Test job, so it is gated
// the way test/perf is: by the perf tag, and the `perf-linux` job of
// ci.yaml runs it on every pull request.
//
//	env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT go test -tags perf \
//	    ./test/acceptance -run '^TestDetectorPrecision_Table$' -count=1 -v
//
// That job runs this one test, so the snapshot of everything the replays
// printed (detector_replay_test.go) is compared here, as its last subtest.
// What stays in the default build is what needs no sweep: the parsers, the
// statistics, the label rules, the inventory, and the replay of one fixture
// (detector_stats_test.go, detector_fixture_test.go,
// detector_precision_test.go, detector_replay_test.go).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadSheet reads the labelling sheet.
func loadSheet(t *testing.T) []sheetRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(detectorCorpus, "labelling-sheet.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseSheet(b, knownDetector)
	if err != nil {
		t.Fatalf("labelling-sheet.tsv: %v", err)
	}
	return rows
}

// sweepOutput is what the sweep collects beside the tallies: every block of
// the snapshot in corpus order, the fires a fixture's label does not read,
// and the whole line each false positive printed.
type sweepOutput struct {
	blocks   [][]string
	fixtures int
	others   []string
	fpLines  []string
}

// TestDetectorPrecision_Table replays every fixture, holds each label to the
// binary's behaviour, and prints one row per detector: N, precision, the
// Wilson 95% interval and a status. Only labels a person confirmed count; a
// detector below the floor is printed unmeasured, whatever its seeds say.
// Beside the rows it lists the fires no label reads and each false
// positive's whole line, and its last subtest holds everything the replays
// printed to the committed snapshot.
func TestDetectorPrecision_Table(t *testing.T) {
	tallies := map[string]*fireTally{}
	hasFixtures := map[string]bool{}
	// reproduced is each false-positive class a fixture reproduced, with the
	// fixtures that show it: what a fix would have to cover.
	reproduced := map[string][]string{}
	var out sweepOutput
	for _, spec := range detectorInventory {
		id := spec.name()
		tallies[id] = &fireTally{}
		names, fixtures := loadDetectorFixtures(t, id)
		hasFixtures[id] = len(fixtures) > 0
		if len(fixtures) > 0 && spec.reportOnly {
			t.Fatalf("%s is report-only: its fire condition is not in the digest the replay reads", id)
		}
		for i, f := range fixtures {
			out.fixtures++
			t.Run(id+"/"+names[i], func(t *testing.T) {
				fired := out.replayDetectorFixture(t, spec, names[i], f)
				if fired {
					tallies[id].add(f.Label, f.LabelledBy)
					if f.Label == labelFalsePositive {
						reproduced[f.FPClass] = append(reproduced[f.FPClass], id+"/"+names[i])
					}
				}
			})
		}
	}
	names, lines := loadLineFixtures(t)
	for i, f := range lines {
		out.fixtures++
		t.Run("lines/"+names[i], func(t *testing.T) {
			turns, norm := replayRun(t, f.fixtureRun)
			out.blocks = append(out.blocks, renderRun(t, "lines/"+names[i], turns, norm))
		})
	}
	for _, row := range loadSheet(t) {
		tallies[row.Detector].add(row.Label, row.LabelledBy)
		hasFixtures[row.Detector] = true
	}

	var rows []string
	for _, spec := range detectorInventory {
		rows = append(rows, precisionRow{spec.name(), *tallies[spec.name()], hasFixtures[spec.name()]}.String())
	}
	var classes []string
	for _, c := range fpClasses {
		if len(reproduced[c]) > 0 {
			classes = append(classes, fmt.Sprintf("%-26s %s", c, strings.Join(reproduced[c], ", ")))
		}
	}
	t.Logf("false-positive classes reproduced (class, fixtures):\n%s", strings.Join(classes, "\n"))
	t.Logf("fires no label reads (fixture turn: detectors other than the fixture's own):\n%s", orNone(strings.Join(out.others, "\n")))
	t.Logf("false positives and the whole line each printed:\n%s", orNone(strings.Join(out.fpLines, "\n")))
	t.Logf("detector precision (counted = confirmed by a person; seed labels are listed, never counted; floor %d)\n%s",
		minLabelledFires, strings.Join(rows, "\n"))

	t.Run("snapshot", func(t *testing.T) { out.compare(t) })
}

// replayDetectorFixture replays one detector fixture, holds its label to its
// label turn, and collects its blocks, the other detectors that fired on any
// of its turns and, for a false positive, the whole line. It reports whether
// the fixture's detector fired with nothing wrong.
func (out *sweepOutput) replayDetectorFixture(t *testing.T, spec detectorSpec, name string, f detectorFixture) bool {
	t.Helper()
	full := "detectors/" + spec.name() + "/" + name
	turns, norm := replayRun(t, f.fixtureRun)
	out.blocks = append(out.blocks, renderRun(t, full, turns, norm))
	for i, tr := range turns {
		var others []string
		for _, id := range tr.fired() {
			if id != spec.name() {
				others = append(others, id)
			}
		}
		if len(others) > 0 {
			out.others = append(out.others, fmt.Sprintf("%s turn %d: %s", full, i+1, strings.Join(others, ", ")))
		}
	}
	judged := turns[f.labelTurn()].judged(t)
	if f.Label == labelFalsePositive {
		line := " the Stop printed nothing"
		if judged.spoke {
			line = "\n    | " + strings.ReplaceAll(norm.apply(judged.line), "\n", "\n    | ")
		}
		out.fpLines = append(out.fpLines, fmt.Sprintf("%s turn %d:%s", full, f.labelTurn()+1, line))
	}
	fired, problems := fixtureProblems(spec, f, judged)
	for _, p := range problems {
		t.Error(p)
	}
	return fired && len(problems) == 0
}

// compare holds the sweep's blocks to the committed snapshot, or with
// -update rewrites it. Only a sweep that replayed every fixture may rewrite
// it or call a committed block gone: one filtered with -run has not seen
// the rest.
func (out *sweepOutput) compare(t *testing.T) {
	var got []string
	for _, b := range out.blocks {
		got = append(got, b...)
	}
	complete := len(out.blocks) == out.fixtures
	if *updateGoldens {
		if !complete {
			t.Fatalf("-update needs every fixture replayed; %d of %d were", len(out.blocks), out.fixtures)
		}
		if err := os.WriteFile(replaySnapshot, []byte(formatSnapshot(got)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) == 0 {
		// A -run filter that replayed no fixture compared nothing, and a
		// pass would say the snapshot held: the absence shows as a skip.
		t.Skipf("no fixture replayed (0 of %d); nothing was compared", out.fixtures)
	}
	problems := snapshotProblems(got, readSnapshot(t), complete)
	for _, p := range problems {
		t.Error(p)
	}
	if len(problems) > 0 {
		t.Errorf("%d blocks differ from %s: rewrite it with -update only for a change you mean, and read the diff", len(problems), replaySnapshot)
	}
}
